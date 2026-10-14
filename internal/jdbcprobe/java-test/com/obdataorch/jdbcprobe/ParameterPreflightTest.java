package com.obdataorch.jdbcprobe;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.lang.reflect.Field;
import java.lang.reflect.InvocationHandler;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.lang.reflect.Proxy;
import java.nio.charset.StandardCharsets;
import java.sql.Connection;
import java.sql.DatabaseMetaData;
import java.sql.Driver;
import java.sql.DriverManager;
import java.sql.DriverPropertyInfo;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.Properties;
import java.util.logging.Logger;

/** 所有 JDBC 对象均为合成代理；检查固定 SQL、旧版本权限、sys 拒绝与协议清零。 */
public final class ParameterPreflightTest {
    private ParameterPreflightTest() {}

    public static void main(String[] args) throws Exception {
        List<String> reads = new ArrayList<String>();
        Connection current = connection("OceanBase 4.3.5", false, reads, null);
        assertStatus("ACCESSIBLE", verify(current, true, "synthetic_user", 3));
        if (!reads.equals(Arrays.asList(
                "SELECT /*+ READ_CONSISTENCY(WEAK) */ 1 FROM `synthetic_db`.`synthetic_table` LIMIT 1",
                "SELECT `__pk_increment` FROM `synthetic_db`.`synthetic_table` WHERE 1 = 0"))) {
            throw new AssertionError("读取越过固定常量和隐藏列范围");
        }
        reads.clear();
        assertStatus("NOT_ACCESSIBLE", verify(connection("OceanBase 3.2.4", false, reads, null), true, "synthetic_user", 2));
        if (!reads.isEmpty()) throw new AssertionError("旧版普通用户触发了隐藏列读取");
        assertStatus("ACCESSIBLE", verify(connection("OceanBase 3.2.4", false, reads, null), true, "__oceanbase_inner_drc_user@synthetic_tenant", 2));
        reads.clear();
        assertStatus("ACCESSIBLE", verify(connection("OceanBase 4.3.5", true, reads, null), true, "synthetic_user", 2));
        if (!reads.isEmpty()) throw new AssertionError("有主键表被当作隐藏主键表");
        assertStatus("UNAVAILABLE", verify(connection("unknown", false, reads, null), true, "synthetic_user", 2));
        assertStatus("NOT_ACCESSIBLE", verify(connection("OceanBase 4.3.5", false, reads, "42501"), true, "synthetic_user", 1));
        assertStatus("UNAVAILABLE", verify(connection("OceanBase 4.3.5", false, reads, "42000"), true, "synthetic_user", 1));
        reads.clear();
        assertStatus("ACCESSIBLE", verify(current, false, "synthetic_user", 1));
        if (!reads.get(0).endsWith(" WHERE ROWNUM <= 1")) throw new AssertionError("Oracle 弱读边界错误");

        Driver driver = new Driver() {
            public Connection connect(String url, Properties properties) throws SQLException {
                if (!acceptsURL(url)) return null;
                if (!"synthetic_sys@sys#synthetic_cluster".equals(properties.getProperty("user"))
                        || !"synthetic_sys_password".equals(properties.getProperty("password"))) throw new AssertionError("sys 材料未使用冻结身份");
                return connection("OceanBase 4.3.5", true, reads, null);
            }
            public boolean acceptsURL(String url) { return "jdbc:synthetic:sys".equals(url); }
            public DriverPropertyInfo[] getPropertyInfo(String url, Properties properties) { return new DriverPropertyInfo[0]; }
            public int getMajorVersion() { return 1; }
            public int getMinorVersion() { return 0; }
            public boolean jdbcCompliant() { return false; }
            public Logger getParentLogger() { return Logger.getGlobal(); }
        };
        DriverManager.registerDriver(driver);
        try {
            reads.clear();
            assertStatus("ACCESSIBLE", ParameterPreflight.verify(current, true, "synthetic_db", new ArrayList<String>(), true,
                    "synthetic_user", 4, "jdbc:synthetic:sys", bytes("synthetic_sys@sys#synthetic_cluster"), bytes("synthetic_sys_password")));
            if (!reads.equals(Arrays.asList("SELECT TABLEGROUP_NAME FROM oceanbase.DBA_OB_TABLEGROUPS WHERE 1 = 0"))) throw new AssertionError("sys 探针没有验证固定视图");
        } finally {
            DriverManager.deregisterDriver(driver);
        }
        Object input = readProtocol(7, "synthetic_sys", "synthetic_sys_password");
        Class<?> type = input.getClass();
        Field username = type.getDeclaredField("sysUsername");
        Field password = type.getDeclaredField("sysPassword");
        username.setAccessible(true);
        password.setAccessible(true);
        byte[] userBytes = (byte[]) username.get(input);
        byte[] passwordBytes = (byte[]) password.get(input);
        Method destroy = type.getDeclaredMethod("destroy");
        destroy.setAccessible(true);
        destroy.invoke(input);
        for (byte[] buffer : new byte[][] { userBytes, passwordBytes }) for (byte value : buffer) if (value != 0) throw new AssertionError("sys 输入未清零");
        for (int flags : new int[] { 0, 8, 4 }) {
            try {
                readProtocol(flags, "", "");
                throw new AssertionError("无效参数协议被接受");
            } catch (InvocationTargetException expected) {
                if (!(expected.getCause() instanceof IllegalArgumentException)) throw expected;
            }
        }
    }

    private static String verify(Connection connection, boolean mysql, String user, int flags) {
        return ParameterPreflight.verify(connection, mysql, "synthetic_db", Arrays.asList("synthetic_table"), false, user, flags, "", new byte[0], new byte[0]);
    }

    private static Connection connection(String version, boolean primary, List<String> reads, String failure) {
        DatabaseMetaData metadata = proxy(DatabaseMetaData.class, (p, method, args) -> {
            if ("getDatabaseProductName".equals(method.getName())) return "OceanBase";
            if ("getDatabaseProductVersion".equals(method.getName())) return version;
            if ("getPrimaryKeys".equals(method.getName())) return rows(primary);
            throw new AssertionError("非固定元数据操作");
        });
        return proxy(Connection.class, (p, method, args) -> {
            if ("getMetaData".equals(method.getName())) return metadata;
            if ("close".equals(method.getName())) return null;
            if ("createStatement".equals(method.getName())) return proxy(Statement.class, (s, operation, values) -> {
                if ("setQueryTimeout".equals(operation.getName()) || "close".equals(operation.getName())) return null;
                if ("executeQuery".equals(operation.getName())) {
                    reads.add((String) values[0]);
                    if (failure != null) throw new SQLException("synthetic failure", failure);
                    return rows(false);
                }
                throw new AssertionError("非固定 Statement 操作");
            });
            throw new AssertionError("非固定 Connection 操作");
        });
    }

    private static ResultSet rows(boolean present) {
        final boolean[] first = { present };
        return proxy(ResultSet.class, (p, method, args) -> {
            if ("close".equals(method.getName())) return null;
            if ("next".equals(method.getName())) { boolean value = first[0]; first[0] = false; return value; }
            throw new AssertionError("探针读取了源数据");
        });
    }

    private static Object readProtocol(int flags, String user, String password) throws Exception {
        ByteArrayOutputStream buffer = new ByteArrayOutputStream();
        DataOutputStream output = new DataOutputStream(buffer);
        output.writeInt(6);
        value(output, "synthetic.invalid"); output.writeInt(2883);
        value(output, "synthetic_user"); value(output, "synthetic_password");
        value(output, "MYSQL"); value(output, "synthetic_db"); output.writeInt(0);
        output.writeInt(flags); value(output, user); value(output, password);
        Class<?> type = Class.forName("com.obdataorch.jdbcprobe.ConnectionProbe$ProbeInput");
        Method read = type.getDeclaredMethod("read", DataInputStream.class);
        read.setAccessible(true);
        return read.invoke(null, new DataInputStream(new ByteArrayInputStream(buffer.toByteArray())));
    }

    private static void value(DataOutputStream output, String value) throws Exception { byte[] content = bytes(value); output.writeInt(content.length); output.write(content); }
    private static byte[] bytes(String value) { return value.getBytes(StandardCharsets.UTF_8); }
    private static void assertStatus(String expected, String actual) { if (!expected.equals(actual)) throw new AssertionError("固定检查没有失败关闭：" + actual); }
    private static <T> T proxy(Class<T> type, InvocationHandler handler) { return type.cast(Proxy.newProxyInstance(type.getClassLoader(), new Class<?>[] { type }, handler)); }
}
