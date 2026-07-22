// Package agentexec 提供 G2 阶段的 Agent 本地执行适配纯核心。
// 本包只处理无秘密意图和合成事实，不解析凭据、发起网络连接或创建操作系统进程。
package agentexec

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ob-data-orch/internal/credential"
)

var (
	ErrStartIntentInvalid = errors.New("启动意图无效")
	ErrStartIntentExists  = errors.New("启动意图已存在，必须先核对")
)

const startIntentFileName = "start-intent.json"

var (
	opaqueIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
	digestPattern   = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// StartIntent 是创建进程前必须原子保存的无秘密事实。
// 崩溃恢复只要看到该记录就必须进入核对，不能因缺少 PID 再次启动。
type StartIntent struct {
	ExecutionID    string    `json:"executionId"`
	TaskID         string    `json:"taskId"`
	LeaseID        string    `json:"leaseId"`
	LeaseEpoch     int64     `json:"leaseEpoch"`
	EnvelopeDigest string    `json:"envelopeDigest"`
	CreatedAt      time.Time `json:"createdAt"`
}

// WriteStartIntent 在 execution 私有证据目录中以创建新文件的方式写入意图。
// 已有意图一律拒绝覆盖，避免重启或重放路径将同一 execution 启动两次。
func WriteStartIntent(workspace credential.Workspace, intent StartIntent) error {
	if err := validateStartIntent(intent); err != nil {
		return err
	}
	directory := workspace.EvidenceDirectory()
	if directory == "" {
		return ErrStartIntentInvalid
	}
	path := filepath.Join(directory, startIntentFileName)
	if _, err := os.Lstat(path); err == nil {
		return ErrStartIntentExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("检查启动意图: %w", err)
	}
	encoded, err := json.Marshal(intent)
	if err != nil {
		return ErrStartIntentInvalid
	}
	encoded = append(encoded, '\n')
	temporary, err := os.CreateTemp(directory, ".start-intent-")
	if err != nil {
		return fmt.Errorf("创建启动意图临时文件: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("写入启动意图: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("同步启动意图: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭启动意图: %w", err)
	}
	if err := os.Link(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrStartIntentExists
		}
		return fmt.Errorf("发布启动意图: %w", err)
	}
	return nil
}

// ReadStartIntent 读取恢复所需的无秘密意图；不存在时返回 found=false。
func ReadStartIntent(workspace credential.Workspace) (intent StartIntent, found bool, err error) {
	directory := workspace.EvidenceDirectory()
	if directory == "" {
		return StartIntent{}, false, ErrStartIntentInvalid
	}
	content, err := os.ReadFile(filepath.Join(directory, startIntentFileName))
	if errors.Is(err, os.ErrNotExist) {
		return StartIntent{}, false, nil
	}
	if err != nil {
		return StartIntent{}, false, fmt.Errorf("读取启动意图: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&intent); err != nil || validateStartIntent(intent) != nil {
		return StartIntent{}, false, ErrStartIntentInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return StartIntent{}, false, ErrStartIntentInvalid
	}
	return intent, true, nil
}

// RemoveStartIntent 只在终态已取证且不再需要恢复时删除启动意图。
func RemoveStartIntent(workspace credential.Workspace) error {
	directory := workspace.EvidenceDirectory()
	if directory == "" {
		return ErrStartIntentInvalid
	}
	err := os.Remove(filepath.Join(directory, startIntentFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("删除启动意图: %w", err)
	}
	return nil
}

func validateStartIntent(intent StartIntent) error {
	if blank(intent.ExecutionID, intent.TaskID, intent.LeaseID, intent.EnvelopeDigest) || !opaqueIDPattern.MatchString(intent.ExecutionID) || !opaqueIDPattern.MatchString(intent.TaskID) || !opaqueIDPattern.MatchString(intent.LeaseID) || !digestPattern.MatchString(intent.EnvelopeDigest) || intent.LeaseEpoch < 1 || intent.CreatedAt.IsZero() {
		return ErrStartIntentInvalid
	}
	return nil
}

func blank(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
	}
	return false
}
