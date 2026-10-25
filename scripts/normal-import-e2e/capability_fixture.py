"""固定的十九项补齐夹具；仅操作已核对所有权的独立库和随机命名对象。"""
import gzip
import zlib

KEYS = ['ctl_path', 'exclude_data_types', 'skip_footer', 'compress', 'compression_algo', 'compression_level']


def operate(cursor, root, schema, run_id, action):
    prefix = 'nivc_' + run_id[:12] + '_'
    user, group = prefix + 'user', prefix + 'tg'
    if action.endswith('-create'):
        # 全局对象必须先证明不存在，再将精确名称记入独立库的所有权表。
        cursor.execute('SELECT COUNT(*) FROM mysql.user WHERE User=%s', (user,))
        if cursor.fetchone()[0]:
            raise ValueError('GLOBAL_USER_EXISTS')
        cursor.execute('SELECT COUNT(*) FROM oceanbase.DBA_OB_TABLEGROUPS WHERE TABLEGROUP_NAME=%s', (group,))
        if cursor.fetchone()[0]:
            raise ValueError('GLOBAL_GROUP_EXISTS')
        cursor.execute('CREATE TABLE capability_owner (run_id VARCHAR(32), user_name VARCHAR(64), group_name VARCHAR(64))')
        cursor.execute('INSERT INTO capability_owner VALUES (%s,%s,%s)', (run_id, user, group))
        (root / 'parameters').mkdir()
        for key in KEYS:
            table = 'nivc_' + key
            directory = root / 'parameters' / key
            directory.mkdir()
            cursor.execute('CREATE TABLE `' + table + '` (id INT PRIMARY KEY,label VARCHAR(32),note ' + ('BLOB' if key == 'exclude_data_types' else 'VARCHAR(64)') + ')')
            data = b'"1","alpha","plain"\n'
            suffix = '.csv'
            if key == 'ctl_path':
                data = b'"alpha","1","plain"\n'
                ctl = root / 'control'
                ctl.mkdir()
                (ctl / (table + '.ctrl')).write_text('lang=java (id "NONE" map(2),label "NONE" map(1),note "NONE" map(3));\n', encoding='utf-8')
            elif key == 'exclude_data_types':
                data = b'"1","alpha"\n'
            elif key == 'skip_footer':
                data, suffix = b'1|alpha|plain\nfooter\n', '.dat'
            elif key in ['compress', 'compression_algo']:
                data, suffix = gzip.compress(data, mtime=0), '.csv.gz'
            elif key == 'compression_level':
                data, suffix = zlib.compress(data, level=1), '.csv.zz'
            (directory / (table + suffix)).write_bytes(data)
        cursor.execute('CREATE TABLE nivc_base (id INT PRIMARY KEY)')
        definitions = {
            'table_group': ('table-group', group, 'CREATE TABLEGROUP ' + group + " SHARDING='ADAPTIVE';"),
            'view': ('view', 'nivc_view', 'CREATE VIEW nivc_view AS SELECT 1 AS id;'),
            'trigger': ('trigger', 'nivc_trigger', 'CREATE TRIGGER nivc_trigger BEFORE INSERT ON nivc_base FOR EACH ROW SET NEW.id=NEW.id;'),
            'obj_user': ('obj-user', user, "CREATE USER " + user + "@'0.0.0.0';"),
            'function': ('function', 'nivc_function', 'CREATE FUNCTION nivc_function() RETURNS INT RETURN 1;'),
            'procedure': ('procedure', 'nivc_procedure', 'CREATE PROCEDURE nivc_procedure() SELECT 1;'),
        }
        for key, (_, name, sql) in definitions.items():
            directory = root / 'parameters' / key
            directory.mkdir()
            (directory / (name + '-schema.sql')).write_text(sql + '\n', encoding='utf-8')
        return {'status': 'CREATED', 'ownerVerified': True, 'dataKeys': KEYS, 'objects': [{'key': key, 'kind': item[0], 'name': item[1]} for key, item in definitions.items()]}
    cursor.execute('SELECT run_id,user_name,group_name FROM capability_owner')
    if cursor.fetchall() != ((run_id, user, group),):
        raise ValueError('CAPABILITY_OWNER_MISMATCH')
    if action.endswith('-repair'):
        cursor.execute('SELECT id,label,note FROM nivc_ctl_path')
        if cursor.fetchall() != ((1, 'alpha', 'alpha'),):
            raise ValueError('REPAIR_SOURCE_CHANGED')
        cursor.execute('SELECT COUNT(*) FROM nivc_compression_level')
        if cursor.fetchone()[0] != 0:
            raise ValueError('REPAIR_COMPRESSED_TARGET_CHANGED')
        cursor.execute('SELECT COUNT(*) FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA=%s AND ROUTINE_NAME=%s', (schema, 'nivc_procedure'))
        if cursor.fetchone()[0] != 0:
            raise ValueError('REPAIR_PROCEDURE_EXISTS')
        cursor.execute('DELETE FROM nivc_ctl_path WHERE id=1')
        for key in ['ctl_path', 'compression_level', 'procedure']:
            (root / 'parameters' / ('retry_' + key)).mkdir()
        (root / 'parameters' / 'retry_ctl_path' / 'nivc_ctl_path.csv').write_bytes(b'"alpha","1","plain"\n')
        ctl = root / 'retry-control'
        ctl.mkdir()
        (ctl / 'nivc_ctl_path.ctrl').write_text('lang=java (id "NONE" map(2),label "NONE" map(1),note "NONE" map(3));\n', encoding='utf-8')
        (root / 'parameters' / 'retry_compression_level' / 'nivc_compression_level.csv.zz').write_bytes(zlib.compress(b'"1","alpha","plain"\n', level=1))
        (root / 'parameters' / 'retry_procedure' / 'nivc_procedure-schema.sql').write_text('CREATE PROCEDURE nivc_procedure() SELECT 1;\n', encoding='utf-8')
        return {'status': 'REPAIRED', 'ownerVerified': True, 'resetOnlyOwnedControlRow': True, 'otherRetryTargetsAbsentOrEmpty': True}
    if action.endswith('-cleanup'):
        cursor.execute('SELECT Host FROM mysql.user WHERE User=%s', (user,))
        hosts = cursor.fetchall()
        if hosts not in [(), (('0.0.0.0',),)]:
            raise ValueError('GLOBAL_USER_SCOPE_CHANGED')
        if hosts:
            cursor.execute('DROP USER `' + user + "`@'0.0.0.0'")
        cursor.execute('SELECT COUNT(*) FROM oceanbase.DBA_OB_TABLEGROUPS WHERE TABLEGROUP_NAME=%s', (group,))
        if cursor.fetchone()[0]:
            cursor.execute('DROP TABLEGROUP `' + group + '`')
        cursor.execute('SELECT COUNT(*) FROM mysql.user WHERE User=%s', (user,))
        user_absent = cursor.fetchone()[0] == 0
        cursor.execute('SELECT COUNT(*) FROM oceanbase.DBA_OB_TABLEGROUPS WHERE TABLEGROUP_NAME=%s', (group,))
        return {'status': 'CLEANED', 'ownerVerified': True, 'globalUserAbsent': user_absent, 'globalGroupAbsent': cursor.fetchone()[0] == 0}
    records = []
    for key in KEYS:
        table = 'nivc_' + key
        cursor.execute('SELECT id,label,note FROM `' + table + '` ORDER BY id')
        rows = cursor.fetchall()
        expected = ((1, 'alpha', None if key == 'exclude_data_types' else 'plain'),)
        cursor.execute('SELECT COLUMN_NAME,DATA_TYPE,IS_NULLABLE,COLUMN_KEY FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=%s AND TABLE_NAME=%s ORDER BY ORDINAL_POSITION', (schema, table))
        columns = cursor.fetchall()
        structure = columns == (('id', 'int', 'NO', 'PRI'), ('label', 'varchar', 'YES', ''), ('note', 'blob' if key == 'exclude_data_types' else 'varchar', 'YES', ''))
        records.append({'key': key, 'actualRows': len(rows), 'contentMatched': rows == expected, 'structureMatched': structure, 'syntheticRows': [list(row) for row in rows] if key == 'ctl_path' else []})
    checks = [
        ('table_group', 'SELECT COUNT(*) FROM oceanbase.DBA_OB_TABLEGROUPS WHERE TABLEGROUP_NAME=%s', (group,)),
        ('view', 'SELECT COUNT(*) FROM information_schema.VIEWS WHERE TABLE_SCHEMA=%s AND TABLE_NAME=%s', (schema, 'nivc_view')),
        ('trigger', "SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=%s AND TRIGGER_NAME=%s AND EVENT_MANIPULATION='INSERT' AND ACTION_TIMING='BEFORE' AND EVENT_OBJECT_TABLE='nivc_base'", (schema, 'nivc_trigger')),
        ('obj_user', 'SELECT COUNT(*) FROM mysql.user WHERE User=%s AND Host=%s', (user, '0.0.0.0')),
        ('function', "SELECT COUNT(*) FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA=%s AND ROUTINE_NAME=%s AND ROUTINE_TYPE='FUNCTION'", (schema, 'nivc_function')),
        ('procedure', "SELECT COUNT(*) FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA=%s AND ROUTINE_NAME=%s AND ROUTINE_TYPE='PROCEDURE'", (schema, 'nivc_procedure')),
    ]
    for key, sql, params in checks:
        cursor.execute(sql, params)
        exists = cursor.fetchone()[0] == 1
        semantic = exists
        if exists and key in ['view', 'function']:
            cursor.execute('SELECT id FROM nivc_view' if key == 'view' else 'SELECT nivc_function()')
            semantic = cursor.fetchall() == ((1,),)
        if exists and key == 'procedure':
            cursor.execute('CALL nivc_procedure()')
            semantic = cursor.fetchall() == ((1,),)
            while cursor.nextset():
                pass
        if exists and key == 'obj_user':
            cursor.execute("SHOW GRANTS FOR `" + user + "`@'0.0.0.0'")
            grants = cursor.fetchall()
            semantic = len(grants) == 1 and grants[0][0].startswith('GRANT USAGE ON *.* TO ')
        if exists and key == 'trigger':
            cursor.execute('SELECT ACTION_STATEMENT FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=%s AND TRIGGER_NAME=%s', (schema, 'nivc_trigger'))
            definition = cursor.fetchone()[0]
            semantic = 'NEW' in definition.upper() and 'ID' in definition.upper()
        records.append({'key': key, 'exists': exists, 'contentMatched': semantic, 'structureMatched': exists})
    cursor.execute('SHOW GRANTS')
    grants = [row[0] for row in cursor.fetchall()]
    import re
    global_all = any(re.match(r'^GRANT ALL(?: PRIVILEGES)? ON \*\.\* TO ', text, re.I) for text in grants)
    has_trigger = any(re.search(r'\bTRIGGER\b', text.split(' ON ', 1)[0], re.I) for text in grants)
    cursor.execute('SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=%s AND TABLE_NAME=%s', (schema, 'nivc_base'))
    return {'status': 'INSPECTED', 'ownerVerified': True, 'records': records, 'diagnostic': {'globalAllPrivileges': global_all, 'explicitTriggerPrivilege': has_trigger, 'referenceTableVisible': cursor.fetchone()[0] == 1}}
