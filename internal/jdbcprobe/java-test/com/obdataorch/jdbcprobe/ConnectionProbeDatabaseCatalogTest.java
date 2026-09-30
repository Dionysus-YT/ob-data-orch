package com.obdataorch.jdbcprobe;

import java.io.ByteArrayOutputStream;
import java.io.PrintStream;
import java.lang.reflect.Constructor;
import java.lang.reflect.InvocationHandler;
import java.lang.reflect.Method;
import java.lang.reflect.Proxy;
import java.nio.charset.StandardCharsets;
import java.sql.DatabaseMetaData;
import java.sql.ResultSet;

/** 数据库目录测试只使用合成 JDBC 元数据，不建立真实数据库连接。 */
public final class ConnectionProbeDatabaseCatalogTest {
    private ConnectionProbeDatabaseCatalogTest() {
    }

    public static void main(String[] args) throws Exception {
        String mysql = query("MYSQL", "app", new String[] { "app_main", "system", "app_archive" });
        if (!mysql.contains("\"objects\":[\"app_main\",\"app_archive\"]") || !mysql.contains("\"truncated\":false")) {
            throw new AssertionError("MySQL 数据库目录筛选错误");
        }
        String oracle = query("ORACLE", "", new String[] { "APP_SCHEMA" });
        if (!oracle.contains("\"objects\":[\"APP_SCHEMA\"]")) {
            throw new AssertionError("Oracle Schema 目录读取错误");
        }
        String[] many = new String[101];
        for (int index = 0; index < many.length; index++) {
            many[index] = "synthetic_" + index;
        }
        String limited = query("MYSQL", "", many);
        if (!limited.contains("\"truncated\":true") || limited.contains("synthetic_100")) {
            throw new AssertionError("数据库目录未按 100 项截断");
        }
    }

    private static String query(String mode, String keyword, final String[] names) throws Exception {
        Class<?> inputClass = Class.forName("com.obdataorch.jdbcprobe.ConnectionProbe$ProbeInput");
        Constructor<?> constructor = inputClass.getDeclaredConstructor(int.class, byte[].class, int.class,
                byte[].class, byte[].class, byte[].class, byte[].class, byte[].class, byte[].class);
        constructor.setAccessible(true);
        Object input = constructor.newInstance(4, bytes("synthetic.invalid"), 2883, bytes("synthetic_user"),
                bytes("synthetic_password"), bytes(mode), bytes(""), bytes("DATABASE"), bytes(keyword));
        Method print = ConnectionProbe.class.getDeclaredMethod("printDatabaseCatalogSuccess", DatabaseMetaData.class, inputClass);
        print.setAccessible(true);
        final String expectedMethod = "MYSQL".equals(mode) ? "getCatalogs" : "getSchemas";
        InvocationHandler metadataHandler = (proxy, method, args) -> {
            if (!expectedMethod.equals(method.getName())) {
                throw new AssertionError("调用了未授权的元数据方法：" + method.getName());
            }
            final int[] position = { -1 };
            InvocationHandler resultHandler = (result, resultMethod, resultArgs) -> {
                if ("next".equals(resultMethod.getName())) {
                    position[0]++;
                    return position[0] < names.length;
                }
                if ("getString".equals(resultMethod.getName())) {
                    String expectedColumn = "MYSQL".equals(mode) ? "TABLE_CAT" : "TABLE_SCHEM";
                    if (!expectedColumn.equals(resultArgs[0])) {
                        throw new AssertionError("读取了错误的元数据列");
                    }
                    return names[position[0]];
                }
                if ("close".equals(resultMethod.getName())) {
                    return null;
                }
                throw new AssertionError("调用了未授权的结果集方法：" + resultMethod.getName());
            };
            return Proxy.newProxyInstance(ConnectionProbeDatabaseCatalogTest.class.getClassLoader(),
                    new Class<?>[] { ResultSet.class }, resultHandler);
        };
        DatabaseMetaData metadata = (DatabaseMetaData) Proxy.newProxyInstance(
                ConnectionProbeDatabaseCatalogTest.class.getClassLoader(), new Class<?>[] { DatabaseMetaData.class }, metadataHandler);
        ByteArrayOutputStream output = new ByteArrayOutputStream();
        PrintStream previous = System.out;
        try {
            System.setOut(new PrintStream(output, true, "UTF-8"));
            print.invoke(null, metadata, input);
        } finally {
            System.setOut(previous);
        }
        return output.toString("UTF-8");
    }

    private static byte[] bytes(String value) {
        return value.getBytes(StandardCharsets.UTF_8);
    }
}
