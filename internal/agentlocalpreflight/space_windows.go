//go:build windows

package agentlocalpreflight

import (
	"errors"
	"syscall"
	"unsafe"
)

var getDiskFreeSpaceExW = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

// AvailableBytes 读取当前 Agent 所在 Windows 文件系统对调用服务账户可用的剩余空间。
// 它不启动命令、不会输出路径或底层系统错误，调用者只将结果映射为固定预检查证据码。
func AvailableBytes(path string) (uint64, error) {
	windowsPath, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, errors.New("磁盘空间路径无效")
	}
	var available uint64
	result, _, callErr := getDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(windowsPath)),
		uintptr(unsafe.Pointer(&available)),
		0,
		0,
	)
	if result == 0 {
		return 0, callErr
	}
	return available, nil
}
