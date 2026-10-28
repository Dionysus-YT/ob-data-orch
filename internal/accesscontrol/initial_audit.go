package accesscontrol

import "time"

// InitialAuditAccess 只描述部署初始身份的一次性审计授予资格，不授予其他角色或对象。
type InitialAuditAccess struct {
	Revision int64
	Granted  bool
	Consumed bool
}

// InitialAuditGrant 由部署入口在校验原管理员口令和明确确认后构造，不接受浏览器输入。
type InitialAuditGrant struct {
	SubjectID        string
	ExpectedRevision int64
	Confirmed        bool
	At               time.Time
}
