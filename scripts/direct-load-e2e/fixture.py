"""旁路真实验证的固定 MySQL 小夹具；凭据只经原 Go helper 的匿名输入传入。"""
import base64
import json
import pathlib
import re
import shutil
import socket
import sys

sys.path.insert(0, str(pathlib.Path('artifacts/e2e-mysql-deps').resolve()))
import pymysql


def quote(value):
    if not re.fullmatch(r'[A-Za-z_][A-Za-z_0-9]{0,63}', value):
        raise ValueError('IDENTIFIER_REJECTED')
    return '`' + value + '`'


def probe(cursor, host):
    result = {'status': 'PROBED', 'mode': 'MYSQL', 'observerVersion': '', 'odpVersion': '', 'ports': {}, 'views': {}}
    for name, statement in [('observerVersion', 'SELECT OB_VERSION() FROM DUAL'), ('odpVersion', 'SHOW PROXYINFO BINARY')]:
        try:
            cursor.execute(statement)
            rows = cursor.fetchmany(4)
            if name == 'observerVersion':
                raw = str(rows[0][0]) if len(rows) == 1 else ''
                if re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+(?:\.[0-9]+)?', raw):
                    result[name] = raw
                else:
                    result[name + 'Status'] = 'UNPARSEABLE'
            else:
                text = ' '.join(str(field) for row in rows for field in row)
                match = re.search(r'(?:ObProxy-OceanBase|obproxy|OceanBase Proxy)\s+([0-9]+\.[0-9]+\.[0-9]+(?:\.[0-9]+)?)', text, re.I)
                result[name] = match.group(1) if match else ''
                result[name + 'Status'] = 'READ' if match else 'UNPARSEABLE'
        except pymysql.MySQLError as error:
            result[name + 'Status'] = 'SQL_REJECTED'
            result[name + 'ErrorCode'] = int(error.args[0]) if error.args and isinstance(error.args[0], int) else 0
    for view in ['GV$OB_MEMSTORE', 'DBA_OB_MAJOR_COMPACTION', 'DBA_OB_LS_LOCATIONS', '__all_virtual_proxy_schema']:
        try:
            cursor.execute('SELECT 1 FROM oceanbase.' + view + ' WHERE 1=0')
            result['views'][view] = 'READABLE'
        except pymysql.MySQLError as error:
            result['views'][view] = int(error.args[0]) if error.args and isinstance(error.args[0], int) else 0
    for port in [2882, 2885]:
        try:
            with socket.create_connection((host, port), timeout=3):
                result['ports'][str(port)] = 'TCP_REACHABLE'
        except OSError:
            result['ports'][str(port)] = 'TCP_UNAVAILABLE'
    return result


def main():
    raw = bytearray(sys.stdin.buffer.read(65536))
    payload = json.loads(raw)
    raw[:] = b'\0' * len(raw)
    schema, run_id, action = payload['database'], payload['runId'], payload['action']
    if not re.fullmatch(r'odo_e2e_[0-9]{8}_[a-f0-9]{12}', schema) or not re.fullmatch(r'[a-f0-9]{32}', run_id) or not schema.endswith(run_id[:12]):
        raise ValueError('SCOPE_REJECTED')
    root = pathlib.Path('ob-data').resolve() / ('dlvt-direct-' + run_id[:12])
    secret = bytearray(base64.b64decode(payload.pop('password')))
    connection = None
    try:
        connection = pymysql.connect(host=payload['host'], port=payload['port'], user=payload['username'], password=bytes(secret), charset='utf8mb4', connect_timeout=10, read_timeout=15, write_timeout=15, autocommit=True, local_infile=False)
        with connection.cursor() as cursor:
            if action == 'direct-probe':
                result = probe(cursor, payload['host'])
                # 仅核查同一获授权 TEST 主机的官方默认 ODP SQL 端口，不枚举网络或改写已登记源。
                if payload['port'] != 2883:
                    try:
                        with pymysql.connect(host=payload['host'], port=2883, user=payload['username'], password=bytes(secret), charset='utf8mb4', connect_timeout=5, read_timeout=10, write_timeout=10, autocommit=True, local_infile=False) as proxy_connection:
                            with proxy_connection.cursor() as proxy_cursor:
                                result['sameHostDefaultODP'] = probe(proxy_cursor, payload['host'])
                    except pymysql.MySQLError as error:
                        result['sameHostDefaultODP'] = {'status': 'SQL_UNAVAILABLE', 'errorCode': int(error.args[0]) if error.args and isinstance(error.args[0], int) else 0}
            elif action == 'direct-create':
                if root.exists():
                    raise ValueError('DIRECTORY_EXISTS')
                cursor.execute('CREATE DATABASE ' + quote(schema) + ' DEFAULT CHARACTER SET utf8mb4')
                cursor.execute('USE ' + quote(schema))
                cursor.execute('CREATE TABLE odo_owner (run_id VARCHAR(32) PRIMARY KEY)')
                cursor.execute('INSERT INTO odo_owner VALUES (%s)', (run_id,))
                root.mkdir()
                definition = '(id INT PRIMARY KEY, label VARCHAR(64), amount DECIMAL(12,4))'
                tables = ['dlvt_basic', 'dlvt_shards', 'dlvt_separator', 'dlvt_no_external', 'dlvt_incremental', 'dlvt_replace', 'dlvt_truncate', 'dlvt_thread', 'dlvt_parallel', 'dlvt_mismatch', 'dlvt_unique']
                for table in tables:
                    cursor.execute('CREATE TABLE ' + quote(table) + ' ' + definition)
                    directory = root / table
                    directory.mkdir()
                    separator = '|' if table == 'dlvt_separator' else ','
                    (directory / (table + '.csv')).write_text(separator.join(['1', 'alpha', '12.3400']) + '\n' + separator.join(['2', '中文', '-0.0100']) + '\n', encoding='utf-8')
                for table in ['dlvt_incremental', 'dlvt_replace', 'dlvt_truncate', 'dlvt_unique']:
                    cursor.execute('INSERT INTO ' + quote(table) + " VALUES (9,'original',9.0000)")
                cursor.execute('CREATE INDEX dlvt_replace_label_idx ON dlvt_replace(label)')
                cursor.execute('CREATE TABLE dlvt_bit (id INT PRIMARY KEY, flag BIT(1))')
                cursor.execute('CREATE TABLE dlvt_binary (id INT PRIMARY KEY, payload VARBINARY(16))')
                cursor.execute('CREATE TABLE dlvt_virtual (id INT PRIMARY KEY, next_id INT GENERATED ALWAYS AS (id+1) VIRTUAL)')
                cursor.execute('CREATE TABLE dlvt_lob (id INT PRIMARY KEY, payload TEXT)')
                cursor.execute("INSERT INTO dlvt_lob VALUES (9,'original')")
                for table in ['dlvt_bit', 'dlvt_binary', 'dlvt_virtual', 'dlvt_lob']:
                    (root / table).mkdir()
                    (root / table / (table + '.csv')).write_text('1,1\n', encoding='utf-8')
                shards = root / 'dlvt_shards'
                (shards / 'dlvt_shards.csv').unlink()
                (shards / 'dlvt_shards.0.csv').write_text('1,alpha,12.3400\n', encoding='utf-8')
                (shards / 'dlvt_shards.1.csv').write_text('2,中文,-0.0100\n', encoding='utf-8')
                (root / 'multi').mkdir()
                (root / 'multi' / 'dlvt_basic.csv').write_text('1,alpha,12.3400\n', encoding='utf-8')
                (root / 'multi' / 'dlvt_other.csv').write_text('2,beta,1.0000\n', encoding='utf-8')
                (root / 'empty').mkdir()
                (root / 'dlvt_mismatch' / 'dlvt_mismatch.csv').write_text('1,alpha,12.3400,extra\n', encoding='utf-8')
                result = {'status': 'CREATED', 'ownerVerified': True, 'database': schema, 'inputRoot': str(root), 'tables': tables + ['dlvt_bit', 'dlvt_binary', 'dlvt_virtual', 'dlvt_lob']}
            else:
                cursor.execute('USE ' + quote(schema))
                cursor.execute('SELECT run_id FROM odo_owner')
                if cursor.fetchall() != ((run_id,),):
                    raise ValueError('OWNER_MISMATCH')
                if action == 'direct-inspect':
                    cursor.execute('SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=%s', (schema,))
                    tables = [row[0] for row in cursor.fetchall() if re.fullmatch(r'dlvt_[a-z_]+', row[0])]
                    facts = {}
                    for table in tables:
                        cursor.execute('SELECT * FROM ' + quote(table) + ' ORDER BY id')
                        rows = cursor.fetchmany(101)
                        if len(rows) > 100:
                            raise ValueError('FIXTURE_ROWS_LIMIT')
                        cursor.execute('SHOW CREATE TABLE ' + quote(table))
                        structure = cursor.fetchone()[1]
                        facts[table] = {'rows': [[str(v) if v is not None else None for v in row] for row in rows], 'structure': structure}
                    runtime_files = [str(p.relative_to(root)) for p in root.rglob('*') if p.is_file() and p.suffix.lower() in {'.log', '.info', '.warn', '.error'}]
                    result = {'status': 'INSPECTED', 'ownerVerified': True, 'tables': facts, 'uncontrolledRuntimeFiles': runtime_files[:1024]}
                elif action == 'direct-cleanup':
                    cursor.execute('DROP DATABASE ' + quote(schema))
                    cursor.execute('SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME=%s', (schema,))
                    if cursor.fetchone()[0] != 0:
                        raise ValueError('CLEANUP_UNCONFIRMED')
                    # 精确新建目录绑定本轮 run，不以通配符或目录年龄判断可删除对象。
                    if root.parent != pathlib.Path('ob-data').resolve() or root.name != 'dlvt-direct-' + run_id[:12]:
                        raise ValueError('DIRECTORY_SCOPE_REJECTED')
                    if root.exists():
                        shutil.rmtree(root)
                    result = {'status': 'CLEANED', 'ownerVerified': True, 'databaseAbsent': True, 'inputAbsent': not root.exists()}
                else:
                    raise ValueError('ACTION_REJECTED')
        # 固定 ASCII JSON 避免 Windows 管道使用本地代码页，独立核对不能把输出转码误认成库内数据失真。
        print(json.dumps(result, ensure_ascii=True))
    finally:
        secret[:] = b'\0' * len(secret)
        if connection:
            connection.close()


if __name__ == '__main__':
    try:
        main()
    except Exception:
        print(json.dumps({'status': 'BLOCKED', 'code': 'DIRECT_FIXTURE_OPERATION_UNAVAILABLE'}))
        sys.exit(1)
