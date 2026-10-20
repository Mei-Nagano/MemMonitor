// menuprobe: 找到卡片窗口，在窗口中心模拟右键，截取弹出的菜单，然后按 ESC 关闭。
// 用法: menuprobe <out.png>
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	gdi32  = syscall.NewLazyDLL("gdi32.dll")

	pFindWindowW        = user32.NewProc("FindWindowW")
	pGetWindowRect      = user32.NewProc("GetWindowRect")
	pSetCursorPos       = user32.NewProc("SetCursorPos")
	pPostMessageW       = user32.NewProc("PostMessageW")
	pKeybdEvent         = user32.NewProc("keybd_event")
	pGetDC              = user32.NewProc("GetDC")
	pRelDC              = user32.NewProc("ReleaseDC")
	pBitBlt             = gdi32.NewProc("BitBlt")
	pCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	pCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	pSelectObject       = gdi32.NewProc("SelectObject")
	pDeleteObject       = gdi32.NewProc("DeleteObject")
	pDeleteDC           = gdi32.NewProc("DeleteDC")
)

type rect struct{ Left, Top, Right, Bottom int32 }

type bih struct {
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

type bi struct {
	Header bih
	Colors [1]uint32
}

func grab(x, y, w, h int, out string) {
	screen, _, _ := pGetDC.Call(0)
	defer pRelDC.Call(0, screen)
	mdc, _, _ := pCreateCompatibleDC.Call(screen)
	var b bi
	b.Header.Size = uint32(unsafe.Sizeof(b.Header))
	b.Header.Width = int32(w)
	b.Header.Height = -int32(h)
	b.Header.Planes = 1
	b.Header.BitCount = 32
	var bits unsafe.Pointer
	hb, _, _ := pCreateDIBSection.Call(mdc, uintptr(unsafe.Pointer(&b)), 0,
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	pSelectObject.Call(mdc, hb)
	pBitBlt.Call(mdc, 0, 0, uintptr(w), uintptr(h), screen,
		uintptr(x), uintptr(y), 0x00CC0020|0x40000000)

	px := unsafe.Slice((*byte)(bits), w*h*4)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		img.Set(i%w, i/w, color.RGBA{px[i*4+2], px[i*4+1], px[i*4], 255})
	}
	f, _ := os.Create(out)
	png.Encode(f, img)
	f.Close()
	pSelectObject.Call(mdc, 0)
	pDeleteObject.Call(hb)
	pDeleteDC.Call(mdc)
}

func main() {
	out := "menu.png"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	x, _ := strconv.Atoi("0")

	cls, _ := syscall.UTF16PtrFromString("MemMonCard")
	hwnd, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0)
	if hwnd == 0 {
		fmt.Println("main window not found")
		return
	}
	var rc rect
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	cx := int((rc.Left + rc.Right) / 2)
	cy := int((rc.Top + rc.Bottom) / 2)

	pSetCursorPos.Call(uintptr(cx), uintptr(cy))
	time.Sleep(80 * time.Millisecond)
	pPostMessageW.Call(hwnd, 0x0205, 0, 0) // WM_RBUTTONUP
	time.Sleep(700 * time.Millisecond)

	// 菜单从光标位置向右下展开，抓一块足够大的区域
	grab(cx-10, cy-10, 520, 560, out)
	fmt.Printf("menu -> %s (at %d,%d)\n", out, cx, cy)

	// ESC 关闭菜单
	pKeybdEvent.Call(0x1B, 0, 0, 0)
	time.Sleep(40 * time.Millisecond)
	pKeybdEvent.Call(0x1B, 0, 2, 0)
	_ = x
}
