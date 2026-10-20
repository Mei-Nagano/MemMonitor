// winstat: 打印 MemMonCard / MemMonPicker 两个窗口的存在与可见状态。
package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	user32              = syscall.NewLazyDLL("user32.dll")
	pFindWindowW        = user32.NewProc("FindWindowW")
	pGetWindowRect      = user32.NewProc("GetWindowRect")
	pIsWindowVisible    = user32.NewProc("IsWindowVisible")
	pSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
)

type rect struct{ Left, Top, Right, Bottom int32 }

func stat(class string) {
	pSetProcessDPIAware.Call()
	cls, _ := syscall.UTF16PtrFromString(class)
	h, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0)
	if h == 0 {
		fmt.Printf("%-12s 不存在\n", class)
		return
	}
	var r rect
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r)))
	v, _, _ := pIsWindowVisible.Call(h)
	fmt.Printf("%-12s hwnd=%d visible=%d rect=(%d,%d)-(%d,%d)\n",
		class, h, v, r.Left, r.Top, r.Right, r.Bottom)
}

func main() {
	stat("MemMonCard")
	stat("MemMonPicker")
}
