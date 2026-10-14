package agentwire

import (
	"context"
	"time"

	"ob-data-orch/internal/nodefacts"
)

// CompleteExecutionNodeEnvironmentCheck 使用当前受保护机器身份回传固定运行时检查结果。
// 检查标识只能来自刚收到的控制面心跳响应，调用方不能借此传送路径、命令或自定义检查内容。
func (s *StateStore) CompleteExecutionNodeEnvironmentCheck(ctx context.Context, bootID, checkID string, factsRevision int64, status, code string, sentAt time.Time) error {
	return s.completeNodeEnvironmentCheck(ctx, bootID, checkID, factsRevision, status, code, sentAt, nil)
}

// CompleteExecutionNodeEnvironmentInspection 回传当前固定清单，不能借版本查询扩展任意命令。
func (s *StateStore) CompleteExecutionNodeEnvironmentInspection(ctx context.Context, bootID, checkID string, factsRevision int64, status, code string, sentAt time.Time, inspection *nodefacts.RuntimeInspection) error {
	if inspection == nil || nodefacts.ValidateInspection(inspection) != nil || inspection.CheckID != checkID {
		return ErrProtocolRejected
	}
	return s.completeNodeEnvironmentCheck(ctx, bootID, checkID, factsRevision, status, code, sentAt, inspection)
}

func (s *StateStore) completeNodeEnvironmentCheck(ctx context.Context, bootID, checkID string, factsRevision int64, status, code string, sentAt time.Time, inspection *nodefacts.RuntimeInspection) error {
	state, found, err := s.loadState()
	if err != nil {
		return err
	}
	if !found {
		return ErrIdentityUnavailable
	}
	defer state.destroy()
	if state.pendingEnrollment() || !validOpaqueValue(bootID, 256) || !validEnvironmentCheckID(checkID) || factsRevision < 1 || !validEnvironmentCheckResult(status, code) || sentAt.IsZero() {
		return ErrProtocolRejected
	}
	client, err := newHTTPSClient(state.ControlPlaneURL, state.CAFile)
	if err != nil {
		return ErrIdentityUnavailable
	}
	requestID, err := newOpaqueID()
	if err != nil {
		return ErrIdentityUnavailable
	}
	return client.completeExecutionNodeEnvironmentCheck(ctx, &state, requestID, bootID, checkID, factsRevision, status, code, sentAt, inspection)
}
