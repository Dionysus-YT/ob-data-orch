package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"ob-data-orch/internal/accesscontrol"
	"ob-data-orch/internal/logquery"
)

// logQueryTimeKey 将已登记的 UTC RFC3339Nano 补齐到纳秒后比较，避免省略小数造成错序。
const logQueryTimeKey = `(substr(b.received_at,1,19)||substr((CASE WHEN substr(b.received_at,20,1)='.' THEN substr(b.received_at,21,length(b.received_at)-21) ELSE '' END)||'000000000',1,9))`
const logQueryKey = `(` + logQueryTimeKey + `||'|'||stream.source_kind||'|'||stream.stream_id||'|'||printf('%019d',b.first_sequence)||'|'||b.batch_id)`
const logQueryFrom = ` FROM tasks t JOIN task_executions execution ON execution.task_id=t.task_id
    JOIN log_streams stream ON stream.execution_id=execution.execution_id
    JOIN log_batches b ON b.stream_id=stream.stream_id
    JOIN log_query_positions qp ON qp.batch_id=b.batch_id
    JOIN log_segments segment ON segment.segment_id=b.segment_id `

func logQueryAuthorization(ctx context.Context, tx *sql.Tx, subject string, f logquery.Filter) (string, []any, string, error) {
	profile, err := permissionProfile(ctx, tx, subject)
	if err != nil {
		if errors.Is(err, ErrPermissionDenied) {
			err = logquery.ErrDenied
		}
		return "", nil, "", err
	}
	if profile.Subject.AccountStatus != "ACTIVE" || (!profile.Assignment.HasRole(accesscontrol.BasicUser) && !profile.Assignment.HasRole(accesscontrol.Operator) && !initialGlobalTaskRead(profile)) {
		return "", nil, "", logquery.ErrDenied
	}
	predicate, args, err := buildTaskListPredicate(TaskListQuery{SubjectID: subject, GlobalRead: initialGlobalTaskRead(profile), TaskID: f.TaskID})
	if err != nil {
		return "", nil, "", err
	}
	if f.TaskID != "" {
		var found bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tasks t WHERE `+predicate+`)`, args...).Scan(&found); err != nil {
			return "", nil, "", err
		}
		if !found {
			return "", nil, "", logquery.ErrDenied
		}
	}
	raw, err := json.Marshal(struct {
		Revision   int64
		Active     string
		Initial    bool
		Assignment accesscontrol.Assignment
	}{profile.Revision, profile.Subject.AccountStatus, profile.InitialAdministrator, accesscontrol.Canonical(profile.Assignment)})
	if err != nil {
		return "", nil, "", err
	}
	digest := sha256.Sum256(raw)
	return predicate, args, hex.EncodeToString(digest[:]), nil
}

// LogQueryScope 先在当前 TASK_READ 范围中取元数据；不扫描正文，不返回对象总数或目录路径。
func (s *Store) LogQueryScope(ctx context.Context, subject string, f logquery.Filter) (logquery.Scope, error) {
	result := logquery.Scope{Sources: []string{}}
	if s == nil || s.db == nil {
		return result, logquery.ErrUnavailable
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	predicate, args, fingerprint, err := logQueryAuthorization(ctx, tx, subject, f)
	if err != nil {
		return result, err
	}
	result.Fingerprint = fingerprint
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0) FROM log_query_positions`).Scan(&result.Watermark); err != nil {
		return result, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 `+logQueryFrom+` WHERE `+predicate+`)`, args...).Scan(&result.Retained); err != nil {
		return result, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tasks t JOIN log_segment_cleanup expired ON expired.task_id=t.task_id WHERE `+predicate+`)`, args...).Scan(&result.Expired); err != nil {
		return result, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT stream.source_kind,b.parser_version `+logQueryFrom+` WHERE `+predicate+` AND `+logQueryTimeKey+` >= ? AND `+logQueryTimeKey+` < ? ORDER BY stream.source_kind,b.parser_version`, append(args, timeQueryKey(f.From), timeQueryKey(f.To))...)
	if err != nil {
		return result, err
	}
	seen := map[string]bool{}
	for rows.Next() {
		var source, parser string
		if err = rows.Scan(&source, &parser); err != nil {
			rows.Close()
			return result, err
		}
		kind, e := storedLogQuerySource(source, parser)
		if e != nil {
			rows.Close()
			return result, e
		}
		if !seen[kind] {
			result.Sources = append(result.Sources, kind)
			seen[kind] = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}

func timeQueryKey(t time.Time) string {
	v := t.UTC().Format("2006-01-02T15:04:05.000000000")
	return v[:19] + v[20:]
}

func storedLogQuerySource(source, parser string) (string, error) {
	if parser == "obloader-4.3.5-v1" || parser == "obloader-direct-4.3.5-v1" {
		switch source {
		case "STDOUT":
			return "OBLOADER_STDOUT", nil
		case "STDERR":
			return "OBLOADER_STDERR", nil
		default:
			return "", logquery.ErrUnavailable
		}
	}
	switch source {
	case "STDOUT":
		return "OBDUMPER_STDOUT", nil
	case "STDERR":
		return "OBDUMPER_STDERR", nil
	case "TOOL_FILE":
		return "OBDUMPER_FILE", nil
	default:
		return "", logquery.ErrUnavailable
	}
}

// LogQueryBatches 只返回授权索引；每次短读取事务复验授权指纹，正文读取不持有 SQLite 事务。
func (s *Store) LogQueryBatches(ctx context.Context, w logquery.Window) ([]logquery.Candidate, error) {
	if s == nil || s.db == nil {
		return nil, logquery.ErrUnavailable
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	predicate, args, fingerprint, err := logQueryAuthorization(ctx, tx, w.SubjectID, w.Filter)
	if err != nil {
		return nil, err
	}
	if fingerprint != w.Fingerprint {
		return nil, logquery.ErrCursor
	}
	predicate += ` AND qp.sequence <= ?`
	args = append(args, w.Watermark)
	limit := 64
	if w.StreamID != "" {
		predicate += ` AND stream.stream_id=? AND b.last_sequence>=? AND b.first_sequence<=?`
		args = append(args, w.StreamID, w.MinSeq, w.MaxSeq)
		limit = 102
	} else {
		if !w.AllRetained {
			predicate += ` AND ` + logQueryTimeKey + `>=? AND ` + logQueryTimeKey + `<?`
			args = append(args, timeQueryKey(w.Filter.From), timeQueryKey(w.Filter.To))
		}
		if w.BeforeKey != "" {
			op := "<"
			if w.Include {
				op = "<="
			}
			predicate += ` AND ` + logQueryKey + op + `?`
			args = append(args, w.BeforeKey)
		}
		if w.Filter.Source != "" {
			physical := ""
			parser := ""
			switch w.Filter.Source {
			case "OBDUMPER_STDOUT":
				physical = "STDOUT"
			case "OBDUMPER_STDERR":
				physical = "STDERR"
			case "OBDUMPER_FILE":
				physical = "TOOL_FILE"
			case "OBLOADER_STDOUT":
				physical = "STDOUT"
				parser = "loader"
			case "OBLOADER_STDERR":
				physical = "STDERR"
				parser = "loader"
			default:
				return nil, logquery.ErrInvalid
			}
			predicate += ` AND stream.source_kind=?`
			args = append(args, physical)
			if parser == "loader" {
				predicate += ` AND b.parser_version IN ('obloader-4.3.5-v1','obloader-direct-4.3.5-v1')`
			} else {
				predicate += ` AND b.parser_version NOT IN ('obloader-4.3.5-v1','obloader-direct-4.3.5-v1')`
			}
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+logQueryKey+`,t.task_id,b.batch_id,stream.execution_id,stream.stream_id,stream.source_kind,stream.source_ref,
        b.stream_epoch,b.first_sequence,b.last_sequence,b.batch_digest,
        COALESCE((SELECT prior.batch_digest FROM log_batches prior WHERE prior.stream_id=b.stream_id AND prior.gap_summary_json IS NULL AND prior.last_sequence<b.first_sequence ORDER BY prior.last_sequence DESC LIMIT 1),''),
        COALESCE(b.gap_summary_json,''),b.policy_version,b.parser_version,b.segment_id,segment.segment_ordinal,
        segment.storage_key,segment.state,segment.byte_length,b.segment_offset_start,b.segment_offset_end,b.record_count,b.received_at
        `+logQueryFrom+` WHERE `+predicate+` ORDER BY `+logQueryKey+` DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	items := []logquery.Candidate{}
	for rows.Next() {
		var item logquery.Candidate
		var received, gap string
		b := &item.Batch
		if err = rows.Scan(&item.Key, &item.TaskID, &b.BatchID, &b.ExecutionID, &b.StreamID, &b.SourceKind, &b.SourceRef, &b.SourceEpoch, &b.FirstSequence, &b.LastSequence, &b.Digest, &b.PreviousDigest, &gap, &b.PolicyVersion, &b.ParserVersion, &b.SegmentID, &b.SegmentOrdinal, &b.SegmentStorageKey, &b.SegmentState, &b.SegmentByteLength, &b.SegmentOffsetStart, &b.SegmentOffsetEnd, &b.RecordCount, &received); err != nil {
			rows.Close()
			return nil, err
		}
		b.ReceivedAt, err = time.Parse(time.RFC3339Nano, received)
		if err != nil {
			rows.Close()
			return nil, err
		}
		b.GapReasonCode, err = parseLogGapSummary(gap)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return items, tx.Commit()
}

// AuditLogQuery 只保存固定动作与范围摘要；正文、关键字原文和游标均不进入审计。
func (s *Store) AuditLogQuery(ctx context.Context, a logquery.Audit) error {
	if !slicesLogAuditAction(a.Action) || (a.Result != "SUCCEEDED" && a.Result != "FAILED") || a.SubjectID == "" || a.RequestID == "" {
		return logquery.ErrInvalid
	}
	digest := sha256.Sum256([]byte(a.Filter.Keyword))
	raw, err := json.Marshal(map[string]any{"taskId": a.Filter.TaskID, "from": a.Filter.From, "to": a.Filter.To, "source": a.Filter.Source, "keywordSha256": hex.EncodeToString(digest[:]), "records": a.Count, "redacted": true})
	if err != nil {
		return err
	}
	return s.withWrite(ctx, func(tx *sql.Tx) error {
		if a.Result == "SUCCEEDED" {
			_, _, fingerprint, e := logQueryAuthorization(ctx, tx, a.SubjectID, a.Filter)
			if e != nil {
				return e
			}
			if fingerprint != a.Fingerprint {
				return logquery.ErrCursor
			}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO audit_events(audit_event_id,actor_type,actor_id,action,object_type,object_id,result,request_id,safe_diff_json,occurred_at) VALUES(?,'SUBJECT',?,?,'LOG_QUERY','task-logs',?,?,?,?)`, auditID(a.Action, a.RequestID), a.SubjectID, a.Action, a.Result, a.RequestID, string(raw), utcText(a.At))
		return err
	})
}

func slicesLogAuditAction(action string) bool {
	return action == "TASK_LOG_DOWNLOAD" || action == "TASK_LOG_CONTEXT_COPY"
}
