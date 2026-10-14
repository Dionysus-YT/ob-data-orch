package nodefacts

import (
	"strings"
	"testing"
	"time"
)

func Test实际版本自报通过不能扩大当前工具支持范围(t *testing.T) {
	report := &RuntimeInspection{CheckID: "synthetic-check", RuntimeConfigurationDigest: strings.Repeat("a", 64), ObservedAt: time.Now().UTC(), VersionProbeAllowed: true, DirectLoad: &DirectLoadConfiguration{Status: "MISSING"}}
	for _, kind := range InspectionKinds() {
		item := InspectionItem{Kind: kind, Status: "PASSED", Code: "READY", Required: kind == "JAVA_RUNTIME" || kind == "OBDUMPER_RUNTIME" || kind == "CONNECTOR" || kind == "WORKSPACE" || kind == "LOG_QUEUE" || kind == "RESOURCES" || kind == "RUNTIME_LAYOUT"}
		if kind == "JAVA_RUNTIME" {
			item.Version = "1.8.0_491"
		}
		if kind == "OBDUMPER_RUNTIME" || kind == "OBLOADER_RUNTIME" {
			item.Version = "4.3.5-RELEASE"
		}
		report.Items = append(report.Items, item)
	}
	if ValidateInspection(report) != nil {
		t.Fatal("当前完整版本事实被拒绝")
	}
	for _, test := range []struct{ kind, version string }{{"JAVA_RUNTIME", "17.0.1"}, {"JAVA_RUNTIME", "1.8.0_private"}, {"OBDUMPER_RUNTIME", "4.3.6"}, {"OBLOADER_RUNTIME", "4.3.4"}} {
		for index := range report.Items {
			if report.Items[index].Kind != test.kind {
				continue
			}
			previous := report.Items[index].Version
			report.Items[index].Version = test.version
			if ValidateInspection(report) == nil {
				t.Fatal("Agent 自报通过扩大了支持版本")
			}
			if status, _ := InspectionResult(report); status == "PASSED" {
				t.Fatal("无效版本仍派生通过")
			}
			report.Items[index].Version = previous
		}
	}
}

func Test主机摘要拒绝非法地址且不推算缺省物理核数(t *testing.T) {
	host := &HostInventory{Hostname: "synthetic-host", Addresses: []string{"192.0.2.12", "2001:db8::12"}, LogicalCPUs: 8}
	if ValidateHost(host) != nil || host.PhysicalCores != nil {
		t.Fatal("可取得的系统事实被拒绝或补造物理核数")
	}
	for _, addresses := range [][]string{nil, {"192.0.2.12", "192.0.2.12"}, {"127.0.0.1"}, {"::1"}, {"https://synthetic.invalid"}, {"fe80::1%synthetic"}} {
		host.Addresses = addresses
		if ValidateHost(host) == nil {
			t.Fatal("非法或非受控接口地址进入心跳")
		}
	}
}
