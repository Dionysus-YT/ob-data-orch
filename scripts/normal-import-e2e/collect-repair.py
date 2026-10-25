"""只读复核本次修复批次：完整目标结构/行集、精确取值、事件与拒绝后零任务。"""
import collections
import json
import pathlib
import sqlite3
import subprocess

ROOT = pathlib.Path(__file__).resolve().parents[2]
WORK = ROOT / 'artifacts/normal-import-full-20261020'


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def main():
    run = read(ROOT / 'artifacts/normal-import-retest-20261019/run.json')
    batch = read(WORK / 'api-repair-batch.json')
    content = read(WORK / 'content-after.json')
    assert batch.get('done') and len(batch['records']) == 7 and content['ownerVerified']
    contents = {row['id']: row for row in content['records']}
    versions = {'NI-PARAM-compat': 'mysql80', 'NI-PARAM-compat_fivesix': 'mysql56', 'NI-PARAM-compat_fiveseven': 'mysql57'}
    db = sqlite3.connect((ROOT / 'var/dev/control-plane/data/local-mvp.db').as_uri() + '?mode=ro', uri=True)
    records = []
    try:
        for raw in batch['records']:
            row = dict(raw, outcome='FAIL')
            if row['id'].startswith('NI-PARAM-cut_'):
                no_task = db.execute('SELECT COUNT(*) FROM tasks WHERE precheck_id=?', (row['precheckId'],)).fetchone()[0] == 0
                unknown = any(item['check'] == 'INPUT_MAPPING' and item['status'] == 'UNKNOWN' and item['evidenceCode'] == 'IMPORT_INPUT_MAPPING_UNSUPPORTED' for item in row['checks'])
                row['noTaskCreated'] = no_task
                if raw['outcome'] == 'PASS' and row['checkStatus'] == 'FAILED' and row['submitStatus'] == 422 and no_task and unknown:
                    row['outcome'] = 'PASS'
            else:
                fact = db.execute('SELECT t.planned_argv_json,t.config_fingerprint,e.execution_id,e.state,e.reconciliation_required,e.result_summary_json,e.process_evidence_json FROM tasks t JOIN task_executions e ON e.task_id=t.task_id WHERE t.task_id=? AND t.task_type=?', (row.get('taskId',''), 'OBLOADER_NORMAL_IMPORT')).fetchone()
                assert fact
                argv, summary, process = json.loads(fact[0]), json.loads(fact[5]), json.loads(fact[6])
                assert argv[argv.index('--database') + 1] == run['database']
                target = 'NI-PARAM-compat' if row['id'] in versions else row['id']
                result = contents[target]
                events = collections.Counter(item[0] for item in db.execute('SELECT event_type FROM execution_events WHERE execution_id=? AND accepted=1', (fact[2],)))
                value_matched = argv[argv.index('--compat-mode') + 1] == versions[row['id']] if row['id'] in versions else argv[argv.index('--line-separator') + 1] == r'\n'
                evidence = dict(state=fact[3], result=summary.get('result'), normalImport=summary.get('normalImport') is True, processExitCode=process.get('exitCode'), plannedActualMatch=process.get('plannedActualMatch'), reconciliationRequired=bool(fact[4]), snapshotMatched=fact[1] == row.get('fingerprint'), exactValueMatched=value_matched, processStartedEvents=events['PROCESS_STARTED'], resultEvents=events['RESULT_FACTS_OBSERVED'])
                row.update(persistedEvidence=evidence, content=result)
                passed = fact[3] == 'SUCCEEDED' and not fact[4] and summary.get('result') == 'VERIFIED' and evidence['normalImport'] and process.get('exitCode') == 0 and process.get('plannedActualMatch') == 'MATCHED' and evidence['snapshotMatched'] and value_matched and events['PROCESS_STARTED'] == 1 and events['RESULT_FACTS_OBSERVED'] == 1
                passed = passed and raw.get('commandMatched') and raw.get('snapshotMatched') and raw.get('submitIdempotent') and raw.get('successLog') and result['contentMatched'] and result['structureMatched']
                if row['id'].startswith('NI-PARAM-compat_'):
                    passed = passed and raw.get('unconfirmedRiskRejected')
                if passed:
                    row['outcome'] = 'PASS'
            records.append(row)
    finally:
        db.close()
    report = dict(runId=run['runId'], scope='REAL_API_REPAIR_AND_DIALECT_VARIANTS', records=records, counts=dict(collections.Counter(row['outcome'] for row in records)))
    audit = subprocess.run([str(WORK/'fixture.exe'),'-action','audit','-database',run['database'],'-run-id',run['runId']], input=json.dumps(report,ensure_ascii=False).encode(), capture_output=True,cwd=ROOT,check=True)
    assert json.loads(audit.stdout)['plaintextOrRecoverableEncodingPresent'] is False
    report['secretAuditPassed'] = True
    (WORK/'verified-repair.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps(report['counts']))
    assert report['counts'].get('PASS') == 7


if __name__ == '__main__':
    main()
