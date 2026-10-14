package agenttelemetry

// ExecutionWorkerLimit 给出基于实际物理内存的本机 Worker 安全上界。
// 每 GiB 最多一个调度 Worker，最多 1024；控制面仍按实际任务堆及节点预算进行更严格的原子准入。
// 无采样或不足一 GiB 时不领取任务，不用固定单任务容量冒充真实资源。
func ExecutionWorkerLimit() int {
	total, _ := physicalMemory()
	return int(min(uint64(1024), total/(1<<30)))
}
