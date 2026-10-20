package main

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	mainWndProcCB   = syscall.NewCallback(mainProc)
	pickerWndProcCB = syscall.NewCallback(pickerProc)
)

func createMainWindow() uintptr {
	className := utf16("MemMonCard")
	var wc WNDCLASSEXW
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.Style = CS_DBLCLKS
	wc.LpfnWndProc = mainWndProcCB
	wc.HInstance = hInst
	wc.HCursor = cursorArrow()
	wc.LpszClassName = className
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	exStyle := WS_EX_TOPMOST | WS_EX_LAYERED | WS_EX_TOOLWINDOW
	title := utf16("MemMonitor")
	hwnd, _, _ := pCreateWindowExW.Call(uintptr(exStyle),
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)),
		uintptr(WS_POPUP),
		i32(state.X), i32(state.Y), 10, 10, 0, 0, hInst, 0)
	return hwnd
}

func mainProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch uint32(msg) {
	case WM_PAINT:
		// 内容由 UpdateLayeredWindow 提交，这里只需要清掉无效区域
		var ps PAINTSTRUCT
		pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_TIMER:
		if wParam == 1 {
			tick()
		}
		return 0
	case WM_ENTERSIZEMOVE:
		dragging = true
		return 0
	case WM_EXITSIZEMOVE:
		dragging = false
		renderCard()
		return 0
	case WM_LBUTTONDOWN:
		// 拖动窗口（位置锁定时不响应）
		if cfg.LockPos {
			return 0
		}
		pReleaseCapture.Call()
		pSendMessageW.Call(hwnd, WM_NCLBUTTONDOWN, HTCAPTION, 0)
		return 0

	case WM_NCHITTEST:
		// 左右边缘可拖动改变宽度（大小锁定时不响应）
		if cfg.LockSize {
			return HTCLIENT
		}
		var wr RECT
		pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
		x := int32(int16(lParam & 0xFFFF))
		cx := x - wr.Left
		edge := sc(7)
		if cx < edge {
			return HTLEFT
		}
		if cx >= (wr.Right-wr.Left)-edge {
			return HTRIGHT
		}
		return HTCLIENT

	case WM_SIZE:
		if state.Hwnd == 0 {
			return 0
		}
		var rc RECT
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		nw := clampWidth(scBack(rc.Right))
		if nw != state.W {
			state.W = nw
			cfg.Width = nw
			renderCard()
			saveConfig()
			// 被最小宽度顶回去时，把窗口尺寸也纠正过来
			if L := layoutCard(); int32(rc.Right) != L.W || int32(rc.Bottom) != L.H {
				pSetWindowPos.Call(hwnd, 0, 0, 0, uintptr(L.W), uintptr(L.H),
					SWP_NOMOVE|SWP_NOZORDER|SWP_NOACTIVATE)
			}
		}
		return 0
	case WM_LBUTTONDBLCLK:
		showPicker()
		return 0
	case WM_RBUTTONUP:
		showMenu(hwnd)
		return 0
	case WM_MOUSEWHEEL:
		// Ctrl + 滚轮微调透明度
		ks, _, _ := pGetKeyState.Call(0x11) // VK_CONTROL
		if int16(ks) < 0 {
			d := int32(int16((wParam >> 16) & 0xFFFF))
			if d > 0 {
				setAlphaPercent(alphaPercent() + 5)
			} else {
				setAlphaPercent(alphaPercent() - 5)
			}
			return 0
		}
	case WM_DESTROY:
		var rc RECT
		pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		cfg.X = rc.Left
		cfg.Y = rc.Top
		cfg.PosSet = true
		saveConfig()
		logger.closeFile()
		pKillTimer.Call(hwnd, 1)
		if pk.Hwnd != 0 {
			pDestroyWindow.Call(pk.Hwnd)
		}
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

func clampToScreen(w, h int32) {
	vx, _, _ := pGetSystemMetrics.Call(76)
	vy, _, _ := pGetSystemMetrics.Call(77)
	vw, _, _ := pGetSystemMetrics.Call(78)
	vh, _, _ := pGetSystemMetrics.Call(79)
	minX, minY := int32(vx), int32(vy)
	maxX := minX + int32(vw) - w
	maxY := minY + int32(vh) - h
	if state.X > maxX {
		state.X = maxX
	}
	if state.Y > maxY {
		state.Y = maxY
	}
	if state.X < minX {
		state.X = minX
	}
	if state.Y < minY {
		state.Y = minY
	}
}

// probe 是无界面自检：打印进程列表与进程树汇总，用于验证底层采样逻辑。
func probe() {
	total, avail := memStatus()
	fmt.Printf("RAM: total=%.1f MB avail=%.1f MB\n", float64(total)/1048576, float64(avail)/1048576)

	procs := snapshotProcs()
	fmt.Printf("processes: %d\n\n", len(procs))

	children := make(map[uint32][]uint32, len(procs))
	for _, p := range procs {
		children[p.PPid] = append(children[p.PPid], p.Pid)
	}

	// 指定进程名时，打印原始字节值，便于与系统工具逐字节核对
	if len(os.Args) > 2 {
		target := os.Args[2]
		found := false
		for _, p := range procs {
			if !strings.EqualFold(p.Name, target) {
				continue
			}
			m, ok := procMemory(p.Pid)
			if !ok {
				fmt.Printf("%s pid=%d 读取失败\n", p.Name, p.Pid)
				continue
			}
			found = true
			fmt.Printf("%s pid=%d privateWS=%d workingSet=%d privateBytes=%d\n",
				p.Name, p.Pid, m.PrivateWS, m.WorkingSet, m.PrivateB)
		}
		if !found {
			fmt.Printf("未找到进程 %s\n", target)
		}
		fmt.Printf("\nRAM total bytes=%d\n", total)
		return
	}

	type row struct {
		name string
		pid  uint32
		mem  uint64
	}
	rows := make([]row, 0, len(procs))
	for _, p := range procs {
		if m, ok := procMemory(p.Pid); ok {
			rows = append(rows, row{p.Name, p.Pid, m.Value("private")})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].mem > rows[j].mem })
	fmt.Println("Top memory processes (private working set):")
	for i, r := range rows {
		if i >= 12 {
			break
		}
		fmt.Printf("  %-28s %6d  %10s\n", r.name, r.pid, fmtBytes(r.mem))
	}

	if len(rows) > 0 {
		top := rows[0]
		m, n := treeMem(top.pid, children, "private")
		fmt.Printf("\ntree of %s (pid %d): %d processes, total %s\n", top.name, top.pid, n, fmtBytes(m))
		fmt.Printf("path: %s\n", procPath(top.pid))
		h := iconFor(procPath(top.pid))
		fmt.Printf("icon handle: 0x%x\n", h)
	}

	// 找一个子进程最多的进程，验证进程树递归汇总
	bestPid, bestN := uint32(0), 0
	for pid, ch := range children {
		if len(ch) > bestN {
			bestPid, bestN = pid, len(ch)
		}
	}
	if bestN > 0 {
		name := ""
		for _, p := range procs {
			if p.Pid == bestPid {
				name = p.Name
				break
			}
		}
		m, n := treeMem(bestPid, children, "private")
		fmt.Printf("\ntree of %s (pid %d): %d processes, total %s\n", name, bestPid, n, fmtBytes(m))
	}
	fmt.Println("\nprobe ok")
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--probe" || os.Args[1] == "-probe") {
		probe()
		return
	}
	// 命令行自测清理逻辑：--clean target|all|sys
	if len(os.Args) > 1 && os.Args[1] == "--memlist" {
		enablePrivilege("SeProfileSingleProcessPrivilege")
		debugMemList()
		return
	}
	// 开发用：导出合成结果
	if len(os.Args) > 2 && os.Args[1] == "--dump" {
		dumpCard(os.Args[2])
		return
	}
	// 命令行自测：--export <分钟数，0=全部> <输出.csv>
	if len(os.Args) > 3 && os.Args[1] == "--export" {
		mins, _ := strconv.Atoi(os.Args[2])
		to := time.Now()
		var from time.Time
		if mins > 0 {
			from = to.Add(-time.Duration(mins) * time.Minute)
		}
		n, err := exportLogs(from, to, os.Args[3])
		if err != nil {
			fmt.Println("export error:", err)
			return
		}
		fmt.Printf("exported %d rows -> %s\n", n, os.Args[3])
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "--clean" {		cfg = loadConfig()
		total, avail := memStatus()
		fmt.Printf("RAM total=%.1f MB avail=%.1f MB load=%d%% standby=%.1f MB\n",
			float64(total)/1048576, float64(avail)/1048576, memLoad(),
			float64(standbyBytes())/1048576)
		var r cleanResult
		switch os.Args[2] {
		case "target":
			state.TargetName = cfg.TargetName
			state.TargetPid = cfg.TargetPid
			sample()
			fmt.Printf("before: %s\n", fmtBytes(state.MemBytes))
			r = cleanTarget()
		case "all":
			r = cleanAll()
		case "sys":
			r = cleanSystem()
		}
		total2, avail2 := memStatus()
		fmt.Printf("result: %s (%s) freed=%.1f MB\n", r.Text, r.Detail, float64(r.Freed)/1048576)
		fmt.Printf("RAM total=%.1f MB avail=%.1f MB load=%d%% standby=%.1f MB\n",
			float64(total2)/1048576, float64(avail2)/1048576, memLoad(),
			float64(standbyBytes())/1048576)
		return
	}

	runtime.LockOSThread()
	initDPI()
	cfg = loadConfig()
	pruneLogs()

	h, _, _ := pGetModuleHandleW.Call(0)
	hInst = h

	createFonts()
	initGdiplus()
	initImageList()
	initPickerResources()

	L := layoutCard()
	if !cfg.PosSet {
		sw, _, _ := pGetSystemMetrics.Call(0)
		state.X = int32(sw) - L.W - sc(28)
		state.Y = sc(28)
	} else {
		state.X = cfg.X
		state.Y = cfg.Y
	}
	clampToScreen(L.W, L.H)

	state.TargetName = cfg.TargetName
	state.TargetPid = cfg.TargetPid
	state.W = cfg.Width

	hwnd := createMainWindow()
	if hwnd == 0 {
		return
	}
	state.Hwnd = hwnd

	createPickerWindow()
	applyLayout()

	// 调试用：MEMMON_DEMO=文案 启动时弹一条提示，方便截图核对
	if t := os.Getenv("MEMMON_DEMO"); t != "" {
		showToast(t)
	}

	sample()
	pInvalidateRect.Call(hwnd, 0, 0)
	pSetTimer.Call(hwnd, 1, uintptr(cfg.Interval), 0)

	var msg MSG
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}
