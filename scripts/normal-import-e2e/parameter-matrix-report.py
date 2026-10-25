"""从真实测试回执生成逐参数状态；合成成功和夹具准备均不能提升真实验证状态。"""
import collections
import hashlib
import json
import pathlib
import sqlite3

ROOT = pathlib.Path(__file__).resolve().parents[2]
WORK = ROOT / 'artifacts/normal-import-full-20261020'


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def main():
    matrix_path = ROOT / 'internal/importdomain/testdata/normal-parameter-matrix.json'
    matrix = read(matrix_path)
    catalog = read(ROOT / 'internal/parammeta/normal/obloader-4.3.5-normal-v1.json')
    previous = read(ROOT / 'docs/03-technical/evidence/normal-import-parameter-coverage-2026-10-20.json')
    baseline = {row['name']: row for row in previous['parameters']}
    events = [json.loads(line) for line in (WORK / 'parameter-tests.jsonl').read_text(encoding='utf-8-sig').splitlines()]
    if any(event.get('Action') == 'fail' for event in events):
        raise ValueError('LOCAL_TEST_FAILED')
    passed = {event['Test'] for event in events if event.get('Action') == 'pass' and 'Test' in event}
    before = read(WORK / 'content-before.json')
    if before['status'] != 'PARAMETERS_INSPECTED' or not before['ownerVerified'] or len(before['records']) != 28 or not all(row['beforeMatched'] for row in before['records']) or not before['excludedTableUnchanged']:
        raise ValueError('FIXTURE_BEFORE_STATE_INVALID')
    local = read(WORK / 'fixture-local-validation.json')
    if local.get('scope') != 'LOCAL_SYNTHETIC_FILES_ONLY' or local.get('passed') is not True or len(local['records']) != 28 or not all(row['status'] == 'PASS' for row in local['records']):
        raise ValueError('LOCAL_FIXTURE_VALIDATION_FAILED')
    run = read(ROOT / 'artifacts/normal-import-retest-20261019/run.json')
    db = sqlite3.connect((ROOT / 'var/dev/control-plane/data/local-mvp.db').as_uri() + '?mode=ro', uri=True)
    try:
        task_count = db.execute("SELECT COUNT(*) FROM tasks t WHERE task_type='OBLOADER_NORMAL_IMPORT' AND EXISTS(SELECT 1 FROM json_each(t.planned_argv_json) n JOIN json_each(t.planned_argv_json) v ON v.key=n.key+1 WHERE n.value='--database' AND v.value=?)", (run['database'],)).fetchone()[0]
    finally:
        db.close()
    # 本收集器只交付尚未执行批次；已有真实任务时必须使用执行与行集核对，不能保留准备状态。
    if task_count:
        raise ValueError('REAL_RESULTS_REQUIRE_EXECUTION_COLLECTION')
    parameters = []
    for item in matrix['parameters']:
        prefix = 'Test普通导入72参数配置生成与执行边界矩阵/' + item['name']
        required = [prefix, prefix + '/类型错误', prefix + '/来源越权', prefix + '/重复参数']
        if not all(name in passed for name in required):
            raise ValueError('LOCAL_PARAMETER_EVIDENCE_MISSING')
        blocker = item['knownBlocker'] or ('ORACLE_FIXTURE_NOT_PREPARED' if item['mode'] == 'ORACLE' else 'LOGIN_REQUIRED')
        parameters.append(dict(name=item['name'], mode=item['mode'], localContractStatus='PASS',
                               localChecks=['合法配置及活动投影', '确定性命令及Agent白名单', '类型错误拒绝', '来源越权拒绝', '重复参数拒绝'] + (['选项注入拒绝'] if prefix + '/选项注入' in passed else []),
                               previousRealStatus=baseline[item['name']]['status'], previousPassedCaseIds=baseline[item['name']]['passedCaseIds'],
                               currentRealStatus='NOT_EXECUTED', primaryBlocker=blocker,
                               blockerEvidence='CURRENT_CONTRACT_AND_SOURCE' if item['knownBlocker'] else 'FIXTURE_SCOPE' if item['mode'] == 'ORACLE' else 'BROWSER_API_HTTP_401'))
    if {row['name'] for row in parameters} != {row['longName'] for row in catalog['definitions']} or len(parameters) != 72:
        raise ValueError('PARAMETER_COVERAGE_INCOMPLETE')
    report = dict(metadataVersion=matrix['metadataVersion'], definitionCount=72, localContractPassed=72,
                  localNegativeCases=sum('/--' in name and name.count('/') == 2 for name in passed),
                  localBoundaryConditionCases=sum(name.startswith('Test普通导入参数取值与条件矩阵/') for name in passed),
                  preparedRealCases=28, fixtureBeforeMatched=28, fixtureLocalValidationPassed=28,
                  currentNewRealTasks=task_count, fullParameterValidationPassed=False,
                  previousReal=dict(hasRealScenario=previous['hasRealScenario'], failedScenarioOnly=previous['failedScenarioOnly'], notExercised=previous['notExercised']),
                  primaryBlockerCounts=dict(collections.Counter(row['primaryBlocker'] for row in parameters)),
                  evidenceDigests={path.name: hashlib.sha256(path.read_bytes()).hexdigest() for path in [matrix_path, WORK / 'parameter-tests.jsonl', WORK / 'content-before.json', WORK / 'fixture-local-validation.json']},
                  meaning='本地合同验证已覆盖72条定义；当前真实验证没有新增任务。历史单场景通过、正确阻断和本地通过均不等于全参数真实通过。Oracle、非UTF8、压缩、控制文件、尾行与复杂DDL仍须按各自条件验收。',
                  parameters=parameters)
    output = ROOT / 'docs/03-technical/evidence/normal-import-full-parameter-validation-2026-10-20.json'
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({key: report[key] for key in ['definitionCount', 'localContractPassed', 'localNegativeCases', 'localBoundaryConditionCases', 'preparedRealCases', 'currentNewRealTasks', 'fullParameterValidationPassed']}, ensure_ascii=False))


if __name__ == '__main__':
    main()
