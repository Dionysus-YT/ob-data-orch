// Package nodefacts 定义节点诊断链共享的有界、无秘密事实，不提供执行或远程文件访问能力。
package nodefacts

import (
	"errors"
	"regexp"
	"time"
)

// InstalledVersion 是安装文件中的版本声明；不等同于工具真实运行通过。
type InstalledVersion struct {
	Version string `json:"version"`
	Source  string `json:"source"`
}

// RuntimeVersions 的缺项保持未知，禁止使用平台目标版本或目录名补造事实。
type RuntimeVersions struct {
	Java     *InstalledVersion `json:"java,omitempty"`
	OBLoader *InstalledVersion `json:"obloader,omitempty"`
	OBDumper *InstalledVersion `json:"obdumper,omitempty"`
}

// ActiveExecution 只包含本机 Worker 当前持有的执行与租约标识。
// LastEventSeq 是本地已尝试发出的序号，不能当作控制面已接受事件或最终状态。
type ActiveExecution struct {
	ExecutionID  string `json:"executionId"`
	LeaseID      string `json:"leaseId"`
	LeaseEpoch   int64  `json:"leaseEpoch"`
	LastEventSeq int64  `json:"lastEventSeq"`
}

// ExecutionObservation 使用非空对象区分已观察到零活动执行与旧 Agent 尚未上报。
type ExecutionObservation struct {
	Active []ActiveExecution `json:"active"`
}

// LogQueue 是已脱敏日志可靠队列的整体统计，不含正文、来源、任务标识或原始错误。
type LogQueue struct {
	PendingBatches   int        `json:"pendingBatches"`
	PendingGaps      int        `json:"pendingGaps"`
	PendingBytes     int64      `json:"pendingBytes"`
	LastUploadResult string     `json:"lastUploadResult"`
	LastUploadAt     *time.Time `json:"lastUploadAt,omitempty"`
}

var versionPattern = regexp.MustCompile(`^[0-9][0-9A-Za-z._+\-]{0,63}$`)
var idPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z_\-]{0,255}$`)

// ValidVersion 限制安装元数据为纯版本标识，拒绝路径、命令、多行及任意工具输出。
func ValidVersion(version string) bool { return versionPattern.MatchString(version) }

// Validate 在 Agent 发送和控制面持久化两端复用相同的边界。
func Validate(startedAt *time.Time, observedAt time.Time, versions *RuntimeVersions, executions *ExecutionObservation, queue *LogQueue) error {
	invalid := errors.New("node diagnostic facts are invalid")
	if startedAt != nil && (startedAt.IsZero() || startedAt.After(observedAt)) {
		return invalid
	}
	if versions != nil {
		for index, version := range []*InstalledVersion{versions.Java, versions.OBLoader, versions.OBDumper} {
			if version == nil {
				continue
			}
			if !ValidVersion(version.Version) || (index == 0 && version.Source != "JAVA_RELEASE") || (index > 0 && version.Source != "JAR_MANIFEST" && version.Source != "JAR_POM") {
				return invalid
			}
		}
	}
	if executions != nil {
		if executions.Active == nil || len(executions.Active) > 1024 {
			return invalid
		}
		seen := make(map[string]bool, len(executions.Active))
		for _, execution := range executions.Active {
			if !idPattern.MatchString(execution.ExecutionID) || !idPattern.MatchString(execution.LeaseID) || execution.LeaseEpoch < 1 || execution.LeaseEpoch > 1<<53-1 || execution.LastEventSeq < 0 || execution.LastEventSeq > 1<<53-1 || seen[execution.ExecutionID] {
				return invalid
			}
			seen[execution.ExecutionID] = true
		}
	}
	if queue != nil {
		if queue.PendingBatches < 0 || queue.PendingGaps < 0 || queue.PendingBatches > 1<<30 || queue.PendingGaps > 1<<30 || queue.PendingBytes < 0 || queue.PendingBytes > (1<<30)+(16<<20) {
			return invalid
		}
		switch queue.LastUploadResult {
		case "UNKNOWN":
			if queue.LastUploadAt != nil {
				return invalid
			}
		case "SUCCEEDED", "FAILED":
			if queue.LastUploadAt == nil || queue.LastUploadAt.IsZero() || queue.LastUploadAt.After(observedAt) || (startedAt != nil && queue.LastUploadAt.Before(*startedAt)) {
				return invalid
			}
		default:
			return invalid
		}
	}
	return nil
}
