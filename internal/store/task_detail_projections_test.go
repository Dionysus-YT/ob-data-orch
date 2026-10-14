package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func Test任务操作记录授权分页与安全投影(t *testing.T) {
	s := seedTaskListFixture(t)
	ctx := context.Background()
	for index := 0; index < 55; index++ {
		if err := s.withWrite(ctx, func(tx *sql.Tx) error {
			return insertAudit(ctx, tx, "SUBJECT", "subject-1", "TASK_DRAFT_DERIVED", "TASK", "task-alpha", "SUCCEEDED", fmt.Sprintf("safe-operation-%d", index), testTime.Add(time.Duration(index)*time.Second))
		}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.ListTaskOperations(ctx, TaskListQuery{TaskID: "task-alpha", SubjectID: "subject-1"}, time.Time{}, "")
	if err != nil || len(first) != 51 {
		t.Fatalf("首页数量=%d err=%v", len(first), err)
	}
	last := first[49]
	second, err := s.ListTaskOperations(ctx, TaskListQuery{TaskID: "task-alpha", SubjectID: "subject-1"}, last.OccurredAt, last.ID)
	if err != nil || len(second) != 6 {
		t.Fatalf("下一页数量=%d err=%v", len(second), err)
	}
	seen := map[string]bool{}
	for _, item := range append(first[:50], second...) {
		if seen[item.ID] {
			t.Fatal("分页重复")
		}
		seen[item.ID] = true
		if item.ActorName != "Synthetic User" {
			t.Fatal("操作人展示错误")
		}
	}
	foreign, err := s.ListTaskOperations(ctx, TaskListQuery{TaskID: "task-gamma", SubjectID: "subject-1"}, time.Time{}, "")
	if err != nil || len(foreign) != 0 {
		t.Fatal("操作记录扩大读取范围", err)
	}
	if _, err := s.ListTaskOperations(ctx, TaskListQuery{SubjectID: "subject-1"}, time.Time{}, ""); err == nil {
		t.Fatal("缺少精确任务范围应拒绝")
	}
	raw, _ := json.Marshal(first)
	for _, key := range []string{"diff_json", "payload_json", "command", "credential", "config"} {
		if strings.Contains(string(raw), key) {
			t.Fatal("操作记录泄露原始字段", key)
		}
	}
}

func Test配置快照五步白名单与创建者路径裁剪(t *testing.T) {
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	ctx := context.Background()
	cfg := ExportConfig{ObjectScope: ObjectScope{Database: "synthetic_db", ScopeKind: "SPECIFIED", Expressions: []ObjectExpression{{Name: "orders"}}}, DataFormat: DataFormat{FormatKind: "CSV"}, OutputConfig: OutputConfig{OutputKind: "S3", FilePath: "s3://synthetic-output/only-owner", LogPath: "/synthetic/logs", StorageCredential: &StorageCredentialBinding{StorageCredentialID: "synthetic-private-reference", Revision: 1}}}
	scn := int64(9007199254740993)
	cfg.FilterConfig.FlashbackScn = &scn
	raw, _ := json.Marshal(struct {
		Config  ExportConfig `json:"config"`
		Unknown string       `json:"unrecognized"`
	}{cfg, "synthetic-hidden"})
	submission := validTaskSubmission("safe-snapshot")
	submission.SnapshotJSON = string(raw)
	if err := s.SubmitTask(ctx, submission); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		subject string
		global  bool
		path    bool
	}{{"subject-1", false, true}, {"subject-other", true, false}} {
		if tc.subject == "subject-other" {
			if err := s.EnsureAuthSubject(ctx, AuthSubject{SubjectID: tc.subject, ExternalSubject: "snapshot-other", DisplayName: "Other", AccountStatus: "ACTIVE", CreatedAt: testTime, UpdatedAt: testTime}); err != nil {
				t.Fatal(err)
			}
		}
		groups, err := s.GetAuthorizedTaskSnapshotGroups(ctx, "safe-snapshot", tc.subject, tc.global)
		if err != nil || len(groups) != 5 {
			t.Fatal("快照分组不可用", err)
		}
		raw, _ := json.Marshal(groups)
		text := string(raw)
		if !strings.Contains(text, "9007199254740993") {
			t.Fatal("冻结大整数展示精度丢失")
		}
		if strings.Contains(text, "synthetic-private-reference") || strings.Contains(text, "synthetic-hidden") {
			t.Fatal("快照泄露白名单外字段")
		}
		if strings.Contains(text, "only-owner") != tc.path {
			t.Fatal("路径未按创建者裁剪")
		}
		if !strings.Contains(text, "已脱敏") {
			t.Fatal("秘密引用未标记脱敏")
		}
		for _, group := range groups {
			for _, field := range group.Fields {
				if field.Source != "UNKNOWN" {
					t.Fatal("伪造历史来源")
				}
			}
		}
	}
	if _, err := s.GetAuthorizedTaskSnapshotGroups(ctx, "safe-snapshot", "subject-other", false); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("无权对象可被读取", err)
	}
}
