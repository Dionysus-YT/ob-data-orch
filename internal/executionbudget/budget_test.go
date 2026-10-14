package executionbudget

import "testing"

func Test实际堆与预算统一且非法输入失败关闭(t *testing.T) {
	for _, tc := range []struct {
		argv []string
		heap uint64
	}{{nil, 4 * GiB}, {[]string{"--mem", "8G"}, 8 * GiB}, {[]string{"--mem", "512M"}, GiB / 2}, {[]string{"--mem", "1T"}, 1024 * GiB}, {[]string{"--mem", "1P"}, 1 << 50}, {[]string{"--mem", "4294967296"}, 4 * GiB}} {
		heap, err := EffectiveHeap(tc.argv)
		if err != nil || heap != tc.heap {
			t.Fatalf("堆大小 %v: %d %v", tc.argv, heap, err)
		}
		cost, err := (Policy{}).Cost(heap)
		if err != nil || cost != heap+max(GiB, heap/4) {
			t.Fatal("实际堆未进入预算")
		}
	}
	for _, argv := range [][]string{{"--mem"}, {"--mem", "0G"}, {"--mem", "4GB"}, {"--mem", "1G", "--mem", "2G"}, {"--mem=4G"}, {"--mem", "999999999999999T"}, {"--mem", "4G -Dunsafe=true"}} {
		if _, err := EffectiveHeap(argv); err == nil {
			t.Fatalf("非法堆参数未被拒绝: %v", argv)
		}
	}
	if _, err := (Policy{}).Total(0); err == nil {
		t.Fatal("未知资源生成额度")
	}
	if total, err := (Policy{}).Total(20 * GiB); err != nil || total != 16*GiB {
		t.Fatal("系统预留计算错误")
	}
	if _, err := (Policy{SystemReservedBytes: 20 * GiB}).Total(20 * GiB); err == nil {
		t.Fatal("系统预留未阻断零额度")
	}
	if cost, err := (Policy{TaskOverheadBytes: 2 * GiB}).Cost(4 * GiB); err != nil || cost != 6*GiB {
		t.Fatal("配置覆盖未生效")
	}
}
