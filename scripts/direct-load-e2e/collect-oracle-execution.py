"""独立核对 Oracle 真实任务的完整行集、DDL、持久事件与精确清理，不读取秘密列。"""
import decimal
import hashlib
import json
import pathlib
import re
import sqlite3
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
relative = sys.argv[1] if len(sys.argv) == 2 else ''
if not re.fullmatch(r'artifacts[\\/]direct-load-e2e-\d{8}-[a-f0-9]{12}', relative):
    raise ValueError('ORACLE_EXECUTION_SCOPE_INVALID')
work = ROOT / relative


def load(name):
    return json.loads((work / name).read_text(encoding='utf-8-sig'))


def canonical(rows):
    return [[row[0], row[1], str(decimal.Decimal(row[2]).normalize())] for row in rows]


state, before, after = load('oracle-execution-api.json'), load('fixture-before.json'), load('fixture-after.json')
if not state.get('finishedAt') or not before['ownerVerified'] or not after['ownerVerified']:
    raise ValueError('ORACLE_EXECUTION_NOT_FINISHED')
db = sqlite3.connect((ROOT / 'var/dev/control-plane/data/local-mvp.db').as_uri() + '?mode=ro', uri=True)
backup = sqlite3.connect((ROOT / 'artifacts/direct-load-oracle-repair-20261022/pre-install-backup.db').as_uri() + '?mode=ro', uri=True)
columns = 'revision,connection_kind,compatibility_mode,state,current_credential_revision'
original_id = '55208ffb-2171-4e69-bb97-5af53c2f0d81'
original_unchanged = db.execute(f'SELECT {columns} FROM data_sources WHERE data_source_id=?', (original_id,)).fetchone() == backup.execute(f'SELECT {columns} FROM data_sources WHERE data_source_id=?', (original_id,)).fetchone()
backup.close()
mapping = {'BASIC', 'SHARDS', 'SEPARATOR', 'INCREMENTAL', 'TRUNCATE', 'THREAD', 'PARALLEL'}
targets = {}
for row in state['records']:
    name = row['id']
    if name not in mapping or not row.get('taskId'):
        continue
    observed = db.execute('SELECT e.execution_id,e.state,e.reconciliation_required,e.result_summary_json,e.process_evidence_json,t.config_fingerprint,t.task_type,t.data_source_id FROM task_executions e JOIN tasks t ON t.task_id=e.task_id WHERE t.task_id=?', (row['taskId'],)).fetchone()
    if not observed:
        raise ValueError('ORACLE_TASK_EVIDENCE_MISSING')
    summary, process = json.loads(observed[3]), json.loads(observed[4])
    events = dict(db.execute('SELECT event_type,count(*) FROM execution_events WHERE execution_id=? AND accepted=1 GROUP BY event_type', (observed[0],)).fetchall())
    accepted = json.loads(db.execute("SELECT payload_json FROM execution_events WHERE execution_id=? AND accepted=1 AND event_type='RESULT_FACTS_OBSERVED' ORDER BY event_seq DESC LIMIT 1", (observed[0],)).fetchone()[0])
    expected = [['1', 'alpha', '12.3400'], ['2', '中文', '-0.0100']] + ([['9', 'original', '9.0000']] if name == 'INCREMENTAL' else [])
    facts = {'taskId': row['taskId'], 'state': observed[1], 'reconciliationRequired': bool(observed[2]), 'resultVerified': summary.get('result') == 'VERIFIED', 'directLoad': summary.get('directLoad') is True, 'serverCommitObserved': summary.get('serverCommitObserved') is True and accepted.get('serverCommitObserved') is True, 'checkpointAbsent': summary.get('checkpointPresent') is False, 'exitCode': process.get('exitCode'), 'plannedActualMatched': process.get('plannedActualMatch') == 'MATCHED', 'fingerprintMatched': observed[5] == row['fingerprint'], 'taskTypeMatched': observed[6] == 'OBLOADER_DIRECT_LOAD', 'sourceMatched': observed[7] == state['sourceId'], 'rowsMatched': canonical(after['tables'][name]['rows']) == canonical(expected), 'structureMatched': before['tables'][name]['structure'] == after['tables'][name]['structure'], 'fullDDLObserved': bool(before['tables'][name]['structure']['ddl']) and bool(after['tables'][name]['structure']['ddl']), 'oneStartEvent': events.get('PROCESS_STARTED') == 1, 'oneResultEvent': events.get('RESULT_FACTS_OBSERVED') == 1, 'commandAndSnapshotMatched': row['commandMatched'] and row['snapshotMatched'], 'noExportResultCounts': all(key not in summary for key in ['fileCount', 'totalBytes', 'files'])}
    facts['passed'] = facts['state'] == 'SUCCEEDED' and not facts['reconciliationRequired'] and facts['exitCode'] == 0 and all(facts[key] for key in ['resultVerified', 'directLoad', 'serverCommitObserved', 'checkpointAbsent', 'plannedActualMatched', 'fingerprintMatched', 'taskTypeMatched', 'sourceMatched', 'rowsMatched', 'structureMatched', 'fullDDLObserved', 'oneStartEvent', 'oneResultEvent', 'commandAndSnapshotMatched', 'noExportResultCounts'])
    targets[name] = facts
source = db.execute('SELECT state,compatibility_mode FROM data_sources WHERE data_source_id=?', (state['sourceId'],)).fetchone()
remaining = sum(db.execute('SELECT count(*) FROM export_drafts WHERE draft_id=? AND deleted_at IS NULL', (item['id'],)).fetchone()[0] for item in state['drafts'] if not item.get('submitted'))
cleanup = load('fixture-cleanup.json')
untouched = all(value == after['tables'][name] for name, value in before['tables'].items() if name not in targets)
agent = db.execute('SELECT a.status,a.last_heartbeat_at,a.capacity_used,n.environment_check_status FROM agents a JOIN execution_nodes n ON n.node_id=a.node_id ORDER BY a.last_heartbeat_at DESC LIMIT 1').fetchone()
result = {'scope': 'ORACLE_MYSCHEMA_FIXED_4_3_5_OBSERVER_4_4_2_3_API_E2E', 'apiPassed': state['apiPassed'], 'records': len(state['records']), 'realTasks': len(targets), 'targetResults': targets, 'originalSharedSourceUnchanged': original_unchanged, 'allFixtureTablesPreserved': set(before['tables']) == set(after['tables']), 'untouchedTablesUnchanged': untouched, 'newTestSourceArchived': source == ('ARCHIVED', 'ORACLE'), 'unsubmittedDraftsRemaining': remaining, 'sourceTaskCount': db.execute('SELECT count(*) FROM tasks WHERE data_source_id=?', (state['sourceId'],)).fetchone()[0], 'fixtureTablesAbsent': cleanup.get('ownerVerified') is True and cleanup.get('tablesAbsent') is True, 'inputAbsent': cleanup.get('inputAbsent') is True and not (ROOT / 'ob-data' / ('dlvt-oracle-' + state['runId'][:12])).exists(), 'uncontrolledRuntimeFiles': after['uncontrolledRuntimeFiles'], 'nonTerminalExecutions': db.execute("SELECT count(*) FROM task_executions WHERE state NOT IN ('SUCCEEDED','FAILED','CANCELLED')").fetchone()[0], 'latestAgent': {'status': agent[0], 'lastHeartbeat': agent[1], 'capacityUsed': agent[2], 'environmentStatus': agent[3]}}
for name, path in [('controlPlane', 'var/dev/control-plane/control-plane.exe'), ('agent', 'var/dev/agent/agent.exe')]:
    result[name + 'SHA256'] = hashlib.sha256((ROOT / path).read_bytes()).hexdigest()
result['passed'] = bool(targets) and state['apiPassed'] and all(row['passed'] for row in targets.values()) and original_unchanged and untouched and result['allFixtureTablesPreserved'] and result['newTestSourceArchived'] and remaining == 0 and result['sourceTaskCount'] == len(targets) and result['fixtureTablesAbsent'] and result['inputAbsent'] and after['uncontrolledRuntimeFiles'] == [] and result['nonTerminalExecutions'] == 0
(work / 'independent-verification.json').write_text(json.dumps(result, ensure_ascii=True, indent=2) + '\n', encoding='utf-8')
print(json.dumps({'passed': result['passed'], 'records': result['records'], 'realTasks': len(targets)}))
db.close()
if not result['passed']:
    sys.exit(1)
