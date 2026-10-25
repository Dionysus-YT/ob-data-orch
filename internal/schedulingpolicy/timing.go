package schedulingpolicy

import "time"

// DefaultHeartbeatTimeout 沿用控制面的既有心跳判定基线，不改变 Agent 本机心跳周期。
const DefaultHeartbeatTimeout = 2 * time.Minute

// TimingOverrides 仅持有明确保存的时间覆盖；空启动期限和空检查间隔保留既有行为。
type TimingOverrides struct {
	HeartbeatSeconds *int
	StartupSeconds   *int
	PollingSeconds   *int
	Revision         int64
	ModifiedBy       string
	ModifiedAt       *time.Time
}

// TimingPolicy 是可公开的实际策略，不包含部署路径或 Agent 网络配置。
type TimingPolicy struct {
	HeartbeatTimeoutSeconds int        `json:"heartbeatTimeoutSeconds"`
	HeartbeatTimeoutSource  string     `json:"heartbeatTimeoutSource"`
	StartupTimeoutSeconds   *int       `json:"startupTimeoutSeconds"`
	PollingIntervalSeconds  *int       `json:"pollingIntervalSeconds"`
	CandidateStrategy       string     `json:"candidateStrategy"`
	Revision                int64      `json:"revision"`
	ModifiedBy              string     `json:"modifiedBy,omitempty"`
	ModifiedAt              *time.Time `json:"modifiedAt,omitempty"`
	ApplicationMode         string     `json:"applicationMode"`
	RestartRequired         bool       `json:"restartRequired"`
}

// TimingChange 只接收时间策略意图；身份、基线和事务元信息必须来自可信入口。
type TimingChange struct {
	ActorID          string
	ExpectedRevision int64
	// BaselineHeartbeatTimeout 来自控制面启动配置，不接受浏览器自报，也不冒充平台覆盖。
	BaselineHeartbeatTimeout time.Duration
	HeartbeatTimeoutSeconds  int
	StartupTimeoutSeconds    *int
	PollingIntervalSeconds   *int
	Reason                   string
	Confirmed                bool
	IdempotencyKey           string
	RequestID                string
	At                       time.Time
}

// ResolveTiming 对损坏覆盖失败关闭；未配置心跳覆盖时展示实际控制面启动基线。
func ResolveTiming(raw TimingOverrides, baseline time.Duration) (TimingPolicy, error) {
	policy := TimingPolicy{Revision: raw.Revision, ModifiedBy: raw.ModifiedBy, ModifiedAt: raw.ModifiedAt,
		HeartbeatTimeoutSource: "CONTROL_PLANE", StartupTimeoutSeconds: raw.StartupSeconds, PollingIntervalSeconds: raw.PollingSeconds,
		CandidateStrategy: "EXPLICIT_SELECTION", ApplicationMode: "HEARTBEAT_NOW_STARTUP_NEXT_CLAIM_POLL_NEXT_CHECK"}
	if raw.Revision < 1 || raw.Revision >= 9007199254740991 || !validSeconds(raw.HeartbeatSeconds, 60, 3600) || !validSeconds(raw.StartupSeconds, 15, 3600) || !validSeconds(raw.PollingSeconds, 1, 60) {
		return policy, ErrUnavailable
	}
	modified := raw.HeartbeatSeconds != nil || raw.StartupSeconds != nil || raw.PollingSeconds != nil
	if modified != (raw.ModifiedBy != "") || modified != (raw.ModifiedAt != nil) || raw.ModifiedAt != nil && raw.ModifiedAt.IsZero() {
		return policy, ErrUnavailable
	}
	if raw.HeartbeatSeconds != nil {
		policy.HeartbeatTimeoutSeconds = *raw.HeartbeatSeconds
		policy.HeartbeatTimeoutSource = "PLATFORM_SETTING"
	} else {
		if baseline <= 0 || baseline%time.Second != 0 || baseline > 24*time.Hour {
			return policy, ErrUnavailable
		}
		policy.HeartbeatTimeoutSeconds = int(baseline / time.Second)
	}
	return policy, nil
}

func validSeconds(value *int, minimum, maximum int) bool {
	return value == nil || *value >= minimum && *value <= maximum
}

// TimingChangeRequiresConfirmation 覆盖首次启动期限、缩短判定期限及任何检查频率变化的影响。
func TimingChangeRequiresConfirmation(before TimingPolicy, after TimingChange) bool {
	return after.HeartbeatTimeoutSeconds < before.HeartbeatTimeoutSeconds ||
		after.StartupTimeoutSeconds != nil && (before.StartupTimeoutSeconds == nil || *after.StartupTimeoutSeconds < *before.StartupTimeoutSeconds) ||
		!SameSeconds(before.PollingIntervalSeconds, after.PollingIntervalSeconds)
}

// SameSeconds 比较可空配置，空值不能冒充零秒。
func SameSeconds(a, b *int) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
