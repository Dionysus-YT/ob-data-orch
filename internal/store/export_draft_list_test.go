package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func Test草稿列表只显示自己的未提交对象且关键字按字面匹配(t *testing.T) {
	s, _ := openTestStore(t)
	seedBaseFixture(t, s)
	ctx := context.Background()
	items, total, err := s.ListOwnedExportDrafts(ctx, ExportDraftListQuery{SubjectID: "subject-1", Limit: 10})
	if err != nil || total != 1 || len(items) != 1 || items[0].Revision != 1 {
		t.Fatal("草稿列表错误", total, items, err)
	}
	items, total, err = s.ListOwnedExportDrafts(ctx, ExportDraftListQuery{SubjectID: "foreign", Limit: 10})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatal("外部主体看到了草稿", err)
	}
	items, total, err = s.ListOwnedExportDrafts(ctx, ExportDraftListQuery{SubjectID: "subject-1", Keyword: "%", Limit: 10})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatal("LIKE 通配符扩大了范围", err)
	}
	if err := s.SubmitTask(ctx, validTaskSubmission("draft-submitted")); err != nil {
		t.Fatal(err)
	}
	items, total, err = s.ListOwnedExportDrafts(ctx, ExportDraftListQuery{SubjectID: "subject-1", Limit: 10})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatal("已提交草稿仍进入列表", err)
	}
}

func Test草稿删除版本所有权幂等及已提交和租约反例(t *testing.T) {
	for _, scenario := range []string{"普通删除", "外部主体", "旧版本", "已提交", "活动租约", "租约未知"} {
		t.Run(scenario, func(t *testing.T) {
			s, _ := openTestStore(t)
			seedBaseFixture(t, s)
			ctx := context.Background()
			subject := "subject-1"
			revision := int64(1)
			expected := error(nil)
			switch scenario {
			case "外部主体":
				subject = "foreign"
				expected = ErrDataSourceNotFound
			case "旧版本":
				revision = 2
				expected = ErrRevisionConflict
			case "已提交":
				if err := s.SubmitTask(ctx, validTaskSubmission("used-draft")); err != nil {
					t.Fatal(err)
				}
				expected = ErrExportDraftDeleteIneligible
			case "活动租约":
				if _, err := s.db.Exec(`UPDATE precheck_runs SET status='LEASED',lease_id='synthetic-lease',lease_epoch=1,lease_expires_at=? WHERE precheck_id='precheck-1'`, utcText(testTime.Add(time.Hour))); err != nil {
					t.Fatal(err)
				}
				expected = ErrExportDraftDeleteIneligible
			case "租约未知":
				if _, err := s.db.Exec(`UPDATE precheck_runs SET status='LEASED',lease_id='synthetic-unknown-lease',lease_epoch=1,lease_expires_at=NULL WHERE precheck_id='precheck-1'`); err != nil {
					t.Fatal(err)
				}
				expected = ErrExportDraftDeleteIneligible
			}
			replayed, err := s.DeleteOwnedExportDraft(ctx, "draft-1", subject, revision, "synthetic-delete-key", "delete-request", testTime)
			if !errors.Is(err, expected) || replayed {
				t.Fatal("删除资格错误", err)
			}
			if expected != nil {
				draft, err := s.GetExportDraft(ctx, "draft-1")
				if err != nil || draft.Revision != 1 {
					t.Fatal("失败删除修改了草稿")
				}
				return
			}
			if _, err := s.GetExportDraft(ctx, "draft-1"); !errors.Is(err, ErrDataSourceNotFound) {
				t.Fatal("删除对象仍可编辑", err)
			}
			replayed, err = s.DeleteOwnedExportDraft(ctx, "draft-1", subject, revision, "synthetic-delete-key", "delete-replay", testTime.Add(time.Minute))
			if err != nil || !replayed {
				t.Fatal("重复删除未幂等", err)
			}
			if _, err = s.DeleteOwnedExportDraft(ctx, "draft-1", subject, 2, "synthetic-delete-key", "different-digest", testTime); !errors.Is(err, ErrIdempotencyConflict) {
				t.Fatal("不同请求重用了幂等键", err)
			}
			assertCount(t, s.db, `SELECT COUNT(*) FROM precheck_runs WHERE draft_id='draft-1'`, 1)
			assertCount(t, s.db, `SELECT COUNT(*) FROM audit_events WHERE action='EXPORT_DRAFT_DELETED'`, 1)
			assertCount(t, s.db, `SELECT COUNT(*) FROM export_drafts WHERE deleted_at IS NOT NULL`, 1)
			_, total, err := s.ListOwnedExportDrafts(ctx, ExportDraftListQuery{SubjectID: subject, Limit: 10})
			if err != nil || total != 0 {
				t.Fatal("删除对象仍被计数", err)
			}
			input := validTaskSubmission("deleted-task")
			if err := s.SubmitTask(ctx, input); !errors.Is(err, ErrPrecheckInvalid) {
				t.Fatal("删除后的预检查仍可用于提交", err)
			}
		})
	}
}
