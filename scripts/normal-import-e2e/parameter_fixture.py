"""未覆盖 MySQL 参数的固定微型夹具；调用方必须先核实本轮独立库的所有权。"""
import datetime
import decimal
import json


def cases():
    result = []

    def add(key, names, patch=None, text='1,alpha,plain\n', expected=None, **extra):
        table = 'nivt_p_' + key
        item = dict(id='NI-PARAM-' + key, names=names, table=table, directory='parameters/' + key,
                    patch=patch or {}, text=text, expected=expected if expected is not None else [[1, 'alpha', 'plain']],
                    definition='(id INT PRIMARY KEY, label VARCHAR(32), note VARCHAR(64))', suffix='.csv', before=[])
        item.update(extra)
        result.append(item)

    add('suffix', ['--file-suffix'], {'fileSuffix': '.custom'}, suffix='.custom')
    add('encoding', ['--file-encoding'], {'fileEncoding': 'UTF-8'})
    add('regex', ['--file-regular-expression'], {'fileRegex': r'^nivt_p_regex\.csv$'}, ignored=True)
    add('all', ['--all'], {'scope': 'ALL'})
    add('exclude', ['--exclude-table'], {'scope': 'ALL', 'excludeTables': 'nivt_p_excluded'}, excluded=True)
    add('include', ['--include-column-names'], {'mapping': {'kind': 'EXPLICIT', 'includeColumns': 'id,label'}},
        '1,alpha\n', [[1, 'alpha', None]])
    add('exclude_col', ['--exclude-column-names'], {'mapping': {'kind': 'EXPLICIT', 'excludeColumns': 'note'}},
        '1,alpha\n', [[1, 'alpha', None]])
    add('separator', ['--column-separator'], {'parser': {'columnSeparator': '||', 'columnQuote': '"'}},
        '1||"alpha"||"comma,value"\n', [[1, 'alpha', 'comma,value']])
    add('line', ['--line-separator'], {'parser': {'columnSeparator': ',', 'columnQuote': '"', 'lineSeparator': r'\r\n'}},
        '1,alpha,plain\r\n')
    add('escape', ['--escape-character'], {'parser': {'columnSeparator': ',', 'columnQuote': '"', 'escapeCharacter': '\\'}},
        '1,"alpha","escaped\\"quote"\n', [[1, 'alpha', 'escaped"quote']])
    add('null', ['--null-string'], {'parser': {'columnSeparator': ',', 'columnQuote': '"', 'nullString': 'NIL'}},
        '1,alpha,NIL\n', [[1, 'alpha', None]])
    add('empty', ['--empty-string'], {'parser': {'columnSeparator': ',', 'columnQuote': '"', 'emptyString': 'EMPTY'}},
        '1,alpha,EMPTY\n', [[1, 'alpha', '']])
    add('ignore', ['--ignore-escape'], {'format': 'CUT', 'parser': {'columnSplitter': '|', 'ignoreEscape': True, 'lineSeparator': r'\n'}},
        '1|alpha|literal\\n\n', [[1, 'alpha', r'literal\n']], suffix='.dat')
    add('trail', ['--trail-delimiter'], {'format': 'CUT', 'parser': {'columnSplitter': '|', 'trailDelimiter': True, 'lineSeparator': r'\n'}},
        '1|alpha|plain|\n', suffix='.dat')
    add('auto', ['--auto-column-mapping'], {'mapping': {'kind': 'AUTO'}}, 'note,id,label\nplain,1,alpha\n')
    for key, field, name, value in [('batch', 'batch', '--batch', '1'), ('block', 'blockSize', '--block-size', '1'),
                                     ('tps', 'maxTps', '--max-tps', '1'), ('wait', 'maxWaitTimeout', '--max-wait-timeout', '1000'),
                                     ('rw', 'rw', '--rw', '0.5')]:
        add(key, [name], {'execution': {'memory': '1G', 'thread': '1', field: value}})
    add('replace', ['--replace-data'], {'replaceData': True}, before=[[1, 'old', 'old']])
    for key, cleanup, name in [('truncate', 'TRUNCATE', '--truncate-table'), ('delete', 'DELETE', '--delete-from-table')]:
        add(key, [name, '--yes'], {'cleanup': cleanup}, before=[[9, 'old', 'old']])
    add('compat', ['--compat-mode'], {'content': 'DDL_ONLY', 'compatMysql': True, 'compatMysqlVersion': 'mysql80'}, '', [], ddl=True)
    add('replace_obj', ['--replace-object'], {'content': 'DDL_ONLY', 'replaceObject': True}, '', [],
        ddl=True, before=[[9, 'old', 'old']], target=True)
    add('date', ['--date-format'], {'parser': {'columnSeparator': ',', 'columnQuote': '"', 'dateFormat': 'yyyy-MM-dd'}},
        '1,alpha,2026-10-20\n', [[1, 'alpha', '2026-10-20']], definition='(id INT PRIMARY KEY, label VARCHAR(32), note DATE)')
    add('default_date', ['--default-date'], {'parser': {'columnSeparator': ',', 'columnQuote': '"', 'dateFormat': 'yyyy-MM-dd', 'defaultDate': '2000-01-01'}},
        '1,alpha,invalid-date\n', [[1, 'alpha', '2000-01-01']], definition='(id INT PRIMARY KEY, label VARCHAR(32), note DATE)')
    add('single_file', ['--file-path'], single=True)
    return result


def prepare(cursor, root):
    destination = root / 'parameters'
    if destination.exists():
        raise ValueError('PARAMETER_FIXTURE_EXISTS')
    destination.mkdir()
    output = []
    for item in cases():
        directory = root / item['directory']
        directory.mkdir()
        table = item['table']
        # 标识符、定义与数据全部来自本文件固定枚举，外部不能传入 SQL。
        if not item.get('ddl') or item.get('target'):
            cursor.execute('CREATE TABLE `' + table + '` ' + item['definition'])
            if item['before']:
                cursor.executemany('INSERT INTO `' + table + '` VALUES (%s,%s,%s)', item['before'])
        name = table + ('-schema.sql' if item.get('ddl') else item['suffix'])
        text = 'CREATE TABLE `' + table + '` ' + item['definition'] + ';\n' if item.get('ddl') else item['text']
        (directory / name).write_bytes(text.encode('utf-8'))
        if item.get('ignored') or item.get('excluded'):
            if item.get('excluded'):
                cursor.execute('CREATE TABLE nivt_p_excluded (id INT PRIMARY KEY, label VARCHAR(32), note VARCHAR(64))')
            (directory / ('nivt_p_excluded.csv' if item.get('excluded') else 'unmatched.csv')).write_text('2,excluded,excluded\n', encoding='utf-8')
        config = dict(dataSourceId='', nodeId='', database='', filePath=str(directory / name if item.get('single') else directory),
                      logPath=str(root / ('logs-parameter-' + table)), content='DATA_ONLY', format='CSV', scope='SPECIFIC',
                      objects={'table': table}, parser={'columnSeparator': ',', 'columnQuote': '"'}, mapping={'kind': 'POSITION'},
                      execution={'memory': '1G', 'thread': '1'}, cleanup='NONE')
        config.update(item['patch'])
        output.append({'id': item['id'], 'names': item['names'], 'title': '参数真实作用 ' + ','.join(item['names']), 'input': {'configVersion': 'normal-import-v1', 'config': config}})
    # 本轮目录内仅保存合成配置；不包含连接、密码、cookie 或 CSRF。
    (root / 'parameter-cases.json').write_text(json.dumps(output, ensure_ascii=False, indent=2), encoding='utf-8')
    return {'status': 'PARAMETERS_PREPARED', 'ownerVerified': True, 'caseCount': len(output)}


def repair(cursor):
    # 调用方已核对独立库所有权；只重置本轮两个失败夹具，不接收表名或 SQL 输入。
    counts = {}
    for table in ['nivt_p_ignore', 'nivt_p_trail']:
        cursor.execute('SELECT COUNT(*) FROM `' + table + '`')
        counts[table] = cursor.fetchone()[0]
        if counts[table] > 1:
            raise ValueError('REPAIR_FIXTURE_STATE_CHANGED')
    for table in counts:
        cursor.execute('DELETE FROM `' + table + '`')
    return dict(status='REPAIR_FIXTURES_RESET', ownerVerified=True, previousCounts=counts)


def inspect(cursor, schema):
    result = []
    for item in cases():
        cursor.execute('SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=%s AND TABLE_NAME=%s', (schema, item['table']))
        exists = cursor.fetchone()[0] == 1
        rows = []
        if exists:
            cursor.execute('SELECT id,label,note FROM `' + item['table'] + '` ORDER BY id')
            rows = [[v.isoformat() if isinstance(v, (datetime.date, datetime.datetime)) else str(v) if isinstance(v, decimal.Decimal) else v for v in row] for row in cursor.fetchall()]
        cursor.execute('SELECT COLUMN_NAME,DATA_TYPE,IS_NULLABLE,COLUMN_KEY,CHARACTER_MAXIMUM_LENGTH FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=%s AND TABLE_NAME=%s ORDER BY ORDINAL_POSITION', (schema, item['table']))
        columns = cursor.fetchall()
        expected_type = 'date' if item['table'] in ['nivt_p_date', 'nivt_p_default_date'] else 'varchar'
        structure = len(columns) == 3 and [v[0] for v in columns] == ['id', 'label', 'note'] and [v[1] for v in columns] == ['int', 'varchar', expected_type] and [v[2] for v in columns] == ['NO', 'YES', 'YES'] and [v[3] for v in columns] == ['PRI', '', ''] and columns[1][4] == 32 and (columns[2][4] is None if expected_type == 'date' else columns[2][4] == 64)
        before_exists = not item.get('ddl') or item.get('target', False)
        result.append(dict(id=item['id'], names=item['names'], exists=exists, actualRows=len(rows), expectedRows=len(item['expected']),
                           beforeMatched=exists == before_exists and rows == item['before'], contentMatched=exists and rows == item['expected'], structureMatched=structure))
    cursor.execute('SELECT COUNT(*) FROM nivt_p_excluded')
    return dict(status='PARAMETERS_INSPECTED', ownerVerified=True, excludedTableUnchanged=cursor.fetchone()[0] == 0, records=result)
