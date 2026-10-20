package main

// 渲染基础：32 位 DIB 离屏表面、抗锯齿圆角、分层窗口提交。
// 用逐像素 alpha 做圆角遮罩，避免 SetWindowRgn 的 1 位裁剪锯齿。

import (
	"math"
	"unsafe"
)

// ---------------- GDI+ ----------------

var gdipReady bool

func initGdiplus() {
	var token uintptr
	var input gdiplusStartupInput
	input.GdiplusVersion = 1
	r, _, _ := pGdiplusStartup.Call(uintptr(unsafe.Pointer(&token)),
		uintptr(unsafe.Pointer(&input)), 0)
	gdipReady = r == 0 && token != 0
}

// fillRoundRectAA 用 GDI+ 画抗锯齿圆角矩形（中间矩形 + 四角圆）。
func fillRoundRectAA(dc uintptr, r RECT, radius int32, color uintptr) {
	if radius < 1 {
		radius = 1
	}
	w := r.Right - r.Left
	h := r.Bottom - r.Top
	if w <= 0 || h <= 0 {
		return
	}
	if radius*2 > w {
		radius = w / 2
	}
	if radius*2 > h {
		radius = h / 2
	}

	if gdipReady {
		var g uintptr
		hr, _, _ := pGdipCreateFromHDC.Call(dc, uintptr(unsafe.Pointer(&g)))
		if hr == 0 && g != 0 {
			pGdipSetSmoothingMode.Call(g, SMOOTHING_AA)
			// COLORREF(0x00BBGGRR) -> ARGB(0xAARRGGBB)
			argb := uint32(0xFF000000) |
				uint32(color&0xFF)<<16 |
				uint32((color>>8)&0xFF)<<8 |
				uint32((color>>16)&0xFF)
			var br uintptr
			hr2, _, _ := pGdipCreateSolidFill.Call(uintptr(argb), uintptr(unsafe.Pointer(&br)))
			if hr2 == 0 && br != 0 {
				d := radius * 2
				// 中间十字
				pGdipFillRectangleI.Call(g, br,
					uintptr(r.Left+radius), uintptr(r.Top), uintptr(w-radius*2), uintptr(h))
				pGdipFillRectangleI.Call(g, br,
					uintptr(r.Left), uintptr(r.Top+radius), uintptr(w), uintptr(h-radius*2))
				// 四角
				pGdipFillEllipseI.Call(g, br, uintptr(r.Left), uintptr(r.Top), uintptr(d), uintptr(d))
				pGdipFillEllipseI.Call(g, br, uintptr(r.Right-d), uintptr(r.Top), uintptr(d), uintptr(d))
				pGdipFillEllipseI.Call(g, br, uintptr(r.Left), uintptr(r.Bottom-d), uintptr(d), uintptr(d))
				pGdipFillEllipseI.Call(g, br, uintptr(r.Right-d), uintptr(r.Bottom-d), uintptr(d), uintptr(d))
				pGdipDeleteBrush.Call(br)
			}
			pGdipDeleteGraphics.Call(g)
			return
		}
	}

	// GDI+ 不可用时退回 GDI
	br, _, _ := pCreateSolidBrush.Call(color)
	ob, _, _ := pSelectObject.Call(dc, br)
	pn, _, _ := pGetStockObject.Call(NULL_PEN)
	op, _, _ := pSelectObject.Call(dc, pn)
	pRoundRect.Call(dc, i32(r.Left), i32(r.Top), i32(r.Right), i32(r.Bottom),
		uintptr(radius*2), uintptr(radius*2))
	pSelectObject.Call(dc, ob)
	pSelectObject.Call(dc, op)
	pDeleteObject.Call(br)
}

// ---------------- 离屏表面 ----------------

type surface struct {
	dc     uintptr // 内存 DC
	bitmap uintptr // DIB section
	bits   unsafe.Pointer
	w, h   int32
	screen uintptr
}

func (s *surface) release() {
	if s.dc != 0 {
		if s.bitmap != 0 {
			null, _, _ := pGetStockObject.Call(0)
			pSelectObject.Call(s.dc, null)
		}
		pDeleteDC.Call(s.dc)
	}
	if s.bitmap != 0 {
		pDeleteObject.Call(s.bitmap)
	}
	s.dc, s.bitmap, s.bits, s.w, s.h = 0, 0, nil, 0, 0
}

// ensure 准备指定尺寸的 32bpp 顶朝下 DIB。
func (s *surface) ensure(w, h int32) bool {
	if w <= 0 || h <= 0 {
		return false
	}
	if s.dc != 0 && s.w == w && s.h == h {
		return true
	}
	s.release()

	s.screen, _, _ = pGetDC.Call(0)
	if s.screen == 0 {
		return false
	}
	mdc, _, _ := pCreateCompatibleDC.Call(s.screen)
	if mdc == 0 {
		return false
	}

	var bi BITMAPINFO
	bi.Header.Size = uint32(unsafe.Sizeof(bi.Header))
	bi.Header.Width = w
	bi.Header.Height = -h // 顶朝下
	bi.Header.Planes = 1
	bi.Header.BitCount = 32
	bi.Header.Compression = BI_RGB

	var bits unsafe.Pointer
	hbmp, _, _ := pCreateDIBSection.Call(mdc, uintptr(unsafe.Pointer(&bi)), 0,
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbmp == 0 || bits == nil {
		pDeleteDC.Call(mdc)
		return false
	}
	pSelectObject.Call(mdc, hbmp)

	s.dc = mdc
	s.bitmap = hbmp
	s.bits = bits
	s.w = w
	s.h = h
	return true
}

// pixels 返回可写的 BGRA 像素切片。
func (s *surface) pixels() []byte {
	n := int(s.w) * int(s.h) * 4
	return unsafe.Slice((*byte)(s.bits), n)
}

// ---------------- 圆角 alpha 遮罩 ----------------

// cornerCoverage 返回像素 (x,y) 在圆角矩形内的覆盖率（0~1），用于抗锯齿。
func cornerCoverage(x, y, w, h, radius int32) float64 {
	r := float64(radius)
	if r < 1 {
		return 1
	}
	var cx, cy float64
	corner := false
	fx, fy := float64(x), float64(y)
	switch {
	case fx < r && fy < r:
		cx, cy, corner = r, r, true
	case fx >= float64(w)-r && fy < r:
		cx, cy, corner = float64(w)-r, r, true
	case fx < r && fy >= float64(h)-r:
		cx, cy, corner = r, float64(h)-r, true
	case fx >= float64(w)-r && fy >= float64(h)-r:
		cx, cy, corner = float64(w)-r, float64(h)-r, true
	}
	if !corner {
		return 1
	}
	dx := fx + 0.5 - cx
	dy := fy + 0.5 - cy
	d := math.Sqrt(dx*dx + dy*dy)
	cov := r - d + 0.5
	if cov < 0 {
		return 0
	}
	if cov > 1 {
		return 1
	}
	return cov
}

// ---------------- 分层窗口提交 ----------------

// commitLayered 把离屏位图提交为窗口内容（per-pixel alpha）。
func commitLayered(hwnd uintptr, s *surface) {
	if s.dc == 0 {
		return
	}
	screen, _, _ := pGetDC.Call(0)
	if screen == 0 {
		return
	}
	defer pReleaseDC.Call(0, screen)

	var wr RECT
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))

	ptDst := POINT{X: wr.Left, Y: wr.Top}
	size := SIZE{Cx: s.w, Cy: s.h}
	ptSrc := POINT{X: 0, Y: 0}
	blend := BLENDFUNCTION{
		BlendOp:             0, // AC_SRC_OVER
		BlendFlags:          0,
		SourceConstantAlpha: 255,
		AlphaFormat:         AC_SRC_ALPHA,
	}

	pUpdateLayeredWindow.Call(hwnd, screen,
		uintptr(unsafe.Pointer(&ptDst)), uintptr(unsafe.Pointer(&size)),
		s.dc, uintptr(unsafe.Pointer(&ptSrc)),
		0, uintptr(unsafe.Pointer(&blend)), ULW_ALPHA)
}
