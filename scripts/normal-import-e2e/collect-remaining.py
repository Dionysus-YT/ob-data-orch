"""独立收集双数据源剩余场景；工具终态、完整目标事实与阻断分开计数。"""
import collections
import datetime
import json
import pathlib
import sqlite3
import subprocess

ROOT = pathlib.Path(__file__).resolve().parents[2]
WORK = ROOT/'artifacts/normal-import-remaining-20261020'
EVIDENCE = ROOT/'docs/03-technical/evidence'


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def main():
    run, api = read(WORK/'run.json'), read(WORK/'oracle-api.json')
    before, after = read(WORK/'oracle-content-before.json'), read(WORK/'oracle-content-after.json')
    gates = read(WORK/'api-capability-prechecks.json')
    assert api.get('done') and len(api['records']) == 3 and gates.get('done') and len(gates['records']) == 19
    assert before['ownerVerified'] and after['ownerVerified'] and all(row['actualRows'] == 0 and row['structureMatched'] for row in before['records'])
    contents = {row['key']: row for row in after['records']}
    expected = {
        'NI-PARAM-nls_date': ('DATE','--nls-date-format','DD/MM/YYYY HH24:MI:SS'),
        'NI-PARAM-nls_timestamp': ('TIMESTAMP','--nls-timestamp-format','DD/MM/YYYY HH24:MI:SS.FF6'),
        'NI-PARAM-nls_timezone': ('TZ','--nls-timestamp-tz-format','DD/MM/YYYY HH24:MI:SS.FF6 TZH:TZM'),
    }
    db = sqlite3.connect((ROOT/'var/dev/control-plane/data/local-mvp.db').as_uri()+'?mode=ro',uri=True)
    records = []
    try:
        source = db.execute('SELECT default_database,username FROM data_sources WHERE data_source_id=? AND environment=? AND compatibility_mode=? AND connection_kind=? AND state=?',(run['oracleSourceId'],'TEST','ORACLE','ODP','ENABLED')).fetchone()
        assert source
        schema = source[0] or source[1].upper()
        for raw in api['records']:
            row = dict(raw, outcome='FAIL')
            suffix, name, value = expected[row['id']]
            row['content'] = contents[suffix]
            fact = db.execute('SELECT t.data_source_id,t.node_id,t.config_fingerprint,t.planned_argv_json,t.snapshot_version,e.execution_id,e.state,e.reconciliation_required,e.result_summary_json,e.process_evidence_json FROM tasks t JOIN task_executions e ON e.task_id=t.task_id WHERE t.task_id=? AND t.task_type=?',(row.get('taskId',''),'OBLOADER_NORMAL_IMPORT')).fetchone()
            if fact:
                argv, summary, process = json.loads(fact[3]), json.loads(fact[8]), json.loads(fact[9])
                assert fact[0] == run['oracleSourceId'] and fact[1] == run['nodeId'] and fact[4] == 'normal-import-v1'
                assert argv[argv.index('--database')+1] == schema and argv[argv.index('--table')+1] == 'NIVR_'+run['runId'][:12].upper()+'_'+suffix
                events = collections.Counter(value[0] for value in db.execute('SELECT event_type FROM execution_events WHERE execution_id=? AND accepted=1',(fact[5],)))
                evidence = dict(state=fact[6], result=summary.get('result'), normalImport=summary.get('normalImport') is True, processExitCode=process.get('exitCode'), plannedActualMatch=process.get('plannedActualMatch'), reconciliationRequired=bool(fact[7]), snapshotMatched=fact[2] == row.get('fingerprint'), exactValueMatched=argv[argv.index(name)+1] == value, processStartedEvents=events['PROCESS_STARTED'], resultEvents=events['RESULT_FACTS_OBSERVED'])
                row['persistedEvidence'] = evidence
                passed = fact[6] == 'SUCCEEDED' and not fact[7] and summary.get('result') == 'VERIFIED' and evidence['normalImport'] and evidence['snapshotMatched'] and evidence['exactValueMatched'] and process.get('exitCode') == 0 and process.get('plannedActualMatch') == 'MATCHED' and events['PROCESS_STARTED'] == events['RESULT_FACTS_OBSERVED'] == 1
                passed = passed and raw.get('commandMatched') and raw.get('snapshotMatched') and raw.get('submitIdempotent') and raw.get('successLog') and contents[suffix]['contentMatched'] and contents[suffix]['structureMatched']
                if passed:
                    row['outcome'] = 'PASS'
            records.append(row)
        for row in gates['records']:
            count = db.execute('SELECT COUNT(*) FROM tasks WHERE precheck_id=?',(row.get('precheckId',''),)).fetchone()[0]
            assert count == 0
            row['noTaskCreated'] = True
            assert row['outcome'] in ['BLOCKED_PREFLIGHT_ONLY','BLOCKED_WITH_SUBMIT_REJECTION_VERIFIED']
    finally:
        db.close()
    report = dict(runId=run['runId'],observedAt=datetime.datetime.now(datetime.timezone.utc).isoformat(),scope='REAL_API_TEST_MYSQL_ORACLE_ODP_WINDOWS_AMD64',oracleRecords=records,oracleCounts=dict(collections.Counter(row['outcome'] for row in records)),gates=gates,gateCounts=dict(collections.Counter(row['outcome'] for row in gates['records'])),gateTasksCreated=0,independentOracleBeforeMatched=True,diagnostic=read(WORK/'oracle-jdbc-diagnostic.json'),fullParameterValidationPassed=False)
    if (WORK/'directory-cleanup.json').exists() and (WORK/'api-closeout.json').exists():
        mysql, oracle, directory = read(WORK/'mysql-cleanup.json'), read(WORK/'oracle-cleanup.json'), read(WORK/'directory-cleanup.json')
        services, closing = read(WORK/'service-after.json'), read(WORK/'api-closeout.json')
        assert mysql['ownerVerified'] and mysql['databaseAbsent'] and oracle['ownerVerified'] and oracle['tablesAbsent'] and directory['directoryAbsent'] and not pathlib.Path(directory['root']).exists()
        assert closing['health'] == 200 and closing['environmentCheckStatus'] == 'PASSED' and closing['heartbeatStatus'] == 'ONLINE'
        assert closing['taskSummary']['running'] == closing['taskSummary']['waiting'] == closing['taskSummary']['reconciling'] == 0
        assert closing['agentLogQueue']['pendingBatches'] == closing['agentLogQueue']['pendingGaps'] == closing['agentLogQueue']['pendingBytes'] == 0
        assert all(row['matched'] for row in services['staticMaterials']) and all(row.get('cleanupStatus') == 200 for row in gates['records'])
        service_before = read(WORK/'service-before.json')
        prior = {row['path']: row['sha256'] for row in service_before['binaries']}
        # 本次只改 Agent 内嵌探针；控制面重建后的确定性字节可保持相同，不能据此误判安装失败。
        assert all(datetime.datetime.fromisoformat(row['builtAt']) >= datetime.datetime.fromisoformat(service_before['at']) for row in services['binaries'])
        assert all(prior[row['path']] != row['sha256'] for row in services['binaries'] if row['path'].endswith('/agent.exe'))
        report['closeout'] = dict(mysql=mysql,oracle=oracle,directory=directory,services=services,api=closing)
    raw = json.dumps(report,ensure_ascii=False,indent=2).encode('utf-8')
    for action in ['audit','import-oracle-audit']:
        audit = subprocess.run([str(WORK/'fixture.exe'),'-action',action,'-database',run['database'],'-run-id',run['runId']],input=raw,capture_output=True,cwd=ROOT,check=True)
        assert json.loads(audit.stdout)['plaintextOrRecoverableEncodingPresent'] is False
    report['mysqlAndOracleSecretAuditsPassed'] = True
    for path in [WORK/'verified-results.json',EVIDENCE/'normal-import-remaining-2026-10-20-results.json']:
        path.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({key:report[key] for key in ['oracleCounts','gateCounts','gateTasksCreated','mysqlAndOracleSecretAuditsPassed']}))


if __name__ == '__main__':
    main()
