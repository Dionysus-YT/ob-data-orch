package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/precheckcontract"
)

func normalImportFixture(t *testing.T) (*Store, NormalImportDraftWrite, importdomain.Config) {
	t.Helper()
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	if _, err := s.db.Exec(`UPDATE data_sources SET last_test_status='SUCCEEDED',last_test_source='AGENT_JDBC' WHERE data_source_id='source-1'`); err != nil {
		t.Fatal(err)
	}
	c := importdomain.Config{DataSourceID: "source-1", NodeID: "node-1", Database: "synthetic_db", FilePath: `E:\tmp\input`, Content: "DATA_ONLY", Format: "CSV", Scope: "ALL", Cleanup: "NONE", TaskName: "合成普通导入"}
	raw, _ := json.Marshal(c)
	w := NormalImportDraftWrite{NormalImportDraft: NormalImportDraft{ID: "normal-1", OwnerSubjectID: "subject-1", DataSourceID: c.DataSourceID, NodeID: c.NodeID, ConfigJSON: string(raw), ConfigFingerprint: testFingerprint, UpdatedAt: testTime}, RequestID: "normal-create", IdempotencyKey: "normal-create-key-0001", RequestDigest: strings.Repeat("c", 64)}
	return s, w, c
}

func Test普通导入草稿隔离幂等修订与删除(t *testing.T) {
	s, w, _ := normalImportFixture(t)
	ctx := context.Background()
	draft, err := s.SaveNormalImportDraft(ctx, w)
	if err != nil || draft.Revision != 1 {
		t.Fatalf("保存草稿: %v", err)
	}
	w.ID = "normal-replayed"
	replay, err := s.SaveNormalImportDraft(ctx, w)
	if err != nil || replay.ID != draft.ID {
		t.Fatal("创建幂等失效", err)
	}
	w.RequestDigest = strings.Repeat("d", 64)
	if _, err = s.SaveNormalImportDraft(ctx, w); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatal("冲突幂等未拒绝", err)
	}
	if _, err = s.GetExportDraft(ctx, draft.ID); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("导出读取跨类型", err)
	}
	if _, err = s.GetNormalImportDraft(ctx, "draft-1", "subject-1"); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("导入读取跨类型", err)
	}
	if _, err = s.GetNormalImportDraft(ctx, draft.ID, "other-subject"); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("跨主体读取", err)
	}
	if _, err = s.DeleteOwnedExportDraft(ctx, draft.ID, "subject-1", 1, "delete-export-key-normal", "request-delete", testTime); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("导出删除跨类型", err)
	}
	w.ID = draft.ID
	w.ExpectedRevision = 1
	w.UpdatedAt = testTime.Add(time.Second)
	w.RequestID = "normal-update"
	updated, err := s.SaveNormalImportDraft(ctx, w)
	if err != nil || updated.Revision != 2 {
		t.Fatal("更新草稿", err)
	}
	if _, err = s.SaveNormalImportDraft(ctx, w); !errors.Is(err, ErrRevisionConflict) {
		t.Fatal("旧修订未拒绝", err)
	}
	if err = s.DeleteNormalImportDraft(ctx, draft.ID, "subject-1", 2, "normal-delete", testTime.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetNormalImportDraft(ctx, draft.ID, "subject-1"); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("删除后仍可读", err)
	}
}

func Test普通导入检查清单冻结证据与失效(t *testing.T) {
	for _, kind := range []string{"NORMAL_IMPORT_FILES", "NORMAL_IMPORT_PREFLIGHT"} {
		t.Run(kind, func(t *testing.T) {
			s, w, c := normalImportFixture(t)
			ctx := context.Background()
			draft, err := s.SaveNormalImportDraft(ctx, w)
			if err != nil {
				t.Fatal(err)
			}
			create := PrecheckCreate{CheckKind: kind, PrecheckRun: PrecheckRun{PrecheckID: "normal-check", DraftID: draft.ID, DraftRevision: 1, ConfigFingerprint: draft.ConfigFingerprint, DataSourceID: c.DataSourceID, CredentialID: "credential-1", CredentialRevision: 1, NodeID: c.NodeID, CreatedAt: testTime, ValidUntil: testTime.Add(10 * time.Minute)}, CreatorSubjectID: "subject-1", RequestID: "normal-check-create", IdempotencyKey: "normal-check-key-0001", RequestDigest: strings.Repeat("b", 64)}
			if _, err = s.CreatePrecheck(ctx, create); err == nil {
				t.Fatal("导出入口接受普通导入检查")
			}
			if _, err = s.CreateNormalImportPrecheck(ctx, create); err != nil {
				t.Fatal("创建检查", err)
			}
			grant, err := s.ClaimPrecheck(ctx, PrecheckClaim{AgentID: "agent-1", NodeID: c.NodeID, PrecheckID: create.PrecheckID, LeaseID: "normal-lease", RequestID: "normal-claim", RequestDigest: strings.Repeat("d", 64), LeaseTTL: time.Minute, Now: testTime})
			if err != nil || grant.ExecutionContext.NormalImport == nil || grant.ExecutionContext.CheckKind != kind || grant.ExecutionContext.OutputPath != "" {
				t.Fatal("领取检查上下文", err)
			}
			if _, err = s.AcknowledgePrecheck(ctx, PrecheckAcknowledgement{AgentID: "agent-1", PrecheckID: create.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, BindingDigest: grant.Binding.BindingDigest, RequestID: "normal-ack", RequestDigest: strings.Repeat("e", 64), Now: testTime}); err != nil {
				t.Fatal(err)
			}
			if kind == "NORMAL_IMPORT_FILES" {
				var secretErr error
				_ = s.withWrite(ctx, func(tx *sql.Tx) error {
					_, secretErr = readEncryptedPrecheckDatabaseConnection(ctx, tx, grant.Binding)
					return nil
				})
				if !errors.Is(secretErr, ErrPrecheckLeaseRejected) {
					t.Fatal("文件发现可解析数据库秘密", secretErr)
				}
			}
			files := []importfiles.File{{Path: "synthetic.csv", Kind: "DATA", Object: "synthetic", Size: 2, Modified: testTime.UnixNano(), SHA256: strings.Repeat("a", 64)}}
			raw, _ := json.Marshal(files)
			hash := sha256.Sum256(raw)
			manifest := importfiles.Manifest{Files: files, FileCount: 1, TotalBytes: 2, Digest: hex.EncodeToString(hash[:])}
			completion := AgentPrecheckCompletion{AgentID: "agent-1", PrecheckID: create.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, BindingDigest: grant.Binding.BindingDigest, RequestID: "normal-complete", RequestDigest: strings.Repeat("f", 64), InputManifest: &manifest, Now: testTime.Add(time.Second)}
			for _, check := range precheckcontract.NormalImportChecks(kind) {
				completion.Results = append(completion.Results, PrecheckCheckResult{Check: check, Status: "PASSED", EvidenceCode: "SYNTHETIC_OK"})
			}
			bad := completion
			bad.InputManifest = nil
			if _, err = s.CompleteAgentPrecheck(ctx, bad); err == nil {
				t.Fatal("无输入证据接受通过报告")
			}
			if _, err = s.CompleteAgentPrecheck(ctx, completion); err != nil {
				t.Fatal("完成检查", err)
			}
			check, err := s.GetNormalImportCheck(ctx, create.PrecheckID, "subject-1")
			if err != nil || check.Run.Status != "SUCCEEDED" || check.InputManifest == nil {
				t.Fatal("读取冻结证据", err)
			}
			if _, err = s.GetNormalImportCheck(ctx, create.PrecheckID, "other-subject"); !errors.Is(err, ErrDataSourceNotFound) {
				t.Fatal("泄露检查", err)
			}
			snapshot, _ := json.Marshal(NormalImportSnapshot{ConfigVersion: importdomain.ConfigVersion, TaskType: importdomain.TaskType, Config: c, InputManifest: manifest})
			argv, _ := json.Marshal([]string{"--host", "127.0.0.1", "--port", "2883", "--user", "synthetic_user", "--database", c.Database, "--no-sys", "--file-path", c.FilePath, "--csv", "--all"})
			submission := validTaskSubmission("normal-task")
			submission.TaskType = importdomain.TaskType
			submission.PrecheckID = create.PrecheckID
			submission.MetadataVersion = importdomain.MetadataVersion
			submission.CapabilityVersion = importdomain.CapabilityVersion
			submission.SnapshotVersion = importdomain.ConfigVersion
			submission.SnapshotJSON = string(snapshot)
			submission.PlannedArgvJSON = string(argv)
			submission.PlannedCommandRedacted = "obloader --csv --all -p ******"
			if kind == "NORMAL_IMPORT_FILES" {
				if err = s.SubmitTask(ctx, submission); !errors.Is(err, ErrPrecheckInvalid) {
					t.Fatal("文件发现可用于提交", err)
				}
			} else {
				if err = s.SubmitTask(ctx, submission); err != nil {
					t.Fatal("冻结普通导入任务", err)
				}
				summary, readErr := s.GetTaskSummary(ctx, submission.TaskID)
				if readErr != nil || summary.TaskType != importdomain.TaskType || summary.Database != c.Database || summary.ResultSummary != nil {
					t.Fatal("导入冻结投影错误", readErr)
				}
				items, listErr := s.ListTaskSummaries(ctx, TaskListQuery{SubjectID: "subject-1", Limit: 10, Types: []string{importdomain.TaskType}})
				if listErr != nil || len(items) != 1 || items[0].TaskType != importdomain.TaskType {
					t.Fatal("导入列表类型或筛选错误", listErr)
				}
				groups, groupErr := s.GetAuthorizedTaskSnapshotGroups(ctx, submission.TaskID, "subject-1", false)
				if groupErr != nil || len(groups) != 5 || groups[1].Key != "input" || groups[3].Key != "execution" {
					t.Fatal("导入快照被导出规则解释", groupErr)
				}
				claim := validClaim("normal-execution", "normal-execution-lease", "normal-event", "normal-execution-request")
				claim.TaskID = submission.TaskID
				if err = s.ClaimTask(ctx, claim); !errors.Is(err, ErrClaimIneligible) {
					t.Fatal("普通导入进入导出执行器", err)
				}
			}
			w.ExpectedRevision = 1
			w.UpdatedAt = testTime.Add(2 * time.Second)
			w.RequestID = "normal-update-after-check"
			if _, err = s.SaveNormalImportDraft(ctx, w); err != nil {
				t.Fatal(err)
			}
			check, err = s.GetNormalImportCheck(ctx, create.PrecheckID, "subject-1")
			if err != nil || check.Run.Status != "INVALIDATED" {
				t.Fatal("保存未撤销检查", err)
			}
		})
	}
}
