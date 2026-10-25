"""独立核对 Oracle API 验证、目标不变、提交拒绝和精确清理；只读产品安全列。"""
import json
import pathlib
import re
import sqlite3
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
relative = sys.argv[1] if len(sys.argv) == 2 else ''
if not re.fullmatch(r'artifacts[\\/]direct-load-e2e-\d{8}-[a-f0-9]{12}', relative):
    raise ValueError('ORACLE_RUN_SCOPE_INVALID')
work = ROOT / relative


def load(name):
    return json.loads((work / name).read_text(encoding='utf-8-sig'))


state = load('oracle-api.json')
before, after, cleanup = load('fixture-before.json'), load('fixture-after.json'), load('fixture-cleanup.json')
original = load('original-source-before.json')
db = sqlite3.connect((ROOT / 'var/dev/control-plane/data/local-mvp.db').as_uri() + '?mode=ro', uri=True)
current = db.execute('SELECT revision,connection_kind,compatibility_mode,state,current_credential_revision,port FROM data_sources WHERE data_source_id=?', (original['id'],)).fetchone()
original_expected = tuple(original[key] for key in ['revision', 'connectionKind', 'compatibilityMode', 'state', 'credentialRevision', 'port'])
checks = {row['check']: row['status'] for row in state['fullPrecheck']['results']}
unresolved = {key: value for key, value in checks.items() if value != 'PASSED'}
rows = db.execute('SELECT status,last_heartbeat_at,capacity_used FROM agents ORDER BY last_heartbeat_at DESC LIMIT 1').fetchone()
result = {
    'scope': 'ORACLE_DIRECT_API_BLOCKER_VERIFICATION',
    'overall': state['overall'],
    'observerVersion': state['observerVersion'],
    'records': len(state['records']),
    'passed': sum(row['outcome'] == 'PASS' for row in state['records']),
    'blocked': sum(row['outcome'] == 'BLOCKED' for row in state['records']),
    'failed': sum(row['outcome'] == 'FAIL' for row in state['records']),
    'realOracleTasks': db.execute('SELECT count(*) FROM tasks WHERE data_source_id=?', (state['sourceId'],)).fetchone()[0],
    'temporarySourceAbsent': db.execute('SELECT count(*) FROM data_sources WHERE data_source_id=?', (state['sourceId'],)).fetchone()[0] == 0,
    'originalSourceUnchanged': current == original_expected,
    'draftsRemaining': sum(db.execute('SELECT count(*) FROM export_drafts WHERE draft_id=? AND deleted_at IS NULL', (item['id'],)).fetchone()[0] for item in state['drafts']),
    'targetRowsUnchanged': before['rows'] == after['rows'] == [],
    'targetStructureUnchanged': before['columns'] == after['columns'] and len(after['columns']) == 3 and before['primaryKeys'] == after['primaryKeys'] == 1,
    'fixtureOwnerVerified': before['ownerVerified'] and after['ownerVerified'] and cleanup['ownerVerified'],
    'fixtureTablesAbsent': cleanup['tablesAbsent'],
    'inputAbsent': not (ROOT / 'ob-data' / ('dlvt-oracle-' + state['runId'][:12])).exists(),
    'checks': checks,
    'unresolved': unresolved,
    'submissionRejected': any(row['id'] == 'ORACLE_UNPROVEN_SUBMISSION_REJECTED' and row['outcome'] == 'PASS' and row['status'] == 422 and row['code'] == 'PRECHECK_REQUIRED' for row in state['records']),
    'latestAgent': {'status': rows[0], 'lastHeartbeat': rows[1], 'capacityUsed': rows[2]},
    'functionalE2EPassed': False,
}
result['evidenceConsistent'] = state['apiSemanticsPassed'] and result['overall'] == 'ORACLE_API_E2E_BLOCKED' and result['failed'] == 0 and result['realOracleTasks'] == 0 and result['temporarySourceAbsent'] and result['originalSourceUnchanged'] and result['draftsRemaining'] == 0 and result['targetRowsUnchanged'] and result['targetStructureUnchanged'] and result['fixtureOwnerVerified'] and result['fixtureTablesAbsent'] and result['inputAbsent'] and result['submissionRejected'] and unresolved == {'DIRECT_LOAD_VERSION': 'UNKNOWN', 'WRITE_ACCESS': 'UNKNOWN', 'DIRECT_LOAD_STRUCTURE': 'UNKNOWN'}
(work / 'independent-verification.json').write_text(json.dumps(result, ensure_ascii=True, indent=2) + '\n', encoding='utf-8')
print(json.dumps({key: result[key] for key in ['overall', 'evidenceConsistent', 'passed', 'blocked', 'failed', 'realOracleTasks']}))
db.close()
if not result['evidenceConsistent']:
    sys.exit(1)
