package agentexec

import (
	"strconv"
	"strings"

	"ob-data-orch/internal/executionbudget"
)

// applyFrozenJVMHeap 替换固定启动器的堆参数，而非添加会被后续默认值覆盖的选项。
// 堆仅来自冻结的 --mem；旧任务未设置时继续使用官方 4G。任意自定义堆选项均拒绝。
func applyFrozenJVMHeap(profile, business, options []string) ([]string, error) {
	heap, err := executionbudget.EffectiveHeap(business)
	if err != nil {
		return nil, ErrDirectJavaLaunchInvalid
	}
	for _, option := range options {
		if strings.HasPrefix(option, "-Xms") || strings.HasPrefix(option, "-Xmx") || strings.HasPrefix(option, "-XX:InitialHeapSize") || strings.HasPrefix(option, "-XX:MaxHeapSize") {
			return nil, ErrDirectJavaLaunchInvalid
		}
	}
	result := append([]string(nil), profile...)
	value := strconv.FormatUint(heap/1024, 10) + "K"
	// 无后缀沿用字节语义；不能通过取整让启动堆与分配堆不一致。
	if heap%1024 != 0 {
		value = strconv.FormatUint(heap, 10)
	}
	for i, option := range result {
		if strings.HasPrefix(option, "-Xms") {
			result[i] = "-Xms" + value
		}
		if strings.HasPrefix(option, "-Xmx") {
			result[i] = "-Xmx" + value
		}
	}
	return result, nil
}
