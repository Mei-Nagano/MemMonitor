package main

// 极简 Win32 绑定：全部通过 syscall 动态调用，不依赖 cgo，也不依赖任何第三方模块。

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	psapi    = syscall.NewLazyDLL("psapi.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	gdiplus  = syscall.NewLazyDLL("gdiplus.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")
)

var (
	pRegisterClassExW           = user32.NewProc("RegisterClassExW")
	pCreateWindowExW            = user32.NewProc("CreateWindowExW")
	pDefWindowProcW             = user32.NewProc("DefWindowProcW")
	pDestroyWindow              = user32.NewProc("DestroyWindow")
	pShowWindow                 = user32.NewProc("ShowWindow")
	pGetMessageW                = user32.NewProc("GetMessageW")
	pTranslateMessage           = user32.NewProc("TranslateMessage")
	pDispatchMessageW           = user32.NewProc("DispatchMessageW")
	pPostQuitMessage            = user32.NewProc("PostQuitMessage")
	pSetTimer                   = user32.NewProc("SetTimer")
	pKillTimer                  = user32.NewProc("KillTimer")
	pInvalidateRect             = user32.NewProc("InvalidateRect")
	pBeginPaint                 = user32.NewProc("BeginPaint")
	pEndPaint                   = user32.NewProc("EndPaint")
	pGetClientRect              = user32.NewProc("GetClientRect")
	pGetWindowRect              = user32.NewProc("GetWindowRect")
	pLoadCursorW                = user32.NewProc("LoadCursorW")
	pTrackPopupMenu             = user32.NewProc("TrackPopupMenu")
	pCreatePopupMenu            = user32.NewProc("CreatePopupMenu")
	pAppendMenuW                = user32.NewProc("AppendMenuW")
	pDestroyMenu                = user32.NewProc("DestroyMenu")
	pSetForegroundWindow        = user32.NewProc("SetForegroundWindow")
	pGetCursorPos               = user32.NewProc("GetCursorPos")
	pReleaseCapture             = user32.NewProc("ReleaseCapture")
	pSendMessageW               = user32.NewProc("SendMessageW")
	pSetWindowPos               = user32.NewProc("SetWindowPos")
	pSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	pSetWindowRgn               = user32.NewProc("SetWindowRgn")
	pDrawIconEx                 = user32.NewProc("DrawIconEx")
	pDestroyIcon                = user32.NewProc("DestroyIcon")
	pGetDC                      = user32.NewProc("GetDC")
	pReleaseDC                  = user32.NewProc("ReleaseDC")
	pDrawTextW                  = user32.NewProc("DrawTextW")
	pGetSystemMetrics           = user32.NewProc("GetSystemMetrics")
	pSetProcessDPIAware         = user32.NewProc("SetProcessDPIAware")
	pGetWindowTextW             = user32.NewProc("GetWindowTextW")
	pSetWindowTextW             = user32.NewProc("SetWindowTextW")
	pIsWindowVisible            = user32.NewProc("IsWindowVisible")
	pSetFocus                   = user32.NewProc("SetFocus")
	pGetFocus                   = user32.NewProc("GetFocus")
	pGetKeyState                = user32.NewProc("GetKeyState")
	pDwmSetWindowAttribute      = dwmapi.NewProc("DwmSetWindowAttribute")
	pSetWindowLongPtrW          = user32.NewProc("SetWindowLongPtrW")
	pSetWindowLongW             = user32.NewProc("SetWindowLongW")
	pCallWindowProcW            = user32.NewProc("CallWindowProcW")
	pSetCursor                  = user32.NewProc("SetCursor")
	pSetCapture                 = user32.NewProc("SetCapture")
	pReleaseCapture2            = user32.NewProc("ReleaseCapture")
	pSetBkColor                 = gdi32.NewProc("SetBkColor")
	pEllipse                    = gdi32.NewProc("Ellipse")
	pMoveToEx2                  = gdi32.NewProc("MoveToEx")
	pLineTo2                    = gdi32.NewProc("LineTo")
	pSHGetImageList             = shell32.NewProc("SHGetImageList")
	pCoInitializeEx             = ole32.NewProc("CoInitializeEx")
	pGetStockObject             = gdi32.NewProc("GetStockObject")
	pIntersectClipRect          = gdi32.NewProc("IntersectClipRect")
	pSelectClipRgn              = gdi32.NewProc("SelectClipRgn")
	pFillRect                   = user32.NewProc("FillRect")
	pCreateDIBSection           = gdi32.NewProc("CreateDIBSection")
	pUpdateLayeredWindow        = user32.NewProc("UpdateLayeredWindow")
	pGdiplusStartup             = gdiplus.NewProc("GdiplusStartup")
	pGdipCreateFromHDC          = gdiplus.NewProc("GdipCreateFromHDC")
	pGdipDeleteGraphics         = gdiplus.NewProc("GdipDeleteGraphics")
	pGdipSetSmoothingMode       = gdiplus.NewProc("GdipSetSmoothingMode")
	pGdipCreateSolidFill        = gdiplus.NewProc("GdipCreateSolidFill")
	pGdipDeleteBrush            = gdiplus.NewProc("GdipDeleteBrush")
	pGdipFillRectangleI         = gdiplus.NewProc("GdipFillRectangleI")
	pGdipFillEllipseI           = gdiplus.NewProc("GdipFillEllipseI")
	pGetForegroundWindow        = user32.NewProc("GetForegroundWindow")
	pGetSaveFileNameW           = comdlg32.NewProc("GetSaveFileNameW")
	pShellExecuteW              = shell32.NewProc("ShellExecuteW")
)

var (
	pCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	pSelectObject           = gdi32.NewProc("SelectObject")
	pDeleteObject           = gdi32.NewProc("DeleteObject")
	pDeleteDC               = gdi32.NewProc("DeleteDC")
	pCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	pCreatePen              = gdi32.NewProc("CreatePen")
	pRoundRect              = gdi32.NewProc("RoundRect")
	pSetBkMode              = gdi32.NewProc("SetBkMode")
	pSetTextColor           = gdi32.NewProc("SetTextColor")
	pCreateFontW            = gdi32.NewProc("CreateFontW")
	pBitBlt                 = gdi32.NewProc("BitBlt")
	pGetDeviceCaps          = gdi32.NewProc("GetDeviceCaps")
	pCreateRoundRectRgn     = gdi32.NewProc("CreateRoundRectRgn")
	pPolyline               = gdi32.NewProc("Polyline")
	pGetTextExtentPoint32W  = gdi32.NewProc("GetTextExtentPoint32W")
)

var (
	pGetModuleHandleW          = kernel32.NewProc("GetModuleHandleW")
	pCreateToolhelp32Snapshot  = kernel32.NewProc("CreateToolhelp32Snapshot")
	pProcess32FirstW           = kernel32.NewProc("Process32FirstW")
	pProcess32NextW            = kernel32.NewProc("Process32NextW")
	pOpenProcess               = kernel32.NewProc("OpenProcess")
	pCloseHandle               = kernel32.NewProc("CloseHandle")
	pQueryFullProcessImageName = kernel32.NewProc("QueryFullProcessImageNameW")
	pGlobalMemoryStatusEx      = kernel32.NewProc("GlobalMemoryStatusEx")
)

var (
	pSHGetFileInfoW       = shell32.NewProc("SHGetFileInfoW")
	pGetProcessMemoryInfo = psapi.NewProc("GetProcessMemoryInfo")
)

// ---------------- 常量 ----------------

const (
	WS_POPUP        = 0x80000000
	WS_CHILD        = 0x40000000
	WS_VISIBLE      = 0x10000000
	WS_BORDER       = 0x00800000
	WS_VSCROLL      = 0x00200000
	WS_TABSTOP      = 0x00010000
	WS_CAPTION      = 0x00C00000
	WS_SYSMENU      = 0x00080000
	WS_OVERLAPPED   = 0x00000000
	WS_CLIPCHILDREN = 0x02000000

	WS_EX_TOPMOST    = 0x00000008
	WS_EX_LAYERED    = 0x00080000
	WS_EX_TOOLWINDOW = 0x00000080

	CS_DBLCLKS = 0x0008

	LWA_ALPHA = 0x00000002

	SW_HIDE = 0
	SW_SHOW = 5

	SWP_NOSIZE     = 0x0001
	SWP_NOMOVE     = 0x0002
	SWP_NOZORDER   = 0x0004
	SWP_NOACTIVATE = 0x0010
	SWP_SHOWWINDOW = 0x0040

	WM_DESTROY       = 0x0002
	WM_CLOSE         = 0x0010
	WM_PAINT         = 0x000F
	WM_ERASEBKGND    = 0x0014
	WM_GETMINMAXINFO = 0x0024
	WM_COMMAND       = 0x0111
	WM_TIMER         = 0x0113
	WM_SETFONT       = 0x0030
	WM_LBUTTONDOWN   = 0x0201
	WM_LBUTTONDBLCLK = 0x0203
	WM_LBUTTONUP     = 0x0202
	WM_MOUSEMOVE     = 0x0200
	WM_MOUSEWHEEL    = 0x020A
	WM_KEYDOWN       = 0x0100
	WM_CTLCOLOREDIT  = 0x0133
	WM_SETCURSOR     = 0x0020
	WM_RBUTTONUP     = 0x0205
	WM_NCLBUTTONDOWN = 0x00A1
	WM_SIZE          = 0x0005
	WM_NCHITTEST     = 0x0084
	WM_ENTERSIZEMOVE = 0x0231
	WM_EXITSIZEMOVE  = 0x0232
	WM_GETTEXT       = 0x000D
	WM_GETTEXTLENGTH = 0x000E

	VK_UP     = 0x26
	VK_DOWN   = 0x28
	VK_RETURN = 0x0D
	VK_ESCAPE = 0x1B

	EM_SETCUEBANNER = 0x1501

	GWLP_WNDPROC = ^uintptr(0) - 3 // -4

	SHGFI_SYSICONINDEX = 0x00004000
	ILD_TRANSPARENT    = 0x00000001
	SHIL_EXTRALARGE    = 0x2
	SHIL_JUMBO         = 0x4

	HTCAPTION                      = 2
	HTCLIENT                       = 1
	HTLEFT                         = 10
	HTRIGHT                        = 11
	DWMWA_WINDOW_CORNER_PREFERENCE = 33
	DWMWCP_ROUND                   = 2

	LB_ADDSTRING    = 0x0180
	LB_RESETCONTENT = 0x0184
	LB_SETCURSEL    = 0x0186
	LB_GETCURSEL    = 0x0188
	LB_GETCOUNT     = 0x018B

	EN_CHANGE  = 0x0300
	LBN_DBLCLK = 2

	ES_AUTOHSCROLL       = 0x0080
	LBS_NOTIFY           = 0x0001
	LBS_NOINTEGRALHEIGHT = 0x0100
	BS_DEFPUSHBUTTON     = 0x0001

	MF_STRING    = 0x00000000
	MF_SEPARATOR = 0x00000800
	MF_CHECKED   = 0x00000008
	MF_POPUP     = 0x00000010
	MF_GRAYED    = 0x00000001

	TPM_LEFTALIGN   = 0x0000
	TPM_TOPALIGN    = 0x0000
	TPM_RIGHTBUTTON = 0x0002
	TPM_RETURNCMD   = 0x0100

	DT_LEFT         = 0x00000000
	DT_CENTER       = 0x00000001
	DT_RIGHT        = 0x00000002
	DT_VCENTER      = 0x00000004
	DT_SINGLELINE   = 0x00000020
	DT_NOPREFIX     = 0x00000800
	DT_END_ELLIPSIS = 0x00008000

	TRANSPARENT         = 1
	PS_SOLID            = 0
	SRCCOPY             = 0x00CC0020
	DI_NORMAL           = 0x0003
	DEFAULT_CHARSET     = 1
	OUT_TT_PRECIS       = 4
	CLIP_DEFAULT_PRECIS = 0
	CLEARTYPE_QUALITY   = 5
	ANTIALIASED_QUALITY = 4
	DEFAULT_PITCH       = 0
	FF_DONTCARE         = 0
	FW_NORMAL           = 400
	FW_SEMIBOLD         = 600
	FW_BOLD             = 700
	LOGPIXELSX          = 88

	ULW_ALPHA    = 0x00000002
	AC_SRC_ALPHA = 0x01
	BI_RGB       = 0
	SMOOTHING_AA = 4

	IDC_ARROW = 32512
)

// ---------------- 结构体 ----------------

type RECT struct {
	Left, Top, Right, Bottom int32
}

type POINT struct {
	X, Y int32
}

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
	Private uint32
}

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type PAINTSTRUCT struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

type PROCESSENTRY32W struct {
	Size            uint32
	CntUsage        uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	CntThreads      uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [260]uint16
}

type PROCESS_MEMORY_COUNTERS struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

type PROCESS_MEMORY_COUNTERS_EX struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
}

type PROCESS_MEMORY_COUNTERS_EX2 struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
	PrivateWorkingSetSize      uintptr
	SharedCommitUsage          uint64
}

type MEMORYSTATUSEX struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

type SHFILEINFOW struct {
	HIcon         uintptr
	IIcon         int32
	DwAttributes  uint32
	SzDisplayName [260]uint16
	SzTypeName    [80]uint16
}

type SIZE struct {
	Cx, Cy int32
}

type BLENDFUNCTION struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

type BITMAPINFOHEADER struct {
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

type BITMAPINFO struct {
	Header BITMAPINFOHEADER
	Colors [1]uint32
}

type gdiplusStartupInput struct {
	GdiplusVersion           uint32
	DebugEventCallback       uintptr
	SuppressBackgroundThread int32
	SuppressExternalCodecs   int32
}

// openFileNameW 对应 OPENFILENAMEW（x64 布局，字段顺序不能改）。
type openFileNameW struct {
	StructSize      uint32
	pad1            uint32
	Owner           uintptr
	Instance        uintptr
	Filter          *uint16
	CustomFilter    *uint16
	MaxCustomFilter uint32
	FilterIndex     uint32
	File            *uint16
	MaxFile         uint32
	pad2            uint32
	FileTitle       *uint16
	MaxFileTitle    uint32
	pad3            uint32
	InitialDir      *uint16
	Title           *uint16
	Flags           uint32
	FileOffset      uint16
	FileExtension   uint16
	DefExt          *uint16
	CustData        uintptr
	FnHook          uintptr
	TemplateName    *uint16
	PvReserved      unsafe.Pointer
	DwReserved      uint32
	FlagsEx         uint32
}

// ---------------- 小工具 ----------------

func utf16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

// i32 把 int32 按位转换成 uintptr（保留负数位模式，用于坐标）。
func i32(v int32) uintptr { return uintptr(uint32(v)) }

func rgb(r, g, b uint32) uintptr { return uintptr(r | g<<8 | b<<16) }

func cursorArrow() uintptr {
	h, _, _ := pLoadCursorW.Call(0, IDC_ARROW)
	return h
}

var (
	hwndTopMost   = ^uintptr(0) // (HWND)-1
	hwndNoTopMost = ^uintptr(1) // (HWND)-2
)
