package executionbudget

import (
	"math"
	"testing"
)

func Test静态资格使用正式执行的开销与总额度(t *testing.T) {
	for _, item := range []struct {
		heap         string
		memory       uint64
		ready, known bool
	}{
		{"", 16 * GiB, true, true}, {"12G", 8 * GiB, false, true}, {"4G", 0, false, false}, {"invalid", 16 * GiB, false, false}, {"4G", math.MaxUint64, false, false},
	} {
		requirement := Preflight(item.heap, item.memory, Policy{})
		ready, known := requirement.Qualification()
		if ready != item.ready || known != item.known {
			t.Fatalf("静态资格错误: %v/%v", ready, known)
		}
		if item.heap == "" && (requirement.HeapBytes != 4*GiB || requirement.RequiredBytes != 5*GiB) {
			t.Fatal("默认堆或堆外开销与正式执行不一致")
		}
	}
	if _, known := (PreflightRequirement{HeapBytes: GiB, RequiredBytes: 0, TotalBudgetBytes: 16 * GiB}).Qualification(); known {
		t.Fatal("损坏要求未失败关闭")
	}
}
