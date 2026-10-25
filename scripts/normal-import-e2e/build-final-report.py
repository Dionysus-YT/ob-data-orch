"""汇总已完成的真实批次；按定义区分 API 合同、通过场景与运行阻断，不推导全取值成功。"""
import collections
import datetime
import hashlib
import json
import pathlib
import sqlite3
import subprocess

ROOT = pathlib.Path(__file__).resolve().parents[2]
WORK = ROOT / 'artifacts/normal-import-full-20261020'
EVIDENCE = ROOT / 'docs/03-technical/evidence'


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def main():
    api = read(WORK / 'api-parameter-contracts.json')
    gates = read(WORK / 'api-capability-prechecks.json')
    retest = read(ROOT / 'artifacts/normal-import-retest-20261019/verified-results.json')
    parameter = read(WORK / 'verified-results.json')
    initial = read(WORK / 'initial-failures/verified-results.json')
    repair = read(WORK / 'verified-repair.json')
    boundaries = read(WORK / 'api-repair-contract-boundaries.json')
    run = read(ROOT / 'artifacts/normal-import-retest-20261019/run.json')
    catalog = read(ROOT / 'internal/parammeta/normal/obloader-4.3.5-normal-v1.json')
    if not api.get('done') or not gates.get('done') or len(api['records']) != 72 or len(gates['records']) != 23 or len(retest['records']) != 23 or len(parameter['records']) != 28:
        raise ValueError('REAL_BATCH_MATRIX_INCOMPLETE')
    if not retest.get('secretAuditPassed') or not parameter.get('secretAuditPassed') or not repair.get('secretAuditPassed'):
        raise ValueError('REAL_BATCH_AUDIT_MISSING')
    if api['records'] and any(row['outcome'] != 'PASS' for row in api['records'] + retest['records'] + parameter['records'] + repair['records'] + boundaries['records']):
        raise ValueError('REPAIRED_BATCH_NOT_PASSED')
    names = {row['longName'] for row in catalog['definitions']}
    if {row['name'] for row in api['records']} != names:
        raise ValueError('API_PARAMETER_COVERAGE_INVALID')
    rows = {name: dict(name=name, apiContractStatus='NOT_PASSED', localContractStatus='PASS', passedRealCaseIds=[], failedRealCaseIds=[], runtimeBranches=[]) for name in names}
    for item in api['records']:
        rows[item['name']]['apiContractStatus'] = item['outcome']
    db = sqlite3.connect((ROOT / 'var/dev/control-plane/data/local-mvp.db').as_uri() + '?mode=ro', uri=True)
    tasks = {}
    try:
        for case in retest['records'] + parameter['records'] + repair['records']:
            if not case.get('taskId'):
                continue
            task = db.execute('SELECT planned_argv_json,task_type FROM tasks WHERE task_id=?', (case['taskId'],)).fetchone()
            if not task or task[1] != 'OBLOADER_NORMAL_IMPORT':
                raise ValueError('TASK_BINDING_INVALID')
            argv = json.loads(task[0])
            if argv[argv.index('--database') + 1] != run['database']:
                raise ValueError('TASK_OWNERSHIP_INVALID')
            emitted = {value.split('=', 1)[0] for value in argv} & names
            if case.get('persistedEvidence', {}).get('result') == 'VERIFIED':
                emitted.add('--password')
            tasks[case['taskId']] = case.get('persistedEvidence', {}).get('state')
            for name in emitted:
                rows[name]['passedRealCaseIds' if case['outcome'] == 'PASS' else 'failedRealCaseIds'].append(case['id'])
        # 初轮失败不能被复验覆盖；总任务数包含被替换的三条历史执行。
        for case in initial['records']:
            task = db.execute('SELECT t.planned_argv_json,e.state FROM tasks t JOIN task_executions e ON e.task_id=t.task_id WHERE t.task_id=?', (case['taskId'],)).fetchone()
            argv = json.loads(task[0])
            if argv[argv.index('--database') + 1] != run['database']:
                raise ValueError('INITIAL_TASK_OWNERSHIP_INVALID')
            tasks[case['taskId']] = task[1]
            if case['outcome'] == 'FAIL':
                for name in {value.split('=', 1)[0] for value in argv} & names:
                    rows[name]['failedRealCaseIds'].append(case['id'])
        for case in gates['records']:
            count = db.execute('SELECT COUNT(*) FROM tasks WHERE precheck_id=?', (case.get('precheckId', ''),)).fetchone()[0]
            case['noTaskCreated'] = count == 0
            if count:
                raise ValueError('UNSUPPORTED_GATE_TASK_CREATED')
            for name in case['names']:
                rows[name]['runtimeBranches'].append(dict(id=case['id'], outcome=case['outcome'], checkStatus=case.get('checkStatus'), code=case.get('code'), submitStatus=case.get('submitStatus'), noTaskCreated=True,
                    checks=[{key: item.get(key) for key in ['check', 'status', 'evidenceCode']} for item in case.get('checks', [])]))
    finally:
        db.close()
    for row in rows.values():
        row['passedRealCaseIds'] = sorted(set(row['passedRealCaseIds']))
        row['failedRealCaseIds'] = sorted(set(row['failedRealCaseIds']))
        row['realScenarioStatus'] = 'HAS_VERIFIED_SCENARIO' if row['passedRealCaseIds'] else 'NOT_VERIFIED'
        row['allValuesAndConditionsValidated'] = False
    report = dict(metadataVersion=catalog['metadataVersion'], scope='REAL_API_TEST_ODP_WINDOWS_AMD64_LOCAL_NORMAL_IMPORT', observedAt=datetime.datetime.now(datetime.timezone.utc).isoformat(), definitionCount=72,
        apiContractCounts=dict(collections.Counter(item['outcome'] for item in api['records'])), regressionCounts=retest['counts'], initialParameterScenarioCounts=initial['counts'], parameterScenarioCounts=parameter['counts'], repairCounts=repair['counts'], compatibilityBoundaryCounts=dict(collections.Counter(item['outcome'] for item in boundaries['records'])), gateCounts=dict(collections.Counter(item['outcome'] for item in gates['records'])),
        resolvedFailures=[dict(id=item['id'], initialTaskId=item['taskId'], initialOutcome=item['outcome'], repairedTaskId=next(row['taskId'] for row in parameter['records'] if row['id'] == item['id'])) for item in initial['records'] if item['outcome'] == 'FAIL'],
        hasVerifiedRealScenario=sum(bool(row['passedRealCaseIds']) for row in rows.values()), noVerifiedRealScenario=sum(not row['passedRealCaseIds'] for row in rows.values()),
        realTaskCount=len(tasks), realTaskStates=dict(collections.Counter(tasks.values())), gateTasksCreated=0,
        fullParameterValidationPassed=False, meaning='API合同覆盖72条定义；真实通过场景只证明本轮小型取值与条件。正确拒绝未支持配置、只读预检通过、参数出现和工具退出0均不单独构成全参数/全取值通过。',
        parameters=[rows[item['longName']] for item in catalog['definitions']],
        evidenceDigests={str(path.relative_to(ROOT)).replace('\\','/'): hashlib.sha256(path.read_bytes()).hexdigest() for path in [WORK/'api-parameter-contracts.json',WORK/'api-capability-prechecks.json',ROOT/'artifacts/normal-import-retest-20261019/verified-results.json',WORK/'verified-results.json',WORK/'initial-failures/verified-results.json',WORK/'verified-repair.json',WORK/'api-repair-contract-boundaries.json']})
    raw = json.dumps(report, ensure_ascii=False, indent=2).encode('utf-8')
    audited_content = b'\n'.join([raw] + [json.dumps(item,ensure_ascii=False).encode('utf-8') for item in [gates,retest,parameter,initial,repair,boundaries]])
    audit = subprocess.run([str(WORK/'fixture.exe'),'-action','audit','-database',run['database'],'-run-id',run['runId']],input=audited_content,capture_output=True,cwd=ROOT,check=True)
    if json.loads(audit.stdout)['plaintextOrRecoverableEncodingPresent']:
        raise ValueError('SECRET_AUDIT_FAILED')
    report['secretAuditPassed'] = True
    (EVIDENCE/'normal-import-full-parameter-validation-2026-10-20.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    (EVIDENCE/'normal-import-api-regression-2026-10-20-results.json').write_text(json.dumps(retest,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    (EVIDENCE/'normal-import-api-parameters-2026-10-20-results.json').write_text(json.dumps(parameter,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    (EVIDENCE/'normal-import-api-gates-2026-10-20-results.json').write_text(json.dumps(gates,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    (EVIDENCE/'normal-import-api-initial-parameters-2026-10-20-results.json').write_text(json.dumps(initial,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    (EVIDENCE/'normal-import-api-repair-2026-10-20-results.json').write_text(json.dumps(repair,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    (EVIDENCE/'normal-import-api-contracts-2026-10-20-results.json').write_text(json.dumps(dict(contracts=api,compatibilityBoundaries=boundaries),ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({key:report[key] for key in ['apiContractCounts','regressionCounts','parameterScenarioCounts','gateCounts','hasVerifiedRealScenario','noVerifiedRealScenario','realTaskCount','realTaskStates','fullParameterValidationPassed']},ensure_ascii=False))


if __name__ == '__main__':
    main()
