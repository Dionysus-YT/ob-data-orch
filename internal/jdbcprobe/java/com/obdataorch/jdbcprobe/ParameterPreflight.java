package com.obdataorch.jdbcprobe;

import java.nio.charset.StandardCharsets;
import java.sql.Connection;
import java.sql.DatabaseMetaData;
import java.sql.DriverManager;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.Properties;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/** 固定专家参数检查；只读取元数据、零行结构与常量，不接受外部 SQL。 */
final class ParameterPreflight {
    private ParameterPreflight() {}

    static String verify(Connection connection, boolean mysql, String database, List<String> selectedTables,
            boolean all, String username, int flags, String sysUrl, byte[] sysUsername, byte[] sysPassword) {
        try {
            DatabaseMetaData metadata = connection.getMetaData();
            List<String> tables = new ArrayList<String>(selectedTables);
            if (all && (flags & 3) != 0) {
                try (ResultSet rows = metadata.getTables(mysql ? database : null, mysql ? null : database, "%", new String[] { "TABLE" })) {
                    while (rows.next()) {
                        String namespace = rows.getString(mysql ? "TABLE_CAT" : "TABLE_SCHEM");
                        String name = rows.getString("TABLE_NAME");
                        if (!database.equals(namespace)) continue;
                        if (name == null || name.length() == 0 || name.length() > 256 || name.indexOf('\u0000') >= 0) return "UNAVAILABLE";
                        tables.add(name);
                    }
                }
            }
            if ((flags & 3) != 0 && tables.isEmpty()) return "UNAVAILABLE";
            int major = (flags & 2) != 0 ? oceanBaseMajor(metadata) : 0;
            if ((flags & 2) != 0 && major == 0) return "UNAVAILABLE";
            String quote = mysql ? "`" : "\"";
            for (String table : tables) {
                String qualified = quoted(database, quote) + "." + quoted(table, quote);
                if ((flags & 1) != 0) {
                    // 读取一个常量以实际触达弱读副本；不返回源列、数据值或读取结果。
                    execute(connection, "SELECT /*+ READ_CONSISTENCY(WEAK) */ 1 FROM " + qualified
                            + (mysql ? " LIMIT 1" : " WHERE ROWNUM <= 1"));
                }
                if ((flags & 2) != 0) {
                    boolean hasPrimaryKey;
                    try (ResultSet keys = metadata.getPrimaryKeys(mysql ? database : null, mysql ? null : database, table)) {
                        hasPrimaryKey = keys.next();
                    }
                    if (!hasPrimaryKey) {
                        if (major < 4 && !internalUser(username, mysql)) return "NOT_ACCESSIBLE";
                        execute(connection, "SELECT " + quoted("__pk_increment", quote) + " FROM " + qualified + " WHERE 1 = 0");
                    }
                }
            }
            if ((flags & 4) != 0) {
                Properties properties = new Properties();
                properties.setProperty("user", new String(sysUsername, StandardCharsets.UTF_8));
                properties.setProperty("password", new String(sysPassword, StandardCharsets.UTF_8));
                try (Connection sys = DriverManager.getConnection(sysUrl, properties)) {
                    // 权限以固定视图的实际访问确认，不能把 sys 认证成功当作视图权限通过。
                    execute(sys, "SELECT TABLEGROUP_NAME FROM oceanbase.DBA_OB_TABLEGROUPS WHERE 1 = 0");
                } finally {
                    properties.clear();
                }
            }
            return "ACCESSIBLE";
        } catch (SQLException exception) {
            String state = exception.getSQLState();
            return state != null && (state.startsWith("28") || state.equals("42501")) ? "NOT_ACCESSIBLE" : "UNAVAILABLE";
        } catch (RuntimeException exception) {
            return "UNAVAILABLE";
        }
    }

    private static void execute(Connection connection, String sql) throws SQLException {
        try (Statement statement = connection.createStatement()) {
            statement.setQueryTimeout(5);
            try (ResultSet ignored = statement.executeQuery(sql)) {
                // 不读取或输出源数据，只保留访问成功事实。
            }
        }
    }

    private static String quoted(String value, String quote) {
        return quote + value.replace(quote, quote + quote) + quote;
    }

    private static boolean internalUser(String username, boolean mysql) {
        int boundary = username.indexOf('@');
        String user = boundary < 0 ? username : username.substring(0, boundary);
        return mysql ? "__oceanbase_inner_drc_user".equals(user) : "__OCEANBASE_INNER_DRC_USER".equals(user);
    }

    private static int oceanBaseMajor(DatabaseMetaData metadata) throws SQLException {
        String version = metadata.getDatabaseProductVersion();
        String product = metadata.getDatabaseProductName();
        if (version == null || product == null) return 0;
        int marker = version.toUpperCase(Locale.ROOT).indexOf("OCEANBASE");
        if (marker >= 0) version = version.substring(marker + "OCEANBASE".length());
        else if (!product.toUpperCase(Locale.ROOT).contains("OCEANBASE")) return 0;
        Matcher match = Pattern.compile("(?:^|[^0-9])v?([1-9][0-9]*)\\.[0-9]+", Pattern.CASE_INSENSITIVE).matcher(version);
        return match.find() ? Integer.parseInt(match.group(1)) : 0;
    }
}
