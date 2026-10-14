package agenttelemetry

import (
	"net"
	"net/netip"
	"os"
	"runtime"
	"sort"
	"strings"

	"ob-data-orch/internal/nodefacts"
)

// HostSnapshot 只查询本机接口地址和操作系统信息，不做 DNS、端口或外部网络探测。
func HostSnapshot() *nodefacts.HostInventory {
	host := nativeHostInventory()
	host.LogicalCPUs = runtime.NumCPU()
	host.Addresses = make([]string, 0)
	name, err := os.Hostname()
	if err == nil && nodefacts.ValidDiagnosticText(name, 253) {
		host.Hostname = name
	}
	addresses, err := net.InterfaceAddrs()
	if err == nil {
		seen := make(map[string]bool)
		for _, address := range addresses {
			prefix, err := netip.ParsePrefix(address.String())
			if err != nil {
				continue
			}
			ip := prefix.Addr().Unmap()
			if ip.IsGlobalUnicast() && !ip.IsLoopback() && ip.Zone() == "" && !seen[ip.String()] {
				host.Addresses = append(host.Addresses, ip.String())
				seen[ip.String()] = true
			}
		}
	}
	sort.Strings(host.Addresses)
	if len(host.Addresses) > 32 {
		host.Addresses = host.Addresses[:32]
	}
	if nodefacts.ValidateHost(host) != nil {
		return nil
	}
	return host
}

func metadataText(value string, maximum int) string {
	value = strings.TrimSpace(value)
	if !nodefacts.ValidDiagnosticText(value, maximum) {
		return ""
	}
	return value
}
