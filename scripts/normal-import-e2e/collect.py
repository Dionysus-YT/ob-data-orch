"""独立核对本轮内容与本机持久事件，只输出固定安全字段，不读取凭据列。"""
import collections
import json
import pathlib
import sqlite3
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
WORK = ROOT / 'artifacts' / (sys.argv[1] if len(sys.argv) > 1 else 'normal-import-e2e')
if WORK.resolve().parent != (ROOT / 'artifacts').resolve():
    raise ValueError('EVIDENCE_DIRECTORY_REJECTED')


def load(name):
    return json.loads((WORK / name).read_text(encoding='utf-8-sig'))


def main():
    run = load('run.json')
    records = {}
    for name in ['browser-initial.json', 'browser-extra.json', 'browser-duplicate.json', 'lifecycle-initial.json', 'lifecycle-recheck.json']:
        if not (WORK / name).exists():
            continue
        value = load(name)
        if not value.get('done'):
            raise ValueError('INCOMPLETE_BATCH')
        records.update({item['id']: item for item in value['records']})
    expected = {f'NI-E2E-{index:02}' for index in range(1, 24 if run.get('retest') else 21)}
    if set(records) != expected:
        raise ValueError('CASE_MATRIX_INCOMPLETE')
    supplemental, contents = load('supplemental.json'), load('content.json')
    db = sqlite3.connect((ROOT / 'var/dev/control-plane/data/local-mvp.db').as_uri() + '?mode=ro', uri=True)
    tables = {'NI-E2E-01': 'nivt_csv', 'NI-E2E-02': 'nivt_cut', 'NI-E2E-03': 'nivt_sql', 'NI-E2E-04': 'nivt_ddl', 'NI-E2E-05': 'nivt_both', 'NI-E2E-13': 'nivt_quoted', 'NI-E2E-21': 'nivt_trim', 'NI-E2E-22': 'nivt_both_quoted', 'NI-E2E-23': 'nivt_strict_false'}
    try:
        for record in records.values():
            task_id = record.get('taskId')
            if not task_id:
                continue
            row = db.execute('SELECT t.task_type,t.snapshot_version,t.config_fingerprint,e.execution_id,e.state,e.reconciliation_required,e.result_summary_json,e.process_evidence_json FROM tasks t JOIN task_executions e ON e.task_id=t.task_id WHERE t.task_id=?', (task_id,)).fetchone()
            if not row or row[0] != 'OBLOADER_NORMAL_IMPORT' or row[1] != 'normal-import-v1':
                raise ValueError('TASK_BINDING_INVALID')
            summary, process = json.loads(row[6]), json.loads(row[7])
            events = db.execute('SELECT event_type,payload_json FROM execution_events WHERE execution_id=? AND accepted=1 ORDER BY event_seq', (row[3],)).fetchall()
            record['persistedEvidence'] = {
                'state': row[4], 'result': summary.get('result'), 'normalImport': summary.get('normalImport') is True,
                'noExportCounts': all(name not in summary for name in ['fileCount', 'totalBytes', 'files']),
                'checkpointPresent': summary.get('checkpointPresent'), 'reconciliationRequired': bool(row[5]),
                'processExitCode': process.get('exitCode'), 'plannedActualMatch': process.get('plannedActualMatch'),
                'processStartedEvents': sum(kind == 'PROCESS_STARTED' for kind, _ in events),
                'resultEvents': sum(kind == 'RESULT_FACTS_OBSERVED' for kind, _ in events),
            }
            record['snapshotMatched'] = row[2] == (record.get('fingerprint') or supplemental['uiFingerprint'])
            verified = summary.get('normalImport') is True and all(name not in summary for name in ['fileCount', 'totalBytes', 'files']) and not row[5] and record['snapshotMatched'] and process.get('plannedActualMatch') == 'MATCHED'
            if record['id'] in tables:
                content = contents['tables'][tables[record['id']]]
                record['content'] = content
                record['outcome'] = 'PASS' if verified and row[4] == 'SUCCEEDED' and summary.get('result') == 'VERIFIED' and content.get('contentMatched') and content.get('structureMatched') else 'FAIL'
                if not content.get('contentMatched'):
                    record['code'] = 'UNQUOTED_CSV_WHITESPACE_CHANGED'
            elif record['id'] == 'NI-E2E-14' and not run.get('retest'):
                record.update(outcome='FAIL', code='STRICT_ARGUMENT_REJECTED', strictArgumentRejected=supplemental['strictArgumentRejected'])
            elif record['id'] in ['NI-E2E-14', 'NI-E2E-20']:
                discard = record.get('duplicateLog') if run.get('retest') else supplemental['discardLimitFailure']
                passed = verified and row[4] == 'FAILED' and summary.get('result') == 'FAILED' and process.get('exitCode') == 1 and discard and contents['duplicateTargetUnchanged']
                record.update(outcome='PASS' if passed else 'FAIL', duplicateTargetUnchanged=contents['duplicateTargetUnchanged'], discardLimitFailure=supplemental['discardLimitFailure'])
                if passed:
                    record.pop('code', None)
        if run.get('retest'):
            for key, unchanged in [('NI-E2E-01', contents['tables']['nivt_csv'].get('actualRows') == 0), ('NI-E2E-05', not contents['tables']['nivt_both']['exists'])]:
                record = records[key]
                count = db.execute('SELECT COUNT(*) FROM tasks t JOIN precheck_runs p ON p.precheck_id=t.precheck_id WHERE p.draft_id=?', (record['draftId'],)).fetchone()[0]
                record['noTaskCreated'] = count == 0
                record['targetUnchanged'] = unchanged
                if not unchanged or count:
                    record.update(outcome='FAIL', code='UNSAFE_CSV_REJECTION_INCOMPLETE')
    finally:
        db.close()
    # 与真实密码及其可恢复编码仅做内存比较；审计失败时不生成交付证据。
    report = {'runId': run['runId'], 'scope': 'TEST_MYSQL_ODP_WINDOWS_AMD64_LOCAL_CSV_CUT_SQL', 'records': sorted(records.values(), key=lambda r: int(r['id'].split('-')[-1])),
              'counts': dict(collections.Counter(r['outcome'] for r in records.values())), 'ownerVerified': contents['ownerVerified'], 'rejectedTargetUnchanged': contents['rejectedTargetUnchanged']}
    raw = json.dumps(report, ensure_ascii=False, indent=2).encode('utf-8')
    audited = subprocess.run([str(WORK / 'fixture.exe'), '-action', 'audit', '-database', run['database'], '-run-id', run['runId']], input=raw, capture_output=True, cwd=ROOT, check=True)
    if json.loads(audited.stdout)['plaintextOrRecoverableEncodingPresent']:
        raise ValueError('SECRET_AUDIT_FAILED')
    report['secretAuditPassed'] = True
    (WORK / 'verified-results.json').write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
    print(json.dumps({'counts': report['counts'], 'records': len(records), 'secretAuditPassed': True}))


if __name__ == '__main__':
    main()
