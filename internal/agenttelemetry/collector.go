// Package agenttelemetry 采集已关联 Agent 的固定机器资源与数据目录空间事实。
// 它不接收控制面下发的路径：采样范围只能来自首次关联后加密保存的本机运行时配置。
package agenttelemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/outputpath"
)

// RuntimeConfigurationReader 收窄采集器读取本机固化配置的能力。
type RuntimeConfigurationReader interface {
	RuntimeConfiguration() (agentwire.RuntimeConfiguration, error)
}

// Snapshot 是单次心跳可安全上报的资源事实。
type Snapshot struct {
	CPUUsagePercent            *int
	MemoryUsagePercent         *int
	RuntimeConfigurationDigest string
	DataRootUsages             []agentwire.DataRootUsage
}

// Collector 在一个 Agent 进程中保留 CPU 前后两次采样，避免把累计系统时间误报为瞬时利用率。
type Collector struct {
	configuration RuntimeConfigurationReader
	system        systemSampler
}

// NewCollector 构造只会读取本机已固化配置的资源采集器。
func NewCollector(configuration RuntimeConfigurationReader) *Collector {
	return &Collector{configuration: configuration, system: newSystemSampler()}
}

// Sample 采集 CPU、内存及全部已配置导出数据目录的空间；任一目录无法读取时失败关闭。
func (c *Collector) Sample() (Snapshot, error) {
	if c == nil || c.configuration == nil || c.system == nil {
		return Snapshot{}, errors.New("agent telemetry configuration is unavailable")
	}
	configuration, err := c.configuration.RuntimeConfiguration()
	if err != nil || configuration.Digest == "" || len(configuration.AllowedRoots) == 0 {
		return Snapshot{}, errors.New("agent telemetry runtime configuration is unavailable")
	}
	cpu, memory := c.system.Sample()
	usages := make([]agentwire.DataRootUsage, 0, len(configuration.AllowedRoots))
	for _, root := range configuration.AllowedRoots {
		localRoot, pathOK := outputpath.LocalFilesystemPath(configuration.Platform, root)
		if !pathOK {
			return Snapshot{}, errors.New("agent telemetry data root is unavailable")
		}
		total, available, usageErr := dataRootUsage(localRoot)
		if usageErr != nil || available > total {
			return Snapshot{}, errors.New("agent telemetry data root is unavailable")
		}
		usages = append(usages, agentwire.DataRootUsage{RootDigest: DataRootDigest(root), TotalBytes: total, AvailableBytes: available})
	}
	return Snapshot{CPUUsagePercent: cpu, MemoryUsagePercent: memory, RuntimeConfigurationDigest: configuration.Digest, DataRootUsages: usages}, nil
}

// DataRootDigest 把已登记的数据目录映射为稳定摘要，供控制面在不接收 Agent 路径的前提下关联空间数据。
func DataRootDigest(root string) string {
	digest := sha256.Sum256([]byte(root))
	return hex.EncodeToString(digest[:])
}

type systemSampler interface {
	Sample() (*int, *int)
}
