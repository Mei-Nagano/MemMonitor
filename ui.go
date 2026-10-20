package main

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// ---------------- 全局状态 ----------------

var (
	cfg      Config
	dpiScale = 1.0
	hInst    uintptr

	fontName   uintptr
	fontSub    uintptr
	fontMem    uintptr
	fontPct    uintptr
	fontList   uintptr
	fontUI     uintptr
	fontRowMem uintptr
)

type AppState struct {
	Hwnd uintptr
	X    int32
	Y    int32
	W    int32

	TargetName string
	TargetPid  uint32
	Alive      bool
	ProcName   string
	ExePath    string
	MemBytes   uint64
	ProcCount  int
	Pct        float64

	hist    [90]uint64
	histLen int
	histPos int

	Toast      string
	ToastUntil int64 // Unix 毫秒
}

// toastActive 判断清理结果提示是否还在展示期。
func toastActive() bool {
	return state.Toast != "" && time.Now().UnixMilli() < state.ToastUntil
}

// showToast 在卡片副标题行显示一条临时提示。
func showToast(text string) {
	state.Toast = text
	state.ToastUntil = time.Now().UnixMilli() + 3000
	renderCard()
}

var state = &AppState{}

func (s *AppState) push(v uint64) {
	s.hist[s.histPos] = v
	s.histPos = (s.histPos + 1) % len(s.hist)
	if s.histLen < len(s.hist) {
		s.histLen++
	}
}

func (s *AppState) histAt(i int) uint64 {
	start := (s.histPos - s.histLen + len(s.hist)) % len(s.hist)
	return s.hist[(start+i)%len(s.hist)]
}

func (s *AppState) resetHist() {
	s.histLen = 0
	s.histPos = 0
	s.hist = [90]uint64{}
}

// cardMaxW 是卡片逻辑宽度上限。
const cardMaxW = 480

// uiScale 是用户可调的界面缩放（菜单「界面大小」）。
func uiScale() float64 {
	s := cfg.Scale
	if s < 60 || s > 200 {
		s = 100
	}
	return float64(s) / 100
}

// sc 按 DPI 与界面缩放换算逻辑尺寸。
func sc(v int32) int32 {
	return int32(float64(v)*dpiScale*uiScale() + 0.5)
}

// scBack 把物理像素换算回逻辑尺寸。
func scBack(px int32) int32 {
	return int32(float64(px)/(dpiScale*uiScale()) + 0.5)
}

// ---------------- 文本测量 ----------------

var measureDC uintptr

// textWidth 返回字符串在指定字体下的像素宽度。
func textWidth(font uintptr, s string) int32 {
	if s == "" || font == 0 {
		return 0
	}
	if measureDC == 0 {
		screen, _, _ := pGetDC.Call(0)
		if screen == 0 {
			return 0
		}
		measureDC, _, _ = pCreateCompatibleDC.Call(screen)
		pReleaseDC.Call(0, screen)
		if measureDC == 0 {
			return 0
		}
	}
	pSelectObject.Call(measureDC, font)
	u := syscall.StringToUTF16(s)
	var sz SIZE
	pGetTextExtentPoint32W.Call(measureDC, uintptr(unsafe.Pointer(&u[0])),
		uintptr(len(u)-1), uintptr(unsafe.Pointer(&sz)))
	return sz.Cx
}

// pctColWidth 是占比列的宽度：按 "100.0%" 的实际宽度算，留一点余量。
func pctColWidth() int32 {
	w := textWidth(fontPct, "100.0%")
	if w < sc(30) {
		w = sc(30)
	}
	return w
}

// minCardWidth 按当前显示项算出不会被裁切的最小逻辑宽度。
// 这样拖动到最窄时也不会出现内存数字被截断的情况。
func minCardWidth() int32 {
	pad := float64(sc(11))
	need := pad * 2
	if cfg.ShowIcon {
		need += float64(sc(26))
	}
	if cfg.ShowName || cfg.ShowPid {
		need += float64(sc(10) + sc(60)) // 图标与文字间距 + 进程名最少可读宽度
	}

	if cfg.ShowMem || cfg.ShowPct {
		row := pad * 2
		if cfg.ShowMem {
			row += float64(textWidth(fontMem, "999.9 MB"))
		}
		if cfg.ShowPct {
			row += float64(sc(6)) + float64(pctColWidth())
		}
		if row > need {
			need = row
		}
	}

	lg := scBack(int32(need + 0.5))
	if lg < 150 {
		lg = 150
	}
	if lg > cardMaxW {
		lg = cardMaxW
	}
	return lg
}

// clampWidth 把逻辑宽度限制在 [最小可用宽度, cardMaxW]。
func clampWidth(w int32) int32 {
	if minW := minCardWidth(); w < minW {
		w = minW
	}
	if w > cardMaxW {
		w = cardMaxW
	}
	return w
}

func initDPI() {
	pSetProcessDPIAware.Call()
	hdc, _, _ := pGetDC.Call(0)
	if hdc != 0 {
		dpi, _, _ := pGetDeviceCaps.Call(hdc, LOGPIXELSX)
		pReleaseDC.Call(0, hdc)
		if dpi >= 72 && dpi <= 480 {
			dpiScale = float64(dpi) / 96.0
		}
	}
}

func makeFont(height int32, weight int32, face string) uintptr {
	p, err := syscall.UTF16PtrFromString(face)
	if err != nil {
		return 0
	}
	// 分层窗口按像素合成 alpha，不能用 ClearType 的次像素抗锯齿（会产生彩色边缘），
	// 这里统一用灰度抗锯齿，内容层与遮罩层的覆盖率才对得上。
	f, _, _ := pCreateFontW.Call(
		i32(-height), 0, 0, 0, uintptr(weight), 0, 0, 0,
		DEFAULT_CHARSET, OUT_TT_PRECIS, CLIP_DEFAULT_PRECIS, ANTIALIASED_QUALITY,
		DEFAULT_PITCH|FF_DONTCARE, uintptr(unsafe.Pointer(p)))
	runtime.KeepAlive(p)
	return f
}

func createFonts() {
	fontName = makeFont(sc(15), FW_SEMIBOLD, "Segoe UI")
	fontSub = makeFont(sc(13), FW_NORMAL, "Segoe UI")
	fontMem = makeFont(sc(21), FW_BOLD, "Segoe UI")
	fontPct = makeFont(sc(13), FW_SEMIBOLD, "Segoe UI")
	fontList = makeFont(sc(13), FW_NORMAL, "Consolas")
	fontUI = makeFont(sc(13), FW_NORMAL, "Segoe UI")
	fontRowMem = makeFont(sc(14), FW_SEMIBOLD, "Segoe UI")
}

// recreateFonts 按当前缩放重建全部字体。
func recreateFonts() {
	for _, f := range []uintptr{fontName, fontSub, fontMem, fontPct, fontList, fontUI, fontRowMem} {
		if f != 0 {
			pDeleteObject.Call(f)
		}
	}
	createFonts()
}

// setScale 切换界面缩放，并让卡片重新排版。
func setScale(s int) {
	cfg.Scale = s
	recreateFonts()
	state.W = clampWidth(state.W)
	applyLayout()
	saveConfig()
}

// ---------------- 卡片布局 ----------------

type cardLayout struct {
	W, H   int32
	Radius int32
	Icon   RECT
	Name   RECT
	Pid    RECT
	Mem    RECT
	Pct    RECT
	Spark  RECT

	HasIcon  bool
	HasName  bool
	HasPid   bool
	HasMem   bool
	HasPct   bool
	HasSpark bool
}

func layoutCard() cardLayout {
	var L cardLayout
	pad := sc(11)
	w := sc(clampWidth(state.W))
	L.W = w
	L.Radius = sc(10)

	L.HasName = cfg.ShowName
	L.HasPid = cfg.ShowPid
	// 图标是独立开关，不受进程名/PID 是否显示影响
	L.HasIcon = cfg.ShowIcon
	L.HasMem = cfg.ShowMem
	L.HasPct = cfg.ShowPct
	L.HasSpark = cfg.ShowSpark

	x := pad
	y := pad
	textX := x
	if L.HasIcon {
		textX = x + sc(26) + sc(10)
	}

	// 顶部信息区：图标 / 进程名 / PID 任意组合都要排得下。
	// 只开图标时也要留够一行高度，否则图标会压到下面的内存行。
	topH := int32(0)
	if L.HasName {
		topH += sc(19)
	}
	if L.HasPid {
		topH += sc(17)
	}
	if L.HasIcon && topH < sc(26) {
		topH = sc(26)
	}
	if topH > 0 {
		top := y
		ty := top
		if L.HasName {
			L.Name = RECT{textX, ty, w - pad, ty + sc(19)}
			ty += sc(19)
		}
		if L.HasPid {
			L.Pid = RECT{textX, ty, w - pad, ty + sc(17)}
			ty += sc(17)
		}
		if L.HasIcon {
			iy := top + (topH-sc(26))/2
			if iy < top {
				iy = top
			}
			L.Icon = RECT{x, iy, x + sc(26), iy + sc(26)}
		}
		y = top + topH + sc(7)
	}

	if L.HasMem || L.HasPct {
		right := w - pad
		if L.HasPct {
			pw := pctColWidth()
			L.Pct = RECT{right - pw, y + sc(4), right, y + sc(25)}
			right -= pw + sc(6)
		}
		if L.HasMem {
			L.Mem = RECT{x, y, right, y + sc(25)}
		}
		y += sc(26)
	}

	if L.HasSpark {
		y += sc(3)
		L.Spark = RECT{x, y, w - pad, y + sc(16)}
		y += sc(16)
	}

	L.H = y + pad
	return L
}

// ---------------- 采样 ----------------

func sample() {
	procs := snapshotProcs()
	if len(procs) == 0 {
		return
	}

	byPid := make(map[uint32]int, len(procs))
	children := make(map[uint32][]uint32, len(procs))
	for i := range procs {
		byPid[procs[i].Pid] = i
		children[procs[i].PPid] = append(children[procs[i].PPid], procs[i].Pid)
	}

	var tgt *ProcEntry
	if state.TargetName != "" {
		if i, ok := byPid[state.TargetPid]; ok && strings.EqualFold(procs[i].Name, state.TargetName) {
			tgt = &procs[i]
		} else {
			// 目标进程重启后 PID 会变，这里按进程名重新绑定
			for i := range procs {
				if strings.EqualFold(procs[i].Name, state.TargetName) {
					tgt = &procs[i]
					break
				}
			}
		}
	}

	if tgt == nil {
		state.Alive = false
		state.MemBytes = 0
		state.ProcCount = 0
		state.Pct = 0
		state.ExePath = ""
		state.push(0)
		return
	}

	if tgt.Pid != state.TargetPid {
		state.TargetPid = tgt.Pid
		state.ExePath = ""
	}
	state.Alive = true
	state.ProcName = tgt.Name

	var mem uint64
	if cfg.Tree {
		mem, state.ProcCount = treeMem(tgt.Pid, children, cfg.Metric)
	} else {
		if m, ok := procMemory(tgt.Pid); ok {
			mem = m.Value(cfg.Metric)
		}
		state.ProcCount = 1
	}
	state.MemBytes = mem

	if total, _ := memStatus(); total > 0 {
		state.Pct = float64(mem) / float64(total) * 100
	}

	if state.ExePath == "" {
		state.ExePath = procPath(tgt.Pid)
	}
	state.push(mem)
}

// dragging 为真时暂停重绘，避免拖动过程中被 UpdateLayeredWindow 拉回原位
var dragging bool

func tick() {
	now := time.Now()
	sample()

	// 落一条采样记录（只在有监控目标时记，避免写一堆 0）
	if cfg.LogEnabled && state.TargetName != "" && state.Alive {
		logger.write(now, state.MemBytes, state.Pct, state.TargetName)
	}

	// 提示到期后清掉
	if state.Toast != "" && !toastActive() {
		state.Toast = ""
	}

	maybeAutoClean()

	if !dragging {
		renderCard()
	}
}

// nextAutoClean 是下一次允许自动清理的时间（Unix 毫秒），用于冷却。
var nextAutoClean int64

// maybeAutoClean 在整机内存占用超过阈值时自动修剪一遍工作集。
func maybeAutoClean() {
	if cfg.AutoCleanPct <= 0 {
		return
	}
	load := memLoad()
	if load < cfg.AutoCleanPct {
		return
	}
	now := time.Now().UnixMilli()
	if now < nextAutoClean {
		return
	}
	nextAutoClean = now + int64(cfg.AutoCleanCool)*1000

	r := cleanAll()
	sample()
	showToast(fmt.Sprintf("内存 %d%% · %s", load, r.Text))
}

func setTarget(name string, pid uint32) {
	state.TargetName = name
	state.TargetPid = pid
	state.ExePath = ""
	state.resetHist()
	cfg.TargetName = name
	cfg.TargetPid = pid
	sample()
	renderCard()
	saveConfig()
}

// ---------------- 绘制 ----------------

var (
	cardSurf    surface // 输出
	cardContent surface // 内容层（黑底 + 正常颜色）
	cardMask    surface // 遮罩层（黑底 + 白色内容，亮度即覆盖率）
)

// monoMode 为真时所有绘制都用白色，用于生成覆盖率遮罩。
var monoMode bool

func fillBlack(dc uintptr, w, h int32) {
	var rc RECT
	rc.Right, rc.Bottom = w, h
	br, _, _ := pCreateSolidBrush.Call(0)
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&rc)), br)
	pDeleteObject.Call(br)
}

// renderCard 渲染卡片：背景层（可透明）+ 内容层（不透明）合成后提交给分层窗口。
func renderCard() {
	L := layoutCard()
	if !cardSurf.ensure(L.W, L.H) || !cardContent.ensure(L.W, L.H) || !cardMask.ensure(L.W, L.H) {
		return
	}

	// 内容层：黑底 + 正常颜色
	fillBlack(cardContent.dc, L.W, L.H)
	monoMode = false
	drawCardContent(cardContent.dc, L)

	// 遮罩层：黑底 + 白色，亮度即覆盖率
	fillBlack(cardMask.dc, L.W, L.H)
	monoMode = true
	drawCardContent(cardMask.dc, L)
	monoMode = false

	composeCard(L)
	if state.Hwnd != 0 {
		commitLayered(state.Hwnd, &cardSurf)
	}
}

// composeCard 把内容层按遮罩覆盖率叠加到背景上，输出预乘 alpha 的位图。
func composeCard(L cardLayout) {
	w, h := int(L.W), int(L.H)
	out := cardSurf.pixels()
	ct := cardContent.pixels()
	mk := cardMask.pixels()

	bgA := float64(cfg.Alpha) / 255
	const bgR, bgG, bgB = 22.0, 24.0, 29.0

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4

			var at float64
			if L.HasIcon && int32(x) >= L.Icon.Left && int32(x) < L.Icon.Right &&
				int32(y) >= L.Icon.Top && int32(y) < L.Icon.Bottom {
				// 图标区：用 白底-黑底 反解真实 alpha
				d := 0
				for c := 0; c < 3; c++ {
					if v := int(mk[i+c]) - int(ct[i+c]); v > d {
						d = v
					}
				}
				at = 1 - float64(d)/255
				if at < 0 {
					at = 0
				}
			} else {
				// 覆盖率：取遮罩层最亮通道
				m := mk[i]
				if mk[i+1] > m {
					m = mk[i+1]
				}
				if mk[i+2] > m {
					m = mk[i+2]
				}
				at = float64(m) / 255
			}
			cov := cornerCoverage(int32(x), int32(y), L.W, L.H, L.Radius)

			a := (at + bgA*(1-at)) * cov
			out[i+3] = uint8(a*255 + 0.5)

			// 预乘颜色：内容层已是预乘值，叠加背景贡献
			k := bgA * (1 - at) * cov
			cr := float64(ct[i+2])*cov + bgR*k
			cg := float64(ct[i+1])*cov + bgG*k
			cb := float64(ct[i])*cov + bgB*k
			if cr > 255 {
				cr = 255
			}
			if cg > 255 {
				cg = 255
			}
			if cb > 255 {
				cb = 255
			}
			out[i+2] = uint8(cr + 0.5)
			out[i+1] = uint8(cg + 0.5)
			out[i] = uint8(cb + 0.5)
		}
	}
}

func drawStr(dc uintptr, s string, rc RECT, font uintptr, color uintptr, flags uintptr) {
	if s == "" {
		return
	}
	if monoMode {
		color = rgb(255, 255, 255)
	}
	pSelectObject.Call(dc, font)
	pSetTextColor.Call(dc, color)
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return
	}
	pDrawTextW.Call(dc, uintptr(unsafe.Pointer(p)), uintptr(^uint32(0)),
		uintptr(unsafe.Pointer(&rc)), flags)
	runtime.KeepAlive(p)
}

func accentFor(pct float64) uintptr {
	switch {
	case pct >= 60:
		return rgb(248, 113, 113)
	case pct >= 30:
		return rgb(250, 204, 21)
	default:
		return rgb(74, 222, 128)
	}
}

func drawCardContent(dc uintptr, L cardLayout) {
	pSetBkMode.Call(dc, TRANSPARENT)

	// 图标
	if L.HasIcon {
		if hIcon := iconFor(state.ExePath); hIcon != 0 {
			if monoMode {
				// 遮罩层里图标区先铺白：白底结果 W = c*a + 255*(1-a)，
				// 与内容层的黑底结果 B = c*a 相减即可反解图标自身 alpha。
				br, _, _ := pCreateSolidBrush.Call(rgb(255, 255, 255))
				pFillRect.Call(dc, uintptr(unsafe.Pointer(&L.Icon)), br)
				pDeleteObject.Call(br)
			}
			pDrawIconEx.Call(dc, i32(L.Icon.Left), i32(L.Icon.Top), hIcon,
				uintptr(L.Icon.Right-L.Icon.Left), uintptr(L.Icon.Bottom-L.Icon.Top), 0, 0, DI_NORMAL)
		}
	}

	// 进程名
	name := state.ProcName
	nameColor := rgb(232, 236, 243)
	if state.TargetName == "" {
		name = "未选择进程"
		nameColor = rgb(186, 194, 212)
	} else if !state.Alive {
		name = state.TargetName
		nameColor = rgb(186, 194, 212)
	}
	if L.HasName {
		drawStr(dc, name, L.Name, fontName, nameColor,
			DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS|DT_NOPREFIX)
	}

	// PID / 进程树信息（清理提示优先占用这一行）
	if L.HasPid {
		var sub string
		subColor := rgb(186, 194, 212)
		switch {
		case toastActive():
			sub = state.Toast
			subColor = rgb(74, 222, 128)
		case state.TargetName == "":
			sub = "右键选择要监控的进程"
		case !state.Alive:
			sub = "未运行 · 等待启动"
		case cfg.Tree:
			sub = fmt.Sprintf("PID %d · 进程树 %d 个", state.TargetPid, state.ProcCount)
		default:
			sub = fmt.Sprintf("PID %d", state.TargetPid)
		}
		drawStr(dc, sub, L.Pid, fontSub, subColor,
			DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS|DT_NOPREFIX)
	} else if toastActive() {
		// 关掉 PID 行时，提示退到进程名行右侧
		drawStr(dc, state.Toast, L.Name, fontSub, rgb(74, 222, 128),
			DT_RIGHT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
	}

	accent := rgb(110, 116, 132)
	if state.Alive {
		accent = accentFor(state.Pct)
	}

	// 内存占用
	if L.HasMem {
		memStr := "--"
		memColor := rgb(120, 126, 142)
		if state.Alive {
			memStr = fmtBytes(state.MemBytes)
			memColor = rgb(255, 255, 255)
		}
		drawStr(dc, memStr, L.Mem, fontMem, memColor,
			DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
	}

	// 占比
	if L.HasPct {
		pctStr := ""
		if state.Alive {
			pctStr = fmt.Sprintf("%.1f%%", state.Pct)
		}
		drawStr(dc, pctStr, L.Pct, fontPct, accent,
			DT_RIGHT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
	}

	// 趋势图
	if L.HasSpark {
		drawSpark(dc, L.Spark, accent)
	}
}

func drawSpark(dc uintptr, rc RECT, color uintptr) {
	if monoMode {
		color = rgb(255, 255, 255)
	}
	n := state.histLen
	if n < 2 {
		return
	}
	var mx uint64 = 1
	for i := 0; i < n; i++ {
		if v := state.histAt(i); v > mx {
			mx = v
		}
	}
	if mx <= 1 {
		return
	}
	mx = mx * 13 / 10 // 留出上方空间，平稳时曲线不会贴着顶边

	w := rc.Right - rc.Left
	h := rc.Bottom - rc.Top
	if w < 4 || h < 4 {
		return
	}

	var pts [90]POINT
	for i := 0; i < n; i++ {
		x := rc.Left + int32(float64(w)*float64(i)/float64(n-1))
		y := rc.Bottom - 1 - int32(float64(h-2)*float64(state.histAt(i))/float64(mx))
		pts[i] = POINT{X: x, Y: y}
	}

	pen, _, _ := pCreatePen.Call(PS_SOLID, 2, color)
	old, _, _ := pSelectObject.Call(dc, pen)
	pPolyline.Call(dc, uintptr(unsafe.Pointer(&pts[0])), uintptr(n))
	pSelectObject.Call(dc, old)
	pDeleteObject.Call(pen)
}

// ---------------- 窗口尺寸 / 置顶 / 透明度 ----------------

func applyLayout() {
	// 先同步窗口当前实际位置，避免改配置时把窗口弹回初始位置
	if state.Hwnd != 0 {
		var wr RECT
		if ok, _, _ := pGetWindowRect.Call(state.Hwnd, uintptr(unsafe.Pointer(&wr))); ok != 0 {
			state.X = wr.Left
			state.Y = wr.Top
		}
	}

	L := layoutCard()
	ins := hwndNoTopMost
	if cfg.TopMost {
		ins = hwndTopMost
	}
	pSetWindowPos.Call(state.Hwnd, ins, i32(state.X), i32(state.Y),
		uintptr(L.W), uintptr(L.H), SWP_NOACTIVATE|SWP_SHOWWINDOW)
	renderCard()
}

// ---------------- 透明度 ----------------

func alphaPercent() int {
	p := int(float64(cfg.Alpha)/255*100 + 0.5)
	if p > 100 {
		p = 100
	}
	if p < 10 {
		p = 10
	}
	return p
}

func setAlphaPercent(p int) {
	if p < 10 {
		p = 10
	}
	if p > 100 {
		p = 100
	}
	cfg.Alpha = uint8(float64(p)*255/100 + 0.5)
	renderCard()
	saveConfig()
}

func restartTimer() {
	pKillTimer.Call(state.Hwnd, 1)
	pSetTimer.Call(state.Hwnd, 1, uintptr(cfg.Interval), 0)
}

// ---------------- 右键菜单 ----------------

const (
	cmdPick = 100 + iota
	cmdTree
	cmdIcon
	cmdName
	cmdPid
	cmdMem
	cmdPct
	cmdSpark
	cmdTop
	cmdInt05
	cmdInt1
	cmdInt2
	cmdMetricPrivate
	cmdMetricWorking
	cmdLockPos
	cmdLockSize
	cmdCleanTarget
	cmdCleanAll
	cmdCleanSys
	cmdAutoCleanOff
	cmdExport1h
	cmdExport6h
	cmdExport24h
	cmdExport7d
	cmdExportAll
	cmdOpenLogs
	cmdToggleLog
	cmdExit
)

// 带参数的菜单项单独编号，绝不能和上面按 iota 递增的编号重叠，
// 否则 handleMenu 里的区间判断会把这些命令误判成"第 N 档透明度"。
const (
	cmdAlphaBase     = 200 // +0..+9 → 100%..10%
	cmdAutoCleanBase = 220 // +0..+3 → 60/70/80/90%
	cmdScaleBase     = 240 // +0..+3 → 85/100/115/130%
)

// 界面缩放档位
var scaleSteps = [4]int{85, 100, 115, 130}
var scaleNames = [4]string{"小", "标准", "大", "特大"}

func appendItem(menu uintptr, id int, text string, checked, grayed bool) {
	flags := uintptr(MF_STRING)
	if checked {
		flags |= MF_CHECKED
	}
	if grayed {
		flags |= MF_GRAYED
	}
	p, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	pAppendMenuW.Call(menu, flags, uintptr(id), uintptr(unsafe.Pointer(p)))
	runtime.KeepAlive(p)
}

// exportRange 导出最近 d 时间段的记录，d<=0 表示全部。
func exportRange(d time.Duration) {
	to := time.Now()
	var from time.Time
	if d > 0 {
		from = to.Add(-d)
	}

	name := "memlog"
	if state.TargetName != "" {
		name += "-" + strings.TrimSuffix(state.TargetName, ".exe")
	}
	name += "-" + to.Format("20060102-1504") + ".csv"

	path := saveFileDialog(state.Hwnd, name, "导出内存占用记录")
	if path == "" {
		return
	}
	n, err := exportLogs(from, to, path)
	if err != nil {
		showToast("导出失败")
		return
	}
	if n == 0 {
		showToast("该时间段没有记录")
		return
	}
	showToast(fmt.Sprintf("已导出 %d 条记录", n))
}

func showMenu(hwnd uintptr) {
	menu, _, _ := pCreatePopupMenu.Call()
	if menu == 0 {
		return
	}

	appendItem(menu, cmdPick, "选择要监控的进程…\t双击卡片", false, false)
	pAppendMenuW.Call(menu, MF_SEPARATOR, 0, 0)

	// 内存清理
	subClean, _, _ := pCreatePopupMenu.Call()
	appendItem(subClean, cmdCleanTarget, "清理当前进程", false, state.TargetName == "")
	appendItem(subClean, cmdCleanAll, "清理所有进程", false, false)
	appendItem(subClean, cmdCleanSys, "清理系统缓存（需管理员）", false, false)
	pAppendMenuW.Call(subClean, MF_SEPARATOR, 0, 0)

	// 自动清理：整机内存占用超过阈值时自动执行
	autoPcts := [4]int{60, 70, 80, 90}
	for i, p := range autoPcts {
		appendItem(subClean, cmdAutoCleanBase+i, fmt.Sprintf("内存超过 %d%% 自动清理", p),
			cfg.AutoCleanPct == p, false)
	}
	appendItem(subClean, cmdAutoCleanOff, "关闭自动清理", cfg.AutoCleanPct == 0, false)

	cltxt, _ := syscall.UTF16PtrFromString("清理内存")
	pAppendMenuW.Call(menu, MF_POPUP, subClean, uintptr(unsafe.Pointer(cltxt)))
	runtime.KeepAlive(cltxt)

	// 内存记录：按时间段导出 CSV
	subLog, _, _ := pCreatePopupMenu.Call()
	appendItem(subLog, cmdToggleLog, "记录内存占用", cfg.LogEnabled, false)
	pAppendMenuW.Call(subLog, MF_SEPARATOR, 0, 0)
	appendItem(subLog, cmdExport1h, "导出最近 1 小时", false, false)
	appendItem(subLog, cmdExport6h, "导出最近 6 小时", false, false)
	appendItem(subLog, cmdExport24h, "导出最近 24 小时", false, false)
	appendItem(subLog, cmdExport7d, "导出最近 7 天", false, false)
	appendItem(subLog, cmdExportAll, "导出全部记录", false, false)
	pAppendMenuW.Call(subLog, MF_SEPARATOR, 0, 0)
	appendItem(subLog, cmdOpenLogs, "打开日志文件夹", false, false)

	ltxt, _ := syscall.UTF16PtrFromString("内存记录")
	pAppendMenuW.Call(menu, MF_POPUP, subLog, uintptr(unsafe.Pointer(ltxt)))
	runtime.KeepAlive(ltxt)

	appendItem(menu, cmdTree, "监控进程树（含子进程）", cfg.Tree, false)
	appendItem(menu, cmdIcon, "显示进程图标", cfg.ShowIcon, false)
	appendItem(menu, cmdName, "显示进程名", cfg.ShowName, false)
	appendItem(menu, cmdPid, "显示 PID", cfg.ShowPid, false)
	appendItem(menu, cmdMem, "显示内存占用", cfg.ShowMem, false)
	appendItem(menu, cmdPct, "显示内存占比", cfg.ShowPct, false)
	appendItem(menu, cmdSpark, "显示趋势图", cfg.ShowSpark, false)

	pAppendMenuW.Call(menu, MF_SEPARATOR, 0, 0)

	// 内存口径
	subMetric, _, _ := pCreatePopupMenu.Call()
	appendItem(subMetric, cmdMetricPrivate, "私有工作集（同任务管理器）", cfg.Metric == "private", false)
	appendItem(subMetric, cmdMetricWorking, "工作集（含共享内存）", cfg.Metric == "working", false)
	mtxt, _ := syscall.UTF16PtrFromString("内存口径")
	pAppendMenuW.Call(menu, MF_POPUP, subMetric, uintptr(unsafe.Pointer(mtxt)))
	runtime.KeepAlive(mtxt)

	// 刷新间隔
	subInt, _, _ := pCreatePopupMenu.Call()
	appendItem(subInt, cmdInt05, "0.5 秒", cfg.Interval == 500, false)
	appendItem(subInt, cmdInt1, "1 秒", cfg.Interval == 1000, false)
	appendItem(subInt, cmdInt2, "2 秒", cfg.Interval == 2000, false)
	ptxt, _ := syscall.UTF16PtrFromString("刷新间隔")
	pAppendMenuW.Call(menu, MF_POPUP, subInt, uintptr(unsafe.Pointer(ptxt)))
	runtime.KeepAlive(ptxt)

	// 透明度：10% ~ 100%，每 10% 一档
	subAlpha, _, _ := pCreatePopupMenu.Call()
	cur := alphaPercent()
	for p := 100; p >= 10; p -= 10 {
		appendItem(subAlpha, cmdAlphaBase+(100-p)/10, fmt.Sprintf("%d%%", p), cur == p, false)
	}
	atxt, _ := syscall.UTF16PtrFromString("不透明度")
	pAppendMenuW.Call(menu, MF_POPUP, subAlpha, uintptr(unsafe.Pointer(atxt)))
	runtime.KeepAlive(atxt)

	pAppendMenuW.Call(menu, MF_SEPARATOR, 0, 0)
	appendItem(menu, cmdTop, "窗口置顶", cfg.TopMost, false)

	// 界面大小：整体缩放（高度随内容自动变化，靠这里调大调小）
	subScale, _, _ := pCreatePopupMenu.Call()
	for i, name := range scaleNames {
		appendItem(subScale, cmdScaleBase+i, name, cfg.Scale == scaleSteps[i], false)
	}
	stxt, _ := syscall.UTF16PtrFromString("界面大小")
	pAppendMenuW.Call(menu, MF_POPUP, subScale, uintptr(unsafe.Pointer(stxt)))
	runtime.KeepAlive(stxt)

	appendItem(menu, cmdLockPos, "锁定位置", cfg.LockPos, false)
	appendItem(menu, cmdLockSize, "锁定大小", cfg.LockSize, false)
	pAppendMenuW.Call(menu, MF_SEPARATOR, 0, 0)
	appendItem(menu, cmdExit, "退出", false, false)

	var pt POINT
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(hwnd)
	cmd, _, _ := pTrackPopupMenu.Call(menu,
		TPM_RETURNCMD|TPM_LEFTALIGN|TPM_TOPALIGN|TPM_RIGHTBUTTON,
		i32(pt.X), i32(pt.Y), 0, hwnd, 0)
	pDestroyMenu.Call(menu)

	if cmd != 0 {
		handleMenu(int(cmd))
	}
}

func handleMenu(id int) {
	if id >= cmdAlphaBase && id < cmdAlphaBase+10 {
		setAlphaPercent(100 - (id-cmdAlphaBase)*10)
		return
	}
	if id >= cmdAutoCleanBase && id < cmdAutoCleanBase+4 {
		cfg.AutoCleanPct = [4]int{60, 70, 80, 90}[id-cmdAutoCleanBase]
		nextAutoClean = 0
		saveConfig()
		return
	}
	if id >= cmdScaleBase && id < cmdScaleBase+4 {
		setScale(scaleSteps[id-cmdScaleBase])
		return
	}

	toggled := false
	switch id {
	case cmdPick:
		showPicker()
	case cmdTree:
		cfg.Tree = !cfg.Tree
		toggled = true
		sample()
	case cmdIcon:
		cfg.ShowIcon = !cfg.ShowIcon
		toggled = true
	case cmdName:
		cfg.ShowName = !cfg.ShowName
		toggled = true
	case cmdPid:
		cfg.ShowPid = !cfg.ShowPid
		toggled = true
	case cmdMem:
		cfg.ShowMem = !cfg.ShowMem
		toggled = true
	case cmdPct:
		cfg.ShowPct = !cfg.ShowPct
		toggled = true
	case cmdSpark:
		cfg.ShowSpark = !cfg.ShowSpark
		toggled = true
	case cmdTop:
		cfg.TopMost = !cfg.TopMost
		toggled = true
	case cmdLockPos:
		cfg.LockPos = !cfg.LockPos
		saveConfig()
		return
	case cmdLockSize:
		cfg.LockSize = !cfg.LockSize
		saveConfig()
		return
	case cmdCleanTarget:
		showToast(cleanTarget().Text)
		return
	case cmdCleanAll:
		r := cleanAll()
		sample()
		showToast(r.Text)
		return
	case cmdCleanSys:
		showToast(cleanSystem().Text)
		return
	case cmdAutoCleanOff:
		cfg.AutoCleanPct = 0
		saveConfig()
		return
	case cmdToggleLog:
		cfg.LogEnabled = !cfg.LogEnabled
		logger.flush()
		saveConfig()
		showToast(map[bool]string{true: "已开启内存记录", false: "已停止内存记录"}[cfg.LogEnabled])
		return
	case cmdExport1h:
		exportRange(time.Hour)
		return
	case cmdExport6h:
		exportRange(6 * time.Hour)
		return
	case cmdExport24h:
		exportRange(24 * time.Hour)
		return
	case cmdExport7d:
		exportRange(7 * 24 * time.Hour)
		return
	case cmdExportAll:
		exportRange(0)
		return
	case cmdOpenLogs:
		openFolder(logDir())
		return
	case cmdInt05:
		cfg.Interval = 500
		restartTimer()
	case cmdInt1:
		cfg.Interval = 1000
		restartTimer()
	case cmdInt2:
		cfg.Interval = 2000
		restartTimer()
	case cmdMetricPrivate:
		cfg.Metric = "private"
		toggled = true
		sample()
	case cmdMetricWorking:
		cfg.Metric = "working"
		toggled = true
		sample()
	case cmdExit:
		pDestroyWindow.Call(state.Hwnd)
		return
	}

	if toggled {
		applyLayout()
		saveConfig()
	}
}
