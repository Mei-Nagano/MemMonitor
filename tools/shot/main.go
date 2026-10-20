package main

// 开发用截图工具：抓取 MemMonitor 的悬浮卡片与进程选择窗口，输出 PNG。
// 用法: shot.exe [输出目录]
import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	gdi32  = syscall.NewLazyDLL("gdi32.dll")

	pFindWindowW            = user32.NewProc("FindWindowW")
	pGetWindowRect          = user32.NewProc("GetWindowRect")
	pGetClientRect          = user32.NewProc("GetClientRect")
	pGetDC                  = user32.NewProc("GetDC")
	pReleaseDC              = user32.NewProc("ReleaseDC")
	pSendMessageW           = user32.NewProc("SendMessageW")
	pShowWindow             = user32.NewProc("ShowWindow")
	pIsWindowVisible        = user32.NewProc("IsWindowVisible")
	pGetWindowLongW         = user32.NewProc("GetWindowLongW")
	pGetSystemMetrics       = user32.NewProc("GetSystemMetrics")
	pSetProcessDPIAware     = user32.NewProc("SetProcessDPIAware")
	pPrintWindow            = user32.NewProc("PrintWindow")
	pGetDeviceCaps          = gdi32.NewProc("GetDeviceCaps")
	pCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	pSelectObject           = gdi32.NewProc("SelectObject")
	pBitBlt                 = gdi32.NewProc("BitBlt")
	pGetDIBits              = gdi32.NewProc("GetDIBits")
	pDeleteObject           = gdi32.NewProc("DeleteObject")
	pDeleteDC               = gdi32.NewProc("DeleteDC")
)

type rect struct{ Left, Top, Right, Bottom int32 }

var (
	gwlStyle   uintptr = ^uintptr(15) // GWL_STYLE = -16
	gwlExStyle uintptr = ^uintptr(19) // GWL_EXSTYLE = -20
)

type bmiHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bmi struct {
	Header bmiHeader
	Colors [1]uint32
}

func findWindow(class string) uintptr {
	c, _ := syscall.UTF16PtrFromString(class)
	h, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(c)), 0)
	return h
}

func capture(hwnd uintptr, out string, usePrint bool) error {
	var rc rect
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	w := rc.Right - rc.Left
	h := rc.Bottom - rc.Top
	if w <= 0 || h <= 0 {
		return fmt.Errorf("bad rect %dx%d", w, h)
	}

	screenDC, _, _ := pGetDC.Call(0)
	mdc, _, _ := pCreateCompatibleDC.Call(screenDC)
	bmp, _, _ := pCreateCompatibleBitmap.Call(screenDC, uintptr(w), uintptr(h))
	old, _, _ := pSelectObject.Call(mdc, bmp)

	if usePrint {
		// PW_RENDERFULLCONTENT = 2，可抓到分层 / DWM 合成窗口
		ok, _, _ := pPrintWindow.Call(hwnd, mdc, 2)
		if ok == 0 {
			pBitBlt.Call(mdc, 0, 0, uintptr(w), uintptr(h), screenDC,
				uintptr(rc.Left), uintptr(rc.Top), 0x00CC0020|0x40000000)
		}
	} else {
		pBitBlt.Call(mdc, 0, 0, uintptr(w), uintptr(h), screenDC,
			uintptr(rc.Left), uintptr(rc.Top), 0x00CC0020|0x40000000)
	}
	pSelectObject.Call(mdc, old)

	var bi bmi
	bi.Header.Size = uint32(unsafe.Sizeof(bi.Header))
	bi.Header.Width = w
	bi.Header.Height = -h
	bi.Header.Planes = 1
	bi.Header.BitCount = 32
	bi.Header.Compression = 0

	buf := make([]byte, int(w)*int(h)*4)
	n, _, _ := pGetDIBits.Call(mdc, bmp, 0, uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), 0)

	pDeleteObject.Call(bmp)
	pDeleteDC.Call(mdc)
	pReleaseDC.Call(0, screenDC)
	if n == 0 {
		return fmt.Errorf("GetDIBits failed")
	}

	img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	for y := 0; y < int(h); y++ {
		for x := 0; x < int(w); x++ {
			i := (y*int(w) + x) * 4
			img.Set(x, y, color.RGBA{R: buf[i+2], G: buf[i+1], B: buf[i], A: 255})
		}
	}

	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func main() {
	dir := "."
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	pSetProcessDPIAware.Call()

	sw, _, _ := pGetSystemMetrics.Call(0)
	sh, _, _ := pGetSystemMetrics.Call(1)
	vx, _, _ := pGetSystemMetrics.Call(76)
	vy, _, _ := pGetSystemMetrics.Call(77)
	vw, _, _ := pGetSystemMetrics.Call(78)
	vh, _, _ := pGetSystemMetrics.Call(79)
	hdc, _, _ := pGetDC.Call(0)
	dpi, _, _ := pGetDeviceCaps.Call(hdc, 88)
	pReleaseDC.Call(0, hdc)
	fmt.Printf("screen=%dx%d virtual=(%d,%d) %dx%d dpi=%d\n", sw, sh, vx, vy, vw, vh, dpi)

	main := findWindow("MemMonCard")
	if main == 0 {
		fmt.Println("main window not found")
		os.Exit(1)
	}
	var mr rect
	pGetWindowRect.Call(main, uintptr(unsafe.Pointer(&mr)))
	vis, _, _ := pIsWindowVisible.Call(main)
	style, _, _ := pGetWindowLongW.Call(main, gwlStyle)
	exstyle, _, _ := pGetWindowLongW.Call(main, gwlExStyle)
	fmt.Printf("main hwnd=0x%x rect=(%d,%d)-(%d,%d) visible=%d style=0x%x ex=0x%x\n",
		main, mr.Left, mr.Top, mr.Right, mr.Bottom, vis, style, exstyle)

	if err := capture(main, filepath.Join(dir, "_card.png"), true); err != nil {
		fmt.Println("card:", err)
	} else {
		fmt.Println("card ok")
	}
	if len(os.Args) > 2 && os.Args[2] == "card" {
		return
	}

	// 触发双击 -> 打开进程选择窗口
	pSendMessageW.Call(main, 0x0203, 0, 0)
	time.Sleep(900 * time.Millisecond)

	picker := findWindow("MemMonPicker")
	if picker == 0 {
		fmt.Println("picker window not found")
		return
	}
	var pr, pcr rect
	pGetWindowRect.Call(picker, uintptr(unsafe.Pointer(&pr)))
	pGetClientRect.Call(picker, uintptr(unsafe.Pointer(&pcr)))
	fmt.Printf("picker rect=(%d,%d)-(%d,%d) client=%dx%d\n",
		pr.Left, pr.Top, pr.Right, pr.Bottom, pcr.Right, pcr.Bottom)
	if err := capture(picker, filepath.Join(dir, "_picker.png"), false); err != nil {
		fmt.Println("picker:", err)
	} else {
		fmt.Println("picker ok")
	}
}
