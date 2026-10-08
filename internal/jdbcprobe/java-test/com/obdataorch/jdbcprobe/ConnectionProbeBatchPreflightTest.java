package com.obdataorch.jdbcprobe;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.PrintStream;
import java.lang.reflect.InvocationHandler;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.lang.reflect.Proxy;
import java.nio.charset.StandardCharsets;
import java.sql.Connection;
import java.sql.DatabaseMetaData;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.Statement;
import java.util.HashMap;
import java.util.Map;

/** 批量预检查只使用合成 JDBC 代理，验证单连接调用、五类对象和失败关闭。 */
public final class ConnectionProbeBatchPreflightTest {
    private static final String[] TYPES = { "TABLE", "TABLE", "VIEW", "FUNCTION", "PROCEDURE", "SEQUENCE" };
    private static final String[] NAMES = { "table_one", "table_two", "view_one", "fn_one", "proc_one", "seq_one" };

    private ConnectionProbeBatchPreflightTest() { }

    public static void main(String[] args) throws Exception {
        verify(false, "ACCESSIBLE");
        verify(true, "NOT_ACCESSIBLE");
        try {
            read(new String[] { "TRIGGER" }, new String[] { "unsafe" });
            throw new AssertionError("批量协议接受了未开放对象类型");
        } catch (InvocationTargetException expected) {
            if (!(expected.getCause() instanceof IllegalArgumentException)) throw expected;
        }
    }

    private static void verify(boolean missingSequence, String expected) throws Exception {
        Object input = read(TYPES, NAMES);
        Class<?> inputClass = input.getClass();
        Method print = ConnectionProbe.class.getDeclaredMethod("printBatchPreflightSuccess", Connection.class, inputClass);
        print.setAccessible(true);
        final int[] tableIndex = { 0 };
        final int[] metadataCalls = { 0 };
        final int[] zeroRowReads = { 0 };
        DatabaseMetaData metadata = proxy(DatabaseMetaData.class, (proxy, method, args) -> {
            String operation = method.getName();
            if ("getDatabaseProductName".equals(operation)) return "OceanBase";
            if ("getDatabaseProductVersion".equals(operation)) return "4.3";
            if ("getDriverName".equals(operation)) return "synthetic-driver";
            if ("getDriverVersion".equals(operation)) return "1";
            if ("getSearchStringEscape".equals(operation)) return "\\";
            if ("getTables".equals(operation)) {
                metadataCalls[0]++;
                String type = ((String[]) args[3])[0];
                String name = "VIEW".equals(type) ? "view_one" : NAMES[tableIndex[0]++];
                return result(row("TABLE_CAT", "synthetic_db", "TABLE_NAME", name, "TABLE_TYPE", type));
            }
            if ("getFunctions".equals(operation)) {
                metadataCalls[0]++;
                return result(row("FUNCTION_CAT", "synthetic_db", "FUNCTION_NAME", "fn_one"));
            }
            if ("getProcedures".equals(operation)) {
                metadataCalls[0]++;
                return result(row("PROCEDURE_CAT", "synthetic_db", "PROCEDURE_NAME", "proc_one"));
            }
            throw new AssertionError("调用了未授权的元数据方法：" + operation);
        });
        Connection connection = proxy(Connection.class, (proxy, method, args) -> {
            String operation = method.getName();
            if ("getMetaData".equals(operation)) return metadata;
            if ("createStatement".equals(operation)) return proxy(Statement.class, (statement, statementMethod, statementArgs) -> {
                if ("setQueryTimeout".equals(statementMethod.getName()) || "close".equals(statementMethod.getName())) return null;
                if ("executeQuery".equals(statementMethod.getName())) {
                    String sql = (String) statementArgs[0];
                    if (!sql.startsWith("SELECT 1 FROM `synthetic_db`.`table_") || !sql.endsWith(" WHERE 1 = 0")) {
                        throw new AssertionError("零行读取越出固定表范围");
                    }
                    zeroRowReads[0]++;
                    return result(null);
                }
                throw new AssertionError("调用了未授权的 Statement 方法");
            });
            if ("prepareStatement".equals(operation)) {
                String sql = (String) args[0];
                if (!sql.equals("SELECT SEQUENCE_NAME FROM oceanbase.DBA_SEQUENCES WHERE SEQUENCE_OWNER = ? AND SEQUENCE_NAME = ?")) {
                    throw new AssertionError("序列查询越出固定视图范围");
                }
                return proxy(PreparedStatement.class, (statement, statementMethod, statementArgs) -> {
                    String action = statementMethod.getName();
                    if ("setString".equals(action) || "setQueryTimeout".equals(action) || "close".equals(action)) return null;
                    if ("executeQuery".equals(action)) return result(missingSequence ? null : row("1", "seq_one"));
                    throw new AssertionError("调用了未授权的 PreparedStatement 方法");
                });
            }
            throw new AssertionError("调用了未授权的 Connection 方法：" + operation);
        });
        ByteArrayOutputStream output = new ByteArrayOutputStream();
        PrintStream previous = System.out;
        try {
            System.setOut(new PrintStream(output, true, "UTF-8"));
            print.invoke(null, connection, input);
        } finally {
            System.setOut(previous);
        }
        String response = output.toString("UTF-8");
        if (!response.contains("\"objectAccess\":\"" + expected + "\"") || response.contains("table_one")
                || zeroRowReads[0] != 2 || metadataCalls[0] != 5) {
            throw new AssertionError("批量预检查结果或受控调用不正确");
        }
    }

    private static Object read(String[] types, String[] names) throws Exception {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream output = new DataOutputStream(bytes);
        output.writeInt(5);
        write(output, "synthetic.invalid");
        output.writeInt(2883);
        write(output, "synthetic_user");
        write(output, "synthetic_password");
        write(output, "MYSQL");
        write(output, "synthetic_db");
        output.writeInt(types.length);
        for (int index = 0; index < types.length; index++) {
            write(output, types[index]);
            write(output, names[index]);
        }
        Class<?> inputClass = Class.forName("com.obdataorch.jdbcprobe.ConnectionProbe$ProbeInput");
        Method read = inputClass.getDeclaredMethod("read", DataInputStream.class);
        read.setAccessible(true);
        return read.invoke(null, new DataInputStream(new ByteArrayInputStream(bytes.toByteArray())));
    }

    private static void write(DataOutputStream output, String value) throws Exception {
        byte[] bytes = value.getBytes(StandardCharsets.UTF_8);
        output.writeInt(bytes.length);
        output.write(bytes);
    }

    private static Map<String, String> row(String... pairs) {
        Map<String, String> row = new HashMap<String, String>();
        for (int index = 0; index < pairs.length; index += 2) row.put(pairs[index], pairs[index + 1]);
        return row;
    }

    private static ResultSet result(Map<String, String> row) {
        final boolean[] read = { false };
        return proxy(ResultSet.class, (proxy, method, args) -> {
            if ("next".equals(method.getName())) {
                if (read[0] || row == null) return false;
                read[0] = true;
                return true;
            }
            if ("getString".equals(method.getName())) return row.get(String.valueOf(args[0]));
            if ("close".equals(method.getName())) return null;
            throw new AssertionError("调用了未授权的 ResultSet 方法");
        });
    }

    @SuppressWarnings("unchecked")
    private static <T> T proxy(Class<T> type, InvocationHandler handler) {
        return (T) Proxy.newProxyInstance(ConnectionProbeBatchPreflightTest.class.getClassLoader(), new Class<?>[] { type }, handler);
    }
}
