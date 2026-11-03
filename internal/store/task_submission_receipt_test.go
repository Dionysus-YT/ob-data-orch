package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"ob-data-orch/internal/accesscontrol"
)

// independentTaskProof 为多任务夹具复制独立草稿及证明，保持相同配置不等于重复提交的边界。
func independentTaskProof(t *testing.T, database *Store, input TaskSubmission) TaskSubmission {
	t.Helper()
	ctx := context.Background()
	run, err := readStoredPrecheck(ctx, database.db, input.PrecheckID)
	if err != nil {
		t.Fatal(err)
	}
	draftID, checkID := input.TaskID+"-draft", input.TaskID+"-check"
	for _, spec := range []struct{ table, key, oldID, newID string }{{"export_drafts", "draft_id", run.Binding.DraftID, draftID}, {"precheck_runs", "precheck_id", input.PrecheckID, checkID}} {
		rows, err := database.db.Query(`PRAGMA table_info(` + spec.table + `)`)
		if err != nil {
			t.Fatal(err)
		}
		var columns, expressions []string
		for rows.Next() {
			var index, required, primary int
			var name, kind string
			var defaultValue sql.NullString
			if err := rows.Scan(&index, &name, &kind, &required, &defaultValue, &primary); err != nil {
				t.Fatal(err)
			}
			columns = append(columns, name)
			if name == spec.key {
				expressions = append(expressions, "?")
			} else {
				expressions = append(expressions, name)
			}
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := database.db.Exec(`INSERT INTO `+spec.table+` (`+strings.Join(columns, ",")+`) SELECT `+strings.Join(expressions, ",")+` FROM `+spec.table+` WHERE `+spec.key+`=?`, spec.newID, spec.oldID); err != nil {
			t.Fatal(err)
		}
	}
	run.Binding.DraftID, run.Binding.PrecheckID = draftID, checkID
	digest, err := precheckBindingDigest(run.Binding)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.Exec(`UPDATE precheck_runs SET draft_id=?,binding_digest=? WHERE precheck_id=?`, draftID, digest, checkID); err != nil {
		t.Fatal(err)
	}
	input.PrecheckID = checkID
	return input
}

func Test提交意图原键恢复与新键并发互斥(t *testing.T) {
	database, _ := openTestStore(t)
	seedBaseFixture(t, database)
	ctx := context.Background()
	input := validTaskSubmission("receipt-task")
	key := "synthetic-receipt-original"
	if _, err := database.SubmitTaskIdempotent(ctx, input, key, testFingerprint); err != nil {
		t.Fatal(err)
	}
	lookup := TaskSubmissionLookup{SubjectID: "subject-1", TaskType: "OBDUMPER_EXPORT", DraftID: "draft-1", DraftRevision: 1, PrecheckID: "precheck-1", IdempotencyKey: key, RequestDigest: testFingerprint}
	if _, err := database.db.Exec(`UPDATE precheck_runs SET status='EXPIRED',valid_until=? WHERE precheck_id='precheck-1'`, utcText(testTime.Add(-time.Minute))); err != nil {
		t.Fatal(err)
	}
	result, err := database.LookupTaskSubmission(ctx, lookup)
	if err != nil || !result.Replayed || result.TaskID != input.TaskID {
		t.Fatal("过期证明未找回原回执", result, err)
	}
	wrong := lookup
	wrong.RequestDigest = strings.Repeat("b", 64)
	if _, err := database.LookupTaskSubmission(ctx, wrong); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatal("异内容未冲突", err)
	}
	wrong = lookup
	wrong.SubjectID = "other-subject"
	if found, err := database.LookupTaskSubmission(ctx, wrong); err != nil || found.TaskID != "" {
		t.Fatal("他人可读取回执", found, err)
	}
	wrong = lookup
	wrong.IdempotencyKey = "synthetic-receipt-new-key"
	if _, err := database.LookupTaskSubmission(ctx, wrong); !errors.Is(err, ErrTaskSubmissionConflict) {
		t.Fatal("换键没有阻断", err)
	}
	if err := database.SubmitTask(ctx, validTaskSubmission("receipt-other")); !errors.Is(err, ErrTaskSubmissionConflict) {
		t.Fatal("非幂等入口重复冻结", err)
	}
	draft, err := database.GetExportDraft(ctx, "draft-1")
	if err != nil || draft.SubmittedTaskID != input.TaskID {
		t.Fatal("草稿未投影受理结果", err)
	}
	assertCount(t, database.db, `SELECT COUNT(*) FROM tasks`, 1)
	assertCount(t, database.db, `SELECT COUNT(*) FROM audit_events WHERE action='TASK_SUBMITTED'`, 1)

	// 两个真正并发的新请求竞争同一未消费修订，只有一个任务及一份审计提交。
	next := independentTaskProof(t, database, validTaskSubmission("concurrent-a"))
	if _, err := database.db.Exec(`UPDATE precheck_runs SET status='SUCCEEDED',valid_until=? WHERE precheck_id=?`, utcText(testTime.Add(10*time.Minute)), next.PrecheckID); err != nil {
		t.Fatal(err)
	}
	run, err := readStoredPrecheck(ctx, database.db, next.PrecheckID)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := precheckBindingDigest(run.Binding)
	if _, err := database.db.Exec(`UPDATE precheck_runs SET binding_digest=? WHERE precheck_id=?`, digest, next.PrecheckID); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	errorsFound := make(chan error, 2)
	for _, suffix := range []string{"a", "b"} {
		group.Add(1)
		go func(suffix string) {
			defer group.Done()
			candidate := next
			candidate.TaskID = "concurrent-" + suffix
			candidate.RequestID += suffix
			_, err := database.SubmitTaskIdempotent(ctx, candidate, "synthetic-concurrent-"+suffix, testFingerprint)
			errorsFound <- err
		}(suffix)
	}
	group.Wait()
	close(errorsFound)
	succeeded, conflicts := 0, 0
	for err := range errorsFound {
		if err == nil {
			succeeded++
		} else if errors.Is(err, ErrTaskSubmissionConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if succeeded != 1 || conflicts != 1 {
		t.Fatal("并发重复提交", succeeded, conflicts)
	}
	assertCount(t, database.db, `SELECT COUNT(*) FROM tasks`, 2)
}

func Test提交回执在撤权后拒绝恢复(t *testing.T) {
	database := permissionFixture(t)
	ctx := context.Background()
	if _, err := database.ChangeSubjectPermissions(ctx, permissionChange()); err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.Exec(`UPDATE export_drafts SET owner_subject_id='permission-user' WHERE draft_id='draft-1'`); err != nil {
		t.Fatal(err)
	}
	input := validTaskSubmission("permission-receipt")
	input.CreatorSubjectID, input.AuditActorID = "permission-user", "permission-user"
	key := "permission-receipt-key"
	if _, err := database.SubmitTaskIdempotent(ctx, input, key, testFingerprint); err != nil {
		t.Fatal(err)
	}
	lookup := TaskSubmissionLookup{SubjectID: "permission-user", TaskType: "OBDUMPER_EXPORT", DraftID: "draft-1", DraftRevision: 1, PrecheckID: "precheck-1", IdempotencyKey: key, RequestDigest: testFingerprint}
	if _, err := database.LookupTaskSubmission(ctx, lookup); err != nil {
		t.Fatal(err)
	}
	revoke := permissionChange()
	revoke.ExpectedRevision, revoke.IdempotencyKey, revoke.RequestID = 2, "revoke-receipt-key", "revoke-receipt-request"
	revoke.Assignment = accesscontrol.Assignment{}
	if _, err := database.ChangeSubjectPermissions(ctx, revoke); err != nil {
		t.Fatal(err)
	}
	if _, err := database.LookupTaskSubmission(ctx, lookup); !errors.Is(err, ErrClaimIneligible) {
		t.Fatal("撤权后仍可恢复", err)
	}
	if _, err := database.SubmitTaskIdempotent(ctx, input, key, testFingerprint); !errors.Is(err, ErrClaimIneligible) {
		t.Fatal("底层重放绕过撤权", err)
	}
	draft, err := database.GetExportDraft(ctx, "draft-1")
	if err != nil || draft.SubmittedTaskID != "" {
		t.Fatal("撤权后仍投影任务标识", err)
	}
}

func Test历史重复任务可读取原回执但不得继续冻结(t *testing.T) {
	database, _ := openTestStore(t)
	seedBaseFixture(t, database)
	ctx := context.Background()
	first := validTaskSubmission("legacy-first")
	if _, err := database.SubmitTaskIdempotent(ctx, first, "synthetic-legacy-first", testFingerprint); err != nil {
		t.Fatal(err)
	}
	// 直接复制历史行模拟升级前已发生的重复，不修改现役不可变任务，也不要求历史清洗。
	rows, err := database.db.Query(`PRAGMA table_info(tasks)`)
	if err != nil {
		t.Fatal(err)
	}
	var columns, expressions []string
	for rows.Next() {
		var index, required, primary int
		var name, kind string
		var defaultValue sql.NullString
		if err := rows.Scan(&index, &name, &kind, &required, &defaultValue, &primary); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, name)
		if name == "task_id" {
			expressions = append(expressions, "'legacy-second'")
		} else {
			expressions = append(expressions, name)
		}
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.Exec(`INSERT INTO tasks (` + strings.Join(columns, ",") + `) SELECT ` + strings.Join(expressions, ",") + ` FROM tasks WHERE task_id='legacy-first'`); err != nil {
		t.Fatal(err)
	}
	draft, err := database.GetExportDraft(ctx, "draft-1")
	if err != nil || draft.SubmittedTaskID != "" {
		t.Fatal("历史多任务被猜测为唯一结果", err)
	}
	lookup := TaskSubmissionLookup{SubjectID: "subject-1", TaskType: "OBDUMPER_EXPORT", DraftID: "draft-1", DraftRevision: 1, PrecheckID: "precheck-1", IdempotencyKey: "synthetic-legacy-first", RequestDigest: testFingerprint}
	if result, err := database.LookupTaskSubmission(ctx, lookup); err != nil || result.TaskID != first.TaskID {
		t.Fatal("历史原键丢失", result, err)
	}
	// 同一修订重新取得另一份证明也不能绕过已消费检查。
	fresh := independentTaskProof(t, database, validTaskSubmission("legacy-third"))
	if _, err := database.db.Exec(`UPDATE precheck_runs SET draft_id='draft-1' WHERE precheck_id=?`, fresh.PrecheckID); err != nil {
		t.Fatal(err)
	}
	if err := database.SubmitTask(ctx, fresh); !errors.Is(err, ErrTaskSubmissionConflict) {
		t.Fatal("新证明绕过消费保护", err)
	}
	assertCount(t, database.db, `SELECT COUNT(*) FROM tasks`, 2)
}
