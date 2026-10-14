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
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;

/** 仅用合成 JDBC 代理验证字面列、子分区、绑定参数与未知结果，不建立网络连接。 */
public final class SelectionPreflightTest {
    private SelectionPreflightTest() {}

    public static void main(String[] args) throws Exception {
        expect("ACCESSIBLE", verify(true, false, "id", "sp0"));
        expect("NOT_ACCESSIBLE", verify(true, false, "ID", "sp0"));
        expect("NOT_ACCESSIBLE", verify(true, false, "missing", "sp0"));
        expect("NOT_ACCESSIBLE", verify(true, false, "id", "p0"));
        expect("NOT_ACCESSIBLE", verify(true, false, "id", "SP0"));
        expect("NOT_ACCESSIBLE", verify(true, false, "id", "missing"));
        expect("ACCESSIBLE", verify(false, false, "id", "sp0"));
        expect("UNAVAILABLE", verify(true, true, "id", "sp0"));
        for (int version : new int[] {7, 8}) {
            Object input = read(version, new String[] {"id"});
            Field field = input.getClass().getDeclaredField("includeColumnNames"); field.setAccessible(true);
            @SuppressWarnings("unchecked") List<byte[]> values = (List<byte[]>) field.get(input);
            byte[] retained = values.get(0);
            Method destroy = input.getClass().getDeclaredMethod("destroy"); destroy.setAccessible(true); destroy.invoke(input);
            if (!values.isEmpty()) throw new AssertionError("选择列表未清除");
            for (byte value : retained) if (value != 0) throw new AssertionError("选择字节未清零");
            for (String[] invalid : new String[][] {{"id", "id"}, {"*"}, {"id\n"}, {" id"}, {}}) {
                try { read(version, invalid); throw new AssertionError("协议接受了非法选择列表"); }
                catch (InvocationTargetException expected) { if (!(expected.getCause() instanceof IllegalArgumentException)) throw expected; }
            }
        }
    }

    private static String verify(boolean mysql, boolean unavailable, String column, String partition) {
        DatabaseMetaData metadata = proxy(DatabaseMetaData.class, (proxy, method, args) -> {
            if (method.getName().equals("getSearchStringEscape")) return "\\";
            if (method.getName().equals("getColumns")) {
                if (!"synthetic\\_table".equals(args[2])) throw new AssertionError("未转义元数据模式");
                if (unavailable) throw new SQLException("合成元数据不可用");
                return rows(new String[][] {{"id"}}, mysql);
            }
            throw new AssertionError("未授权元数据方法");
        });
        Connection connection = proxy(Connection.class, (proxy, method, args) -> {
            if (method.getName().equals("getMetaData")) return metadata;
            if (method.getName().equals("prepareStatement")) {
                String sql = (String) args[0];
                String expected = mysql ? "SELECT PARTITION_NAME, SUBPARTITION_NAME FROM information_schema.PARTITIONS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?" : "SELECT SUBPARTITION_NAME FROM ALL_TAB_SUBPARTITIONS WHERE TABLE_OWNER = ? AND TABLE_NAME = ?";
                if (!expected.equals(sql)) throw new AssertionError("执行了非固定 SQL");
                final int[] bindings = {0};
                return proxy(PreparedStatement.class, (statement, operation, values) -> {
                    if (operation.getName().equals("setString")) {
                        if (!((Integer) values[0] == 1 ? "synthetic_db" : "synthetic_table").equals(values[1])) throw new AssertionError("未使用冻结值绑定");
                        bindings[0]++; return null;
                    }
                    if (operation.getName().equals("executeQuery")) {
                        if (bindings[0] != 2) throw new AssertionError("未完整绑定");
                        return rows(mysql ? new String[][] {{"p0", "sp0"}} : new String[][] {{"sp0"}}, mysql);
                    }
                    if (operation.getName().equals("setQueryTimeout") || operation.getName().equals("close")) return null;
                    throw new AssertionError("未授权语句方法");
                });
            }
            throw new AssertionError("未授权连接方法");
        });
        return SelectionPreflight.verify(connection, mysql, "synthetic_db", "synthetic_table", Arrays.asList(column), Collections.<String>emptyList(), Arrays.asList(partition));
    }

    private static ResultSet rows(String[][] rows, boolean mysql) {
        final int[] index = {-1};
        return proxy(ResultSet.class, (proxy, method, args) -> {
            if (method.getName().equals("next")) return ++index[0] < rows.length;
            if (method.getName().equals("close")) return null;
            if (method.getName().equals("getString")) {
                if (args[0] instanceof Integer) return rows[index[0]][(Integer) args[0] - 1];
                if (args[0].equals(mysql ? "TABLE_CAT" : "TABLE_SCHEM")) return "synthetic_db";
                if (args[0].equals("TABLE_NAME")) return "synthetic_table";
                if (args[0].equals("COLUMN_NAME")) return rows[index[0]][0];
            }
            throw new AssertionError("未授权结果读取");
        });
    }

    private static Object read(int version, String[] columns) throws Exception {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream(); DataOutputStream output = new DataOutputStream(bytes);
        output.writeInt(version); write(output, "synthetic.invalid"); output.writeInt(2883);
        write(output, "synthetic_user"); write(output, "synthetic_password"); write(output, "MYSQL"); write(output, "synthetic_db");
        output.writeInt(1); write(output, "TABLE"); write(output, "synthetic_table");
        output.writeInt(columns.length); for (String value : columns) write(output, value); output.writeInt(0); output.writeInt(0);
        if (version == 8) { output.writeInt(1); write(output, ""); write(output, ""); }
        Class<?> type = Class.forName("com.obdataorch.jdbcprobe.ConnectionProbe$ProbeInput");
        Method read = type.getDeclaredMethod("read", DataInputStream.class); read.setAccessible(true);
        return read.invoke(null, new DataInputStream(new ByteArrayInputStream(bytes.toByteArray())));
    }

    private static void write(DataOutputStream output, String value) throws Exception {
        byte[] bytes = value.getBytes(StandardCharsets.UTF_8); output.writeInt(bytes.length); output.write(bytes);
    }
    private static void expect(String expected, String actual) { if (!expected.equals(actual)) throw new AssertionError("选择检查结果不符合固定事实"); }
    @SuppressWarnings("unchecked")
    private static <T> T proxy(Class<T> type, InvocationHandler handler) { return (T) Proxy.newProxyInstance(SelectionPreflightTest.class.getClassLoader(), new Class<?>[] {type}, handler); }
}
