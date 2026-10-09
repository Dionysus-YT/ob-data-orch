package com.obdataorch.jdbcprobe;

import java.io.ByteArrayOutputStream;
import java.io.PrintStream;
import java.lang.reflect.Constructor;
import java.lang.reflect.Method;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Proxy;
import java.nio.charset.StandardCharsets;
import java.sql.Connection;
import java.sql.DatabaseMetaData;
import java.sql.PreparedStatement;
import java.sql.ResultSet;

/** 使用合成 JDBC 接口验证单类及五类大目录，不访问数据库或加载驱动。 */
public final class ConnectionProbeObjectCatalogTest {
    private static final int COUNT = 10000;
    private static final String[] TYPES = { "TABLE", "VIEW", "FUNCTION", "PROCEDURE", "SEQUENCE" };

    public static void main(String[] args) throws Exception {
        for (String mode : new String[] { "MYSQL", "ORACLE" }) {
            for (String type : TYPES) verify(query(mode, type), type, false);
            String combined = query(mode, "ALL");
            for (String type : TYPES) verify(combined, type, true);
        }
        verifyBudget();
    }

    private static void verifyBudget() throws Exception {
        Class<?> rowsClass = Class.forName("com.obdataorch.jdbcprobe.ConnectionProbe$CatalogRows");
        Constructor<?> constructor = rowsClass.getDeclaredConstructor();
        constructor.setAccessible(true);
        Object rows = constructor.newInstance();
        Method add = rowsClass.getDeclaredMethod("add", String.class, String.class);
        add.setAccessible(true);
        String prefix = new String(new char[240]).replace('\0', 'x');
        for (int index = 0; index < 100000; index++) {
            try { add.invoke(rows, prefix + index, ""); }
            catch (InvocationTargetException exception) {
                if (exception.getCause().getClass().getSimpleName().equals("CatalogBudgetExceeded")) return;
                throw exception;
            }
        }
        throw new AssertionError("字节预算超限未失败");
    }

    private static void verify(String output, String type, boolean combined) {
        if (!output.contains("\"" + type + "_9999\"") || output.contains("\"truncated\":true") || output.contains("\"unavailable\":true")) {
            throw new AssertionError("大目录缺失尾部对象：" + type + "/" + combined);
        }
        int count = 0, offset = 0;
        while ((offset = output.indexOf("\"" + type + "_", offset)) >= 0) { count++; offset++; }
        if (count != COUNT) throw new AssertionError("对象数量错误：" + count);
    }

    private static String query(String mode, String type) throws Exception {
        Class<?> inputClass = Class.forName("com.obdataorch.jdbcprobe.ConnectionProbe$ProbeInput");
        Constructor<?> constructor = inputClass.getDeclaredConstructor(int.class, byte[].class, int.class,
                byte[].class, byte[].class, byte[].class, byte[].class, byte[].class, byte[].class);
        constructor.setAccessible(true);
        Object input = constructor.newInstance(4, bytes("synthetic.invalid"), 2883, bytes("synthetic_user"),
                bytes("synthetic_password"), bytes(mode), bytes("synthetic_db"), bytes(type), bytes(""));
        DatabaseMetaData metadata = (DatabaseMetaData) Proxy.newProxyInstance(ConnectionProbeObjectCatalogTest.class.getClassLoader(),
                new Class<?>[] { DatabaseMetaData.class }, (proxy, method, args) -> {
                    if ("getSearchStringEscape".equals(method.getName())) return "\\";
                    if ("getTables".equals(method.getName())) return rows(((String[]) args[3])[0]);
                    if ("getFunctions".equals(method.getName())) return rows("FUNCTION");
                    if ("getProcedures".equals(method.getName())) return rows("PROCEDURE");
                    throw new AssertionError("未授权元数据方法：" + method.getName());
                });
        Connection connection = (Connection) Proxy.newProxyInstance(ConnectionProbeObjectCatalogTest.class.getClassLoader(),
                new Class<?>[] { Connection.class }, (proxy, method, args) -> {
                    if ("getMetaData".equals(method.getName())) return metadata;
                    if (!"prepareStatement".equals(method.getName())) throw new AssertionError("未授权连接方法");
                    String sql = (String) args[0];
                    if (!sql.contains("SEQUENCES") || !sql.contains("?")) throw new AssertionError("序列查询未绑定数据库");
                    return Proxy.newProxyInstance(ConnectionProbeObjectCatalogTest.class.getClassLoader(), new Class<?>[] { PreparedStatement.class },
                            (statement, operation, values) -> {
                                if ("executeQuery".equals(operation.getName())) return rows("SEQUENCE");
                                if ("setString".equals(operation.getName()) || "setQueryTimeout".equals(operation.getName()) || "close".equals(operation.getName())) return null;
                                throw new AssertionError("未授权语句方法");
                            });
                });
        Method print = ConnectionProbe.class.getDeclaredMethod("printCatalogSuccess", Connection.class, inputClass);
        print.setAccessible(true);
        ByteArrayOutputStream output = new ByteArrayOutputStream();
        PrintStream previous = System.out;
        try {
            System.setOut(new PrintStream(output, true, "UTF-8"));
            print.invoke(null, connection, input);
        } finally { System.setOut(previous); }
        return output.toString("UTF-8");
    }

    private static ResultSet rows(String type) {
        int[] position = { -1 };
        return (ResultSet) Proxy.newProxyInstance(ConnectionProbeObjectCatalogTest.class.getClassLoader(), new Class<?>[] { ResultSet.class },
                (proxy, method, args) -> {
                    if ("next".equals(method.getName())) return ++position[0] < COUNT;
                    if ("close".equals(method.getName())) return null;
                    if ("getString".equals(method.getName())) {
                        Object column = args[0];
                        if (column instanceof Integer || column.toString().endsWith("_NAME")) return type + "_" + position[0];
                        if ("TABLE_TYPE".equals(column)) return type;
                        return "synthetic_db";
                    }
                    throw new AssertionError("未授权结果集方法");
                });
    }

    private static byte[] bytes(String value) { return value.getBytes(StandardCharsets.UTF_8); }
}
