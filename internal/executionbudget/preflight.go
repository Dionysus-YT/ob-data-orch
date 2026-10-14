package executionbudget

import "math"

// PreflightRequirement 是控制面按冻结堆配置和节点政策计算的静态资源要求。
// 它不代表资源预留或即时空闲量；执行领取仍使用实时事实和全部分配重新检查。
type PreflightRequirement struct {
	HeapBytes        uint64 `json:"heapBytes"`
	RequiredBytes    uint64 `json:"requiredBytes"`
	TotalBudgetBytes uint64 `json:"totalBudgetBytes"`
}

// Preflight 按与正式执行相同的堆外开销和系统预留规则计算资格；缺失事实保持未知。
func Preflight(heap string, memory uint64, policy Policy) PreflightRequirement {
	if heap == "" {
		heap = DefaultHeap
	}
	value, err := HeapBytes(heap)
	if err != nil {
		return PreflightRequirement{}
	}
	required, err := policy.Cost(value)
	if err != nil {
		return PreflightRequirement{}
	}
	budget, err := policy.Total(memory)
	if err != nil {
		budget = 0
	}
	return PreflightRequirement{HeapBytes: value, RequiredBytes: required, TotalBudgetBytes: budget}
}

// Qualification 返回静态配置能否满足节点总额度；不把未知额度当作零成本通过。
func (r PreflightRequirement) Qualification() (ready, known bool) {
	if r.HeapBytes == 0 || r.RequiredBytes < r.HeapBytes || r.TotalBudgetBytes == 0 || r.RequiredBytes > math.MaxInt64 || r.TotalBudgetBytes > math.MaxInt64 {
		return false, false
	}
	return r.RequiredBytes <= r.TotalBudgetBytes, true
}
