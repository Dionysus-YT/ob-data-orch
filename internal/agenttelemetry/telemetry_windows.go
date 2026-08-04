package agenttelemetry

import (
	"errors"
	"sync"
	"syscall"
	"unsafe"
)

var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	getSystemTimes       = kernel32.NewProc("GetSystemTimes")
	globalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	getDiskFreeSpaceEx   = kernel32.NewProc("GetDiskFreeSpaceExW")
)

type windowsSystemSampler struct {
	mu          sync.Mutex
	initialized bool
	idle        uint64
	kernel      uint64
	user        uint64
}

type memoryStatusEx struct {
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

func newSystemSampler() systemSampler {
	return &windowsSystemSampler{}
}

// Sample 使用 Windows 内核累计时间的两次差值计算 CPU，并以物理内存已用比例展示机器压力。
func (s *windowsSystemSampler) Sample() (*int, *int) {
	if s == nil {
		return nil, nil
	}
	memory := windowsMemoryUsage()
	idle, kernel, user, ok := windowsSystemTimes()
	if !ok {
		return nil, memory
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initialized {
		s.initialized, s.idle, s.kernel, s.user = true, idle, kernel, user
		return nil, memory
	}
	deltaIdle, deltaKernel, deltaUser := idle-s.idle, kernel-s.kernel, user-s.user
	s.idle, s.kernel, s.user = idle, kernel, user
	total := deltaKernel + deltaUser
	if total == 0 || deltaIdle > total {
		return nil, memory
	}
	value := int(((total - deltaIdle) * 100) / total)
	return &value, memory
}

func windowsSystemTimes() (uint64, uint64, uint64, bool) {
	var idle, kernel, user syscall.Filetime
	result, _, _ := getSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if result == 0 {
		return 0, 0, 0, false
	}
	return filetimeValue(idle), filetimeValue(kernel), filetimeValue(user), true
}

func windowsMemoryUsage() *int {
	status := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	result, _, _ := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if result == 0 || status.TotalPhys == 0 || status.AvailPhys > status.TotalPhys {
		return nil
	}
	value := int(((status.TotalPhys - status.AvailPhys) * 100) / status.TotalPhys)
	return &value
}

func dataRootUsage(root string) (uint64, uint64, error) {
	path, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		return 0, 0, errors.New("data root path is invalid")
	}
	var available, total, free uint64
	result, _, _ := getDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(path)), uintptr(unsafe.Pointer(&available)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&free)))
	if result == 0 {
		return 0, 0, errors.New("data root usage is unavailable")
	}
	return total, available, nil
}

func filetimeValue(value syscall.Filetime) uint64 {
	return uint64(value.HighDateTime)<<32 | uint64(value.LowDateTime)
}
