package agenttelemetry

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"ob-data-orch/internal/nodefacts"
)

func nativeHostInventory() *nodefacts.HostInventory {
	host := &nodefacts.HostInventory{}
	host.PhysicalCores = windowsPhysicalCores()
	if version := windows.RtlGetVersion(); version != nil {
		host.OSVersion = fmt.Sprintf("%d.%d.%d", version.MajorVersion, version.MinorVersion, version.BuildNumber)
	}
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE); err == nil {
		name, _, err := key.GetStringValue("ProductName")
		if err == nil {
			host.OSName = metadataText(name, 128)
		}
		_ = key.Close()
	}
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		model, _, err := key.GetStringValue("ProcessorNameString")
		if err == nil {
			host.CPUModel = metadataText(model, 256)
		}
		_ = key.Close()
	}
	return host
}

// windowsPhysicalCores 读取 Windows 原生拓扑记录，不把逻辑处理器数当成物理核数。
func windowsPhysicalCores() *int {
	procedure := kernel32.NewProc("GetLogicalProcessorInformationEx")
	var length uint32
	_, _, _ = procedure.Call(0, 0, uintptr(unsafe.Pointer(&length)))
	if length < 8 || length > 1<<20 {
		return nil
	}
	content := make([]byte, length)
	result, _, _ := procedure.Call(0, uintptr(unsafe.Pointer(&content[0])), uintptr(unsafe.Pointer(&length)))
	if result == 0 || int(length) > len(content) {
		return nil
	}
	count := 0
	offset := 0
	for offset < int(length) {
		if offset+8 > int(length) {
			return nil
		}
		relationship, size := binary.LittleEndian.Uint32(content[offset:]), binary.LittleEndian.Uint32(content[offset+4:])
		if size < 8 || int(size) > int(length)-offset {
			return nil
		}
		if relationship == 0 {
			count++
		}
		offset += int(size)
	}
	if count < 1 {
		return nil
	}
	return &count
}
