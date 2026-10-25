"""逐项结合真实 API、独立目标事实与持久事件，失败不能升级为成功。"""
import collections
import datetime
import json
import pathlib
import sqlite3
import subprocess

ROOT = pathlib.Path(__file__).resolve().parents[2]
WORK = ROOT / 'artifacts/normal-import-capabilities-20261020'
EVIDENCE = ROOT / 'docs/03-technical/evidence'


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def main():
    run = read(WORK / 'run.json')
    db = sqlite3.connect((ROOT / 'var/dev/control-plane/data/local-mvp.db').as_uri() + '?mode=ro', uri=True)
    records = []
    try:
        for stage, mode, expected_count in [('', 'mysql', 12), ('', 'oracle', 7), ('retry-', 'mysql', 3), ('retry-', 'oracle', 4)]:
            if stage and not (WORK / (stage + mode + '-api.json')).exists():
                continue
            api, before, after = [read(WORK / (stage + mode + suffix + '.json')) for suffix in ['-api', '-before', '-after']]
            assert api['done'] and len(api['records']) == expected_count and not api.get('stopped')
            assert before['ownerVerified'] and after['ownerVerified']
            initial = {r['key']: r for r in before['records']}
            contents = {r['key']: r for r in after['records']}
            cases = {r['id']: r for r in read(WORK / (stage + mode + '-cases.json'))}
            for raw in api['records']:
                key = raw['id'][9:]
                if not stage and mode == 'oracle' and key == 'role' and (WORK / 'retry-oracle-after.json').exists():
                    contents[key] = next(r for r in read(WORK / 'retry-oracle-after.json')['records'] if r['key'] == 'role')
                row = dict(raw, outcome='FAIL', mode=mode.upper(), stage='RETRY' if stage else 'INITIAL', content=contents[key])
                row['initialTargetEmptyOrAbsent'] = initial[key].get('actualRows') == 0 or initial[key].get('exists') is False
                fact = db.execute('SELECT t.data_source_id,t.node_id,t.config_fingerprint,t.planned_argv_json,t.snapshot_version,e.execution_id,e.state,e.reconciliation_required,e.result_summary_json,e.process_evidence_json FROM tasks t JOIN task_executions e ON e.task_id=t.task_id WHERE t.task_id=? AND t.task_type=?', (raw.get('taskId', ''), 'OBLOADER_NORMAL_IMPORT')).fetchone()
                if fact:
                    argv, summary, process = json.loads(fact[3]), json.loads(fact[8]), json.loads(fact[9])
                    case = cases[raw['id']]['input']['config']
                    names = raw['parameterNames']
                    parameters_present = all(name in argv for name in names)
                    explicit_values = {'ctl_path': ('--ctl-path', case['mapping'].get('ctlPath')), 'exclude_data_types': ('--exclude-data-types', case['mapping'].get('excludeDataTypes')), 'compression_algo': ('--compression-algo', case['parser'].get('compressionAlgo')), 'compression_level': ('--compression-level', case['parser'].get('compressionLevel'))}
                    exact_values = key not in explicit_values or argv[argv.index(explicit_values[key][0]) + 1] == explicit_values[key][1]
                    objects_matched = all(argv[argv.index('--' + kind) + 1] == value for kind, value in case['objects'].items())
                    file_scope_matched = argv[argv.index('--file-path') + 1] == case['filePath']
                    snapshot = json.loads(db.execute('SELECT snapshot_json FROM tasks WHERE task_id=?', (raw['taskId'],)).fetchone()[0])
                    source_id = run['sourceId'] if mode == 'mysql' else run['oracleSourceId']
                    source = db.execute('SELECT default_database,username FROM data_sources WHERE data_source_id=? AND environment=? AND compatibility_mode=? AND state=?', (source_id, 'TEST', mode.upper(), 'ENABLED')).fetchone()
                    assert source
                    database = run['database'] if mode == 'mysql' else source[0] or source[1].upper()
                    target_matched = argv[argv.index('--database') + 1] == database and snapshot['config']['database'] == database
                    active = {section: dict(case.get(section, {})) for section in ['parser', 'mapping', 'execution', 'objects']}
                    if case['content'] == 'DDL_ONLY':
                        # 与领域契约一致：仅结构任务清除数据解析/映射及数据执行选项。
                        active['parser'], active['mapping'] = {}, {}
                        active['execution'] = {k: v for k, v in active['execution'].items() if k in ['thread', 'memory', 'nlsDateFormat', 'nlsTimestampFormat', 'nlsTimestampTzFormat']}
                    projection_matched = all(snapshot['config'].get(section, {}).get(field) == value for section, values in active.items() for field, value in values.items())
                    control_bound = key != 'ctl_path' or len(snapshot['inputManifest'].get('controlDigest', '')) == 64
                    events = collections.Counter(r[0] for r in db.execute('SELECT event_type FROM execution_events WHERE execution_id=? AND accepted=1', (fact[5],)))
                    evidence = dict(state=fact[6], normalImport=summary.get('normalImport') is True, result=summary.get('result'), processExitCode=process.get('exitCode'), plannedActualMatch=process.get('plannedActualMatch'), reconciliationRequired=bool(fact[7]), snapshotMatched=fact[2] == raw['fingerprint'], parameterNamesMatched=parameters_present, objectsMatched=objects_matched, ownedInputMatched=file_scope_matched, processStartedEvents=events['PROCESS_STARTED'], resultEvents=events['RESULT_FACTS_OBSERVED'])
                    row['persistedEvidence'] = evidence
                    source_id = run['sourceId'] if mode == 'mysql' else run['oracleSourceId']
                    passed = fact[0] == source_id and fact[1] == run['nodeId'] and fact[4] == 'normal-import-v1' and fact[6] == 'SUCCEEDED' and not fact[7] and summary.get('normalImport') is True and summary.get('result') == 'VERIFIED' and process.get('exitCode') == 0 and process.get('plannedActualMatch') == 'MATCHED'
                    evidence['exactParameterValuesMatched'] = exact_values
                    evidence.update(targetDatabaseMatched=target_matched, frozenParameterValuesMatched=projection_matched, controlManifestBound=control_bound)
                    passed = passed and exact_values and target_matched and projection_matched and control_bound and all(evidence[v] for v in ['snapshotMatched', 'parameterNamesMatched', 'objectsMatched', 'ownedInputMatched']) and events['PROCESS_STARTED'] == events['RESULT_FACTS_OBSERVED'] == 1
                    passed = passed and all(raw.get(v) for v in ['commandMatched', 'snapshotMatched', 'submitIdempotent', 'successLog']) and row['initialTargetEmptyOrAbsent'] and contents[key]['contentMatched'] and contents[key]['structureMatched']
                    if key in ['obj_user', 'role']:
                        passed = passed and raw.get('unconfirmedRiskRejected') is True
                    if passed:
                        row['outcome'] = 'PASS'
                records.append(row)
        negatives = read(WORK / 'negative-api.json') if (WORK / 'negative-api.json').exists() else None
        if negatives:
            assert negatives['done'] and len(negatives['records']) == 8
            if (WORK / 'negative-retry-api.json').exists():
                retried = read(WORK / 'negative-retry-api.json')
                assert retried['done'] and len(retried['records']) == 2
                negatives = dict(negatives, initialRecords=negatives['records'], records=list({r['id']: r for r in negatives['records'] + retried['records']}.values()))
            for row in negatives['records']:
                assert row['outcome'] == 'PASS' and not row.get('taskId') and db.execute('SELECT COUNT(*) FROM tasks WHERE precheck_id=?', (row['precheckId'],)).fetchone()[0] == 0
    finally:
        db.close()
    attempts = records
    initial_records = [r for r in attempts if r['stage'] == 'INITIAL']
    records = list({r['id']: r for r in attempts}.values())
    report = dict(initialRecords=initial_records, initialCounts=dict(collections.Counter(r['outcome'] for r in initial_records)), taskCount=len({r['taskId'] for r in attempts if r.get('taskId')}), negatives=negatives, runId=run['runId'], observedAt=datetime.datetime.now(datetime.timezone.utc).isoformat(), scope='REAL_API_TEST_MYSQL_ORACLE_ODP_WINDOWS_AMD64', records=records, counts=dict(collections.Counter(r['outcome'] for r in records)), hasVerifiedRemainingDefinitions=sum(r['outcome'] == 'PASS' for r in records), allRemainingRepresentativeScenariosPassed=all(r['outcome'] == 'PASS' for r in records), meaning='逐定义代表场景，不等于全部取值/语法/格式/权限/平台组合。')
    if (WORK / 'final-mysql-content.json').exists():
        mysql, oracle = read(WORK / 'final-mysql-content.json'), read(WORK / 'final-oracle-content.json')
        assert mysql['ownerVerified'] and oracle['ownerVerified']
        assert all(r['contentMatched'] and r['structureMatched'] for r in mysql['records'] if r['key'] != 'trigger')
        assert all(r['contentMatched'] and r['structureMatched'] for r in oracle['records'])
        assert mysql['diagnostic']['referenceTableVisible']
        report['targetsUnchangedAfterRejectedCases'] = True
    for name in ['api-closeout', 'mysql-cleanup', 'oracle-cleanup', 'database-cleanup', 'directory-cleanup', 'service-after']:
        if (WORK / (name + '.json')).exists():
            report[name] = read(WORK / (name + '.json'))
    if 'directory-cleanup' in report:
        assert report['directory-cleanup']['directoryAbsent'] and not pathlib.Path(report['directory-cleanup']['root']).exists()
        assert report['database-cleanup']['databaseAbsent'] and report['database-cleanup']['ownerVerified']
        assert report['mysql-cleanup']['globalUserAbsent'] and report['mysql-cleanup']['globalGroupAbsent'] and report['mysql-cleanup']['ownerVerified']
        assert report['oracle-cleanup']['objectsAbsent'] and report['oracle-cleanup']['ownerVerified']
        closing, service = report['api-closeout'], report['service-after']
        assert closing['health'] == 200 and closing['heartbeatStatus'] == 'ONLINE' and closing['environmentCheckStatus'] == 'PASSED' and closing['acceptsNewTasks']
        assert all(closing['taskSummary'][k] == 0 for k in ['running', 'waiting', 'reconciling']) and all(closing['agentLogQueue'][k] == 0 for k in ['pendingBatches', 'pendingGaps', 'pendingBytes'])
        assert all(r['matched'] for r in service['staticMaterials']) and all(service['runFlags'].values()) and all(service['currentSource'].values())
        report['closeoutVerified'] = True
    raw = json.dumps(report, ensure_ascii=False, indent=2).encode('utf-8')
    for action in ['audit', 'import-oracle-audit']:
        audit = subprocess.run([str(WORK / 'fixture.exe'), '-action', action, '-database', run['database'], '-run-id', run['runId']], input=raw, capture_output=True, cwd=ROOT, check=True)
        assert not json.loads(audit.stdout)['plaintextOrRecoverableEncodingPresent']
    report['mysqlAndOracleSecretAuditsPassed'] = True
    for path in [WORK / 'verified-results.json', EVIDENCE / 'normal-import-capabilities-2026-10-20-results.json']:
        path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(report['counts']))


if __name__ == '__main__':
    main()
