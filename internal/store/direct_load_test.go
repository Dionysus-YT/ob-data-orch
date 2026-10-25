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

	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/precheckcontract"
)

func directLoadFixture(t *testing.T) (*Store, DirectLoadDraftWrite, directload.Config) {
	t.Helper()
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	if _, err := s.db.Exec(`UPDATE data_sources SET last_test_status='SUCCEEDED',last_test_source='AGENT_JDBC' WHERE data_source_id='source-1'`); err != nil {
		t.Fatal(err)
	}
	c := directload.Config{DataSourceID: "source-1", NodeID: "node-1", Database: "synthetic_db", Scenario: "PRIVATE_ODP", RPCPort: 2885, Table: "synthetic", FilePath: `E:\tmp\input`, Format: "CSV", Applicability: directload.Applicability{SingleTable: true, WholeTableCommit: true, NoCheckpoint: true, StructureLimits: true}}
	raw, _ := json.Marshal(c)
	w := DirectLoadDraftWrite{DirectLoadDraft: DirectLoadDraft{ID: "direct-1", OwnerSubjectID: "subject-1", DataSourceID: c.DataSourceID, NodeID: c.NodeID, ConfigJSON: string(raw), ConfigFingerprint: testFingerprint, UpdatedAt: testTime}, RequestID: "direct-create", IdempotencyKey: "direct-create-key-0001", RequestDigest: strings.Repeat("c", 64)}
	return s, w, c
}

func Test旁路草稿类型所有权幂等修订与删除(t *testing.T) {
	s, w, _ := directLoadFixture(t)
	ctx := context.Background()
	draft, err := s.SaveDirectLoadDraft(ctx, w)
	if err != nil || draft.Revision != 1 {
		t.Fatal("保存", err)
	}
	w.ID = "direct-replay"
	replay, err := s.SaveDirectLoadDraft(ctx, w)
	if err != nil || replay.ID != draft.ID || !replay.Replayed {
		t.Fatal("幂等重放", err)
	}
	w.RequestDigest = strings.Repeat("d", 64)
	if _, err = s.SaveDirectLoadDraft(ctx, w); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatal("幂等冲突未拒绝", err)
	}
	if _, err = s.GetExportDraft(ctx, draft.ID); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("导出跨类型", err)
	}
	if _, err = s.GetNormalImportDraft(ctx, draft.ID, "subject-1"); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("普通导入跨类型", err)
	}
	if _, err = s.GetDirectLoadDraft(ctx, "draft-1", "subject-1"); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("旁路读取导出", err)
	}
	if _, err = s.GetDirectLoadDraft(ctx, draft.ID, "other-subject"); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("所有权泄露", err)
	}
	w.ID = draft.ID
	w.ExpectedRevision = 1
	w.UpdatedAt = testTime.Add(time.Second)
	w.RequestID = "direct-update"
	if updated, err := s.SaveDirectLoadDraft(ctx, w); err != nil || updated.Revision != 2 {
		t.Fatal("更新", err)
	}
	if _, err = s.SaveDirectLoadDraft(ctx, w); !errors.Is(err, ErrRevisionConflict) {
		t.Fatal("旧修订未拒绝", err)
	}
	if err = s.DeleteDirectLoadDraft(ctx, draft.ID, "subject-1", 2, "direct-delete", testTime.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetDirectLoadDraft(ctx, draft.ID, "subject-1"); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("删除后可读", err)
	}
}

func Test旁路固定检查冻结提交不进入既有执行器(t *testing.T) {
	for _, kind := range []string{"DIRECT_LOAD_FILES", "DIRECT_LOAD_PREFLIGHT"} {
		t.Run(kind, func(t *testing.T) {
			s, w, c := directLoadFixture(t)
			ctx := context.Background()
			draft, err := s.SaveDirectLoadDraft(ctx, w)
			if err != nil {
				t.Fatal(err)
			}
			query := TaskDraftListQuery{SubjectID: "subject-1", Limit: 10}
			drafts, draftCount, err := s.ListOwnedTaskDrafts(ctx, query)
			if err != nil || draftCount != 2 || len(drafts) != 2 {
				t.Fatal("统一草稿缺少旁路", draftCount, err)
			}
			directFound := false
			for _, item := range drafts {
				if item.ID == draft.ID {
					directFound = item.Type == directload.TaskType && item.DisplayName == c.Database+"."+c.Table && !strings.Contains(strings.Join(item.AvailableActions, ","), "SUBMIT")
				}
			}
			if !directFound {
				t.Fatal("旁路草稿类型、名称或提交资格错误")
			}
			if legacy, total, err := s.ListOwnedExportDrafts(ctx, query); err != nil || total != 1 || len(legacy) != 1 || legacy[0].Type != "OBDUMPER_EXPORT" {
				t.Fatal("旧导出列表扩大到旁路", err)
			}
			create := PrecheckCreate{CheckKind: kind, PrecheckRun: PrecheckRun{PrecheckID: "direct-check", DraftID: draft.ID, DraftRevision: 1, ConfigFingerprint: draft.ConfigFingerprint, DataSourceID: c.DataSourceID, CredentialID: "credential-1", CredentialRevision: 1, NodeID: c.NodeID, CreatedAt: testTime, ValidUntil: testTime.Add(10 * time.Minute)}, CreatorSubjectID: "subject-1", RequestID: "direct-check-create", IdempotencyKey: "direct-check-key-0001", RequestDigest: strings.Repeat("b", 64)}
			if _, err = s.CreatePrecheck(ctx, create); err == nil {
				t.Fatal("导出入口接受旁路")
			}
			if _, err = s.CreateNormalImportPrecheck(ctx, create); err == nil {
				t.Fatal("普通导入入口接受旁路")
			}
			if _, err = s.CreateDirectLoadPrecheck(ctx, create); err != nil {
				t.Fatal("创建检查", err)
			}
			grant, err := s.ClaimPrecheck(ctx, PrecheckClaim{AgentID: "agent-1", NodeID: c.NodeID, PrecheckID: create.PrecheckID, LeaseID: "direct-lease", RequestID: "direct-claim", RequestDigest: strings.Repeat("d", 64), LeaseTTL: time.Minute, Now: testTime})
			if err != nil || grant.ExecutionContext.DirectLoad == nil || grant.ExecutionContext.NormalImport != nil || grant.ExecutionContext.OutputPath != "" || grant.ExecutionContext.CheckKind != kind {
				t.Fatal("领取隔离上下文", err)
			}
			if _, err = s.AcknowledgePrecheck(ctx, PrecheckAcknowledgement{AgentID: "agent-1", PrecheckID: create.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, BindingDigest: grant.Binding.BindingDigest, RequestID: "direct-ack", RequestDigest: strings.Repeat("e", 64), Now: testTime}); err != nil {
				t.Fatal(err)
			}
			// 本轮完整旁路租约也无真实凭据解析准入；有效身份、租约与确认不能替代独立门禁。
			{
				var secretErr error
				_ = s.withWrite(ctx, func(tx *sql.Tx) error {
					_, secretErr = s.readEncryptedPrecheckDatabaseConnection(ctx, tx, grant.Binding)
					return nil
				})
				if !errors.Is(secretErr, ErrPrecheckLeaseRejected) {
					t.Fatal("旁路用途可解析真实秘密", secretErr)
				}
				material, resolveErr := s.ResolvePrecheckDatabaseConnection(ctx, PrecheckSecretResolutionRequest{AgentID: "agent-1", PrecheckID: create.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, BindingDigest: grant.Binding.BindingDigest, RequestID: "direct-secret-denied", RequestDigest: strings.Repeat("a", 64), Now: testTime})
				if !errors.Is(resolveErr, ErrPrecheckLeaseRejected) || len(material.Ciphertext) != 0 || len(material.Nonce) != 0 || len(material.Username) != 0 {
					t.Fatal("已确认旁路租约取得秘密材料", resolveErr)
				}
				var receipts int
				if err := s.db.QueryRow(`SELECT COUNT(*) FROM agent_precheck_secret_resolution_receipts WHERE request_id='direct-secret-denied'`).Scan(&receipts); err != nil || receipts != 0 {
					t.Fatal("失败秘密请求留下成功回执", err)
				}
			}
			files := []importfiles.File{{Path: "synthetic.csv", Kind: "DATA", Object: "synthetic", Size: 2, Modified: testTime.UnixNano(), SHA256: strings.Repeat("a", 64)}}
			raw, _ := json.Marshal(files)
			hash := sha256.Sum256(raw)
			manifest := importfiles.Manifest{Files: files, FileCount: 1, TotalBytes: 2, Digest: hex.EncodeToString(hash[:])}
			completion := AgentPrecheckCompletion{AgentID: "agent-1", PrecheckID: create.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, BindingDigest: grant.Binding.BindingDigest, RequestID: "direct-complete", RequestDigest: strings.Repeat("f", 64), InputManifest: &manifest, Now: testTime.Add(time.Second)}
			if kind == "DIRECT_LOAD_PREFLIGHT" {
				completion.DirectLoadConfigFingerprint = strings.Repeat("9", 64)
			}
			for _, check := range precheckcontract.DirectLoadChecks(kind) {
				completion.Results = append(completion.Results, PrecheckCheckResult{Check: check, Status: "PASSED", EvidenceCode: "DIRECT_LOAD_VERIFIED"})
			}
			bad := completion
			bad.Results = append([]PrecheckCheckResult(nil), completion.Results...)
			bad.Results[0].EvidenceCode = "IMPORT_VERIFIED"
			if _, err = s.CompleteAgentPrecheck(ctx, bad); err == nil {
				t.Fatal("普通导入证据冒充旁路")
			}
			bad = completion
			bad.InputManifest = nil
			if _, err = s.CompleteAgentPrecheck(ctx, bad); err == nil {
				t.Fatal("未核对清单通过")
			}
			if _, err = s.CompleteAgentPrecheck(ctx, completion); err != nil {
				t.Fatal("完成旁路检查", err)
			}
			check, err := s.GetDirectLoadCheck(ctx, create.PrecheckID, "subject-1")
			if err != nil || check.Run.Status != "SUCCEEDED" || check.InputManifest == nil {
				t.Fatal("读取冻结证据", err)
			}
			if _, err = s.GetNormalImportCheck(ctx, create.PrecheckID, "subject-1"); !errors.Is(err, ErrDataSourceNotFound) {
				t.Fatal("普通检查读取旁路", err)
			}
			if kind == "DIRECT_LOAD_PREFLIGHT" {
				snapshot := DirectLoadSnapshot{ConfigVersion: directload.ConfigVersion, TaskType: directload.TaskType, TaskName: "合成旁路任务", Config: c, InputManifest: manifest, DirectLoadConfigFingerprint: completion.DirectLoadConfigFingerprint, PrecheckProof: DirectLoadPrecheckProof{ID: check.Run.PrecheckID, DraftRevision: 1, ConfigFingerprint: draft.ConfigFingerprint, Results: completion.Results}}
				encoded, _ := json.Marshal(snapshot)
				argv, _ := json.Marshal([]string{"--host", "127.0.0.1", "--port", "2883", "--user", "synthetic_user", "--tenant", "synthetic_tenant", "--cluster", "synthetic_cluster", "--database", c.Database, "--file-path", c.FilePath, "--csv", "--direct", "--rpc-port", "2885", "--table", c.Table})
				submission := validTaskSubmission("direct-task")
				submission.TaskType = directload.TaskType
				submission.PrecheckID = create.PrecheckID
				submission.MetadataVersion = directload.MetadataVersion
				submission.CapabilityVersion = directload.CapabilityVersion
				submission.SnapshotVersion = directload.ConfigVersion
				submission.SnapshotJSON = string(encoded)
				submission.PlannedArgvJSON = string(argv)
				submission.PlannedCommandRedacted = "obloader --direct --csv -p ******"
				if err = s.SubmitTask(ctx, submission); err != nil {
					t.Fatal("G2冻结旁路任务", err)
				}
				if drafts, total, err := s.ListOwnedTaskDrafts(ctx, query); err != nil || total != 1 || len(drafts) != 1 || drafts[0].Type != "OBDUMPER_EXPORT" {
					t.Fatal("提交后旁路仍在未提交草稿列表", err)
				}
				summary, err := s.GetTaskSummary(ctx, submission.TaskID)
				if err != nil || summary.TaskType != directload.TaskType || summary.Table != c.Table || summary.ResultSummary != nil || summary.OutputLocation != nil {
					t.Fatal("旁路任务投影错误", err)
				}
				items, listErr := s.ListTaskSummaries(ctx, TaskListQuery{SubjectID: "subject-1", Limit: 10, Types: []string{directload.TaskType}, Keyword: c.Table})
				if listErr != nil || len(items) != 1 || items[0].TaskType != directload.TaskType || items[0].Table != c.Table || items[0].Database != c.Database || items[0].CheckpointPresent {
					t.Fatal("旁路列表类型、表或恢复资格错误", listErr)
				}
				for _, owner := range []bool{true, false} {
					subject := "subject-1"
					if !owner {
						subject = "subject-other"
						if err := s.EnsureAuthSubject(ctx, AuthSubject{SubjectID: subject, ExternalSubject: "direct-snapshot-other", DisplayName: "Other", AccountStatus: "ACTIVE", CreatedAt: testTime, UpdatedAt: testTime}); err != nil {
							t.Fatal(err)
						}
					}
					groups, groupErr := s.GetAuthorizedTaskSnapshotGroups(ctx, submission.TaskID, subject, !owner)
					if groupErr != nil || len(groups) != 6 {
						t.Fatal("旁路冻结配置分组错误", groupErr)
					}
					for i, key := range []string{"applicability", "connection", "input", "target", "execution", "review"} {
						if groups[i].Key != key {
							t.Fatal("旁路被其他向导解释", groups[i].Key)
						}
					}
					encodedGroups, _ := json.Marshal(groups)
					encodedPath, _ := json.Marshal(c.FilePath)
					if strings.Contains(string(encodedGroups), string(encodedPath)) != owner || strings.Contains(string(encodedGroups), "credential-1") {
						t.Fatal("冻结配置路径或秘密引用裁剪失效")
					}
				}
				claim := validClaim("direct-execution", "direct-execution-lease", "direct-event", "direct-execution-request")
				claim.TaskID = submission.TaskID
				if err = s.ClaimTask(ctx, claim); !errors.Is(err, ErrClaimIneligible) {
					t.Fatal("旁路进入导出执行器", err)
				}
				// 即使合成旧证据声称存在检查点，也不能把旁路任务派生成追加 --retry 的导出任务。
				if _, err := s.db.Exec(`INSERT INTO task_executions(execution_id,task_id,node_id,agent_id,state,revision,result_summary_json,created_at,updated_at) VALUES('direct-recovery','direct-task','node-1','agent-1','FAILED',1,'{"result":"FAILED","checkpointPresent":true}',?,?)`, utcText(testTime), utcText(testTime)); err != nil {
					t.Fatal(err)
				}
				if _, err := s.DeriveTaskFromCheckpoint(ctx, CheckpointResumeDerivation{TaskID: "direct-resume", SourceTaskID: submission.TaskID, CreatorSubjectID: "subject-1", AuditActorID: "subject-1", RequestID: "direct-resume-denied", IdempotencyKey: "direct-resume-denied-key", RequestDigest: strings.Repeat("a", 64), Now: testTime.Add(time.Minute)}); !errors.Is(err, ErrCheckpointResumeUnavailable) {
					t.Fatal("旁路复用了导出检查点恢复", err)
				}
				assertCount(t, s.db, `SELECT COUNT(*) FROM tasks WHERE task_id='direct-resume'`, 0)
			}
			w.ExpectedRevision = 1
			w.UpdatedAt = testTime.Add(2 * time.Second)
			w.RequestID = "direct-update-after-check"
			_, err = s.SaveDirectLoadDraft(ctx, w)
			if kind == "DIRECT_LOAD_PREFLIGHT" {
				if !errors.Is(err, ErrRevisionConflict) {
					t.Fatal("已提交草稿仍可编辑", err)
				}
				if err := s.DeleteDirectLoadDraft(ctx, draft.ID, "subject-1", 1, "direct-delete-submitted", testTime.Add(time.Minute)); !errors.Is(err, ErrRevisionConflict) {
					t.Fatal("已提交草稿仍可删除", err)
				}
			} else if err != nil {
				t.Fatal("未提交草稿无法修改", err)
			}
			check, err = s.GetDirectLoadCheck(ctx, create.PrecheckID, "subject-1")
			wantStatus := "INVALIDATED"
			if kind == "DIRECT_LOAD_PREFLIGHT" {
				wantStatus = "SUCCEEDED"
			}
			if err != nil || check.Run.Status != wantStatus {
				t.Fatal("历史或活动检查失效边界错误", err)
			}
		})
	}
}
