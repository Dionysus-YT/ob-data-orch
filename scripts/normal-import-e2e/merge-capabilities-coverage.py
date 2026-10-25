"""仅把独立核对通过的十九项代表场景并入当前覆盖，保留旧摘要与失败分支。"""
import collections
import hashlib
import json
import pathlib
import shutil
import sqlite3
import subprocess

ROOT = pathlib.Path(__file__).resolve().parents[2]
WORK = ROOT / 'artifacts/normal-import-capabilities-20261020'
EVIDENCE = ROOT / 'docs/03-technical/evidence'


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def main():
    update, run = read(WORK / 'verified-results.json'), read(WORK / 'run.json')
    assert update['counts'] == {'PASS': 19} and update['closeoutVerified'] and update['mysqlAndOracleSecretAuditsPassed']
    assert len(update['negatives']['records']) == 8 and all(r['outcome'] == 'PASS' for r in update['negatives']['records'])
    target = EVIDENCE / 'normal-import-full-parameter-validation-2026-10-20.json'
    baseline = EVIDENCE / 'normal-import-full-parameter-validation-2026-10-20-before-capabilities.json'
    if not baseline.exists():
        assert read(target)['hasVerifiedRealScenario'] == 53
        shutil.copyfile(target, baseline)
    report = read(baseline)
    assert report['hasVerifiedRealScenario'] == 53
    rows = {r['name']: r for r in report['parameters']}
    newly_verified = set()
    for case in update['records']:
        for name in case['parameterNames']:
            row = rows[name]
            assert not row['passedRealCaseIds']
            row['passedRealCaseIds'] = [case['id']]
            row['realScenarioStatus'] = 'HAS_VERIFIED_SCENARIO'
            row['allValuesAndConditionsValidated'] = False
            row['runtimeBranches'].append(dict(id=case['id'], runId=run['runId'], outcome='PASS', taskId=case['taskId'], mode=case['mode'], evidence='normal-import-capabilities-2026-10-20-results.json'))
            newly_verified.add(name)
    assert len(newly_verified) == 19 and sum(bool(r['passedRealCaseIds']) for r in rows.values()) == 72
    task_ids = {r['taskId'] for r in update['initialRecords'] + update['records'] if r.get('taskId')}
    prior_ids = {r['taskId'] for r in read(EVIDENCE / 'normal-import-remaining-2026-10-20-results.json')['oracleRecords']}
    db = sqlite3.connect((ROOT / 'var/dev/control-plane/data/local-mvp.db').as_uri() + '?mode=ro', uri=True)
    try:
        states, identities = collections.Counter(), set()
        for task_id in task_ids | prior_ids:
            state, agent_id, reconciliation = db.execute('SELECT state,agent_id,reconciliation_required FROM task_executions WHERE task_id=?', (task_id,)).fetchone()
            assert state in ['SUCCEEDED', 'FAILED', 'CANCELLED'] and not reconciliation
            identities.add(agent_id)
            if task_id in task_ids:
                states[state] += 1
        assert len(identities) == 1 and len(task_ids) == update['taskCount'] == 25
    finally:
        db.close()
    totals = collections.Counter(report['realTaskStates']) + states
    report.update(observedAt=update['observedAt'], hasVerifiedRealScenario=72, noVerifiedRealScenario=0, realTaskCount=report['realTaskCount'] + len(task_ids), realTaskStates=dict(totals),
                  allParameterDefinitionsHaveVerifiedScenario=True, fullParameterRepresentativeValidationPassed=True, fullParameterValidationPassed=False,
                  meaning='72/72 条定义均有成功的真实代表场景；每项全取值、所有复杂定义、格式、权限、平台和性能组合仍未完成。旧 UNKNOWN 与失败分支保留，不改写为成功。',
                  capabilitiesContinuation=dict(runId=run['runId'], results='normal-import-capabilities-2026-10-20-results.json', baselineSummary=baseline.name, newlyVerifiedDefinitions=sorted(newly_verified), counts=update['counts'], rejectedCaseCount=8, sameAgentIdentityAcrossRestart=True, allOwnedFixturesCleaned=True))
    for source in [baseline, EVIDENCE / 'normal-import-capabilities-2026-10-20-results.json']:
        report['evidenceDigests']['docs/03-technical/evidence/' + source.name] = hashlib.sha256(source.read_bytes()).hexdigest()
    raw = json.dumps(report, ensure_ascii=False, indent=2).encode('utf-8')
    for action in ['audit', 'import-oracle-audit']:
        result = subprocess.run([str(WORK / 'fixture.exe'), '-action', action, '-database', run['database'], '-run-id', run['runId']], input=raw, capture_output=True, cwd=ROOT, check=True)
        assert not json.loads(result.stdout)['plaintextOrRecoverableEncodingPresent']
    report['mysqlAndOracleSecretAuditsPassed'] = True
    target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(dict(verified=72, total=72, taskCount=report['realTaskCount'], taskStates=report['realTaskStates'])))


if __name__ == '__main__':
    main()
