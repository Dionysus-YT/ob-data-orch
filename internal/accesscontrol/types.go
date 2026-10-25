package accesscontrol

import "time"

// Subject 是可信认证投影的只读最小摘要，不含会话或认证材料。
type Subject struct {
	ID            string `json:"id"`
	Login         string `json:"login"`
	DisplayName   string `json:"displayName"`
	AccountStatus string `json:"accountStatus"`
}

// Profile 绑定单个主体的完整授权版本，更新时间不冒充身份目录的实时同步结果。
type Profile struct {
	Subject              Subject    `json:"subject"`
	Assignment           Assignment `json:"assignment"`
	Revision             int64      `json:"revision"`
	ModifiedBy           string     `json:"modifiedBy,omitempty"`
	ModifiedAt           *time.Time `json:"modifiedAt,omitempty"`
	InitialAdministrator bool       `json:"initialAdministrator"`
	Editable             bool       `json:"editable"`
	// ScopeObjects 仅详情填充最小对象元信息；列表为空，不把授权目录扩成业务详情。
	ScopeObjects []ScopeObject `json:"scopeObjects"`
}

// ScopeObject 将已有绑定映射到当前授权目录；已删除对象只标不可新增，不重建历史名称。
type ScopeObject struct {
	Type        ScopeType `json:"type"`
	ObjectID    string    `json:"objectId"`
	DisplayName string    `json:"displayName"`
	Environment string    `json:"environment,omitempty"`
	Available   bool      `json:"available"`
}

// SubjectQuery 的游标只确定后续主体，不能改变授权或客户端声明身份。
type SubjectQuery struct {
	Search     string
	Role       Role
	Status     string
	Capability Capability
	AfterID    string
	Limit      int
}

// SubjectPage 是受权限管理能力保护的有界主体目录，不开放业务对象详情。
type SubjectPage struct {
	Items   []Profile
	HasMore bool
}

// Candidate 只提供授权选择必要的最小元信息，不能容纳连接、路径、命令或秘密。
type Candidate struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Kind        string `json:"kind"`
	Environment string `json:"environment,omitempty"`
	Available   bool   `json:"available"`
}

// Change 是一次完整替换意图，操作者和请求追踪必须来自可信 HTTP 上下文。
type Change struct {
	ActorID          string
	SubjectID        string
	ExpectedRevision int64
	Assignment       Assignment
	Reason           string
	Confirmed        bool
	IdempotencyKey   string
	RequestID        string
	At               time.Time
}

// ChangeResult 只回执已提交版本，不把旧幂等回执当作当前权限快照。
type ChangeResult struct {
	Revision int64 `json:"revision"`
	Changed  bool  `json:"changed"`
	Replayed bool  `json:"replayed"`
}
