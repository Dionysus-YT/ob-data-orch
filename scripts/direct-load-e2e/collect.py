"""独立核对本轮旁路完整目标数据、持久终态和清理事实；只读固定安全列。"""
import hashlib
import json
import pathlib
import re
import sqlite3
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
relative = sys.argv[1] if len(sys.argv) > 1 else ''
if not re.fullmatch(r'artifacts[\\/]direct-load-e2e-\d{8}-[a-f0-9]{12}', relative):
    raise ValueError('RUN_SCOPE_INVALID')
WORK = ROOT / relative
private_logs = '--require-private-logs' in sys.argv[2:]
strict_summary = '--require-summary-commit' in sys.argv[2:]


def load(name):
    return json.loads((WORK / name).read_text(encoding='utf-8-sig'))


def main():
    state, before, after = load('standalone-api.json'), load('fixture-before.json'), load('fixture-after.json')
    if not state.get('finishedAt') or not before.get('ownerVerified') or not after.get('ownerVerified'):
        raise ValueError('INCOMPLETE_BATCH')
    db = sqlite3.connect((ROOT / 'var/dev/control-plane/data/local-mvp.db').as_uri() + '?mode=ro', uri=True)
    source_columns = 'revision,connection_kind,current_credential_revision,state'
    original_id = 'fe776904-5ed8-485d-8623-f53909b4d9fc'
    backup = sqlite3.connect((ROOT / 'artifacts/direct-load-repair-20261022/pre-install-backup.db').as_uri() + '?mode=ro', uri=True)
    original_unchanged = db.execute(f'SELECT {source_columns} FROM data_sources WHERE data_source_id=?', (original_id,)).fetchone() == backup.execute(f'SELECT {source_columns} FROM data_sources WHERE data_source_id=?', (original_id,)).fetchone()
    backup.close()
    mapping = {'BASIC':'dlvt_basic', 'SHARDS':'dlvt_shards', 'SEPARATOR':'dlvt_separator', 'INCREMENTAL':'dlvt_incremental', 'TRUNCATE':'dlvt_truncate', 'THREAD':'dlvt_thread', 'PARALLEL':'dlvt_parallel'}
    target = {}
    expected_rows = [['1','alpha','12.3400'], ['2','中文','-0.0100']]
    for row in state['records']:
        if row['id'] not in mapping or not row.get('taskId'):
            continue
        table = mapping[row['id']]
        expected = expected_rows + ([['9','original','9.0000']] if row['id'] == 'INCREMENTAL' else [])
        result = db.execute('SELECT e.execution_id,e.state,e.reconciliation_required,e.result_summary_json,e.process_evidence_json,t.config_fingerprint,t.task_type FROM tasks t JOIN task_executions e ON e.task_id=t.task_id WHERE t.task_id=?', (row['taskId'],)).fetchone()
        if not result:
            raise ValueError('TASK_EVIDENCE_MISSING')
        summary, process = json.loads(result[3]), json.loads(result[4])
        events = dict(db.execute('SELECT event_type,count(*) FROM execution_events WHERE execution_id=? AND accepted=1 GROUP BY event_type', (result[0],)).fetchall())
        accepted_facts = json.loads(db.execute("SELECT payload_json FROM execution_events WHERE execution_id=? AND accepted=1 AND event_type='RESULT_FACTS_OBSERVED' ORDER BY event_seq DESC LIMIT 1", (result[0],)).fetchone()[0])
        facts = {'taskId':row['taskId'], 'state':result[1], 'reconciliationRequired':bool(result[2]), 'result':summary.get('result'), 'directLoad':summary.get('directLoad') is True, 'serverCommitObserved':summary.get('serverCommitObserved') is True, 'checkpointPresent':summary.get('checkpointPresent'), 'exitCode':process.get('exitCode'), 'plannedActualMatch':process.get('plannedActualMatch'), 'fingerprintMatched':result[5] == row['fingerprint'], 'taskType':result[6], 'rowsMatched':after['tables'][table]['rows'] == expected, 'ddlUnchanged':after['tables'][table]['structure'] == before['tables'][table]['structure'], 'processStartedEvents':events.get('PROCESS_STARTED',0), 'resultEvents':events.get('RESULT_FACTS_OBSERVED',0), 'noExportCounts':all(key not in summary for key in ['fileCount','totalBytes','files'])}
        facts['summaryCommitRecorded'] = facts['serverCommitObserved']
        facts['serverCommitObserved'] = accepted_facts.get('serverCommitObserved') is True
        facts['passed'] = facts['state'] == 'SUCCEEDED' and not facts['reconciliationRequired'] and facts['result'] == 'VERIFIED' and facts['directLoad'] and facts['serverCommitObserved'] and facts['checkpointPresent'] is False and facts['exitCode'] == 0 and facts['plannedActualMatch'] == 'MATCHED' and facts['fingerprintMatched'] and facts['taskType'] == 'OBLOADER_DIRECT_LOAD' and facts['rowsMatched'] and facts['ddlUnchanged'] and facts['processStartedEvents'] == 1 and facts['resultEvents'] == 1 and facts['noExportCounts'] and (not strict_summary or facts['summaryCommitRecorded'])
        target[table] = facts
    untouched = all(value == after['tables'][name] for name,value in before['tables'].items() if name not in target)
    source = db.execute('SELECT state FROM data_sources WHERE data_source_id=?', (state.get('sourceId'),)).fetchone()
    remaining = sum(db.execute('SELECT count(*) FROM export_drafts WHERE draft_id=? AND deleted_at IS NULL', (draft['id'],)).fetchone()[0] for draft in state['drafts'] if not draft.get('submitted'))
    active = db.execute("SELECT count(*) FROM task_executions WHERE state NOT IN ('SUCCEEDED','FAILED','CANCELLED')").fetchone()[0]
    agent = db.execute('SELECT status,last_heartbeat_at,capacity_used FROM agents ORDER BY last_heartbeat_at DESC LIMIT 1').fetchone()
    cleanup = load('fixture-cleanup.json')
    result = {'scope':'SCOPED_MYSQL_OBSERVER_API_E2E', 'runId':state['runId'], 'apiRecords':len(state['records']), 'apiPassed':state.get('apiPassed'), 'realTasks':len(target), 'targetResults':target, 'untouchedTablesUnchanged':untouched, 'allTablesPreserved':set(before['tables']) == set(after['tables']), 'originalSharedSourceUnchanged':original_unchanged, 'newTestSourceArchived':source is not None and source[0] == 'ARCHIVED', 'unsubmittedDraftsRemaining':remaining, 'nonTerminalExecutions':active, 'fixtureDatabaseAbsent':cleanup.get('databaseAbsent') is True, 'fixtureInputAbsent':cleanup.get('inputAbsent') is True, 'latestAgent':{'status':agent[0], 'lastHeartbeat':agent[1], 'capacityUsed':agent[2]}, 'privateLogIsolationRequired':private_logs, 'uncontrolledRuntimeFiles':after.get('uncontrolledRuntimeFiles')}
    for name, relative in [('controlPlane','var/dev/control-plane/control-plane.exe'),('agent','var/dev/agent/agent.exe')]:
        result[name+'SHA256'] = hashlib.sha256((ROOT/relative).read_bytes()).hexdigest()
    result['passed'] = bool(target) and state.get('apiPassed') is True and all(value['passed'] for value in target.values()) and untouched and result['allTablesPreserved'] and original_unchanged and result['newTestSourceArchived'] and remaining == 0 and active == 0 and result['fixtureDatabaseAbsent'] and result['fixtureInputAbsent'] and (not private_logs or after.get('uncontrolledRuntimeFiles') == [])
    (WORK/'independent-verification.json').write_text(json.dumps(result,ensure_ascii=True,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':result['passed'],'realTasks':len(target),'apiRecords':len(state['records']),'privateLogIsolationRequired':private_logs}))
    db.close()
    if not result['passed']:
        sys.exit(1)


if __name__ == '__main__':
    main()
