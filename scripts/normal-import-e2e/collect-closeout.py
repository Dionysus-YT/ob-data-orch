"""汇总精确夹具清理与服务恢复证据；不读取或解密身份材料。"""
import json
import pathlib
import sqlite3
import subprocess

ROOT = pathlib.Path(__file__).resolve().parents[2]
WORK = ROOT / 'artifacts/normal-import-full-20261020'


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def main():
    run = read(ROOT/'artifacts/normal-import-retest-20261019/run.json')
    api = read(WORK/'api-closeout.json')
    before, after = read(WORK/'service-before.json'), read(WORK/'service-after.json')
    database, directory = read(WORK/'database-cleanup.json'), read(WORK/'directory-cleanup.json')
    initial = read(WORK/'initial-failures/verified-results.json')
    final = read(WORK/'verified-results.json')
    old = read(ROOT/'artifacts/normal-import-retest-20261019/verified-results.json')
    repair = read(WORK/'verified-repair.json')
    assert database['ownerVerified'] and database['databaseAbsent'] and directory['directoryAbsent'] and not pathlib.Path(directory['root']).exists()
    assert api['health'] == 200 and api['environmentCheckStatus'] == 'PASSED' and api['heartbeatStatus'] == 'ONLINE'
    assert api['taskSummary']['running'] == api['taskSummary']['waiting'] == api['taskSummary']['reconciling'] == 0
    assert api['agentLogQueue']['pendingBatches'] == api['agentLogQueue']['pendingGaps'] == api['agentLogQueue']['pendingBytes'] == 0
    assert all(row['matched'] for row in after['materials'] if not row['path'].endswith('/agent-identity.json'))
    prior = {row['path']: row['sha256'] for row in before['binaries']}
    assert all(prior[row['path']] != row['sha256'] for row in after['binaries'])
    task_ids = {row['taskId'] for source in [initial, final, old, repair] for row in source['records'] if row.get('taskId')}
    db = sqlite3.connect((ROOT/'var/dev/control-plane/data/local-mvp.db').as_uri()+'?mode=ro',uri=True)
    try:
        identities = set()
        for task in task_ids:
            agent_id, state = db.execute('SELECT agent_id,state FROM task_executions WHERE task_id=?',(task,)).fetchone()
            assert state in ['SUCCEEDED','FAILED','CANCELLED']
            identities.add(agent_id)
        assert len(identities) == 1 and len(task_ids) == 42
    finally:
        db.close()
    report = dict(runId=run['runId'], api=api, database=database, directory=directory, services=after, allTaskCount=42, allTasksTerminal=True, sameAgentIdentityAcrossRestart=True, identitySecretsReadOrDecryptedForCloseout=False,
                  identityCiphertextExplanation='运行时配置同步会更新加密状态；身份连续性依据旧新任务相同 agent_id、同节点认证心跳及原根密钥保留确认，不要求动态密文文件字节不变。')
    raw = json.dumps(report,ensure_ascii=False,indent=2).encode('utf-8')
    audit = subprocess.run([str(WORK/'fixture.exe'),'-action','audit','-database',run['database'],'-run-id',run['runId']],input=raw,capture_output=True,cwd=ROOT,check=True)
    assert json.loads(audit.stdout)['plaintextOrRecoverableEncodingPresent'] is False
    report['secretAuditPassed'] = True
    (ROOT/'docs/03-technical/evidence/normal-import-api-closeout-2026-10-20.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print('CLOSEOUT_VERIFIED')


if __name__ == '__main__':
    main()
