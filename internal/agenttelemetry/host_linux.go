package agenttelemetry

import (
	"io"
	"os"
	"strings"

	"ob-data-orch/internal/nodefacts"
)

func nativeHostInventory() *nodefacts.HostInventory {
	host := &nodefacts.HostInventory{}
	if content, err := readMetadataFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(content), "\n") {
			key, value, found := strings.Cut(line, "=")
			if !found {
				continue
			}
			value = strings.Trim(value, `"`)
			if key == "PRETTY_NAME" {
				host.OSName = metadataText(value, 128)
			}
			if key == "VERSION_ID" {
				host.OSVersion = metadataText(value, 128)
			}
		}
	}
	// procfs 文件不提供可靠的 stat 大小，仍只读取固定预算，避免整机库存无界增长。
	file, err := os.Open("/proc/cpuinfo")
	if err == nil {
		defer file.Close()
		content, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
		if err != nil || len(content) > 1<<20 {
			return host
		}
		cores := make(map[string]bool)
		for _, block := range strings.Split(string(content), "\n\n") {
			physical, core := "", ""
			for _, line := range strings.Split(block, "\n") {
				key, value, found := strings.Cut(line, ":")
				if !found {
					continue
				}
				key, value = strings.TrimSpace(key), strings.TrimSpace(value)
				if key == "physical id" {
					physical = value
				}
				if key == "core id" {
					core = value
				}
			}
			if physical != "" && core != "" {
				cores[physical+":"+core] = true
			}
		}
		if len(cores) > 0 {
			count := len(cores)
			host.PhysicalCores = &count
		}
		for _, line := range strings.Split(string(content), "\n") {
			key, value, found := strings.Cut(line, ":")
			if found && (strings.TrimSpace(key) == "model name" || strings.TrimSpace(key) == "Hardware" || strings.TrimSpace(key) == "Processor") {
				host.CPUModel = metadataText(value, 256)
				break
			}
		}
	}
	return host
}
