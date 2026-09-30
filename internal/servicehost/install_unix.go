//go:build !windows

package servicehost

import "errors"

// Install 的 Linux 服务安装由同一安装包中的 systemd 启动入口处理。
func Install(_ string, _ []byte, _ ...string) error {
	return errors.New("请使用安装包启动入口安装 systemd 服务")
}
