package main

import (
	"fmt"
	"sort"
	"strings"
	"syscall"
	"unsafe"
)

// 进程选择窗口：完全自绘的深色界面，与悬浮卡片风格一致。

const (
	NULL_BRUSH = 5
	NULL_PEN   = 8

	idSearch = 10
)

type pickItem struct {
	Pid   uint32
	Name  string
	Mem   uint64
	Pct   float64
	Path  string
	Tried bool
}

type pickerState struct {
	Hwnd   uintptr
	Search uintptr

	Items    []pickItem
	Filtered []int

	Scroll int // 第一个可见行（Filtered 下标）
	Sel    int // 选中行（Filtered 下标），-1 表示无

	HoverRow int // 鼠标悬停的行
	HoverBtn int // 1=关闭 2=取消 3=开始 5=内存排序 6=名称排序

	SortBy   int  // 0=内存 1=名称
	SortDesc bool // 倒序
	Pressed  int // 鼠标按下中的按钮

	W, H int32
}

var pk = pickerState{}

var (
	handCursor uintptr
	editBrush  uintptr
	editProcCB = syscall.NewCallback(editProc)
)

// initPickerResources 准备光标与搜索框背景刷。
func initPickerResources() {
	h, _, _ := pLoadCursorW.Call(0, 32649) // IDC_HAND
	if h == 0 {
		h = cursorArrow()
	}
	handCursor = h
	editBrush, _, _ = pCreateSolidBrush.Call(rgb(31, 34, 41))
}

// ---------------- 布局 ----------------

type pickLayout struct {
	W, H      int32
	Title     RECT
	Close     RECT
	SearchBox RECT
	Search    RECT
	List      RECT
	BtnCancel RECT
	BtnOK     RECT
	BtnSortMem  RECT
	BtnSortName RECT
	BtnRefresh  RECT
	BtnAuto     RECT
	RowH      int32
}

func layoutPicker() pickLayout {
	var L pickLayout
	L.W = sc(560)
	L.H = sc(620)
	pad := sc(18)

	L.Close = RECT{L.W - pad - sc(30), sc(12), L.W - pad, sc(42)}
	sortW := sc(64)
	L.BtnSortName = RECT{L.Close.Left - sc(10) - sortW, sc(12), L.Close.Left - sc(10), sc(42)}
	L.BtnSortMem = RECT{L.BtnSortName.Left - sc(6) - sortW, sc(12), L.BtnSortName.Left - sc(6), sc(42)}
	L.Title = RECT{pad, sc(14), L.BtnSortMem.Left - sc(8), sc(42)}

	btnW := sc(58)
	autoX := L.W - pad - btnW
	refX := autoX - sc(6) - btnW
	L.SearchBox = RECT{pad, sc(52), refX - sc(8), sc(52) + sc(38)}
	L.BtnRefresh = RECT{refX, sc(52), refX + btnW, sc(52) + sc(38)}
	L.BtnAuto = RECT{autoX, sc(52), autoX + btnW, sc(52) + sc(38)}
	L.Search = RECT{L.SearchBox.Left + sc(12), sc(52) + sc(8), L.SearchBox.Right - sc(12), sc(52) + sc(30)}

	btnH := sc(38)
	btnY := L.H - pad - btnH
	L.BtnOK = RECT{L.W - pad - sc(112), btnY, L.W - pad, btnY + btnH}
	L.BtnCancel = RECT{L.BtnOK.Left - sc(10) - sc(92), btnY, L.BtnOK.Left - sc(10), btnY + btnH}

	L.List = RECT{pad, sc(104), L.W - pad, btnY - sc(20)}
	L.RowH = sc(48)
	return L
}

// ---------------- 窗口创建 ----------------

func createPickerWindow() {
	className := utf16("MemMonPicker")
	var wc WNDCLASSEXW
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.Style = CS_DBLCLKS
	wc.LpfnWndProc = pickerWndProcCB
	wc.HInstance = hInst
	wc.HCursor = cursorArrow()
	wc.LpszClassName = className
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	L := layoutPicker()
	sw, _, _ := pGetSystemMetrics.Call(0)
	sh, _, _ := pGetSystemMetrics.Call(1)
	x := (int32(sw) - L.W) / 2
	y := (int32(sh) - L.H) / 2
	if y < sc(30) {
		y = sc(30)
	}

	title := utf16("MemMonitor")
	hwnd, _, _ := pCreateWindowExW.Call(uintptr(WS_EX_TOOLWINDOW|WS_EX_TOPMOST),
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)),
		uintptr(WS_POPUP|WS_CLIPCHILDREN),
		i32(x), i32(y), uintptr(L.W), uintptr(L.H), 0, 0, hInst, 0)
	if hwnd == 0 {
		return
	}
	pk.Hwnd = hwnd
	pk.W, pk.H = L.W, L.H
	pk.SortBy = cfg.SortBy
	pk.SortDesc = cfg.SortDesc
	// Win11 原生圆角（抗锯齿，比 region 裁剪干净）
	pref := int32(DWMWCP_ROUND)
	pDwmSetWindowAttribute.Call(hwnd, DWMWA_WINDOW_CORNER_PREFERENCE,
		uintptr(unsafe.Pointer(&pref)), 4)
	pk.Sel = -1
	pk.HoverRow = -1

	// 搜索框（无边框，背景由 WM_CTLCOLOREDIT 控制）
	pk.Search, _, _ = pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16("EDIT"))), 0,
		uintptr(WS_CHILD|WS_VISIBLE|ES_AUTOHSCROLL),
		i32(L.Search.Left), i32(L.Search.Top),
		uintptr(L.Search.Right-L.Search.Left), uintptr(L.Search.Bottom-L.Search.Top),
		hwnd, uintptr(idSearch), hInst, 0)
	pSendMessageW.Call(pk.Search, WM_SETFONT, fontUI, 1)

	if pSetWindowLongPtrW.Find() == nil {
		old, _, _ := pSetWindowLongPtrW.Call(pk.Search, GWLP_WNDPROC, editProcCB)
		editOldProc = old
	}

}

var editOldProc uintptr

func editProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch uint32(msg) {
	case WM_KEYDOWN:
		switch wParam {
		case VK_UP, VK_DOWN, VK_RETURN, VK_ESCAPE:
			pSendMessageW.Call(pk.Hwnd, WM_KEYDOWN, wParam, 0)
			return 0
		}
	case WM_PAINT:
		// 先让 EDIT 自己绘制，再在空内容时叠加占位提示
		r, _, _ := pCallWindowProcW.Call(editOldProc, hwnd, msg, wParam, lParam)
		return r
	}
	r, _, _ := pCallWindowProcW.Call(editOldProc, hwnd, msg, wParam, lParam)
	return r
}

// ---------------- 显示 / 隐藏 ----------------

func showPicker() {
	if pk.Hwnd == 0 {
		return
	}
	relayoutPicker()
	refreshPicker()
	pShowWindow.Call(pk.Hwnd, SW_SHOW)
	if cfg.AutoRefresh {
		pSetTimer.Call(pk.Hwnd, 2, 2000, 0)
	}
	pSetForegroundWindow.Call(pk.Hwnd)
	// 焦点给主窗口：占位提示可见，方向键/回车可用；点击搜索框才进入输入
	pSetFocus.Call(pk.Hwnd)
}

func hidePicker() {
	if pk.Hwnd != 0 {
		pKillTimer.Call(pk.Hwnd, 2)
		pShowWindow.Call(pk.Hwnd, SW_HIDE)
	}
}

// relayoutPicker 在界面缩放变化后重新摆正窗口尺寸与位置。
func relayoutPicker() {
	L := layoutPicker()
	if pk.W == L.W && pk.H == L.H {
		return
	}
	pk.W, pk.H = L.W, L.H

	sw, _, _ := pGetSystemMetrics.Call(0)
	sh, _, _ := pGetSystemMetrics.Call(1)
	x := (int32(sw) - L.W) / 2
	y := (int32(sh) - L.H) / 2
	if y < sc(30) {
		y = sc(30)
	}
	if y < 0 {
		y = 0
	}
	pSetWindowPos.Call(pk.Hwnd, 0, i32(x), i32(y), uintptr(L.W), uintptr(L.H),
		SWP_NOZORDER|SWP_NOACTIVATE)
}

func editText(h uintptr) string {
	var buf [512]uint16
	n, _, _ := pGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

// ---------------- 数据 ----------------

func refreshPicker() {
	if pk.Hwnd == 0 {
		return
	}
	prevPid := uint32(0)
	if pk.Sel >= 0 && pk.Sel < len(pk.Filtered) {
		prevPid = pk.Items[pk.Filtered[pk.Sel]].Pid
	}

	procs := snapshotProcs()
	total, _ := memStatus()

	items := make([]pickItem, 0, len(procs))
	for _, p := range procs {
		pm, ok := procMemory(p.Pid)
		if !ok {
			continue
		}
		mem := pm.Value(cfg.Metric)
		pct := 0.0
		if total > 0 {
			pct = float64(mem) / float64(total) * 100
		}
		items = append(items, pickItem{Pid: p.Pid, Name: p.Name, Mem: mem, Pct: pct})
	}
	pk.Items = items
	sortItems()
	reapplyFilter(prevPid)
	pInvalidateRect.Call(pk.Hwnd, 0, 0)
}

// sortItems 按当前排序设置重排列表。
func sortItems() {
	items := pk.Items
	if pk.SortBy == 1 {
		sort.SliceStable(items, func(i, j int) bool {
			a := strings.ToLower(items[i].Name)
			b := strings.ToLower(items[j].Name)
			if a == b {
				return items[i].Pid < items[j].Pid
			}
			if pk.SortDesc {
				return a > b
			}
			return a < b
		})
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Mem == items[j].Mem {
			return items[i].Pid < items[j].Pid
		}
		if pk.SortDesc {
			return items[i].Mem > items[j].Mem
		}
		return items[i].Mem < items[j].Mem
	})
}

// reapplyFilter 重新过滤并尽量保持选中项。
func reapplyFilter(keepPid uint32) {
	fillFilter(editText(pk.Search))
	pk.Sel = -1
	if keepPid != 0 {
		for i, idx := range pk.Filtered {
			if pk.Items[idx].Pid == keepPid {
				pk.Sel = i
				break
			}
		}
	}
	if pk.Sel < 0 {
		for i, idx := range pk.Filtered {
			if pk.Items[idx].Pid == state.TargetPid {
				pk.Sel = i
				break
			}
		}
	}
	if pk.Sel < 0 && len(pk.Filtered) > 0 {
		pk.Sel = 0
	}
	ensureVisible()
}

// toggleSort 切换排序字段或方向。field: 0=内存 1=名称
func toggleSort(field int) {
	keepPid := uint32(0)
	if pk.Sel >= 0 && pk.Sel < len(pk.Filtered) {
		keepPid = pk.Items[pk.Filtered[pk.Sel]].Pid
	}
	if pk.SortBy == field {
		pk.SortDesc = !pk.SortDesc
	} else {
		pk.SortBy = field
		pk.SortDesc = field == 0 // 内存默认降序，名称默认升序
	}
	cfg.SortBy = pk.SortBy
	cfg.SortDesc = pk.SortDesc
	saveConfig()

	sortItems()
	reapplyFilter(keepPid)
	pInvalidateRect.Call(pk.Hwnd, 0, 0)
}

func drawSmallBtn(dc uintptr, r RECT, label string, hover, pressed, active bool) {
	bg := uintptr(0)
	switch {
	case pressed:
		bg = rgb(52, 58, 72)
	case hover:
		bg = rgb(44, 50, 62)
	case active:
		bg = rgb(38, 44, 56)
	}
	if bg != 0 {
		roundRect(dc, r, sc(9), bg, 0)
	}
	col := rgb(150, 158, 176)
	if active {
		col = rgb(74, 222, 128)
	}
	if hover || pressed {
		col = rgb(232, 236, 243)
	}
	drawStr(dc, label, r, fontUI, col, DT_CENTER|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
}

// toggleAutoRefresh 开关列表自动刷新。
func toggleAutoRefresh() {
	cfg.AutoRefresh = !cfg.AutoRefresh
	saveConfig()
	if cfg.AutoRefresh {
		pSetTimer.Call(pk.Hwnd, 2, 2000, 0)
	} else {
		pKillTimer.Call(pk.Hwnd, 2)
	}
	pInvalidateRect.Call(pk.Hwnd, 0, 0)
}

func drawSortBtn(dc uintptr, r RECT, label string, active, desc bool) {
	if active {
		roundRect(dc, r, sc(7), rgb(40, 46, 58), 0)
	}
	col := rgb(120, 128, 146)
	if active {
		col = rgb(232, 236, 243)
	}
	txt := label
	if active {
		if desc {
			txt = label + " ↓"
		} else {
			txt = label + " ↑"
		}
	}
	drawStr(dc, txt, r, fontUI, col, DT_CENTER|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
}

func fillFilter(q string) {
	q = strings.ToLower(strings.TrimSpace(q))
	pk.Filtered = pk.Filtered[:0]
	for i, it := range pk.Items {
		if q != "" &&
			!strings.Contains(strings.ToLower(it.Name), q) &&
			!strings.Contains(fmt.Sprintf("%d", it.Pid), q) {
			continue
		}
		pk.Filtered = append(pk.Filtered, i)
	}
	pk.Scroll = 0
	pk.Sel = 0
	if len(pk.Filtered) == 0 {
		pk.Sel = -1
	}
}

func visibleRows() int {
	L := layoutPicker()
	n := int((L.List.Bottom - L.List.Top) / L.RowH)
	if n < 1 {
		n = 1
	}
	return n
}

func maxScroll() int {
	n := len(pk.Filtered) - visibleRows()
	if n < 0 {
		return 0
	}
	return n
}

func ensureVisible() {
	vis := visibleRows()
	if pk.Sel < pk.Scroll {
		pk.Scroll = pk.Sel
	}
	if pk.Sel >= pk.Scroll+vis {
		pk.Scroll = pk.Sel - vis + 1
	}
	if pk.Scroll > maxScroll() {
		pk.Scroll = maxScroll()
	}
	if pk.Scroll < 0 {
		pk.Scroll = 0
	}
}

func pickerConfirm() {
	if pk.Sel < 0 || pk.Sel >= len(pk.Filtered) {
		return
	}
	it := pk.Items[pk.Filtered[pk.Sel]]
	setTarget(it.Name, it.Pid)
	hidePicker()
}

// ---------------- 绘制 ----------------

func roundRect(dc uintptr, r RECT, radius int32, fill, border uintptr) {
	c := fill
	if c == 0 {
		c = border
	}
	if c == 0 {
		return
	}
	fillRoundRectAA(dc, r, radius, c)
}

func paintPicker(hwnd uintptr) {
	var ps PAINTSTRUCT
	hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))

	var rc RECT
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	w, h := rc.Right, rc.Bottom
	if w <= 0 || h <= 0 {
		return
	}

	mdc, _, _ := pCreateCompatibleDC.Call(hdc)
	bm, _, _ := pCreateCompatibleBitmap.Call(hdc, uintptr(w), uintptr(h))
	old, _, _ := pSelectObject.Call(mdc, bm)

	drawPickerContent(mdc, w, h)

	pBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), mdc, 0, 0, SRCCOPY)
	pSelectObject.Call(mdc, old)
	pDeleteObject.Call(bm)
	pDeleteDC.Call(mdc)
}

func drawPickerContent(dc uintptr, w, h int32) {
	L := layoutPicker()
	pSetBkMode.Call(dc, TRANSPARENT)

	// 背景
	bg, _, _ := pCreateSolidBrush.Call(rgb(22, 24, 29))
	var full RECT
	full.Right, full.Bottom = w, h
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&full)), bg)
	pDeleteObject.Call(bg)

	// 标题
	drawStr(dc, "选择要监控的进程", L.Title, fontName, rgb(232, 236, 243),
		DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS|DT_NOPREFIX)

	// 排序按钮
	drawSortBtn(dc, L.BtnSortMem, "内存", pk.SortBy == 0, pk.SortDesc)
	drawSortBtn(dc, L.BtnSortName, "名称", pk.SortBy == 1, pk.SortDesc)

	// 关闭按钮
	if pk.HoverBtn == 1 {
		roundRect(dc, L.Close, sc(8), rgb(42, 46, 56), 0)
	}
	cx := (L.Close.Left + L.Close.Right) / 2
	cy := (L.Close.Top + L.Close.Bottom) / 2
	r := sc(6)
	pen, _, _ := pCreatePen.Call(PS_SOLID, uintptr(sc(2)), rgb(150, 158, 175))
	op, _, _ := pSelectObject.Call(dc, pen)
	pMoveToEx2.Call(dc, i32(cx-r), i32(cy-r), 0)
	pLineTo2.Call(dc, i32(cx+r+1), i32(cy+r+1))
	pMoveToEx2.Call(dc, i32(cx+r), i32(cy-r), 0)
	pLineTo2.Call(dc, i32(cx-r-1), i32(cy+r+1))
	pSelectObject.Call(dc, op)
	pDeleteObject.Call(pen)

	// 搜索框
	roundRect(dc, L.SearchBox, sc(9), rgb(31, 34, 41), 0)

	// 刷新 / 自动刷新
	drawSmallBtn(dc, L.BtnRefresh, "刷新", pk.HoverBtn == 7, pk.Pressed == 7, false)
	drawSmallBtn(dc, L.BtnAuto, "自动", pk.HoverBtn == 8, pk.Pressed == 8, cfg.AutoRefresh)

	// 列表
	drawList(dc, L)

	// 按钮
	drawPickerButtons(dc, L)
}

func drawList(dc uintptr, L pickLayout) {
	// 裁剪到列表区域，避免行溢出
	pIntersectClipRect.Call(dc, i32(L.List.Left), i32(L.List.Top), i32(L.List.Right), i32(L.List.Bottom))

	n := len(pk.Filtered)
	vis := visibleRows()

	for i := 0; i < vis; i++ {
		idx := pk.Scroll + i
		if idx >= n {
			break
		}
		y := L.List.Top + int32(i)*L.RowH
		if y >= L.List.Bottom {
			break
		}
		drawRow(dc, L, idx, y, idx == pk.Sel, idx == pk.HoverRow)
	}

	if n == 0 {
		drawStr(dc, "没有匹配的进程", RECT{L.List.Left, L.List.Top + sc(40), L.List.Right, L.List.Top + sc(66)},
			fontUI, rgb(110, 118, 134), DT_CENTER|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
	}

	// 解除裁剪后再画滚动条（否则会被裁掉）
	pSelectClipRgn.Call(dc, 0)

	if n > vis && n > 0 {
		trackTop := L.List.Top
		trackH := L.List.Bottom - L.List.Top
		thumbH := trackH * int32(vis) / int32(n)
		if thumbH < sc(30) {
			thumbH = sc(30)
		}
		den := int32(maxScroll())
		var off int32
		if den > 0 {
			off = (trackH - thumbH) * int32(pk.Scroll) / den
		}
		x := L.W - sc(12)
		roundRect(dc, RECT{x, trackTop + off, x + sc(5), trackTop + off + thumbH},
			sc(2), rgb(64, 70, 86), 0)
	}
}

func drawRow(dc uintptr, L pickLayout, idx int, y int32, sel, hover bool) {
	it := &pk.Items[pk.Filtered[idx]]
	r := RECT{L.List.Left, y, L.List.Right, y + L.RowH}

	if sel {
		roundRect(dc, RECT{r.Left, r.Top + sc(2), r.Right, r.Bottom - sc(2)}, sc(8), rgb(36, 43, 56), 0)
		// 左侧强调条
		roundRect(dc, RECT{r.Left + sc(3), r.Top + sc(12), r.Left + sc(6), r.Bottom - sc(12)},
			sc(1), rgb(52, 211, 153), 0)
	} else if hover {
		roundRect(dc, RECT{r.Left, r.Top + sc(2), r.Right, r.Bottom - sc(2)}, sc(8), rgb(30, 34, 42), 0)
	}

	// 图标（懒加载）
	if !it.Tried {
		it.Tried = true
		it.Path = procPath(it.Pid)
	}
	iconSize := sc(26)
	iconX := r.Left + sc(12)
	iconY := r.Top + (L.RowH-iconSize)/2
	if hIcon := iconFor(it.Path); hIcon != 0 {
		pDrawIconEx.Call(dc, i32(iconX), i32(iconY), hIcon,
			uintptr(iconSize), uintptr(iconSize), 0, 0, DI_NORMAL)
	}

	textX := iconX + iconSize + sc(12)

	// 右侧数值
	memStr := fmtBytes(it.Mem)
	pctStr := fmt.Sprintf("%.1f%%", it.Pct)
	drawStr(dc, memStr, RECT{r.Right - sc(150), r.Top + sc(6), r.Right - sc(14), r.Top + sc(26)},
		fontRowMem, rgb(236, 240, 246), DT_RIGHT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
	drawStr(dc, pctStr, RECT{r.Right - sc(150), r.Top + sc(26), r.Right - sc(14), r.Top + sc(42)},
		fontSub, rgb(126, 134, 152), DT_RIGHT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)

	// 名称 + PID
	nameColor := rgb(232, 236, 243)
	if sel {
		nameColor = rgb(255, 255, 255)
	}
	drawStr(dc, it.Name, RECT{textX, r.Top + sc(5), r.Right - sc(160), r.Top + sc(26)},
		fontName, nameColor, DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS|DT_NOPREFIX)
	drawStr(dc, fmt.Sprintf("PID %d", it.Pid),
		RECT{textX, r.Top + sc(25), r.Right - sc(160), r.Top + sc(42)},
		fontSub, rgb(122, 130, 148), DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
}

func drawPickerButtons(dc uintptr, L pickLayout) {
	// 取消
	if pk.HoverBtn == 2 || pk.Pressed == 2 {
		roundRect(dc, L.BtnCancel, sc(8), rgb(44, 49, 60), 0)
	} else {
		roundRect(dc, L.BtnCancel, sc(8), rgb(35, 39, 47), 0)
	}
	drawStr(dc, "取消", L.BtnCancel, fontUI, rgb(200, 206, 218),
		DT_CENTER|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)

	// 开始监控
	okBg := rgb(52, 211, 153)
	if pk.HoverBtn == 3 {
		okBg = rgb(74, 226, 168)
	}
	if pk.Pressed == 3 {
		okBg = rgb(40, 184, 132)
	}
	roundRect(dc, L.BtnOK, sc(8), okBg, 0)
	drawStr(dc, "开始监控", L.BtnOK, fontUI, rgb(9, 24, 17),
		DT_CENTER|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
}

// ---------------- 交互 ----------------

func hitTest(x, y int32) (kind int, row int) {
	L := layoutPicker()
	if x >= L.Close.Left && x < L.Close.Right && y >= L.Close.Top && y < L.Close.Bottom {
		return 1, -1
	}
	if x >= L.BtnCancel.Left && x < L.BtnCancel.Right && y >= L.BtnCancel.Top && y < L.BtnCancel.Bottom {
		return 2, -1
	}
	if x >= L.BtnOK.Left && x < L.BtnOK.Right && y >= L.BtnOK.Top && y < L.BtnOK.Bottom {
		return 3, -1
	}
	if x >= L.BtnSortMem.Left && x < L.BtnSortMem.Right && y >= L.BtnSortMem.Top && y < L.BtnSortMem.Bottom {
		return 5, -1
	}
	if x >= L.BtnSortName.Left && x < L.BtnSortName.Right && y >= L.BtnSortName.Top && y < L.BtnSortName.Bottom {
		return 6, -1
	}
	if x >= L.BtnRefresh.Left && x < L.BtnRefresh.Right && y >= L.BtnRefresh.Top && y < L.BtnRefresh.Bottom {
		return 7, -1
	}
	if x >= L.BtnAuto.Left && x < L.BtnAuto.Right && y >= L.BtnAuto.Top && y < L.BtnAuto.Bottom {
		return 8, -1
	}
	if y >= L.List.Top && y < L.List.Bottom {
		i := int((y - L.List.Top) / L.RowH)
		idx := pk.Scroll + i
		if idx >= 0 && idx < len(pk.Filtered) {
			return 4, idx
		}
		return 4, -1
	}
	return 0, -1
}

func mousePos(lParam uintptr) (int32, int32) {
	x := int32(int16(lParam & 0xFFFF))
	y := int32(int16((lParam >> 16) & 0xFFFF))
	return x, y
}

func pickerProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch uint32(msg) {
	case WM_PAINT:
		paintPicker(hwnd)
		return 0
	case WM_ERASEBKGND:
		return 1

	case WM_CTLCOLOREDIT:
		hdc := wParam
		pSetTextColor.Call(hdc, rgb(232, 236, 243))
		pSetBkColor.Call(hdc, rgb(31, 34, 41))
		return editBrush

	case WM_COMMAND:
		id := int(uint16(wParam & 0xFFFF))
		code := int(uint16((wParam >> 16) & 0xFFFF))
		if id == idSearch && code == EN_CHANGE {
			fillFilter(editText(pk.Search))
			ensureVisible()
			pInvalidateRect.Call(pk.Search, 0, 0)
			pInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0

	case WM_MOUSEMOVE:
		x, y := mousePos(lParam)
		kind, row := hitTest(x, y)
		btn := 0
		if (kind >= 1 && kind <= 3) || (kind >= 5 && kind <= 8) {
			btn = kind
		}
		if btn != pk.HoverBtn || row != pk.HoverRow {
			pk.HoverBtn = btn
			pk.HoverRow = row
			if btn != 0 || row >= 0 {
				pSetCursor.Call(handCursor)
			}
			pInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0

	case WM_SETCURSOR:
		if pk.HoverBtn != 0 || pk.HoverRow >= 0 {
			pSetCursor.Call(handCursor)
			return 1
		}

	case WM_LBUTTONDOWN:
		x, y := mousePos(lParam)
		kind, row := hitTest(x, y)
		if kind == 4 && row >= 0 {
			pk.Sel = row
			ensureVisible()
			pSetFocus.Call(hwnd)
			pInvalidateRect.Call(hwnd, 0, 0)
			return 0
		}
		if (kind >= 1 && kind <= 3) || (kind >= 5 && kind <= 8) {
			pk.Pressed = kind
			pInvalidateRect.Call(hwnd, 0, 0)
			return 0
		}
		// 标题区域拖动
		if y < layoutPicker().SearchBox.Top {
			pReleaseCapture.Call()
			pSendMessageW.Call(hwnd, WM_NCLBUTTONDOWN, HTCAPTION, 0)
		}
		return 0

	case WM_LBUTTONUP:
		if pk.Pressed != 0 {
			x, y := mousePos(lParam)
			kind, _ := hitTest(x, y)
			pressed := pk.Pressed
			pk.Pressed = 0
			pInvalidateRect.Call(hwnd, 0, 0)
			if kind == pressed {
				switch pressed {
				case 1:
					hidePicker()
				case 2:
					hidePicker()
				case 3:
					pickerConfirm()
				case 5:
					toggleSort(0)
				case 6:
					toggleSort(1)
				case 7:
					refreshPicker()
				case 8:
					toggleAutoRefresh()
				}
			}
		}
		return 0

	case WM_LBUTTONDBLCLK:
		x, y := mousePos(lParam)
		kind, row := hitTest(x, y)
		if kind == 4 && row >= 0 {
			pk.Sel = row
			pickerConfirm()
		}
		return 0

	case WM_MOUSEWHEEL:
		delta := int32(int16((wParam >> 16) & 0xFFFF))
		step := int(delta) / 120 * 3
		pk.Scroll -= step
		if pk.Scroll > maxScroll() {
			pk.Scroll = maxScroll()
		}
		if pk.Scroll < 0 {
			pk.Scroll = 0
		}
		pInvalidateRect.Call(hwnd, 0, 0)
		return 0

	case WM_KEYDOWN:
		switch wParam {
		case VK_ESCAPE:
			hidePicker()
			return 0
		case VK_RETURN:
			pickerConfirm()
			return 0
		case VK_UP:
			if pk.Sel > 0 {
				pk.Sel--
				ensureVisible()
				pInvalidateRect.Call(hwnd, 0, 0)
			}
			return 0
		case VK_DOWN:
			if pk.Sel < len(pk.Filtered)-1 {
				pk.Sel++
				ensureVisible()
				pInvalidateRect.Call(hwnd, 0, 0)
			}
			return 0
		}

	case WM_TIMER:
		if wParam == 2 {
			keep := pk.Scroll
			refreshPicker()
			if keep > maxScroll() {
				keep = maxScroll()
			}
			pk.Scroll = keep
			pInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0

	case WM_CLOSE:
		hidePicker()
		return 0
	}

	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}
