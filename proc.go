package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	TH32CS_SNAPPROCESS                = 0x00000002
	PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	PROCESS_QUERY_INFORMATION         = 0x0400
	PROCESS_VM_READ                   = 0x0010
	SHGFI_ICON                        = 0x00000100
	SHGFI_LARGEICON                   = 0x00000000
	SHGFI_SMALLICON                   = 0x00000001
)

// ProcEntry 是进程快照里的一条记录。
type ProcEntry struct {
	Pid  uint32
	PPid uint32
	Name string
}

// snapshotProcs 取一次系统进程快照（Toolhelp32），开销很小。
func snapshotProcs() []ProcEntry {
	h, _, _ := pCreateToolhelp32Snapshot.Call(TH32CS_SNAPPROCESS, 0)
	if h == 0 || h == uintptr(syscall.InvalidHandle) {
		return nil
	}
	defer pCloseHandle.Call(h)

	var pe PROCESSENTRY32W
	pe.Size = uint32(unsafe.Sizeof(pe))
	r, _, _ := pProcess32FirstW.Call(h, uintptr(unsafe.Pointer(&pe)))
	if r == 0 {
		return nil
	}
	out := make([]ProcEntry, 0, 256)
	for r != 0 {
		out = append(out, ProcEntry{
			Pid:  pe.ProcessID,
			PPid: pe.ParentProcessID,
			Name: syscall.UTF16ToString(pe.ExeFile[:]),
		})
		pe.Size = uint32(unsafe.Sizeof(pe))
		r, _, _ = pProcess32NextW.Call(h, uintptr(unsafe.Pointer(&pe)))
	}
	return out
}

func openProc(pid uint32) uintptr {
	h, _, _ := pOpenProcess.Call(PROCESS_QUERY_LIMITED_INFORMATION|PROCESS_VM_READ, 0, uintptr(pid))
	if h == 0 {
		h, _, _ = pOpenProcess.Call(PROCESS_QUERY_INFORMATION|PROCESS_VM_READ, 0, uintptr(pid))
	}
	return h
}

// ProcMem 是一个进程的内存指标。
// 任务管理器「内存」列显示的是私有工作集（Private Working Set），
// 不含与其它进程共享的 DLL/映射页，所以这里以它为准。
type ProcMem struct {
	WorkingSet uint64 // 工作集：含共享页，对应「资源监视器 → 工作集」
	PrivateWS  uint64 // 私有工作集：对应任务管理器「内存」列
	PrivateB   uint64 // 私有提交（Private Bytes / 提交大小）
}

// Value 按当前统计口径取值。
func (m ProcMem) Value(metric string) uint64 {
	if metric == "working" {
		return m.WorkingSet
	}
	return m.PrivateWS
}

// procMemory 读取进程内存指标。优先用 PROCESS_MEMORY_COUNTERS_EX2（Win8.1+），
// 失败时逐级降级到 EX / 基础版本。
func procMemory(pid uint32) (ProcMem, bool) {
	h := openProc(pid)
	if h == 0 {
		return ProcMem{}, false
	}
	defer pCloseHandle.Call(h)

	var ex2 PROCESS_MEMORY_COUNTERS_EX2
	ex2.Cb = uint32(unsafe.Sizeof(ex2))
	r, _, _ := pGetProcessMemoryInfo.Call(h, uintptr(unsafe.Pointer(&ex2)), uintptr(ex2.Cb))
	if r != 0 {
		m := ProcMem{
			WorkingSet: uint64(ex2.WorkingSetSize),
			PrivateWS:  uint64(ex2.PrivateWorkingSetSize),
			PrivateB:   uint64(ex2.PrivateUsage),
		}
		if m.PrivateWS == 0 {
			// 老系统（Win7）没有私有工作集字段，退回私有提交
			m.PrivateWS = m.PrivateB
		}
		return m, true
	}

	var ex PROCESS_MEMORY_COUNTERS_EX
	ex.Cb = uint32(unsafe.Sizeof(ex))
	r, _, _ = pGetProcessMemoryInfo.Call(h, uintptr(unsafe.Pointer(&ex)), uintptr(ex.Cb))
	if r != 0 {
		m := ProcMem{
			WorkingSet: uint64(ex.WorkingSetSize),
			PrivateWS:  uint64(ex.PrivateUsage),
			PrivateB:   uint64(ex.PrivateUsage),
		}
		return m, true
	}

	var pmc PROCESS_MEMORY_COUNTERS
	pmc.Cb = uint32(unsafe.Sizeof(pmc))
	r, _, _ = pGetProcessMemoryInfo.Call(h, uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.Cb))
	if r == 0 {
		return ProcMem{}, false
	}
	m := ProcMem{
		WorkingSet: uint64(pmc.WorkingSetSize),
		PrivateWS:  uint64(pmc.WorkingSetSize),
		PrivateB:   uint64(pmc.PagefileUsage),
	}
	return m, true
}

// procPath 取进程可执行文件完整路径（用于提取图标）。
func procPath(pid uint32) string {
	h, _, _ := pOpenProcess.Call(PROCESS_QUERY_LIMITED_INFORMATION, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer pCloseHandle.Call(h)

	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	r, _, _ := pQueryFullProcessImageName.Call(h, 0,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 || size == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}

// memStatus 返回物理内存总量与可用量。
func memStatus() (total uint64, avail uint64) {
	var ms MEMORYSTATUSEX
	ms.Length = uint32(unsafe.Sizeof(ms))
	r, _, _ := pGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms)))
	if r == 0 {
		return 0, 0
	}
	return ms.TotalPhys, ms.AvailPhys
}

// memLoad 返回整机内存占用百分比（0~100，与任务管理器口径一致）。
func memLoad() int {
	var ms MEMORYSTATUSEX
	ms.Length = uint32(unsafe.Sizeof(ms))
	r, _, _ := pGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms)))
	if r == 0 {
		return 0
	}
	return int(ms.MemoryLoad)
}

// treeMem 汇总某进程及其所有子孙进程的内存占用（按指定口径）。
func treeMem(root uint32, children map[uint32][]uint32, metric string) (uint64, int) {
	var total uint64
	count := 0
	seen := make(map[uint32]bool, 16)

	var walk func(pid uint32, depth int)
	walk = func(pid uint32, depth int) {
		if depth > 16 || seen[pid] {
			return
		}
		seen[pid] = true
		count++
		if m, ok := procMemory(pid); ok {
			total += m.Value(metric)
		}
		for _, c := range children[pid] {
			walk(c, depth+1)
		}
	}
	walk(root, 0)
	return total, count
}

// ---------------- 进程图标 ----------------

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// IID_IImageList {46EB5926-582E-4017-9FDF-E8998DAA0950}
var iidIImageList = guid{0x46EB5926, 0x582E, 0x4017, [8]byte{0x9F, 0xDF, 0xE8, 0x99, 0x8D, 0xAA, 0x09, 0x50}}

var sysImageList unsafe.Pointer

// initImageList 取系统 48x48 图标列表，缩放后比 32x32 大图标清晰得多。
func initImageList() {
	pCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED
	hr, _, _ := pSHGetImageList.Call(SHIL_EXTRALARGE,
		uintptr(unsafe.Pointer(&iidIImageList)), uintptr(unsafe.Pointer(&sysImageList)))
	if hr != 0 {
		sysImageList = nil
	}
}

// imageListVtbl 只声明到 IImageList::GetIcon 为止（索引 10）的虚表布局。
type imageListVtbl struct {
	QueryInterface  uintptr
	AddRef          uintptr
	Release         uintptr
	Add             uintptr
	ReplaceIcon     uintptr
	SetOverlayImage uintptr
	Replace         uintptr
	AddMasked       uintptr
	Draw            uintptr
	Remove          uintptr
	GetIcon         uintptr
}

// imageListIcon 调用 IImageList::GetIcon（虚表索引 10）。
func imageListIcon(il unsafe.Pointer, index int32) uintptr {
	if il == nil {
		return 0
	}
	vt := *(**imageListVtbl)(il)
	if vt == nil || vt.GetIcon == 0 {
		return 0
	}
	var h uintptr
	syscall.SyscallN(vt.GetIcon, uintptr(il), uintptr(uint32(index)), ILD_TRANSPARENT,
		uintptr(unsafe.Pointer(&h)))
	return h
}

var iconCache = map[string]uintptr{}

func iconFor(path string) uintptr {
	if path == "" {
		return 0
	}
	if h, ok := iconCache[path]; ok {
		return h
	}

	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0
	}
	var sfi SHFILEINFOW
	var h uintptr

	// 优先：取图标索引，再从 48x48 系统图像列表取高清图标
	r, _, _ := pSHGetFileInfoW.Call(uintptr(unsafe.Pointer(p)), 0,
		uintptr(unsafe.Pointer(&sfi)), unsafe.Sizeof(sfi), SHGFI_SYSICONINDEX)
	if r != 0 {
		h = imageListIcon(sysImageList, sfi.IIcon)
	}
	// 兜底：直接取 32x32 大图标
	if h == 0 {
		r, _, _ = pSHGetFileInfoW.Call(uintptr(unsafe.Pointer(p)), 0,
			uintptr(unsafe.Pointer(&sfi)), unsafe.Sizeof(sfi), SHGFI_ICON|SHGFI_LARGEICON)
		if r != 0 {
			h = sfi.HIcon
		}
	}
	runtime.KeepAlive(p)

	// 缓存上限，避免长期运行后图标对象堆积
	if len(iconCache) > 128 {
		for k, v := range iconCache {
			if v != 0 {
				pDestroyIcon.Call(v)
			}
			delete(iconCache, k)
			break
		}
	}
	iconCache[path] = h
	return h
}

// fmtBytes 人类可读的内存大小。
func fmtBytes(b uint64) string {
	const k = 1024
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= k:
		return fmt.Sprintf("%.0f KB", float64(b)/k)
	default:
		return fmt.Sprintf("%d B", b)
	}
}
