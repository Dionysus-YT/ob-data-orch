package com.obdataorch.jdbcprobe;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;

/** Oracle 扩展对象与例程只走固定元数据查询；Schema 和精确名称全部绑定，不接收外部 SQL。 */
final class OracleObjectMetadata {
    private OracleObjectMetadata() { }

    @FunctionalInterface
    interface NameConsumer {
        void accept(String name) throws SQLException;
    }

    static boolean supports(String type) {
        return "PACKAGE".equals(type) || "TRIGGER".equals(type) || "SYNONYM".equals(type)
                || "PUBLIC_SYNONYM".equals(type) || "TYPE".equals(type);
    }

    /** 例程避免驱动的 LIKE 转义差异及 getProcedures 混入函数；MySQL 保留原有 JDBC 路径。 */
    static boolean supportsRoutine(String type) {
        return "FUNCTION".equals(type) || "PROCEDURE".equals(type);
    }

    static void readNames(Connection connection, String type, String database, NameConsumer consume) throws SQLException {
        try (PreparedStatement statement = prepare(connection, type, database, null);
                ResultSet result = statement.executeQuery()) {
            while (result.next()) consume.accept(result.getString(1));
        }
    }

    /** 只核对元数据可见性，不读取或执行包、触发器、同义词目标或类型内容。 */
    static String verify(Connection connection, String type, String database, String name) {
        try (PreparedStatement statement = prepare(connection, type, database, name);
                ResultSet result = statement.executeQuery()) {
            if (!result.next()) return "NOT_ACCESSIBLE";
            return name.equals(result.getString(1)) && !result.next() ? "ACCESSIBLE" : "UNAVAILABLE";
        } catch (SQLException | RuntimeException exception) {
            return "UNAVAILABLE";
        }
    }

    private static PreparedStatement prepare(Connection connection, String type, String database, String name) throws SQLException {
        String sql, nameColumn;
        switch (type) {
            case "PACKAGE":
                sql = "SELECT OBJECT_NAME FROM ALL_OBJECTS WHERE OWNER = ? AND OBJECT_TYPE = 'PACKAGE'";
                nameColumn = "OBJECT_NAME";
                break;
            case "TYPE":
                sql = "SELECT OBJECT_NAME FROM ALL_OBJECTS WHERE OWNER = ? AND OBJECT_TYPE = 'TYPE'";
                nameColumn = "OBJECT_NAME";
                break;
            case "FUNCTION":
                sql = "SELECT OBJECT_NAME FROM ALL_OBJECTS WHERE OWNER = ? AND OBJECT_TYPE = 'FUNCTION'";
                nameColumn = "OBJECT_NAME";
                break;
            case "PROCEDURE":
                sql = "SELECT OBJECT_NAME FROM ALL_OBJECTS WHERE OWNER = ? AND OBJECT_TYPE = 'PROCEDURE'";
                nameColumn = "OBJECT_NAME";
                break;
            case "TRIGGER":
                sql = "SELECT TRIGGER_NAME FROM ALL_TRIGGERS WHERE OWNER = ?";
                nameColumn = "TRIGGER_NAME";
                break;
            case "SYNONYM":
                sql = "SELECT SYNONYM_NAME FROM ALL_SYNONYMS WHERE OWNER = ?";
                nameColumn = "SYNONYM_NAME";
                break;
            case "PUBLIC_SYNONYM":
                // 与 4.3.5 --public-synonym 一致，只列出指向当前 Schema 的公共同义词。
                sql = "SELECT SYNONYM_NAME FROM ALL_SYNONYMS WHERE (OWNER = 'PUBLIC' OR OWNER = '__public') AND TABLE_OWNER = ?";
                nameColumn = "SYNONYM_NAME";
                break;
            default:
                throw new IllegalArgumentException();
        }
        if (name != null) sql += " AND " + nameColumn + " = ?";
        PreparedStatement statement = connection.prepareStatement(sql);
        try {
            statement.setQueryTimeout(5);
            statement.setString(1, database);
            if (name != null) statement.setString(2, name);
            return statement;
        } catch (SQLException | RuntimeException exception) {
            statement.close();
            throw exception;
        }
    }
}
