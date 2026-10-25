"""固定 TEST Oracle 源的只读协议探测；不提供任意 SQL、DDL 或用户对象访问入口。"""
import base64
import json
import pathlib
import re
import sys

sys.path.insert(0, str(pathlib.Path('artifacts/e2e-mysql-deps').resolve()))
import pymysql


def main():
    raw = bytearray(sys.stdin.buffer.read(65536))
    payload = json.loads(raw)
    raw[:] = b'\0' * len(raw)
    if payload['action'] != 'import-oracle-probe' or not re.fullmatch(r'[a-f0-9]{32}', payload['runId']):
        raise ValueError('ORACLE_PROBE_SCOPE_INVALID')
    password = bytearray(base64.b64decode(payload.pop('password')))
    connection = None
    try:
        connection = pymysql.connect(host=payload['host'], port=payload['port'], user=payload['username'], password=bytes(password), charset='utf8mb4', connect_timeout=5, read_timeout=5, write_timeout=5, autocommit=True, local_infile=False)
        with connection.cursor() as cursor:
            cursor.execute("SELECT SYS_CONTEXT('USERENV', 'CURRENT_USER') FROM DUAL")
            current = cursor.fetchall()
            matched = current == ((payload['schema'],),)
        print(json.dumps(dict(status='ORACLE_READONLY_PROTOCOL_PROBED', currentSchemaMatched=matched, arbitrarySqlEnabled=False)))
    finally:
        password[:] = b'\0' * len(password)
        if connection:
            connection.close()
            connection.password = b''


if __name__ == '__main__':
    try:
        main()
    except Exception:
        print(json.dumps(dict(status='BLOCKED', code='ORACLE_INDEPENDENT_READER_UNAVAILABLE', arbitrarySqlEnabled=False)))
        sys.exit(1)
