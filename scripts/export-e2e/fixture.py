"""仅创建、检查和清理带本次所有权标识的固定合成夹具，不接收外部 SQL。"""
import base64
import datetime
import hashlib
import json
import pathlib
import re
import sys

sys.path.insert(0, str(pathlib.Path("artifacts/e2e-mysql-deps").resolve()))
import pymysql


def quote(value):
    if not re.fullmatch(r"[A-Za-z_][A-Za-z_0-9]{0,63}", value):
        raise ValueError("FIXTURE_IDENTIFIER_REJECTED")
    return "`" + value + "`"


RICH_COLUMNS = ["id", "ascii_label", "notes", "nullable_text", "empty_text", "amount"]
RICH_ROWS = [
    [1, "alpha", "plain", None, "", "12.340000"],
    [2, "beta", 'comma,quote"backslash\\', "NULL", "", "-0.010000"],
    [3, "gamma", "first\nsecond\r\nthird", "中文🙂", " ", "999.999999"],
    [4, "delta", "  padded  ", "tab\tvalue", "empty", "0.000000"],
]


def main():
    raw = bytearray(sys.stdin.buffer.read(65536))
    payload = json.loads(raw)
    for index in range(len(raw)):
        raw[index] = 0
    schema, run_id = payload["database"], payload["runId"]
    if not re.fullmatch(r"odo_e2e_[0-9]{8}_[a-f0-9]{12}", schema) or not re.fullmatch(r"[a-f0-9]{32}", run_id) or not schema.endswith(run_id[:12]):
        raise ValueError("FIXTURE_SCOPE_REJECTED")
    password = bytearray(base64.b64decode(payload.pop("password")))
    connection = None
    try:
        connection = pymysql.connect(host=payload["host"], port=payload["port"], user=payload["username"], password=bytes(password), charset="utf8mb4", connect_timeout=10, read_timeout=30, write_timeout=30, autocommit=True, local_infile=False)
        with connection.cursor() as cursor:
            action = payload["action"]
            if action == "create":
                # 不使用 IF NOT EXISTS；名称冲突时不接管、更改或清理既有数据库。
                cursor.execute("CREATE DATABASE " + quote(schema) + " DEFAULT CHARACTER SET utf8mb4")
                cursor.execute("USE " + quote(schema))
                cursor.execute("CREATE TABLE odo_owner (run_id VARCHAR(32) PRIMARY KEY)")
                cursor.execute("INSERT INTO odo_owner VALUES (%s)", (run_id,))
                cursor.execute("SET SESSION sql_mode = 'NO_ENGINE_SUBSTITUTION'")
                cursor.execute("CREATE TABLE rich (id INT PRIMARY KEY, ascii_label VARCHAR(32), notes TEXT, nullable_text VARCHAR(128), empty_text VARCHAR(64), amount DECIMAL(18,6))")
                cursor.executemany("INSERT INTO rich VALUES (%s,%s,%s,%s,%s,%s)", RICH_ROWS)
                cursor.execute("CREATE TABLE empty LIKE rich")
                cursor.execute("CREATE TABLE nopk (id INT, ascii_label VARCHAR(32))")
                cursor.executemany("INSERT INTO nopk VALUES (%s,%s)", [[1,"a"],[2,"b"],[3,"c"]])
                cursor.execute("CREATE TABLE bulk (id INT PRIMARY KEY, ascii_label VARCHAR(32), notes VARCHAR(2048))")
                cursor.executemany("INSERT INTO bulk VALUES (%s,%s,%s)", [[i,"row"+str(i),"x"*1024] for i in range(1,1025)])
                cursor.execute("CREATE TABLE slow LIKE bulk")
                for start in range(1,50001,1000):
                    cursor.executemany("INSERT INTO slow VALUES (%s,%s,%s)", [[i,"row"+str(i),"x"*512] for i in range(start,min(start+1000,50001))])
                cursor.execute("CREATE TABLE temporal (id INT PRIMARY KEY, date_col DATE, time_col TIME, datetime_col DATETIME, timestamp_col TIMESTAMP NULL)")
                cursor.execute("INSERT INTO temporal VALUES (1,'2024-02-29','12:34:56','2024-02-29 12:34:56','2024-02-29 12:34:56'),(2,NULL,NULL,NULL,NULL)")
                cursor.execute("CREATE TABLE types (id INT PRIMARY KEY, ascii_label VARCHAR(32), bin_col BLOB)")
                cursor.execute("INSERT INTO types VALUES (1,'binary',%s)", (bytes([0,1,127,255]),))
                capabilities = {}
                optional = {
                    "GENERATED": ["CREATE TABLE generated_values (id INT PRIMARY KEY, value_col INT, doubled INT GENERATED ALWAYS AS (value_col * 2) VIRTUAL)", "INSERT INTO generated_values(id,value_col) VALUES (1,7),(2,9)"],
                    "VIEW": ["CREATE VIEW v_rich AS SELECT id,ascii_label FROM rich"],
                    "FUNCTION": ["CREATE FUNCTION f_identity(value_arg INT) RETURNS INT DETERMINISTIC RETURN value_arg"],
                    "PROCEDURE": ["CREATE PROCEDURE p_identity(IN value_arg INT) SELECT value_arg"],
                    "SEQUENCE": ["CREATE SEQUENCE s_counter START WITH 17 INCREMENT BY 3 MAXVALUE 10000 NOCACHE"],
                    # 保留零日期只适用于 NOT NULL 日期列；可空列的 JDBC NULL 必须仍按 NULL 验证。
                    "ZERO_DATETIME": ["CREATE TABLE zeros (id INT PRIMARY KEY,date_col DATE NOT NULL,datetime_col DATETIME NOT NULL)","INSERT INTO zeros VALUES (1,'0000-00-00','0000-00-00 00:00:00')"],
                    "PARTITION": ["CREATE TABLE partitioned (id INT PRIMARY KEY,ascii_label VARCHAR(32)) PARTITION BY RANGE(id) (PARTITION p0 VALUES LESS THAN(3),PARTITION p1 VALUES LESS THAN MAXVALUE)","INSERT INTO partitioned VALUES (1,'a'),(2,'b'),(3,'c'),(4,'d')"],
                    "SUBPARTITION": ["CREATE TABLE subpartitioned (id INT PRIMARY KEY,ascii_label VARCHAR(32)) PARTITION BY RANGE(id) SUBPARTITION BY HASH(id) (PARTITION p0 VALUES LESS THAN(3) (SUBPARTITION sp00,SUBPARTITION sp01),PARTITION p1 VALUES LESS THAN MAXVALUE (SUBPARTITION sp10,SUBPARTITION sp11))","INSERT INTO subpartitioned VALUES (1,'a'),(2,'b'),(3,'c'),(4,'d')"],
                }
                for capability, statements in optional.items():
                    try:
                        for statement in statements:
                            cursor.execute(statement)
                        capabilities[capability] = True
                    except pymysql.MySQLError:
                        capabilities[capability] = False
                result = {"status":"CREATED","capabilities":capabilities,"richColumns":RICH_COLUMNS,"richRows":RICH_ROWS,"slowRows":50000,"zeroColumnsNullable":False}
                try:
                    cursor.execute("SELECT CURRENT_SCN()")
                    result["scn"] = str(cursor.fetchone()[0])
                    capabilities["SCN"] = True
                except pymysql.MySQLError:
                    capabilities["SCN"] = False
                print(json.dumps(result, ensure_ascii=True))
            elif action in ("augment", "exercise-sequence", "inspect", "cleanup"):
                cursor.execute("SELECT run_id FROM " + quote(schema) + ".odo_owner")
                if list(cursor.fetchall()) != [(run_id,)]:
                    raise ValueError("FIXTURE_OWNERSHIP_REJECTED")
                if action == "exercise-sequence":
                    cursor.execute("USE " + quote(schema))
                    result = {"status":"BLOCKED", "code":"SEQUENCE_ADVANCE_UNAVAILABLE", "ownershipVerified":True}
                    for query in ("SELECT NEXT VALUE FOR s_counter", "SELECT s_counter.NEXTVAL", "SELECT NEXTVAL(s_counter)"):
                        try:
                            cursor.execute(query)
                            first = int(cursor.fetchone()[0])
                            cursor.execute(query)
                            last = int(cursor.fetchone()[0])
                            result = {"status":"SEQUENCE_EXERCISED", "first":first, "last":last, "expectedNext":last+3, "restart":17, "ownershipVerified":True}
                            break
                        except pymysql.MySQLError:
                            continue
                    print(json.dumps(result))
                elif action == "augment":
                    cursor.execute("USE " + quote(schema))
                    result = {"status":"AUGMENTED", "ownershipVerified":True, "partitionRows":{}}
                    try:
                        cursor.execute("CREATE TABLE generated_values (id INT PRIMARY KEY, value_col INT, doubled INT GENERATED ALWAYS AS (value_col * 2) VIRTUAL)")
                        cursor.execute("INSERT INTO generated_values(id,value_col) VALUES (1,7),(2,9)")
                        result["generated"] = True
                    except pymysql.MySQLError as error:
                        result["generated"] = False
                        result["generatedErrorNumber"] = error.args[0] if isinstance(error.args[0], int) else 0
                    for table, partition in (("partitioned", "p0"), ("subpartitioned", "sp00")):
                        try:
                            cursor.execute("SELECT id,ascii_label FROM " + quote(schema) + "." + quote(table) + " PARTITION (" + quote(partition) + ") ORDER BY id")
                            result["partitionRows"][table] = list(cursor.fetchall())
                        except pymysql.MySQLError:
                            result["partitionRows"][table] = []
                    print(json.dumps(result))
                elif action == "cleanup":
                    cursor.execute("DROP DATABASE " + quote(schema))
                    cursor.execute("SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME=%s", (schema,))
                    if cursor.fetchone()[0] != 0:
                        raise ValueError("FIXTURE_CLEANUP_NOT_CONFIRMED")
                    print(json.dumps({"status":"CLEANED","ownershipVerified":True,"databaseAbsent":True}))
                else:
                    cursor.execute("SELECT COUNT(*) FROM " + quote(schema) + ".rich")
                    result = {"status":"READY","richRows":cursor.fetchone()[0],"ownershipVerified":True}
                    # 使用字符转换核对本次合成零日期，避免 Python 日期转换与 JDBC 行为混淆。
                    try:
                        cursor.execute("SELECT CAST(date_col AS CHAR),CAST(datetime_col AS CHAR) FROM " + quote(schema) + ".zeros ORDER BY id")
                        result["sourceZeroStrings"] = list(cursor.fetchall())
                        cursor.execute("SELECT COLUMN_NAME,IS_NULLABLE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=%s AND TABLE_NAME='zeros' AND COLUMN_NAME IN ('date_col','datetime_col') ORDER BY COLUMN_NAME", (schema,))
                        result["zeroColumns"] = list(cursor.fetchall())
                        result["zeroColumnsNullable"] = any(row[1] == "YES" for row in result["zeroColumns"])
                    except pymysql.MySQLError:
                        result["sourceZeroStrings"] = []
                    try:
                        cursor.execute("SELECT __pk_increment FROM " + quote(schema) + ".nopk WHERE 1=0")
                        result["hiddenPkZeroRowReadable"] = True
                    except pymysql.MySQLError as error:
                        result["hiddenPkZeroRowReadable"] = False
                        result["hiddenPkErrorNumber"] = error.args[0] if isinstance(error.args[0], int) else 0
                    try:
                        cursor.execute("SELECT CURRENT_SCN()")
                        result["scn"] = str(cursor.fetchone()[0])
                    except pymysql.MySQLError:
                        pass
                    print(json.dumps(result))
            else:
                raise ValueError("FIXTURE_ACTION_REJECTED")
    finally:
        for index in range(len(password)):
            password[index] = 0
        if connection:
            connection.close()
            connection.password = b""


if __name__ == "__main__":
    try:
        main()
    except Exception:
        # 任何驱动错误、端点、用户名、SQL 文本与密码都不能进入测试标准输出。
        print(json.dumps({"status":"BLOCKED","code":"FIXTURE_DATABASE_OPERATION_UNAVAILABLE"}))
        sys.exit(1)
