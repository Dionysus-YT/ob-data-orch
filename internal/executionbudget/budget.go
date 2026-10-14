// Package executionbudget 统一冻结参数的 JVM 堆与节点执行内存预算，不拼装业务命令。
package executionbudget

import (
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const (
	// GiB 使用二进制单位，与 JVM 内存参数一致。
	GiB uint64 = 1 << 30
	// DefaultHeap 是 OBDUMPER 4.3.5 Windows 官方启动器默认堆大小。
	DefaultHeap = "4G"
)

var memoryPattern = regexp.MustCompile(`^([1-9][0-9]*)([KMGTP]?)$`)

// HeapBytes 只接受已规范化的官方内存单位，拒绝溢出与任意 JVM 参数。
func HeapBytes(value string) (uint64, error) {
	parts := memoryPattern.FindStringSubmatch(strings.ToUpper(value))
	if len(parts) != 3 {
		return 0, errors.New("JVM memory is invalid")
	}
	n, err := strconv.ParseUint(parts[1], 10, 64)
	shift := uint(0)
	if parts[2] != "" {
		shift = uint(10 * (strings.Index("KMGTP", parts[2]) + 1))
	}
	if err != nil || n > math.MaxInt64>>shift {
		return 0, errors.New("JVM memory is invalid")
	}
	return n << shift, nil
}

// EffectiveHeap 从唯一冻结 argv 读取堆；未设置保持官方默认，重复、缺值或不明确的令牌失败关闭。
func EffectiveHeap(argv []string) (uint64, error) {
	value, found := DefaultHeap, false
	for i, token := range argv {
		if token == "--mem" {
			if found || i+1 >= len(argv) {
				return 0, errors.New("JVM memory is ambiguous")
			}
			value, found = argv[i+1], true
		} else if strings.HasPrefix(token, "--mem=") {
			return 0, errors.New("JVM memory token is not canonical")
		}
	}
	return HeapBytes(value)
}

// Policy 只承载节点管理员可配置的调度预算，不是机器资源事实或实际 RSS 上限。
// 零值采用保守默认；字节覆盖只影响新分配，已有分配不可回写。
type Policy struct {
	SystemReservedBytes uint64 `json:"systemReservedBytes,omitempty"`
	TaskOverheadBytes   uint64 `json:"taskOverheadBytes,omitempty"`
}

// Total 先扣除系统/Agent 预留，未取得有效物理内存时不生成可用额度。
func (p Policy) Total(memory uint64) (uint64, error) {
	reserved := p.SystemReservedBytes
	if reserved == 0 {
		reserved = max(2*GiB, memory/5)
	}
	if memory == 0 || memory > math.MaxInt64 || reserved >= memory {
		return 0, errors.New("node memory budget is unavailable")
	}
	return memory - reserved, nil
}

// Cost 对实际堆加入堆外开销；未知或溢出不能作为零预算领取。
func (p Policy) Cost(heap uint64) (uint64, error) {
	overhead := p.TaskOverheadBytes
	if overhead == 0 {
		overhead = max(GiB, heap/4)
	}
	if heap == 0 || overhead > math.MaxInt64 || heap > math.MaxInt64-overhead {
		return 0, errors.New("task memory budget is invalid")
	}
	return heap + overhead, nil
}
