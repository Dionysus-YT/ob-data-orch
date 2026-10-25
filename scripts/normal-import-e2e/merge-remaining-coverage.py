"""以已独立核对的新批次补充现役覆盖结论，保留上一批次摘要和历史阻断。"""
import collections
import datetime
import hashlib
import json
import pathlib
import shutil
import subprocess

ROOT = pathlib.Path(__file__).resolve().parents[2]
WORK = ROOT/'artifacts/normal-import-remaining-20261020'
EVIDENCE = ROOT/'docs/03-technical/evidence'


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def main():
    update, run = read(WORK/'verified-results.json'), read(WORK/'run.json')
    assert update['oracleCounts'] == {'PASS': 3} and update['mysqlAndOracleSecretAuditsPassed']
    assert update['gateCounts'] == {'BLOCKED_WITH_SUBMIT_REJECTION_VERIFIED': 17, 'BLOCKED_PREFLIGHT_ONLY': 2}
    target = EVIDENCE/'normal-import-full-parameter-validation-2026-10-20.json'
    baseline = EVIDENCE/'normal-import-full-parameter-validation-2026-10-20-initial-summary.json'
    if not baseline.exists():
        assert read(target)['hasVerifiedRealScenario'] == 50
        shutil.copyfile(target,baseline)
    report = read(baseline)
    rows = {row['name']: row for row in report['parameters']}
    for case in update['oracleRecords']:
        for name in case['parameterNames']:
            row = rows[name]
            row['passedRealCaseIds'] = sorted(set(row['passedRealCaseIds'] + [case['id']]))
            row['realScenarioStatus'] = 'HAS_VERIFIED_SCENARIO'
    for case in update['gates']['records']:
        for name in case['names']:
            rows[name]['runtimeBranches'].append(dict(id=case['id'],runId=run['runId'],outcome=case['outcome'],checkStatus=case.get('checkStatus'),submitStatus=case.get('submitStatus'),noTaskCreated=case['noTaskCreated'],checks=[{key:item.get(key) for key in ['check','status','evidenceCode']} for item in case['checks']]))
    report.update(scope='REAL_API_TEST_MYSQL_ORACLE_ODP_WINDOWS_AMD64_LOCAL_NORMAL_IMPORT',baselineObservedAt=report['observedAt'],observedAt=update['observedAt'],updatedAt=datetime.datetime.now(datetime.timezone.utc).isoformat(),hasVerifiedRealScenario=53,noVerifiedRealScenario=19,realTaskCount=45,realTaskStates={'SUCCEEDED':41,'FAILED':4},remainingGateCounts=update['gateCounts'],oracleScenarioCounts=update['oracleCounts'],fullParameterValidationPassed=False,
                  continuation=dict(runId=run['runId'],results='normal-import-remaining-2026-10-20-results.json',baselineSummary=baseline.name,newlyVerifiedDefinitions=['--nls-date-format','--nls-timestamp-format','--nls-timestamp-tz-format'],meaning='3项 Oracle 仅证明本轮 DATE/TIMESTAMP/TZ 小型实际值。19项仍被现役能力门禁阻断，不将正确拒绝计为功能成功；旧 NLS UNKNOWN 分支保留为历史。'))
    assert sum(bool(row['passedRealCaseIds']) for row in report['parameters']) == 53
    report['evidenceDigests']['docs/03-technical/evidence/'+baseline.name] = hashlib.sha256(baseline.read_bytes()).hexdigest()
    report['evidenceDigests']['docs/03-technical/evidence/normal-import-remaining-2026-10-20-results.json'] = hashlib.sha256((EVIDENCE/'normal-import-remaining-2026-10-20-results.json').read_bytes()).hexdigest()
    raw = json.dumps(report,ensure_ascii=False,indent=2).encode('utf-8')
    for action in ['audit','import-oracle-audit']:
        result = subprocess.run([str(WORK/'fixture.exe'),'-action',action,'-database',run['database'],'-run-id',run['runId']],input=raw,capture_output=True,cwd=ROOT,check=True)
        assert not json.loads(result.stdout)['plaintextOrRecoverableEncodingPresent']
    report['mysqlAndOracleSecretAuditsPassed'] = True
    target.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print('CURRENT_REAL_PARAMETER_COVERAGE_53_OF_72_REMAINING_19')


if __name__ == '__main__':
    main()
