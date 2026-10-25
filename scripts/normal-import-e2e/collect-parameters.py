"""只读核对参数批次的真实任务、受控进程、持久事件与独立目标行集。"""
import collections
import json
import pathlib
import sqlite3
import subprocess

import parameter_fixture

ROOT = pathlib.Path(__file__).resolve().parents[2]
WORK = ROOT / 'artifacts/normal-import-full-20261020'


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def main():
    run = read(ROOT / 'artifacts/normal-import-retest-20261019/run.json')
    browser = read(WORK / 'browser-parameters.json')
    content = read(WORK / 'content-after.json')
    before = read(WORK / 'content-before.json')
    expected = {row['id']: row for row in parameter_fixture.cases()}
    if browser.get('done') is not True or {row['id'] for row in browser['records']} != set(expected) or len(browser['records']) != 28:
        raise ValueError('PARAMETER_BATCH_INCOMPLETE')
    if content.get('ownerVerified') is not True or before.get('ownerVerified') is not True or not all(row['beforeMatched'] for row in before['records']):
        raise ValueError('PARAMETER_FIXTURE_OWNERSHIP_INVALID')
    contents = {row['id']: row for row in content['records']}
    db = sqlite3.connect((ROOT / 'var/dev/control-plane/data/local-mvp.db').as_uri() + '?mode=ro', uri=True)
    records = []
    try:
        for raw in browser['records']:
            record = dict(raw)
            record['outcome'] = 'FAIL'
            item = expected[record['id']]
            result = contents[record['id']]
            record['content'] = result
            row = db.execute('SELECT t.task_type,t.snapshot_version,t.config_fingerprint,t.planned_argv_json,e.execution_id,e.state,e.reconciliation_required,e.result_summary_json,e.process_evidence_json FROM tasks t JOIN task_executions e ON e.task_id=t.task_id WHERE t.task_id=?', (record.get('taskId', ''),)).fetchone()
            if row:
                if row[0] != 'OBLOADER_NORMAL_IMPORT' or row[1] != 'normal-import-v1':
                    raise ValueError('PARAMETER_TASK_BINDING_INVALID')
                argv, summary, process = json.loads(row[3]), json.loads(row[7]), json.loads(row[8])
                # 必须确认任务属于本 run 的独立库，不靠用例名称接管其他任务。
                try:
                    bound = argv[argv.index('--database') + 1] == run['database']
                except (ValueError, IndexError):
                    bound = False
                if not bound:
                    raise ValueError('PARAMETER_TASK_OWNERSHIP_INVALID')
                events = collections.Counter(value[0] for value in db.execute('SELECT event_type FROM execution_events WHERE execution_id=? AND accepted=1', (row[4],)))
                names = {token.split('=', 1)[0] for token in argv}
                evidence = dict(state=row[5], result=summary.get('result'), normalImport=summary.get('normalImport') is True,
                                processExitCode=process.get('exitCode'), plannedActualMatch=process.get('plannedActualMatch'),
                                reconciliationRequired=bool(row[6]), snapshotMatched=row[2] == record.get('fingerprint'),
                                parametersMatched=set(item['names']).issubset(names), processStartedEvents=events['PROCESS_STARTED'], resultEvents=events['RESULT_FACTS_OBSERVED'])
                record['persistedEvidence'] = evidence
                risk = item['patch'].get('replaceData') or item['patch'].get('replaceObject') or item['patch'].get('cleanup') in ['DELETE', 'TRUNCATE']
                passed = evidence['state'] == 'SUCCEEDED' and evidence['result'] == 'VERIFIED' and evidence['normalImport'] and evidence['processExitCode'] == 0 and evidence['plannedActualMatch'] == 'MATCHED' and not evidence['reconciliationRequired'] and evidence['snapshotMatched'] and evidence['parametersMatched'] and evidence['processStartedEvents'] == 1 and evidence['resultEvents'] == 1
                passed = passed and record.get('commandMatched') and record.get('snapshotMatched') and record.get('submitIdempotent') and record.get('successLog') and result.get('contentMatched') and result.get('structureMatched') and (not risk or record.get('unconfirmedRiskRejected'))
                if passed:
                    record['outcome'] = 'PASS'
            records.append(record)
    finally:
        db.close()
    if content.get('excludedTableUnchanged') is not True:
        for record in records:
            if record['id'] == 'NI-PARAM-exclude':
                record.update(outcome='FAIL', code='EXCLUDED_TABLE_CHANGED')
    report = dict(runId=run['runId'], scope='REAL_API_TEST_MYSQL_ODP_WINDOWS_AMD64', ownerVerified=True, excludedTableUnchanged=content['excludedTableUnchanged'], records=records, counts=dict(collections.Counter(row['outcome'] for row in records)))
    raw = json.dumps(report, ensure_ascii=False, indent=2).encode('utf-8')
    audit = subprocess.run([str(WORK / 'fixture.exe'), '-action', 'audit', '-database', run['database'], '-run-id', run['runId']], input=raw, capture_output=True, cwd=ROOT, check=True)
    if json.loads(audit.stdout)['plaintextOrRecoverableEncodingPresent']:
        raise ValueError('SECRET_AUDIT_FAILED')
    report['secretAuditPassed'] = True
    (WORK / 'verified-results.json').write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
    print(json.dumps(dict(counts=report['counts'], secretAuditPassed=True)))


if __name__ == '__main__':
    main()
