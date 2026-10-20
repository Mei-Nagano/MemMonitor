// setsize: 把卡片窗口改成指定物理尺寸，然后读回实际尺寸，用于验证宽度下限。
// 用法: setsize <w> [h]
package main

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")

	pSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
	pFindWindowW        = user32.NewProc("FindWindowW")
	pGetWindowRect      = user32.NewProc("GetWindowRect")
	pSetWindowPos       = user32.NewProc("SetWindowPos")
)

type rect struct{ Left, Top, Right, Bottom int32 }

func main() {
	pSetProcessDPIAware.Call()
	w, _ := strconv.Atoi(os.Args[1])
	h := 0
	if len(os.Args) > 2 {
		h, _ = strconv.Atoi(os.Args[2])
	}

	cls, _ := syscall.UTF16PtrFromString("MemMonCard")
	hwnd, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0)
	if hwnd == 0 {
		fmt.Println("window not found")
		return
	}
	var r rect
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	if h == 0 {
		h = int(r.Bottom - r.Top)
	}
	pSetWindowPos.Call(hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(w), uintptr(h),
		0x0002|0x0004|0x0010) // NOMOVE|NOZORDER|NOACTIVATE
	time.Sleep(400 * time.Millisecond)

	var r2 rect
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r2)))
	fmt.Printf("request %dx%d -> actual %dx%d\n", w, h, r2.Right-r2.Left, r2.Bottom-r2.Top)
}
