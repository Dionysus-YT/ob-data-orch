// Package securitypolicy 提供只读的控制面安全状态，不接收秘密或修改安全策略。
package securitypolicy

import (
	"context"
	"errors"
	"regexp"
	"time"

	"ob-data-orch/internal/credential"
)

// ErrUnavailable 表示启动事实或密钥可用性无法确认，调用方不得显示猜测状态。
var ErrUnavailable = errors.New("安全状态暂时不可用")

var keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// Startup 只接收实际启动配置及非秘密密钥标识，不接收密钥字节、路径或凭据。
type Startup struct {
	LoadedAt                     time.Time
	CredentialKeyID              string
	RealExecutionEnabled         bool
	NormalImportExecutionEnabled bool
	DirectLoadProbeEnabled       bool
	DirectLoadExecutionEnabled   bool
}

// Snapshot 是系统管理员可读取的白名单投影，密钥标识只是版本别名，绝不是密钥摘要。
type Snapshot struct {
	CapturedAt             time.Time `json:"capturedAt"`
	LoadedAt               time.Time `json:"loadedAt"`
	Source                 string    `json:"source"`
	BuiltInRedaction       bool      `json:"builtInRedaction"`
	CredentialFormat       string    `json:"credentialFormat"`
	CredentialKeyID        string    `json:"credentialKeyId"`
	RootKeyStatus          string    `json:"rootKeyStatus"`
	RootKeySource          string    `json:"rootKeySource"`
	SensitiveCommandView   string    `json:"sensitiveCommandView"`
	ProductionConfirmation string    `json:"productionConfirmation"`
	// AuditRetentionDays 为空表示未配置期限；不能把普通日志期限套入审计策略。
	AuditRetentionDays           *int `json:"auditRetentionDays"`
	AuditAutomaticCleanup        bool `json:"auditAutomaticCleanup"`
	RealExecutionEnabled         bool `json:"realExecutionEnabled"`
	NormalImportExecutionEnabled bool `json:"normalImportExecutionEnabled"`
	DirectLoadProbeEnabled       bool `json:"directLoadProbeEnabled"`
	DirectLoadExecutionEnabled   bool `json:"directLoadExecutionEnabled"`
}

// Service 持有不可变启动事实，读取时只核对内存中的密钥标识可用性，不解密业务凭据。
type Service struct {
	startup  Startup
	keyReady func() bool
}

// New 只接受完整且门禁关系一致的启动事实；缺失依赖不能产生默认安全状态。
func New(startup Startup, keyReady func() bool) (*Service, error) {
	if startup.LoadedAt.IsZero() || !keyIDPattern.MatchString(startup.CredentialKeyID) || keyReady == nil ||
		(!startup.RealExecutionEnabled && (startup.NormalImportExecutionEnabled || startup.DirectLoadProbeEnabled || startup.DirectLoadExecutionEnabled)) ||
		(startup.DirectLoadExecutionEnabled && !startup.DirectLoadProbeEnabled) {
		return nil, ErrUnavailable
	}
	startup.LoadedAt = startup.LoadedAt.UTC()
	return &Service{startup: startup, keyReady: keyReady}, nil
}

// Read 返回本次进程启动配置的事实；载入时间不冒充轮换时间或当前文件权限检查时间。
func (s *Service) Read(ctx context.Context) (Snapshot, error) {
	if s == nil || ctx == nil || ctx.Err() != nil || s.keyReady == nil || !s.keyReady() {
		return Snapshot{}, ErrUnavailable
	}
	now := time.Now().UTC()
	if s.startup.LoadedAt.After(now) || ctx.Err() != nil {
		return Snapshot{}, ErrUnavailable
	}
	return Snapshot{
		CapturedAt: now, LoadedAt: s.startup.LoadedAt, Source: "STARTUP_CONFIGURATION",
		BuiltInRedaction: true, CredentialFormat: credential.FormatVersion,
		CredentialKeyID: s.startup.CredentialKeyID, RootKeyStatus: "LOADED", RootKeySource: "LOCAL_PROTECTED_FILE",
		SensitiveCommandView: "NOT_AVAILABLE", ProductionConfirmation: "TASK_SUBMISSION_RULES",
		AuditRetentionDays: nil, AuditAutomaticCleanup: false,
		RealExecutionEnabled: s.startup.RealExecutionEnabled, NormalImportExecutionEnabled: s.startup.NormalImportExecutionEnabled,
		DirectLoadProbeEnabled: s.startup.DirectLoadProbeEnabled, DirectLoadExecutionEnabled: s.startup.DirectLoadExecutionEnabled,
	}, nil
}
