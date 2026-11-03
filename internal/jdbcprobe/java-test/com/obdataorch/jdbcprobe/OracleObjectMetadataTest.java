package com.obdataorch.jdbcprobe;

import java.lang.reflect.Proxy;
import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.ArrayList;
import java.util.List;

/** 合成 JDBC 验证扩展对象与例程的绑定、空结果、失败关闭及公共同义词 Schema 边界。 */
public final class OracleObjectMetadataTest {
    public static void main(String[] args) throws Exception {
        for (String type : new String[] { "PACKAGE", "TRIGGER", "SYNONYM", "PUBLIC_SYNONYM", "TYPE", "FUNCTION", "PROCEDURE" }) {
            List<String> names = new ArrayList<String>();
            OracleObjectMetadata.readNames(connection(type, null, 1, false), type, "synthetic_'schema", names::add);
            if (names.size() != 1 || !"synthetic_'object".equals(names.get(0))) throw new AssertionError("目录名称未保留");
            for (int count : new int[] { 0, 1, 2 }) {
                String expected = count == 0 ? "NOT_ACCESSIBLE" : count == 1 ? "ACCESSIBLE" : "UNAVAILABLE";
                if (!expected.equals(OracleObjectMetadata.verify(connection(type, "synthetic_'object", count, false), type, "synthetic_'schema", "synthetic_'object"))) throw new AssertionError("精确对象检查错误");
            }
            if (!"UNAVAILABLE".equals(OracleObjectMetadata.verify(connection(type, "synthetic_'object", 1, true), type, "synthetic_'schema", "synthetic_'object"))) throw new AssertionError("权限失败被冒充可访问");
        }
        if (OracleObjectMetadata.supports("UNKNOWN") || OracleObjectMetadata.supportsRoutine("UNKNOWN")) throw new AssertionError("未知对象类型被接受");
        if (OracleObjectMetadata.supports("FUNCTION") || OracleObjectMetadata.supports("PROCEDURE")) throw new AssertionError("MySQL 例程被错误归入 Oracle 专有类型");
    }

    private static Connection connection(String type, String name, int count, boolean fail) {
        return (Connection) Proxy.newProxyInstance(OracleObjectMetadataTest.class.getClassLoader(), new Class<?>[] { Connection.class }, (proxy, method, args) -> {
            if (!"prepareStatement".equals(method.getName())) throw new AssertionError("使用了未授权连接操作");
            String sql = (String) args[0];
            if (!sql.contains("?") || sql.contains("synthetic_")) throw new AssertionError("输入被拼入 SQL");
            if ("PUBLIC_SYNONYM".equals(type) && (!sql.contains("TABLE_OWNER = ?") || !sql.contains("OWNER = 'PUBLIC'") || !sql.contains("OWNER = '__public'"))) throw new AssertionError("公共同义词越出当前 Schema");
            if ("SYNONYM".equals(type) && (!sql.contains("WHERE OWNER = ?") || sql.contains("TABLE_OWNER"))) throw new AssertionError("私有与公共同义词混淆");
            if (OracleObjectMetadata.supportsRoutine(type) && (!sql.contains("OBJECT_TYPE = '" + type + "'") || !sql.contains("WHERE OWNER = ?") || sql.contains("LIKE"))) throw new AssertionError("例程类型混淆或精确名称被 LIKE 转义");
            String[] bound = new String[2];
            return Proxy.newProxyInstance(OracleObjectMetadataTest.class.getClassLoader(), new Class<?>[] { PreparedStatement.class }, (statement, operation, values) -> {
                switch (operation.getName()) {
                    case "setString": bound[((Integer) values[0]) - 1] = (String) values[1]; return null;
                    case "setQueryTimeout": case "close": return null;
                    case "executeQuery":
                        if (!"synthetic_'schema".equals(bound[0]) || (name != null && !name.equals(bound[1]))) throw new AssertionError("名称绑定丢失");
                        if (fail) throw new SQLException("synthetic-only");
                        int[] index = { 0 };
                        return Proxy.newProxyInstance(OracleObjectMetadataTest.class.getClassLoader(), new Class<?>[] { ResultSet.class }, (rows, action, columns) -> {
                            if ("next".equals(action.getName())) return index[0]++ < count;
                            if ("getString".equals(action.getName())) return "synthetic_'object";
                            if ("close".equals(action.getName())) return null;
                            throw new AssertionError("使用了未授权结果操作");
                        });
                    default: throw new AssertionError("使用了未授权语句操作");
                }
            });
        });
    }
}
