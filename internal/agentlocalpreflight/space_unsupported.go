//go:build !windows && !linux

package agentlocalpreflight

import "errors"

// AvailableBytes 在非正式支持的平台上始终拒绝提供空间事实。
func AvailableBytes(string) (uint64, error) {
	return 0, errors.New("当前平台不支持磁盘空间检查")
}
