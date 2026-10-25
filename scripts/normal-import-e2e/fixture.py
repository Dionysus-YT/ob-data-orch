"""普通导入真实验证的固定小型夹具；只操作带本次所有权的独立测试库。"""
import base64
import csv
import decimal
import json
import pathlib
import re
import sys

import parameter_fixture
import capability_fixture

sys.path.insert(0, str(pathlib.Path('artifacts/e2e-mysql-deps').resolve()))
import pymysql

ROWS = [
    [1, 'alpha', 'plain', None, '', '12.340000'],
    [2, 'beta', 'comma,quote"backslash\\', 'NULL', '', '-0.010000'],
    [3, 'gamma', 'first\nsecond\r\nthird', '中文🙂', ' ', '999.999999'],
    [4, 'delta', '  padded  ', 'tab\tvalue', 'empty', '0.000000'],
]
SIMPLE = [[1, 'alpha', '中文', None, '', '12.340000'], [2, 'beta', 'plain', 'value', ' ', '-0.010000']]
COLUMNS = ['id', 'ascii_label', 'notes', 'nullable_text', 'empty_text', 'amount']
DEFINITION = '(id INT PRIMARY KEY, ascii_label VARCHAR(32), notes TEXT, nullable_text VARCHAR(128), empty_text VARCHAR(64), amount DECIMAL(18,6))'


def quote(value):
    if not re.fullmatch(r'[A-Za-z_][A-Za-z_0-9]{0,63}', value):
        raise ValueError('IDENTIFIER_REJECTED')
    return '`' + value + '`'


def write_csv(path, rows):
    with path.open('w', encoding='utf-8', newline='') as stream:
        writer = csv.writer(stream, lineterminator='\n')
        writer.writerow(COLUMNS)
        writer.writerows([[r'\N' if value is None else value for value in row] for row in rows])


def main():
    raw = bytearray(sys.stdin.buffer.read(65536))
    payload = json.loads(raw)
    raw[:] = b'\0' * len(raw)
    schema, run_id, action = payload['database'], payload['runId'], payload['action']
    if not re.fullmatch(r'odo_e2e_[0-9]{8}_[a-f0-9]{12}', schema) or not re.fullmatch(r'[a-f0-9]{32}', run_id) or not schema.endswith(run_id[:12]):
        raise ValueError('SCOPE_REJECTED')
    root = pathlib.Path('ob-data').resolve() / ('nivt-normal-20261019-' + run_id[:12])
    password = bytearray(base64.b64decode(payload.pop('password')))
    connection = None
    try:
        connection = pymysql.connect(host=payload['host'], port=payload['port'], user=payload['username'], password=bytes(password), charset='utf8mb4', connect_timeout=10, read_timeout=30, write_timeout=30, autocommit=True, local_infile=False)
        with connection.cursor() as cursor:
            if action == 'import-create':
                # 冲突即失败；不接管已有库或目录，不使用 IF NOT EXISTS。
                if root.exists():
                    raise ValueError('DIRECTORY_EXISTS')
                cursor.execute('CREATE DATABASE ' + quote(schema) + ' DEFAULT CHARACTER SET utf8mb4')
                cursor.execute('USE ' + quote(schema))
                cursor.execute('CREATE TABLE odo_owner (run_id VARCHAR(32) PRIMARY KEY)')
                cursor.execute('INSERT INTO odo_owner VALUES (%s)', (run_id,))
                for table in ['nivt_csv', 'nivt_cut', 'nivt_sql']:
                    cursor.execute('CREATE TABLE ' + quote(table) + ' ' + DEFINITION)
                cursor.execute('CREATE TABLE nivt_mismatch (id INT PRIMARY KEY, label VARCHAR(32))')
                root.mkdir()
                for name in ['csv', 'cut', 'sql', 'ddl', 'both', 'mismatch', 'empty']:
                    (root / name).mkdir()
                write_csv(root / 'csv' / 'nivt_csv.csv', ROWS)
                write_csv(root / 'both' / 'nivt_both.csv', SIMPLE)
                (root / 'cut' / 'nivt_cut.dat').write_text('1|alpha|中文|\\N||12.340000\n2|beta|plain|value| |-0.010000\n', encoding='utf-8')
                (root / 'sql' / 'nivt_sql.sql').write_text("INSERT INTO `nivt_sql` (`id`,`ascii_label`,`notes`,`nullable_text`,`empty_text`,`amount`) VALUES (1,'alpha','中文',NULL,'',12.340000),(2,'beta','plain','value',' ',-0.010000);\n", encoding='utf-8')
                for directory, table in [('ddl', 'nivt_ddl'), ('both', 'nivt_both')]:
                    (root / directory / (table + '-schema.sql')).write_text('CREATE TABLE ' + quote(table) + ' ' + DEFINITION + ';\n', encoding='utf-8')
                (root / 'mismatch' / 'nivt_mismatch.csv').write_text('1,a,extra\n', encoding='utf-8')
                result = {'status': 'CREATED', 'database': schema, 'inputRoot': str(root), 'expectedTables': ['nivt_csv', 'nivt_cut', 'nivt_sql', 'nivt_ddl', 'nivt_both'], 'expectedRows': {'nivt_csv': 4, 'nivt_cut': 2, 'nivt_sql': 2, 'nivt_ddl': 0, 'nivt_both': 2}}
            else:
                cursor.execute('USE ' + quote(schema))
                cursor.execute('SELECT run_id FROM odo_owner')
                if cursor.fetchall() != ((run_id,),):
                    raise ValueError('OWNER_MISMATCH')
                if action.startswith('import-capabilities-'):
                    result = capability_fixture.operate(cursor, root, schema, run_id, action)
                elif action == 'import-extend':
                    cursor.execute('CREATE TABLE nivt_quoted ' + DEFINITION)
                    cursor.execute('CREATE TABLE nivt_trim ' + DEFINITION)
                    cursor.execute('CREATE TABLE nivt_strict_false ' + DEFINITION)
                    cursor.execute('CREATE TABLE nivt_duplicate (id INT PRIMARY KEY, label VARCHAR(32))')
                    cursor.execute("INSERT INTO nivt_duplicate VALUES (1,'original')")
                    (root / 'quoted').mkdir()
                    (root / 'duplicate').mkdir()
                    for name in ['trim', 'strict_false', 'both_quoted']:
                        (root / name).mkdir()
                    write_csv(root / 'trim' / 'nivt_trim.csv', ROWS)
                    with (root / 'quoted' / 'nivt_quoted.csv').open('w', encoding='utf-8', newline='') as stream:
                        writer = csv.writer(stream, lineterminator='\n', quoting=csv.QUOTE_ALL)
                        writer.writerow(COLUMNS)
                        writer.writerows([[r'\N' if value is None else value for value in row] for row in ROWS])
                    with (root / 'both_quoted' / 'nivt_both_quoted.csv').open('w', encoding='utf-8', newline='') as stream:
                        writer = csv.writer(stream, lineterminator='\n', quoting=csv.QUOTE_ALL)
                        writer.writerow(COLUMNS)
                        writer.writerows([[r'\N' if value is None else value for value in row] for row in SIMPLE])
                    (root / 'both_quoted' / 'nivt_both_quoted-schema.sql').write_text('CREATE TABLE `nivt_both_quoted` ' + DEFINITION + ';\n', encoding='utf-8')
                    (root / 'strict_false' / 'nivt_strict_false.sql').write_text("INSERT INTO `nivt_strict_false` (`id`,`ascii_label`,`notes`,`nullable_text`,`empty_text`,`amount`) VALUES (1,'alpha','中文',NULL,'',12.340000),(2,'beta','plain','value',' ',-0.010000);\n", encoding='utf-8')
                    (root / 'duplicate' / 'nivt_duplicate.sql').write_text("INSERT INTO `nivt_duplicate` (`id`,`label`) VALUES (1,'replacement');\n", encoding='utf-8')
                    result = {'status': 'EXTENDED', 'ownerVerified': True}
                elif action == 'import-parameters-create':
                    result = parameter_fixture.prepare(cursor, root)
                elif action == 'import-parameters-inspect':
                    result = parameter_fixture.inspect(cursor, schema)
                elif action == 'import-parameters-repair':
                    result = parameter_fixture.repair(cursor)
                elif action == 'import-cleanup':
                    cursor.execute('DROP DATABASE ' + quote(schema))
                    cursor.execute('SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME=%s', (schema,))
                    if cursor.fetchone()[0] != 0:
                        raise ValueError('CLEANUP_DATABASE_STILL_PRESENT')
                    result = {'status': 'CLEANED', 'database': schema, 'ownerVerified': True, 'databaseAbsent': True}
                elif action == 'import-inspect':
                    tables = {}
                    trimmed = [[value.strip() if isinstance(value, str) else value for value in row] for row in ROWS]
                    for table, expected in [('nivt_csv', ROWS), ('nivt_cut', SIMPLE), ('nivt_sql', SIMPLE), ('nivt_both', SIMPLE), ('nivt_ddl', []), ('nivt_quoted', ROWS), ('nivt_trim', trimmed), ('nivt_strict_false', SIMPLE), ('nivt_both_quoted', SIMPLE)]:
                        cursor.execute('SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=%s AND table_name=%s', (schema, table))
                        exists = cursor.fetchone()[0] == 1
                        item = {'exists': exists, 'expectedRows': len(expected), 'contentMatched': False}
                        if exists:
                            cursor.execute('SELECT ' + ','.join(quote(c) for c in COLUMNS) + ' FROM ' + quote(table) + ' ORDER BY id')
                            rows = [[str(value) if isinstance(value, decimal.Decimal) else value for value in row] for row in cursor.fetchall()]
                            cursor.execute('SELECT column_name,data_type,character_maximum_length,numeric_precision,numeric_scale,is_nullable,column_key FROM information_schema.columns WHERE table_schema=%s AND table_name=%s ORDER BY ordinal_position', (schema, table))
                            metadata = cursor.fetchall()
                            structure_matched = len(metadata) == 6 and [r[0] for r in metadata] == COLUMNS and [r[1] for r in metadata] == ['int', 'varchar', 'text', 'varchar', 'varchar', 'decimal'] and [r[5] for r in metadata] == ['NO', 'YES', 'YES', 'YES', 'YES', 'YES'] and [r[6] for r in metadata] == ['PRI', '', '', '', '', ''] and metadata[1][2] == 32 and metadata[3][2] == 128 and metadata[4][2] == 64 and metadata[5][3:5] == (18, 6)
                            differences = [{'row': i + 1, 'column': COLUMNS[j], 'expected': wanted, 'actual': actual} for i, (wanted_row, actual_row) in enumerate(zip(expected, rows)) for j, (wanted, actual) in enumerate(zip(wanted_row, actual_row)) if wanted != actual]
                            item.update(actualRows=len(rows), contentMatched=rows == expected, differences=differences, columnsMatched=[r[0] for r in metadata] == COLUMNS, structureMatched=structure_matched)
                        tables[table] = item
                    cursor.execute('SELECT COUNT(*) FROM nivt_mismatch')
                    rejected_unchanged = cursor.fetchone()[0] == 0
                    cursor.execute("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=%s AND table_name='nivt_duplicate'", (schema,))
                    duplicate_unchanged = None
                    if cursor.fetchone()[0] == 1:
                        cursor.execute('SELECT id,label FROM nivt_duplicate ORDER BY id')
                        duplicate_unchanged = cursor.fetchall() == ((1, 'original'),)
                    result = {'status': 'INSPECTED', 'ownerVerified': True, 'tables': tables, 'rejectedTargetUnchanged': rejected_unchanged, 'duplicateTargetUnchanged': duplicate_unchanged}
                else:
                    raise ValueError('ACTION_REJECTED')
            print(json.dumps(result, ensure_ascii=False))
    finally:
        password[:] = b'\0' * len(password)
        if connection:
            connection.close()
            connection.password = b''


if __name__ == '__main__':
    try:
        main()
    except Exception:
        # 固定错误码避免 JDBC/SQL、用户名、端点或秘密泄露到测试输出。
        print(json.dumps({'status': 'BLOCKED', 'code': 'IMPORT_FIXTURE_OPERATION_UNAVAILABLE'}))
        sys.exit(1)
