"""从本轮冻结 argv 和安全结果建立参数追踪，不把出现过等同于全取值验证通过。"""
import json
import pathlib
import sqlite3
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]


def main():
    result_path = (ROOT / sys.argv[1]).resolve()
    output_path = (ROOT / sys.argv[2]).resolve()
    if not result_path.is_relative_to(ROOT) or not output_path.is_relative_to(ROOT):
        raise ValueError('EVIDENCE_PATH_REJECTED')
    results = json.loads(result_path.read_text(encoding='utf-8-sig'))
    catalog = json.loads((ROOT / 'internal/parammeta/normal/obloader-4.3.5-normal-v1.json').read_text(encoding='utf-8'))
    evidence = {item['longName']: {'passedCaseIds': [], 'failedCaseIds': []} for item in catalog['definitions']}
    db = sqlite3.connect((ROOT / 'var/dev/control-plane/data/local-mvp.db').as_uri() + '?mode=ro', uri=True)
    try:
        for case in results['records']:
            if not case.get('taskId'):
                continue
            row = db.execute("SELECT planned_argv_json,task_type FROM tasks WHERE task_id=?", (case['taskId'],)).fetchone()
            if not row or row[1] != 'OBLOADER_NORMAL_IMPORT':
                raise ValueError('TASK_BINDING_INVALID')
            argv = json.loads(row[0])
            names = set()
            for token in argv:
                name = token.split('=', 1)[0]
                if name in evidence:
                    names.add(name)
            # 成功数据库写入证明平台安全槽位被实际消费；不读取或输出秘密本身。
            if case.get('persistedEvidence', {}).get('result') == 'VERIFIED':
                names.add('--password')
            for name in names:
                evidence[name]['passedCaseIds' if case['outcome'] == 'PASS' else 'failedCaseIds'].append(case['id'])
    finally:
        db.close()
    rows = []
    for definition in catalog['definitions']:
        value = evidence[definition['longName']]
        status = 'HAS_REAL_SCENARIO' if value['passedCaseIds'] else 'FAILED_SCENARIO_ONLY' if value['failedCaseIds'] else 'NOT_EXERCISED'
        rows.append({'name': definition['longName'], 'category': definition['category'], 'sourcePolicy': definition['sourcePolicy'], 'status': status, **value})
    report = {'metadataVersion': catalog['metadataVersion'], 'definitionCount': len(rows), 'hasRealScenario': sum(row['status'] == 'HAS_REAL_SCENARIO' for row in rows),
              'failedScenarioOnly': sum(row['status'] == 'FAILED_SCENARIO_ONLY' for row in rows), 'notExercised': sum(row['status'] == 'NOT_EXERCISED' for row in rows),
              'fullParameterValidationPassed': False, 'meaning': 'HAS_REAL_SCENARIO 仅表示至少一个通过场景中实际发射或消费；不代表全部取值、格式、互斥、权限或平台矩阵已验证。', 'parameters': rows}
    output_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
    print(json.dumps({k: v for k, v in report.items() if k not in ['parameters', 'meaning']}, ensure_ascii=False))


if __name__ == '__main__':
    main()
