//go:build windows

package agentexec

import (
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

var toolWideCharToMultiByte = windows.NewLazySystemDLL("kernel32.dll").NewProc("WideCharToMultiByte")

// toolArgumentsRepresentable 防止 Java 8 的 Windows 命令行转换静默替换查询或路径字符。
// 只验证，不转码或改写 argv；未知代码页、替代字符和非往返映射均拒绝启动。
func toolArgumentsRepresentable(arguments []string) bool {
	codePage := windows.GetACP()
	if codePage == 0 {
		return false
	}
	for _, argument := range arguments {
		if !toolArgumentRepresentable(argument, codePage) {
			return false
		}
	}
	return true
}

func toolArgumentRepresentable(argument string, codePage uint32) bool {
	if !utf8.ValidString(argument) {
		return false
	}
	wide, err := windows.UTF16FromString(argument)
	if err != nil {
		return false
	}
	if len(wide) == 1 {
		return true
	}
	defer clear(wide)
	var substituted int32
	flags, used := uintptr(0x400), &substituted
	if codePage == 65001 {
		// UTF-8 代码页不接受最佳映射标志和替代字符指针，仍须做完整往返比较。
		flags, used = 0, nil
	}
	count, _, _ := toolWideCharToMultiByte.Call(uintptr(codePage), flags, uintptr(unsafe.Pointer(&wide[0])), uintptr(len(wide)-1), 0, 0, 0, uintptr(unsafe.Pointer(used)))
	if count == 0 || substituted != 0 {
		return false
	}
	encoded := make([]byte, count)
	defer clear(encoded)
	written, _, _ := toolWideCharToMultiByte.Call(uintptr(codePage), flags, uintptr(unsafe.Pointer(&wide[0])), uintptr(len(wide)-1), uintptr(unsafe.Pointer(&encoded[0])), count, 0, uintptr(unsafe.Pointer(used)))
	if written != count || substituted != 0 {
		return false
	}
	length, err := windows.MultiByteToWideChar(codePage, 0, &encoded[0], int32(len(encoded)), nil, 0)
	if err != nil || length != int32(len(wide)-1) {
		return false
	}
	decoded := make([]uint16, length)
	defer clear(decoded)
	if n, err := windows.MultiByteToWideChar(codePage, 0, &encoded[0], int32(len(encoded)), &decoded[0], length); err != nil || n != length {
		return false
	}
	for index, value := range decoded {
		if value != wide[index] {
			return false
		}
	}
	return true
}
