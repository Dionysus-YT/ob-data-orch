package controlplane

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

type scopeTestAuthorizer struct{ globalRole, globalRead bool }

func (a scopeTestAuthorizer) Authorize(_ context.Context, _ identity.Principal, scope identity.Scope, _ string) error {
	if scope == identity.ScopeTaskReadAll && a.globalRead {
		return nil
	}
	return identity.ErrDenied
}
func (a scopeTestAuthorizer) AuthorizeRole(_ context.Context, _ identity.Principal, role identity.Role) error {
	if role == identity.RoleGlobalAdmin && a.globalRole {
		return nil
	}
	return identity.ErrDenied
}

type scopeTestTaskStore struct {
	recordingTaskStore
	filtered store.TaskListQuery
}

func (s *scopeTestTaskStore) CountFilteredTaskSummaries(_ context.Context, q store.TaskListQuery) (int, error) {
	s.filtered = q
	return 0, nil
}

func Test全局任务范围必须同时持有可信角色与能力(t *testing.T) {
	for _, tc := range []struct{ role, read, allowed bool }{{false, false, false}, {true, false, false}, {false, true, false}, {true, true, true}} {
		tasks := &scopeTestTaskStore{}
		auth := scopeTestAuthorizer{globalRole: tc.role, globalRead: tc.read}
		handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Tasks: tasks, Authorizer: auth, Roles: auth})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks?nodeId=node-1&scope=ALL", nil))
		if tc.allowed {
			if response.Code != http.StatusOK || !tasks.filtered.GlobalRead || tasks.filtered.NodeID != "node-1" {
				t.Fatal("可信全局范围未投影", response.Code, tasks.filtered)
			}
		} else if response.Code != http.StatusForbidden || tasks.filtered.SubjectID != "" {
			t.Fatal("客户端范围绕过授权", response.Code, tasks.filtered)
		}
	}
}

func Test任务游标绑定节点账号范围及当前全局授权(t *testing.T) {
	query := store.TaskListQuery{SubjectID: "subject", NodeID: "node-a", OwnedOnly: true}
	value := encodeTaskListCursor(taskListFilterDigestSubject(query), time.Now(), "task-a")
	if _, ok := decodeTaskListCursor(value, taskListFilterDigestSubject(query)); !ok {
		t.Fatal("同范围游标无法读取")
	}
	for _, other := range []store.TaskListQuery{{SubjectID: "other", NodeID: "node-a", OwnedOnly: true}, {SubjectID: "subject", NodeID: "node-b", OwnedOnly: true}, {SubjectID: "subject", NodeID: "node-a", GlobalRead: true}} {
		if _, ok := decodeTaskListCursor(value, taskListFilterDigestSubject(other)); ok {
			t.Fatal("游标扩大了账号或节点范围")
		}
	}
}
