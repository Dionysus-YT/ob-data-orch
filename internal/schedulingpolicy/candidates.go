package schedulingpolicy

import (
	"sort"
	"time"
)

const (
	// ExistingOrder 保留节点领域的现有稳定顺序。
	ExistingOrder = "EXISTING"
	// OnlineFirst 仅将已确认心跳新鲜的节点提前，不表示任务可执行。
	OnlineFirst = "ONLINE_FIRST"
)

// CandidatePolicy 只改变候选读取顺序，不持有选择、任务或节点配置。
type CandidatePolicy struct {
	Order           string     `json:"order"`
	Revision        int64      `json:"revision"`
	ModifiedBy      string     `json:"modifiedBy,omitempty"`
	ModifiedAt      *time.Time `json:"modifiedAt,omitempty"`
	SelectionMode   string     `json:"selectionMode"`
	ApplicationMode string     `json:"applicationMode"`
	RestartRequired bool       `json:"restartRequired"`
}

// CandidateChange 的身份、时间与版本由受信任的控制面入口提供。
type CandidateChange struct {
	ActorID          string
	ExpectedRevision int64
	Order            string
	Reason           string
	Confirmed        bool
	IdempotencyKey   string
	RequestID        string
	At               time.Time
}

// ValidCandidateOrder 限定内置枚举，不接受脚本、节点组表达式或自动改派策略。
func ValidCandidateOrder(order string) bool { return order == ExistingOrder || order == OnlineFirst }

// ValidateCandidatePolicy 对未知枚举及不完整修改记录失败关闭。
func ValidateCandidatePolicy(policy CandidatePolicy) error {
	if !ValidCandidateOrder(policy.Order) || policy.Revision < 1 || policy.Revision >= 9007199254740991 ||
		policy.SelectionMode != "EXPLICIT_SELECTION" || policy.ApplicationMode != "NEXT_CANDIDATE_READ" || policy.RestartRequired ||
		(policy.ModifiedBy == "") != (policy.ModifiedAt == nil) || policy.ModifiedAt != nil && policy.ModifiedAt.IsZero() ||
		policy.Revision == 1 && (policy.Order != ExistingOrder || policy.ModifiedBy != "") || policy.Revision > 1 && policy.ModifiedBy == "" {
		return ErrUnavailable
	}
	return nil
}

// CandidateOrderIndices 只对已授权候选做稳定分组，未知、未来或过期心跳不作为在线证据。
// 同组保留原顺序；结果只是排列，调用方不得据此自动修改用户选择。
func CandidateOrderIndices(order string, heartbeats []*time.Time, now time.Time, ttl time.Duration) ([]int, error) {
	if !ValidCandidateOrder(order) || order == OnlineFirst && (now.IsZero() || ttl <= 0) {
		return nil, ErrUnavailable
	}
	indices := make([]int, len(heartbeats))
	for index := range indices {
		indices[index] = index
	}
	if order == OnlineFirst {
		online := func(index int) bool {
			stamp := heartbeats[index]
			return stamp != nil && !stamp.IsZero() && !stamp.After(now) && now.Before(stamp.Add(ttl))
		}
		sort.SliceStable(indices, func(a, b int) bool { return online(indices[a]) && !online(indices[b]) })
	}
	return indices, nil
}
