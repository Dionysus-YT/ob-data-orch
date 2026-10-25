package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func Test编辑导出草稿空节点持久化修订审计和执行门禁(t *testing.T) {
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	ctx := context.Background()
	config := `{"configVersion":"v6","draftState":"EDITING","config":{"objectScope":{"scopeKind":"SPECIFIED"},"contentSelection":{"contentKind":"DATA_ONLY"},"outputConfig":{"outputKind":"LOCAL","filePath":""}}}`
	input := ExportDraftCreate{ExportDraft: ExportDraft{DraftID: "editing-draft", OwnerSubjectID: "subject-1", DataSourceID: "source-1", ToolVersion: "4.3.5", MetadataVersion: "obdumper-4.3.5-slice-v11-expert-parameters", CapabilityVersion: ExportEditingCapabilityVersion, ConfigVersion: "v6", ConfigJSON: config, ConfigFingerprint: testFingerprint, InvalidationJSON: `{}`, CreatedAt: testTime, UpdatedAt: testTime}, RequestID: "editing-create", IdempotencyKey: "synthetic-editing-create", RequestDigest: testFingerprint}
	if _, err := s.CreateExportDraft(ctx, input); err != nil {
		t.Fatal(err)
	}
	replayed, err := s.CreateExportDraft(ctx, input)
	if err != nil || !replayed.Replayed {
		t.Fatal("创建重放未幂等", err)
	}
	draft, err := s.GetExportDraft(ctx, input.DraftID)
	if err != nil || draft.NodeID != "" || draft.ConfigJSON != config {
		t.Fatal("未完成草稿未完整往返", err)
	}
	items, _, err := s.ListOwnedExportDrafts(ctx, ExportDraftListQuery{SubjectID: "subject-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.ID == input.DraftID {
			found = true
			if slices.Contains(item.AvailableActions, "SUBMIT") {
				t.Fatal("编辑草稿提供了提交动作")
			}
		}
	}
	if !found {
		t.Fatal("列表缺少未完成草稿")
	}
	update := DraftUpdate{DraftID: input.DraftID, ExpectedRevision: 1, ConfigVersion: "v6", ConfigJSON: config, ConfigFingerprint: testFingerprint, InvalidationJSON: `{}`, UpdatedAt: testTime, MetadataVersion: input.MetadataVersion, CapabilityVersion: ExportEditingCapabilityVersion, RebindEditing: true, DataSourceID: "source-1", NodeID: "node-1", ActorSubjectID: "subject-1", RequestID: "editing-update"}
	if revision, err := s.UpdateDraft(ctx, update); err != nil || revision != 2 {
		t.Fatal("补选节点失败", err)
	}
	if _, err := s.UpdateDraft(ctx, update); !errors.Is(err, ErrRevisionConflict) {
		t.Fatal("旧修订仍可写入", err)
	}
	assertCount(t, s.db, `SELECT COUNT(*) FROM audit_events WHERE action='EXPORT_DRAFT_SAVED'`, 1)
	check := PrecheckCreate{PrecheckRun: PrecheckRun{PrecheckID: "editing-check", DraftID: input.DraftID, DraftRevision: 2, ConfigFingerprint: testFingerprint, DataSourceID: "source-1", CredentialID: "credential-1", CredentialRevision: 1, NodeID: "node-1", CreatedAt: testTime, ValidUntil: testTime.Add(time.Minute)}, CreatorSubjectID: "subject-1", RequestID: "editing-check", IdempotencyKey: "synthetic-editing-check", RequestDigest: testFingerprint}
	if _, err := s.CreatePrecheck(ctx, check); !errors.Is(err, ErrPrecheckInvalid) {
		t.Fatal("编辑草稿创建了预检查", err)
	}
	update.ExpectedRevision = 2
	update.RequestID = "editing-promote"
	update.ConfigJSON, update.CapabilityVersion = `{"configVersion":"v6","config":{}}`, "export-odp-single-table-csv-v1"
	if _, err := s.UpdateDraft(ctx, update); err != nil {
		t.Fatal("完整配置无法提升草稿", err)
	}
	update.ExpectedRevision = 3
	if _, err := s.UpdateDraft(ctx, update); !errors.Is(err, ErrRevisionConflict) {
		t.Fatal("执行草稿被重新绑定", err)
	}
}

func Test编辑标记和执行能力不可混用(t *testing.T) {
	for _, item := range []struct{ document, capability string }{
		{`{"draftState":"EDITING","config":{}}`, "export-odp-single-table-csv-v1"},
		{`{"config":{}}`, ExportEditingCapabilityVersion},
		{`{"draftState":"EDITING"}`, ExportEditingCapabilityVersion},
		{`{"draftState":"UNKNOWN","config":{}}`, ExportEditingCapabilityVersion},
	} {
		if validEditingExportMarker(item.document, item.capability) {
			t.Fatal("不匹配标记被接纳")
		}
	}
	input := validTaskSubmission("editing-task")
	input.CapabilityVersion = ExportEditingCapabilityVersion
	if validateTaskSubmission(input) == nil {
		t.Fatal("编辑摘要可用于执行提交")
	}
}
