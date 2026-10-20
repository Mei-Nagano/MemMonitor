package main

// 内存清理。分三档，力度从小到大：
//
//	1) 修剪当前进程（或进程树）的工作集 —— EmptyWorkingSet，普通权限即可
//	2) 修剪全部进程的工作集 —— 同上，逐个进程调用
//	3) 清空系统备用列表 + 系统文件缓存 —— NtSetSystemInformation，需要管理员
//
// 前两档能把「工作集」里的页换出到页面文件，任务管理器里的内存数字会立刻下降；
// 第三档才是真正把物理内存还给系统（备用列表本身就是可回收缓存）。

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

var (
	ntdll    = syscall.NewLazyDLL("ntdll.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	pEmptyWorkingSet          = psapi.NewProc("EmptyWorkingSet")
	pGetCurrentProcess        = kernel32.NewProc("GetCurrentProcess")
	pGetSystemInfo            = kernel32.NewProc("GetSystemInfo")
	pNtSetSystemInformation   = ntdll.NewProc("NtSetSystemInformation")
	pNtQuerySystemInformation = ntdll.NewProc("NtQuerySystemInformation")
	pOpenProcessToken         = advapi32.NewProc("OpenProcessToken")
	pLookupPrivilegeValueW    = advapi32.NewProc("LookupPrivilegeValueW")
	pAdjustTokenPrivileges    = advapi32.NewProc("AdjustTokenPrivileges")
)

const (
	PROCESS_SET_QUOTA = 0x0100

	TOKEN_ADJUST_PRIVILEGES = 0x0020
	TOKEN_QUERY             = 0x0008
	SE_PRIVILEGE_ENABLED    = 0x00000002

	systemFileCacheInformation  = 0x15
	systemMemoryListInformation = 0x50
	memoryPurgeStandbyList      = 4

	statusSuccess            = 0
	statusPrivilegeNotHeld   = 0xC0000061
	statusAccessDenied       = 0xC0000022
	statusInfoLengthMismatch = 0xC0000004
)

type luid struct {
	LowPart  uint32
	HighPart int32
}

type luidAndAttributes struct {
	Luid       luid
	Attributes uint32
}

type tokenPrivileges struct {
	PrivilegeCount uint32
	Privileges     [1]luidAndAttributes
}

type systemInfo struct {
	ProcessorArchitecture     uint16
	Reserved                  uint16
	PageSize                  uint32
	MinimumApplicationAddress uintptr
	MaximumApplicationAddress uintptr
	ActiveProcessorMask       uintptr
	NumberOfProcessors        uint32
	ProcessorType             uint32
	AllocationGranularity     uint32
	ProcessorLevel            uint16
	ProcessorRevision         uint16
}

// systemFileCacheInformation 对应 SYSTEM_FILECACHE_INFORMATION。
// MinimumWorkingSet / MaximumWorkingSet 都填 (SIZE_T)-1 表示要求系统刷掉文件缓存。
type fileCacheInfo struct {
	CurrentSize                           uint64
	PeakSize                              uint64
	PageFaultCount                        uint32
	MinimumWorkingSet                     uint64
	MaximumWorkingSet                     uint64
	CurrentSizeIncludingTransitionInPages uint64
	PeakSizeIncludingTransitionInPages    uint64
	TransitionRePurposeCount              uint32
	Flags                                 uint32
}

// cleanResult 是一次清理的结果。
type cleanResult struct {
	Freed  uint64 // 释放（或换出）的字节数
	Text   string // 直接展示给用户的文案
	Detail string // 调试用
}

// ---------------- 基础操作 ----------------

// enablePrivilege 启用当前进程的某个特权（需要管理员才可能成功）。
func enablePrivilege(name string) bool {
	var token uintptr
	r, _, _ := pOpenProcessToken.Call(^uintptr(0), TOKEN_ADJUST_PRIVILEGES|TOKEN_QUERY,
		uintptr(unsafe.Pointer(&token)))
	if r == 0 || token == 0 {
		return false
	}
	defer pCloseHandle.Call(token)

	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return false
	}
	var id luid
	r, _, _ = pLookupPrivilegeValueW.Call(0, uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&id)))
	if r == 0 {
		return false
	}

	var tp tokenPrivileges
	tp.PrivilegeCount = 1
	tp.Privileges[0].Luid = id
	tp.Privileges[0].Attributes = SE_PRIVILEGE_ENABLED
	r, _, _ = pAdjustTokenPrivileges.Call(token, 0, uintptr(unsafe.Pointer(&tp)), 0, 0, 0)
	return r != 0
}

// trimProcess 修剪单个进程的工作集。
func trimProcess(pid uint32) bool {
	h, _, _ := pOpenProcess.Call(PROCESS_SET_QUOTA|PROCESS_QUERY_LIMITED_INFORMATION, 0, uintptr(pid))
	if h == 0 {
		h, _, _ = pOpenProcess.Call(PROCESS_SET_QUOTA|PROCESS_QUERY_INFORMATION, 0, uintptr(pid))
	}
	if h == 0 {
		return false
	}
	defer pCloseHandle.Call(h)
	r, _, _ := pEmptyWorkingSet.Call(h)
	return r != 0
}

// trimSelf 修剪监控程序自己的工作集。
func trimSelf() {
	h, _, _ := pGetCurrentProcess.Call()
	if h != 0 {
		pEmptyWorkingSet.Call(h)
	}
}

// pageSize 返回系统页大小。
func pageSize() uint64 {
	var si systemInfo
	pGetSystemInfo.Call(uintptr(unsafe.Pointer(&si)))
	if si.PageSize == 0 {
		return 4096
	}
	return uint64(si.PageSize)
}

// standbyBytes 读取系统备用列表（standby list）占用的字节数。
// 结构体版本随系统变化，这里只读前 21 个 SIZE_T，其中 [5..12] 是按优先级分组的页数。
func standbyBytes() uint64 {
	const n = 32 // 实际返回 176 字节（22 个 SIZE_T），留足余量避免长度不匹配
	buf := make([]uint64, n)
	var ret uint32
	st, _, _ := pNtQuerySystemInformation.Call(systemMemoryListInformation,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)*8), uintptr(unsafe.Pointer(&ret)))
	if st != statusSuccess || ret < 13*8 {
		return 0
	}
	var pages uint64
	for i := 5; i < 13; i++ {
		pages += buf[i]
	}
	return pages * pageSize()
}

// debugMemList 打印内存列表原始数据，用于校准结构体版本差异。
func debugMemList() {
	const n = 64
	buf := make([]uint64, n)
	var ret uint32
	st, _, _ := pNtQuerySystemInformation.Call(systemMemoryListInformation,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)*8), uintptr(unsafe.Pointer(&ret)))
	fmt.Printf("query st=0x%x ret=%d pageSize=%d\n", int32(st), int32(ret), int32(pageSize()))
	for i := 0; i < 24 && i*8 < int(ret); i++ {
		fmt.Printf("  [%d] %d\n", i, int64(buf[i]))
	}
}

// purgeStandby 清空备用列表。
func purgeStandby() uint32 {
	cmd := int32(memoryPurgeStandbyList)
	st, _, _ := pNtSetSystemInformation.Call(systemMemoryListInformation,
		uintptr(unsafe.Pointer(&cmd)), unsafe.Sizeof(cmd))
	return uint32(st)
}

// flushFileCache 清系统文件缓存（工作集）。
func flushFileCache() uint32 {
	var info fileCacheInfo
	info.MinimumWorkingSet = ^uint64(0)
	info.MaximumWorkingSet = ^uint64(0)
	st, _, _ := pNtSetSystemInformation.Call(systemFileCacheInformation,
		uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
	return uint32(st)
}

// ---------------- 三档清理 ----------------

// cleanTarget 修剪当前监控进程（开了进程树则连同子进程）。
func cleanTarget() cleanResult {
	if state.TargetName == "" {
		return cleanResult{Text: "未选择进程"}
	}

	before := state.MemBytes
	procs := snapshotProcs()
	children := make(map[uint32][]uint32, len(procs))
	for _, p := range procs {
		children[p.PPid] = append(children[p.PPid], p.Pid)
	}

	if cfg.Tree {
		seen := map[uint32]bool{}
		var walk func(pid uint32, depth int)
		walk = func(pid uint32, depth int) {
			if depth > 16 || seen[pid] {
				return
			}
			seen[pid] = true
			trimProcess(pid)
			for _, c := range children[pid] {
				walk(c, depth+1)
			}
		}
		walk(state.TargetPid, 0)
	} else if !trimProcess(state.TargetPid) {
		return cleanResult{Text: "清理失败（权限不足）"}
	}

	time.Sleep(120 * time.Millisecond)
	sample()
	after := state.MemBytes
	freed := uint64(0)
	if before > after {
		freed = before - after
	}
	return cleanResult{Freed: freed, Text: "已释放 " + fmtBytes(freed)}
}

// cleanAll 修剪全部可访问进程的工作集。
func cleanAll() cleanResult {
	procs := snapshotProcs()
	if len(procs) == 0 {
		return cleanResult{Text: "清理失败"}
	}

	var before uint64
	for _, p := range procs {
		if m, ok := procMemory(p.Pid); ok {
			before += m.WorkingSet
		}
	}

	done := 0
	for _, p := range procs {
		if trimProcess(p.Pid) {
			done++
		}
	}
	trimSelf()

	time.Sleep(200 * time.Millisecond)

	var after uint64
	for _, p := range snapshotProcs() {
		if m, ok := procMemory(p.Pid); ok {
			after += m.WorkingSet
		}
	}
	freed := uint64(0)
	if before > after {
		freed = before - after
	}
	return cleanResult{
		Freed:  freed,
		Text:   "已释放 " + fmtBytes(freed),
		Detail: "trimmed " + itoa(done) + "/" + itoa(len(procs)),
	}
}

// cleanSystem 清空系统备用列表与文件缓存，需要管理员权限。
func cleanSystem() cleanResult {
	enablePrivilege("SeProfileSingleProcessPrivilege")
	enablePrivilege("SeIncreaseQuotaPrivilege")

	before := standbyBytes()
	st1 := flushFileCache()
	st2 := purgeStandby()
	time.Sleep(150 * time.Millisecond)
	after := standbyBytes()

	if st2 == statusPrivilegeNotHeld || st2 == statusAccessDenied ||
		st1 == statusPrivilegeNotHeld || st1 == statusAccessDenied {
		return cleanResult{Text: "需要管理员权限"}
	}
	if st2 != statusSuccess && st1 != statusSuccess {
		return cleanResult{Text: "清理失败"}
	}

	freed := uint64(0)
	if before > after {
		freed = before - after
	}
	text := "已清理系统缓存"
	if freed > 0 {
		text = "已释放 " + fmtBytes(freed)
	}
	return cleanResult{Freed: freed, Text: text}
}

// itoa 避免为了拼接调试信息引入 strconv。
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}
