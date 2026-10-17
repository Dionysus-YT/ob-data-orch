package importdomain

// MinimumDataHeapBytes 是 OBLOADER 4.3.5 默认数据内存节流器的最低堆要求：
// 300 MiB 不可用区、200 MiB 弹性区和 200 MiB 保底区；不代表 JVM 实际可用堆或节点额度。
const MinimumDataHeapBytes uint64 = 700 << 20

// DataHeapInsufficient 只拒绝包含数据且已知堆低于工具下限的配置。
// 零值仍由资源事实检查保持未知；仅结构导入不套用数据节流器的要求。
func (c Config) DataHeapInsufficient(heapBytes uint64) bool {
	return c.HasData() && heapBytes > 0 && heapBytes < MinimumDataHeapBytes
}
