// grab: 抓取屏幕指定区域（含已合成的分层窗口），存 PNG。
// 用法: grab <x> <y> <w> <h> <out.png>
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strconv"
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	pGetDC   = user32.NewProc("GetDC")
	pRelDC   = user32.NewProc("ReleaseDC")
	pBitBlt = gdi32.NewProc("BitBlt")
	pCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	pCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	pSelectObject       = gdi32.NewProc("SelectObject")
	pDeleteObject       = gdi32.NewProc("DeleteObject")
	pDeleteDC           = gdi32.NewProc("DeleteDC")
)

const (
	srccopy = 0x00CC0020 | 0x40000000 // CAPTUREBLT：包含分层窗口
)

type bitmapInfoHeader struct {
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

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [4]uint32
}

func main() {
	if len(os.Args) < 6 {
		fmt.Println("usage: grab <x> <y> <w> <h> <out.png>")
		return
	}
	x, _ := strconv.Atoi(os.Args[1])
	y, _ := strconv.Atoi(os.Args[2])
	w, _ := strconv.Atoi(os.Args[3])
	h, _ := strconv.Atoi(os.Args[4])
	out := os.Args[5]

	screen, _, _ := pGetDC.Call(0)
	if screen == 0 {
		fmt.Println("GetDC failed")
		return
	}
	defer pRelDC.Call(0, screen)

	mdc, _, _ := pCreateCompatibleDC.Call(screen)
	var bi bitmapInfo
	bi.Header.Size = uint32(unsafe.Sizeof(bi.Header))
	bi.Header.Width = int32(w)
	bi.Header.Height = -int32(h)
	bi.Header.Planes = 1
	bi.Header.BitCount = 32
	var bits unsafe.Pointer
	hbmp, _, _ := pCreateDIBSection.Call(mdc, uintptr(unsafe.Pointer(&bi)), 0,
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbmp == 0 || bits == nil {
		fmt.Println("CreateDIBSection failed")
		return
	}
	pSelectObject.Call(mdc, hbmp)
	ok, _, _ := pBitBlt.Call(mdc, 0, 0, uintptr(w), uintptr(h), screen, uintptr(x), uintptr(y), srccopy)
	if ok == 0 {
		fmt.Println("BitBlt failed")
	}

	px := unsafe.Slice((*byte)(bits), w*h*4)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		img.Set(i%w, i/w, color.RGBA{R: px[i*4+2], G: px[i*4+1], B: px[i*4], A: 255})
	}

	f, err := os.Create(out)
	if err != nil {
		fmt.Println("create:", err)
		return
	}
	png.Encode(f, img)
	f.Close()

	pSelectObject.Call(mdc, 0)
	pDeleteObject.Call(hbmp)
	pDeleteDC.Call(mdc)
	fmt.Printf("grab %d,%d %dx%d -> %s\n", x, y, w, h, out)
}
