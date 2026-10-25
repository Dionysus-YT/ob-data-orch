package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
)

// reuseSourceFixture 仅构造冻结配置与终态仓储事实，不启动 Agent 或证明真实执行。
func reuseSourceFixture(t *testing.T, state string) (*Store, TaskDerivationSource) {
	t.Helper()
	s, _, c := normalImportFixture(t)
	files := []importfiles.File{{Path: "synthetic.csv", Kind: "DATA", Object: "synthetic", Size: 2, Modified: 1, SHA256: strings.Repeat("a", 64)}}
	raw, _ := json.Marshal(files)
	digest := sha256.Sum256(raw)
	manifest := importfiles.Manifest{Files: files, FileCount: 1, TotalBytes: 2, Digest: hex.EncodeToString(digest[:])}
	snapshot, _ := json.Marshal(NormalImportSnapshot{ConfigVersion: importdomain.ConfigVersion, TaskType: importdomain.TaskType, Config: c, InputManifest: manifest})
	_, err := s.db.Exec(`INSERT INTO tasks(task_id,creator_subject_id,data_source_id,node_id,precheck_id,credential_id,credential_revision,config_fingerprint,tool_version,metadata_version,capability_version,snapshot_version,snapshot_json,planned_argv_json,planned_command_redacted,submitted_at,task_type) VALUES('reuse-task','subject-1','source-1','node-1','precheck-1','credential-1',1,?,?,?,?,?,?,'[]','obloader -p ******',?,'OBLOADER_NORMAL_IMPORT')`, testFingerprint, importdomain.ToolVersion, importdomain.MetadataVersion, importdomain.CapabilityVersion, importdomain.ConfigVersion, string(snapshot), utcText(testTime))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO task_executions(execution_id,task_id,node_id,agent_id,state,revision,created_at,updated_at) VALUES('reuse-execution','reuse-task','node-1','agent-1',?,1,?,?)`, state, utcText(testTime), utcText(testTime)); err != nil {
		t.Fatal(err)
	}
	source, err := s.GetAuthorizedTaskDerivationSource(context.Background(), "reuse-task", "subject-1")
	if err != nil {
		t.Fatal(err)
	}
	return s, source
}

func Test统一草稿列表按类型分页且不泄露所有权(t *testing.T) {
	s, w, _ := normalImportFixture(t)
	w.UpdatedAt = testTime.Add(time.Second)
	if _, err := s.SaveNormalImportDraft(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	query := TaskDraftListQuery{SubjectID: "subject-1", Limit: 1}
	items, total, err := s.ListOwnedTaskDrafts(context.Background(), query)
	if err != nil || total != 2 || len(items) != 2 || items[0].Type != importdomain.TaskType || strings.Contains(strings.Join(items[0].AvailableActions, ","), "SUBMIT") {
		t.Fatal("合并分页或类型资格错误", items, total, err)
	}
	query.BeforeUpdated, query.BeforeID = items[0].UpdatedAt, items[0].ID
	items, total, err = s.ListOwnedTaskDrafts(context.Background(), query)
	if err != nil || total != 2 || len(items) != 1 || items[0].Type != "OBDUMPER_EXPORT" {
		t.Fatal("游标遗漏或重复", items, total, err)
	}
	query.SubjectID = "foreign"
	if items, total, err = s.ListOwnedTaskDrafts(context.Background(), query); err != nil || total != 0 || len(items) != 0 {
		t.Fatal("泄露其他主体草稿", err)
	}
	legacy, total, err := s.ListOwnedExportDrafts(context.Background(), ExportDraftListQuery{SubjectID: "subject-1", Limit: 10})
	if err != nil || total != 1 || len(legacy) != 1 || legacy[0].Type != "OBDUMPER_EXPORT" {
		t.Fatal("旧导出接口扩大类型", err)
	}
}

func Test普通导入派生原子来源幂等与重跑锁定(t *testing.T) {
	s, source := reuseSourceFixture(t, "FAILED")
	ctx := context.Background()
	config, err := NormalImportReusableConfig(source)
	if err != nil || config.TaskName != "" {
		t.Fatal("复用配置包含任务名称", err)
	}
	raw, _ := json.Marshal(config)
	w := NormalImportDraftWrite{NormalImportDraft: NormalImportDraft{ID: "derived-normal", OwnerSubjectID: "subject-1", DataSourceID: config.DataSourceID, NodeID: config.NodeID, ConfigJSON: string(raw), ConfigFingerprint: testFingerprint, SourceTaskID: source.TaskID, SourceDerivation: "RERUN_FROM_SCRATCH", UpdatedAt: testTime.Add(time.Minute)}, RequestID: "derive-normal", IdempotencyKey: "synthetic-derive-normal-key", RequestDigest: strings.Repeat("d", 64)}
	item, err := s.SaveNormalImportDraft(ctx, w)
	if err != nil || item.SourceTaskID != source.TaskID || item.SourceDerivation != "RERUN_FROM_SCRATCH" {
		t.Fatal("派生来源未冻结", err)
	}
	w.ID = "replayed-normal"
	if replay, err := s.SaveNormalImportDraft(ctx, w); err != nil || replay.ID != item.ID || !replay.Replayed {
		t.Fatal("重复派生未重放", err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM audit_events WHERE action='TASK_DRAFT_DERIVED'`, 1)
	w.ID, w.ExpectedRevision = item.ID, item.Revision
	config.Execution.Thread = "2"
	raw, _ = json.Marshal(config)
	w.ConfigJSON = string(raw)
	if _, err := s.SaveNormalImportDraft(ctx, w); !errors.Is(err, ErrPrecheckInvalid) {
		t.Fatal("从头重跑允许更改参数", err)
	}
	if _, err := s.GetNormalImportDraft(ctx, item.ID, "foreign"); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("派生草稿跨主体可见", err)
	}
	if _, err := s.db.Exec(`UPDATE task_executions SET reconciliation_required=1 WHERE execution_id='reuse-execution'`); err != nil {
		t.Fatal(err)
	}
	w.ExpectedRevision = 0
	w.ID = "untrusted"
	w.IdempotencyKey = "synthetic-untrusted-key"
	config.Execution.Thread = ""
	raw, _ = json.Marshal(config)
	w.ConfigJSON = string(raw)
	if _, err := s.SaveNormalImportDraft(ctx, w); !errors.Is(err, ErrPrecheckInvalid) {
		t.Fatal("核对中来源仍可派生", err)
	}
}

func Test普通导入模板来源冻结类型隔离与安全投影(t *testing.T) {
	s, source := reuseSourceFixture(t, "SUCCEEDED")
	ctx := context.Background()
	raw, err := NormalImportTemplateConfig(source)
	if err != nil {
		t.Fatal(err)
	}
	config, err := importdomain.Decode(raw)
	if err != nil || config.DataSourceID != "" || config.NodeID != "" || config.TaskName != "" || strings.Contains(raw, "inputManifest") {
		t.Fatal("模板复制绑定或运行证据", err)
	}
	digest := sha256.Sum256([]byte(raw))
	input := ExportConfigTemplateCreate{ExportConfigTemplate: ExportConfigTemplate{TaskType: importdomain.TaskType, TemplateID: "normal-template", OwnerSubjectID: "subject-1", DisplayName: "合成普通导入模板", CapabilityVersion: importdomain.CapabilityVersion, ConfigJSON: raw, ConfigFingerprint: hex.EncodeToString(digest[:]), SourceTaskID: source.TaskID, CreatedAt: testTime, UpdatedAt: testTime}, RequestID: "save-normal-template", IdempotencyKey: "synthetic-template-key", RequestDigest: strings.Repeat("a", 64)}
	if _, err := s.CreateExportConfigTemplate(ctx, input); err != nil {
		t.Fatal(err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM audit_events WHERE action='NORMAL_IMPORT_CONFIG_TEMPLATE_CREATED'`, 1)
	all, err := s.ListConfigTemplates(ctx, "subject-1")
	if err != nil || len(all) != 1 || all[0].TaskType != importdomain.TaskType || all[0].ConfigJSON != "" {
		t.Fatal("模板摘要泄露配置或丢失类型", err)
	}
	legacy, err := s.ListExportConfigTemplates(ctx, "subject-1")
	if err != nil || len(legacy) != 0 {
		t.Fatal("普通导入出现在导出目录", err)
	}
	if _, err := s.db.Exec(`UPDATE export_config_templates SET task_type='OBDUMPER_EXPORT' WHERE template_id='normal-template'`); err == nil {
		t.Fatal("持久模板类型可被修改")
	}
	if _, err := s.GetAuthorizedExportConfigTemplate(ctx, input.TemplateID, "foreign"); !errors.Is(err, ErrDataSourceNotFound) {
		t.Fatal("模板越权读取", err)
	}
	input.TemplateID = "tampered"
	input.IdempotencyKey = "synthetic-tampered-template"
	input.ConfigJSON = `{"direct":true}`
	if _, err := s.CreateExportConfigTemplate(ctx, input); !errors.Is(err, ErrPrecheckInvalid) {
		t.Fatal("模板接受快照外配置", err)
	}
	revision, err := s.UpdateExportConfigTemplate(ctx, ExportConfigTemplateUpdate{TemplateID: "normal-template", ActorSubjectID: "subject-1", DisplayName: "合成普通导入模板改名", ExpectedRevision: 1, RequestID: "rename-normal-template", UpdatedAt: testTime.Add(time.Minute)})
	if err != nil || revision != 2 {
		t.Fatal("普通导入模板改名失败", err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM audit_events WHERE action='NORMAL_IMPORT_CONFIG_TEMPLATE_RENAMED'`, 1)
	if err := s.DeleteExportConfigTemplate(ctx, "normal-template", "subject-1", revision, "delete-normal-template", testTime.Add(2*time.Minute)); err != nil {
		t.Fatal("普通导入模板删除失败", err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM audit_events WHERE action='NORMAL_IMPORT_CONFIG_TEMPLATE_DELETED'`, 1)
}
