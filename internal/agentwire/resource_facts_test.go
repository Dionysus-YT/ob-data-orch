package agentwire

import (
	"testing"
	"time"
)

func Test物理内存和并行事实拒绝矛盾及溢出(t *testing.T) {
	input := Heartbeat{BootID: "synthetic-boot", SentAt: time.Now(), Facts: EnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "synthetic", ObservedAt: time.Now(), CapacityTotal: 16, CapacityUsed: 2, MemoryTotalBytes: 16 << 30, MemoryAvailableBytes: 8 << 30}}
	if !validHeartbeat(input) {
		t.Fatal("有效并行资源被拒绝")
	}
	for _, change := range []func(*Heartbeat){func(v *Heartbeat) { v.Facts.MemoryTotalBytes = 1 << 53 }, func(v *Heartbeat) { v.Facts.MemoryAvailableBytes = v.Facts.MemoryTotalBytes + 1 }, func(v *Heartbeat) { v.Facts.CapacityTotal = 1025 }, func(v *Heartbeat) { v.Facts.CapacityUsed = 17 }, func(v *Heartbeat) { v.Facts.MemoryTotalBytes = 0 }} {
		invalid := input
		change(&invalid)
		if validHeartbeat(invalid) {
			t.Fatal("矛盾或溢出的资源事实被接受")
		}
	}
}
