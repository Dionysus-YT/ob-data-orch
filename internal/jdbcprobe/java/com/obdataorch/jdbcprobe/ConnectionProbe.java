package com.obdataorch.jdbcprobe;

import java.io.BufferedInputStream;
import java.io.DataInputStream;
import java.io.IOException;
import java.nio.charset.Charset;
import java.nio.charset.StandardCharsets;
import java.sql.Connection;
import java.sql.DatabaseMetaData;
import java.sql.DriverManager;
import java.sql.SQLException;
import java.util.Properties;

/**
 * ConnectionProbe 是固定用途的 OceanBase JDBC 连接探针。
 * 它不接受命令行连接参数、不执行用户 SQL、不输出异常原文；连接输入只经标准输入短时传入。
 */
public final class ConnectionProbe {
    private static final int PROTOCOL_VERSION = 1;
    private static final int MAX_HOST_BYTES = 253;
    private static final int MAX_USERNAME_BYTES = 256;
    private static final int MAX_PASSWORD_BYTES = 4096;
    private static final int MAX_METADATA_CHARS = 256;
    private static final Charset UTF8 = StandardCharsets.UTF_8;

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
                DatabaseMetaData metadata = connection.getMetaData();
                System.out.println("{\"status\":\"SUCCESS\",\"productName\":\"" + json(metadata.getDatabaseProductName())
                        + "\",\"productVersion\":\"" + json(metadata.getDatabaseProductVersion())
                        + "\",\"driverName\":\"" + json(metadata.getDriverName())
                        + "\",\"driverVersion\":\"" + json(metadata.getDriverVersion()) + "\"}");
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
        private final byte[] host;
        private final int port;
        private final byte[] username;
        private final byte[] password;

        private ProbeInput(byte[] host, int port, byte[] username, byte[] password) {
            this.host = host;
            this.port = port;
            this.username = username;
            this.password = password;
        }

        static ProbeInput read(DataInputStream input) throws IOException {
            int version = input.readInt();
            byte[] host = readValue(input, MAX_HOST_BYTES);
            int port = input.readInt();
            byte[] username = readValue(input, MAX_USERNAME_BYTES);
            byte[] password = readValue(input, MAX_PASSWORD_BYTES);
            if (version != PROTOCOL_VERSION || port < 1 || port > 65535 || !validHost(host) || username.length == 0 || password.length == 0 || input.read() != -1) {
                zero(host);
                zero(username);
                zero(password);
                throw new IllegalArgumentException();
            }
            return new ProbeInput(host, port, username, password);
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

        String jdbcUrl() {
            return "jdbc:oceanbase://" + new String(host, UTF8) + ":" + port + "/?connectTimeout=5000&socketTimeout=5000";
        }

        String usernameText() {
            return new String(username, UTF8);
        }

        String passwordText() {
            return new String(password, UTF8);
        }

        void destroy() {
            zero(host);
            zero(username);
            zero(password);
        }

        private static void zero(byte[] value) {
            for (int index = 0; index < value.length; index++) {
                value[index] = 0;
            }
        }
    }
}
