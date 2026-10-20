// uidrive: 用键盘驱动卡片右键菜单，做端到端验证。
// 用法: uidrive <downs> [sub] <downs2>
//   downs  : 在主菜单里按几次 ↓
//   sub    : 出现 "sub" 时按 → 进入子菜单
//   downs2 : 子菜单里按几次 ↓
package main

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")

	pSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
	pFindWindowW        = user32.NewProc("FindWindowW")
	pGetWindowRect      = user32.NewProc("GetWindowRect")
	pSetCursorPos       = user32.NewProc("SetCursorPos")
	pPostMessageW       = user32.NewProc("PostMessageW")
	pKeybdEvent         = user32.NewProc("keybd_event")
)

type rect struct{ Left, Top, Right, Bottom int32 }

func key(vk byte) {
	pKeybdEvent.Call(uintptr(vk), 0, 0, 0)
	time.Sleep(30 * time.Millisecond)
	pKeybdEvent.Call(uintptr(vk), 0, 2, 0)
	time.Sleep(60 * time.Millisecond)
}

func main() {
	pSetProcessDPIAware.Call()

	cls, _ := syscall.UTF16PtrFromString("MemMonCard")
	hwnd, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0)
	if hwnd == 0 {
		fmt.Println("window not found")
		return
	}
	var rc rect
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	cx := (rc.Left + rc.Right) / 2
	cy := (rc.Top + rc.Bottom) / 2

	pSetCursorPos.Call(uintptr(cx), uintptr(cy))
	time.Sleep(100 * time.Millisecond)
	pPostMessageW.Call(hwnd, 0x0205, 0, 0) // WM_RBUTTONUP
	time.Sleep(600 * time.Millisecond)

	// 主菜单：按 n 次 ↓
	downs := 11
	if len(os.Args) > 1 {
		fmt.Sscanf(os.Args[1], "%d", &downs)
	}
	for i := 0; i < downs; i++ {
		key(0x28)
	}
	sub := 1
	if len(os.Args) > 2 {
		fmt.Sscanf(os.Args[2], "%d", &sub)
	}
	key(0x27) // → 进入子菜单
	for i := 0; i < sub; i++ {
		key(0x28)
	}
	key(0x0D) // Enter
	time.Sleep(400 * time.Millisecond)
	fmt.Println("done")
}
