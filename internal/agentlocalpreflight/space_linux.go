//go:build linux

package agentlocalpreflight

import (
	"errors"
	"math"
	"syscall"
)

// AvailableBytes 读取当前 Agent 所在 Linux 文件系统对服务账户可用的剩余空间。
// 它不运行 shell 或外部工具，保留系统保留块语义，避免把总空闲空间误判为当前账户可用空间。
func AvailableBytes(path string) (uint64, error) {
	var statistics syscall.Statfs_t
	if err := syscall.Statfs(path, &statistics); err != nil {
		return 0, err
	}
	if statistics.Bavail < 0 || statistics.Bsize <= 0 {
		return 0, errors.New("磁盘空间统计无效")
	}
	blocks := uint64(statistics.Bavail)
	blockSize := uint64(statistics.Bsize)
	if blocks != 0 && blockSize > math.MaxUint64/blocks {
		return 0, errors.New("磁盘空间统计溢出")
	}
	return blocks * blockSize, nil
}
