package com.obdataorch.jdbcprobe;

import java.sql.Connection;
import java.sql.DatabaseMetaData;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

/** 固定列与分区检查；外部输入只作为元数据值，不接收或执行外部 SQL。 */
final class SelectionPreflight {
    private SelectionPreflight() {}

    static String verify(Connection connection, boolean mysql, String database, String table,
            List<String> includes, List<String> excludes, List<String> partitions) {
        if (includes.isEmpty() && excludes.isEmpty() && partitions.isEmpty()) return "ACCESSIBLE";
        try {
            if (!includes.isEmpty() || !excludes.isEmpty()) {
                DatabaseMetaData metadata = connection.getMetaData();
                String escape = metadata.getSearchStringEscape();
                String schemaPattern = mysql ? null : pattern(database, escape);
                String tablePattern = pattern(table, escape);
                if (tablePattern == null || (!mysql && schemaPattern == null)) return "UNAVAILABLE";
                Set<String> names = new HashSet<String>();
                try (ResultSet columns = metadata.getColumns(mysql ? database : null, schemaPattern, tablePattern, "%")) {
                    while (columns.next()) {
                        if (!database.equals(columns.getString(mysql ? "TABLE_CAT" : "TABLE_SCHEM")) || !table.equals(columns.getString("TABLE_NAME"))) return "UNAVAILABLE";
                        String name = columns.getString("COLUMN_NAME");
                        if (name == null) return "UNAVAILABLE";
                        names.add(name);
                    }
                }
                // 官方列筛选要求大小写一致，不进行模糊匹配或静默改名。
                if (!names.containsAll(includes) || !names.containsAll(excludes)) return "NOT_ACCESSIBLE";
            }
            if (!partitions.isEmpty()) {
                Set<String> names = partitionNames(connection, mysql, database, table);
                for (String requested : partitions) {
                    boolean found = false;
                    for (String name : names) if (name.equals(requested)) { found = true; break; }
                    if (!found) return "NOT_ACCESSIBLE";
                }
            }
            return "ACCESSIBLE";
        } catch (SQLException | RuntimeException exception) {
            // 元数据视图不支持、超时和驱动异常保持未知，不伪造成不存在或权限不足。
            return "UNAVAILABLE";
        }
    }

    private static Set<String> partitionNames(Connection connection, boolean mysql, String database, String table) throws SQLException {
        Set<String> parents = new HashSet<String>(), children = new HashSet<String>();
        if (mysql) {
            readPartitions(connection, "SELECT PARTITION_NAME, SUBPARTITION_NAME FROM information_schema.PARTITIONS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?", database, table, parents, children, true);
        } else {
            readPartitions(connection, "SELECT SUBPARTITION_NAME FROM ALL_TAB_SUBPARTITIONS WHERE TABLE_OWNER = ? AND TABLE_NAME = ?", database, table, children, children, false);
            if (children.isEmpty()) readPartitions(connection, "SELECT PARTITION_NAME FROM ALL_TAB_PARTITIONS WHERE TABLE_OWNER = ? AND TABLE_NAME = ?", database, table, parents, parents, false);
        }
        // 复合分区只能导出子分区，父分区名称不能冒充可导出的目标。
        return children.isEmpty() ? parents : children;
    }

    private static void readPartitions(Connection connection, String sql, String database, String table,
            Set<String> parents, Set<String> children, boolean paired) throws SQLException {
        try (PreparedStatement statement = connection.prepareStatement(sql)) {
            statement.setQueryTimeout(5); statement.setString(1, database); statement.setString(2, table);
            try (ResultSet rows = statement.executeQuery()) {
                while (rows.next()) {
                    String first = rows.getString(1);
                    if (first != null) parents.add(first);
                    if (paired) { String second = rows.getString(2); if (second != null) children.add(second); }
                }
            }
        }
    }

    private static String pattern(String value, String escape) {
        if (escape == null || escape.isEmpty()) return value.indexOf('%') >= 0 || value.indexOf('_') >= 0 ? null : value;
        return value.replace(escape, escape + escape).replace("%", escape + "%").replace("_", escape + "_");
    }
}
