// Package logretention 管理普通任务日志期限及保存与清理批次之间的生效边界。
package logretention

import (
	"context"
	"errors"
	"time"
)

// DefaultDays 是未配置普通日志期限时的内置值。
const DefaultDays = 30

// 以下错误只表达安全业务结果，不携带原始请求或部署信息。
var (
	ErrInvalid      = errors.New("普通日志保留设置无效")
	ErrDenied       = errors.New("无权访问普通日志保留设置")
	ErrRevision     = errors.New("普通日志保留设置版本冲突")
	ErrIdempotency  = errors.New("普通日志保留设置幂等冲突")
	ErrConfirmation = errors.New("缩短普通日志保留期限需要确认")
	ErrUnavailable  = errors.New("普通日志保留设置不可用")
)

// Record 是持久化事实；空覆盖继续继承启动配置，不伪造修改时间。
type Record struct {
	Days       *int
	Revision   int64
	ModifiedBy string
	ModifiedAt *time.Time
}

// Policy 是可公开的实际策略投影，不含部署文件或任务目录。
type Policy struct {
	RetentionDays   int        `json:"retentionDays"`
	Revision        int64      `json:"revision"`
	Source          string     `json:"source"`
	ModifiedBy      string     `json:"modifiedBy,omitempty"`
	ModifiedAt      *time.Time `json:"modifiedAt,omitempty"`
	ApplicationMode string     `json:"applicationMode"`
	RestartRequired bool       `json:"restartRequired"`
}

// Change 由可信入口补入身份、版本和请求标识；浏览器只能提交业务字段。
type Change struct {
	ActorID          string
	ExpectedRevision int64
	RetentionDays    int
	Reason           string
	Confirmed        bool
	IdempotencyKey   string
	RequestID        string
	At               time.Time
}

// Receipt 只确认原事务，不把幂等历史快照冒充当前配置。
type Receipt struct {
	Revision int64 `json:"revision"`
	Changed  bool  `json:"changed"`
	Replayed bool  `json:"replayed"`
}

// Repository 隔离授权读取、系统读取和包含审计的原子写入。
type Repository interface {
	ReadAuthorizedLogRetention(context.Context, string) (Record, error)
	ReadCurrentLogRetention(context.Context) (Record, error)
	ChangeLogRetention(context.Context, Change, int) (Receipt, error)
}

// Days 只接受受控范围；零表示启动配置未设置。
func Days(value int) (int, error) {
	if value == 0 {
		return DefaultDays, nil
	}
	if value < 7 || value > 365 {
		return 0, ErrInvalid
	}
	return value, nil
}

// Resolve 根据持久覆盖和可信启动配置生成实际策略；损坏数据失败关闭。
func Resolve(record Record, deploymentDays int) (Policy, error) {
	days, err := Days(deploymentDays)
	if err != nil || record.Revision < 1 || record.Revision >= 9007199254740991 {
		return Policy{}, ErrUnavailable
	}
	source := "DEPLOYMENT_CONFIG"
	if deploymentDays == 0 {
		source = "BUILT_IN_DEFAULT"
	}
	if record.Days != nil {
		if *record.Days < 7 || *record.Days > 365 || record.ModifiedBy == "" || record.ModifiedAt == nil {
			return Policy{}, ErrUnavailable
		}
		days, source = *record.Days, "PLATFORM_SETTING"
	} else if record.ModifiedBy != "" || record.ModifiedAt != nil {
		return Policy{}, ErrUnavailable
	}
	return Policy{RetentionDays: days, Revision: record.Revision, Source: source, ModifiedBy: record.ModifiedBy, ModifiedAt: record.ModifiedAt, ApplicationMode: "NEXT_BATCH"}, nil
}

// Service 的门闩由网页保存与清理批次共用，避免保存成功后登记旧期限的新清理意图。
// 文件操作可以持有门闩，但不会因此延长 SQLite 事务。
type Service struct {
	repository     Repository
	deploymentDays int
	gate           chan struct{}
}

// New 使用可信启动配置创建唯一策略所有者。
func New(repository Repository, deploymentDays int) (*Service, error) {
	if repository == nil {
		return nil, ErrUnavailable
	}
	if _, err := Days(deploymentDays); err != nil {
		return nil, err
	}
	return &Service{repository: repository, deploymentDays: deploymentDays, gate: make(chan struct{}, 1)}, nil
}

// Read 返回当前已授权策略；授权必须由仓储在同一读取快照内复验。
func (s *Service) Read(ctx context.Context, actor string) (Policy, error) {
	if s == nil || actor == "" {
		return Policy{}, ErrDenied
	}
	record, err := s.repository.ReadAuthorizedLogRetention(ctx, actor)
	if err != nil {
		return Policy{}, err
	}
	return Resolve(record, s.deploymentDays)
}

func (s *Service) acquire(ctx context.Context) error {
	if s == nil {
		return ErrUnavailable
	}
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		<-s.gate
		return err
	}
	return nil
}

// Change 等待当前批次结束，再原子保存；取消等待不会提交配置。
func (s *Service) Change(ctx context.Context, input Change) (Receipt, error) {
	if err := s.acquire(ctx); err != nil {
		return Receipt{}, err
	}
	defer func() { <-s.gate }()
	return s.repository.ChangeLogRetention(ctx, input, s.deploymentDays)
}

// WithPolicy 为一个清理批次读取最新期限；读取失败时绝不调用文件清理。
func (s *Service) WithPolicy(ctx context.Context, batch func(int) error) error {
	if batch == nil {
		return ErrInvalid
	}
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer func() { <-s.gate }()
	record, err := s.repository.ReadCurrentLogRetention(ctx)
	if err != nil {
		return err
	}
	policy, err := Resolve(record, s.deploymentDays)
	if err != nil {
		return err
	}
	return batch(policy.RetentionDays)
}
