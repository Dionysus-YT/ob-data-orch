// Package schedulingpolicy 定义平台执行并发硬上限，不替代节点预算或 Agent 承载约束。
package schedulingpolicy

import (
	"errors"
	"time"
)

// MaxExecutions 限定首版设置输入，不表示平台已验证的承载容量。
const MaxExecutions = 1024

// 以下错误只返回安全业务语义，不携带任务目录或部署信息。
var (
	ErrInvalid      = errors.New("调度策略字段无效")
	ErrDenied       = errors.New("无权访问调度策略")
	ErrRevision     = errors.New("调度策略版本冲突")
	ErrIdempotency  = errors.New("调度策略幂等冲突")
	ErrConfirmation = errors.New("收紧调度上限需要确认")
	ErrUnavailable  = errors.New("调度策略不可用")
	ErrCapacity     = errors.New("达到平台执行并发上限")
)

// Usage 区分运行与尚未释放的分配，未授权的全局数量必须为空。
type Usage struct {
	Visibility        string    `json:"visibility"`
	HeldExecutions    *int      `json:"heldExecutions"`
	RunningExecutions *int      `json:"runningExecutions"`
	ObservedAt        time.Time `json:"observedAt"`
}

// Policy 是安全配置投影；空上限仅表示尚未配置，不能推断节点有无限容量。
type Policy struct {
	MaxConcurrentExecutions *int       `json:"maxConcurrentExecutions"`
	Revision                int64      `json:"revision"`
	Source                  string     `json:"source"`
	ModifiedBy              string     `json:"modifiedBy,omitempty"`
	ModifiedAt              *time.Time `json:"modifiedAt,omitempty"`
	ApplicationMode         string     `json:"applicationMode"`
	RestartRequired         bool       `json:"restartRequired"`
	Usage                   Usage      `json:"usage"`
}

// Change 由可信入口补入身份、时间和请求标识，浏览器只提供业务意图。
type Change struct {
	ActorID                 string
	ExpectedRevision        int64
	MaxConcurrentExecutions int
	Reason                  string
	Confirmed               bool
	IdempotencyKey          string
	RequestID               string
	At                      time.Time
}

// Receipt 仅回执原事务，不能将幂等历史版本作为当前策略。
type Receipt struct {
	Revision int64 `json:"revision"`
	Changed  bool  `json:"changed"`
	Replayed bool  `json:"replayed"`
}

// Tightens 首次设置会约束原本没有平台硬上限的调度，因此也要求收紧确认。
func Tightens(before *int, after int) bool { return before == nil || after < *before }
