// dblclick: 给卡片窗口发一次双击，用于验证选择窗弹出。
package main

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32              = syscall.NewLazyDLL("user32.dll")
	pFindWindowW        = user32.NewProc("FindWindowW")
	pGetWindowRect      = user32.NewProc("GetWindowRect")
	pPostMessageW       = user32.NewProc("PostMessageW")
	pSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
)

type rect struct{ Left, Top, Right, Bottom int32 }

func main() {
	pSetProcessDPIAware.Call()
	cls, _ := syscall.UTF16PtrFromString("MemMonCard")
	hwnd, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0)
	if hwnd == 0 {
		fmt.Println("no window")
		return
	}
	var r rect
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	pPostMessageW.Call(hwnd, 0x0203, 1, 0) // WM_LBUTTONDBLCLK
	time.Sleep(600 * time.Millisecond)

	pcls, _ := syscall.UTF16PtrFromString("MemMonPicker")
	ph, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(pcls)), 0)
	var pr rect
	if ph != 0 {
		pGetWindowRect.Call(ph, uintptr(unsafe.Pointer(&pr)))
	}
	fmt.Printf("picker hwnd=%d rect=(%d,%d)-(%d,%d)\n", ph, pr.Left, pr.Top, pr.Right, pr.Bottom)
}
