package com.obdataorch.jdbcprobe;

import java.io.BufferedInputStream;
import java.io.DataInputStream;
import java.io.IOException;
import java.nio.ByteBuffer;
import java.nio.charset.Charset;
import java.nio.charset.CharacterCodingException;
import java.nio.charset.CodingErrorAction;
import java.nio.charset.StandardCharsets;
import java.sql.Connection;
import java.sql.DatabaseMetaData;
import java.sql.DriverManager;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.Properties;

/**
 * ConnectionProbe 是固定用途的 OceanBase JDBC 连接探针。
 * 它不接受命令行连接参数、不执行用户 SQL、不输出异常原文；连接输入只经标准输入短时传入。
 */
public final class ConnectionProbe {
    private static final int CONNECTION_PROTOCOL_VERSION = 1;
    private static final int PREFLIGHT_PROTOCOL_VERSION = 3;
    private static final int MAX_HOST_BYTES = 253;
    private static final int MAX_USERNAME_BYTES = 256;
    private static final int MAX_PASSWORD_BYTES = 4096;
    private static final int MAX_IDENTIFIER_BYTES = 256;
    private static final int MAX_METADATA_CHARS = 256;
    private static final Charset UTF8 = StandardCharsets.UTF_8;
    private static final String OBJECT_ACCESSIBLE = "ACCESSIBLE";
    private static final String OBJECT_NOT_ACCESSIBLE = "NOT_ACCESSIBLE";
    private static final String OBJECT_UNAVAILABLE = "UNAVAILABLE";

    private ConnectionProbe() {
    }

    public static void main(String[] args) {
        if (args.length != 0) {
            fail("INVALID_INPUT", 11);
            return;
        }
        ProbeInput input = null;
        try {
            input = ProbeInput.read(new DataInputStream(new BufferedInputStream(System.in)));
            Class.forName("com.oceanbase.jdbc.Driver");
            Properties properties = new Properties();
            properties.setProperty("user", input.usernameText());
            properties.setProperty("password", input.passwordText());
            try (Connection connection = DriverManager.getConnection(input.jdbcUrl(), properties)) {
                if (input.isPreflight()) {
                    printPreflightSuccess(connection, input);
                } else {
                    printConnectionSuccess(connection.getMetaData());
                }
            }
        } catch (IllegalArgumentException exception) {
            fail("INVALID_INPUT", 11);
        } catch (ClassNotFoundException exception) {
            fail("DRIVER_UNAVAILABLE", 13);
        } catch (SQLException exception) {
            fail("CONNECTION_FAILED", 12);
        } catch (IOException exception) {
            fail("INVALID_INPUT", 11);
        } finally {
            if (input != null) {
                input.destroy();
            }
        }
    }

    private static void printConnectionSuccess(DatabaseMetaData metadata) throws SQLException {
        System.out.println("{\"status\":\"SUCCESS\",\"productName\":\"" + json(metadata.getDatabaseProductName())
                + "\",\"productVersion\":\"" + json(metadata.getDatabaseProductVersion())
                + "\",\"driverName\":\"" + json(metadata.getDriverName())
                + "\",\"driverVersion\":\"" + json(metadata.getDriverVersion()) + "\"}");
    }

    private static void printPreflightSuccess(Connection connection, ProbeInput input) {
        String productName = "";
        String productVersion = "";
        String driverName = "";
        String driverVersion = "";
        String objectAccess = OBJECT_UNAVAILABLE;
        try {
            DatabaseMetaData metadata = connection.getMetaData();
            productName = json(metadata.getDatabaseProductName());
            productVersion = json(metadata.getDatabaseProductVersion());
            driverName = json(metadata.getDriverName());
            driverVersion = json(metadata.getDriverVersion());
            objectAccess = checkObjectAccess(connection, metadata, input);
        } catch (SQLException exception) {
            // v3 已建立连接后无法读取元数据时，只能报告对象事实未知，不能伪造访问失败或泄露异常原文。
        }
        System.out.println("{\"status\":\"SUCCESS\",\"productName\":\"" + productName
                + "\",\"productVersion\":\"" + productVersion
                + "\",\"driverName\":\"" + driverName
                + "\",\"driverVersion\":\"" + driverVersion
                + "\",\"objectAccess\":\"" + objectAccess + "\"}");
    }

    private static String checkObjectAccess(Connection connection, DatabaseMetaData metadata, ProbeInput input) {
        try {
            String tablePattern = metadataPattern(input.tableText(), metadata.getSearchStringEscape());
            String catalog = input.isMySQL() ? input.databaseText() : null;
            String schema = input.isOracle() ? input.databaseText() : null;
            try (ResultSet tables = metadata.getTables(catalog, schema, tablePattern, new String[] { "TABLE" })) {
                if (!tables.next()) {
                    return OBJECT_NOT_ACCESSIBLE;
                }
            }
            return canReadTable(connection, input) ? OBJECT_ACCESSIBLE : OBJECT_NOT_ACCESSIBLE;
        } catch (SQLException exception) {
            return OBJECT_UNAVAILABLE;
        } catch (RuntimeException exception) {
            return OBJECT_UNAVAILABLE;
        }
    }

    /**
     * 只对已通过元数据定位的冻结对象执行固定零行读取。
     * 标识符不能使用 PreparedStatement 参数化，因此只能由兼容模式分支逐段引用和转义；输入从不作为 SQL 片段透传。
     */
    private static boolean canReadTable(Connection connection, ProbeInput input) {
        String quote = input.isMySQL() ? "`" : "\"";
        String qualifiedTable = quoteIdentifier(input.databaseText(), quote) + "." + quoteIdentifier(input.tableText(), quote);
        try (Statement statement = connection.createStatement()) {
            statement.setQueryTimeout(5);
            try (ResultSet ignored = statement.executeQuery("SELECT 1 FROM " + qualifiedTable + " WHERE 1 = 0")) {
                return true;
            }
        } catch (SQLException exception) {
            return false;
        } catch (RuntimeException exception) {
            return false;
        }
    }

    private static String quoteIdentifier(String value, String quote) {
        return quote + value.replace(quote, quote + quote) + quote;
    }

    private static String metadataPattern(String value, String escape) throws SQLException {
        if (escape == null || escape.length() != 1 || escape.charAt(0) == '%' || escape.charAt(0) == '_') {
            if (value.indexOf('%') >= 0 || value.indexOf('_') >= 0) {
                // 无法安全转义 JDBC 模式通配符时宁可使对象结论未知，不能扩大为其他对象匹配。
                throw new SQLException();
            }
            return value;
        }
        return value.replace(escape, escape + escape).replace("%", escape + "%").replace("_", escape + "_");
    }

    private static void fail(String code, int exitCode) {
        System.out.println("{\"status\":\"FAILED\",\"code\":\"" + code + "\"}");
        System.exit(exitCode);
    }

    private static String json(String value) {
        if (value == null) {
            return "";
        }
        int length = Math.min(value.length(), MAX_METADATA_CHARS);
        StringBuilder result = new StringBuilder(length);
        for (int index = 0; index < length; index++) {
            char character = value.charAt(index);
            if (character == '"' || character == '\\') {
                result.append('\\');
            }
            if (character >= 0x20) {
                result.append(character);
            }
        }
        return result.toString();
    }

    private static final class ProbeInput {
        private final int version;
        private final byte[] host;
        private final int port;
        private final byte[] username;
        private final byte[] password;
        private final byte[] compatibilityMode;
        private final byte[] database;
        private final byte[] table;

        private ProbeInput(int version, byte[] host, int port, byte[] username, byte[] password, byte[] compatibilityMode, byte[] database, byte[] table) {
            this.version = version;
            this.host = host;
            this.port = port;
            this.username = username;
            this.password = password;
            this.compatibilityMode = compatibilityMode;
            this.database = database;
            this.table = table;
        }

        static ProbeInput read(DataInputStream input) throws IOException {
            byte[] host = null;
            byte[] username = null;
            byte[] password = null;
            byte[] compatibilityMode = null;
            byte[] database = null;
            byte[] table = null;
            try {
                int version = input.readInt();
                host = readValue(input, MAX_HOST_BYTES);
                int port = input.readInt();
                username = readValue(input, MAX_USERNAME_BYTES);
                password = readValue(input, MAX_PASSWORD_BYTES);
                if (version == PREFLIGHT_PROTOCOL_VERSION) {
                    compatibilityMode = readValue(input, 6);
                    database = readValue(input, MAX_IDENTIFIER_BYTES);
                    table = readValue(input, MAX_IDENTIFIER_BYTES);
                }
                if ((version != CONNECTION_PROTOCOL_VERSION && version != PREFLIGHT_PROTOCOL_VERSION)
                        || port < 1 || port > 65535 || !validHost(host) || username.length == 0 || password.length == 0
                        || (version == PREFLIGHT_PROTOCOL_VERSION && (!validCompatibilityMode(compatibilityMode) || !validIdentifier(database) || !validIdentifier(table)))
                        || input.read() != -1) {
                    throw new IllegalArgumentException();
                }
                return new ProbeInput(version, host, port, username, password, compatibilityMode, database, table);
            } catch (IOException exception) {
                zero(host);
                zero(username);
                zero(password);
                zero(compatibilityMode);
                zero(database);
                zero(table);
                throw exception;
            } catch (IllegalArgumentException exception) {
                zero(host);
                zero(username);
                zero(password);
                zero(compatibilityMode);
                zero(database);
                zero(table);
                throw exception;
            }
        }

        private static byte[] readValue(DataInputStream input, int maximum) throws IOException {
            int length = input.readInt();
            if (length < 0 || length > maximum) {
                throw new IllegalArgumentException();
            }
            byte[] value = new byte[length];
            input.readFully(value);
            return value;
        }

        private static boolean validHost(byte[] value) {
            if (value.length == 0) {
                return false;
            }
            for (byte character : value) {
                boolean digit = character >= '0' && character <= '9';
                boolean lower = character >= 'a' && character <= 'z';
                boolean upper = character >= 'A' && character <= 'Z';
                if (!digit && !lower && !upper && character != '.' && character != '-') {
                    return false;
                }
            }
            return true;
        }

        private static boolean validIdentifier(byte[] value) {
            if (value == null || value.length == 0) {
                return false;
            }
            final String text;
            try {
                text = UTF8.newDecoder().onMalformedInput(CodingErrorAction.REPORT)
                        .onUnmappableCharacter(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(value)).toString();
            } catch (CharacterCodingException exception) {
                return false;
            }
            if (text.length() == 0 || boundaryWhitespace(text)) {
                return false;
            }
            for (int offset = 0; offset < text.length();) {
                int character = text.codePointAt(offset);
                if (character == 0 || Character.isISOControl(character)) {
                    return false;
                }
                offset += Character.charCount(character);
            }
            return true;
        }

        private static boolean validCompatibilityMode(byte[] value) {
            return value != null && ("MYSQL".equals(new String(value, UTF8)) || "ORACLE".equals(new String(value, UTF8)));
        }

        private static boolean boundaryWhitespace(String value) {
            int first = value.codePointAt(0);
            int last = value.codePointBefore(value.length());
            return Character.isWhitespace(first) || Character.isSpaceChar(first)
                    || Character.isWhitespace(last) || Character.isSpaceChar(last);
        }

        String jdbcUrl() {
            return "jdbc:oceanbase://" + new String(host, UTF8) + ":" + port + "/?connectTimeout=5000&socketTimeout=5000";
        }

        String usernameText() {
            return new String(username, UTF8);
        }

        String passwordText() {
            return new String(password, UTF8);
        }

        boolean isPreflight() {
            return version == PREFLIGHT_PROTOCOL_VERSION;
        }

        boolean isMySQL() {
            return "MYSQL".equals(new String(compatibilityMode, UTF8));
        }

        boolean isOracle() {
            return "ORACLE".equals(new String(compatibilityMode, UTF8));
        }

        String databaseText() {
            return new String(database, UTF8);
        }

        String tableText() {
            return new String(table, UTF8);
        }

        void destroy() {
            zero(host);
            zero(username);
            zero(password);
            zero(compatibilityMode);
            zero(database);
            zero(table);
        }

        private static void zero(byte[] value) {
            if (value == null) {
                return;
            }
            for (int index = 0; index < value.length; index++) {
                value[index] = 0;
            }
        }
    }
}
