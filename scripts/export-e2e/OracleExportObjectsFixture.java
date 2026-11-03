import java.io.DataInputStream;
import java.nio.charset.StandardCharsets;
import java.sql.*;
import java.util.*;

/** 测试专用固定夹具，只操作本轮随机前缀；不创建用户、授权或提供任意 SQL 入口。 */
public final class OracleExportObjectsFixture {
    private static final String[] KINDS = {"TABLE", "VIEW", "FUNCTION", "PROCEDURE", "PACKAGE", "TRIGGER", "SYNONYM", "PUBLIC_SYNONYM", "TYPE", "SEQUENCE"};
    private static final String[] SUFFIXES = {"DATA", "VIEW", "FN", "PROC", "PKG", "TRG", "SYN", "PUB", "TYP", "SEQ"};
    private static String stage = "INPUT";

    public static void main(String[] args) {
        byte[] password = null;
        Properties properties = new Properties();
        int exit = 0;
        try {
            if (args.length != 0) throw new IllegalArgumentException();
            DataInputStream input = new DataInputStream(System.in);
            if (!"OEO1".equals(text(input))) throw new IllegalArgumentException();
            String action = text(input), run = text(input), schema = text(input), host = text(input), port = text(input), user = text(input);
            int length = input.readInt();
            if (length < 1 || length > 4096) throw new IllegalArgumentException();
            password = new byte[length]; input.readFully(password);
            if (input.read() != -1 || !run.matches("[a-f0-9]{32}") || !schema.matches("[A-Z_][A-Z0-9_$#]{0,127}") || !host.matches("[A-Za-z0-9][A-Za-z0-9.-]{0,252}") || !port.matches("[0-9]{1,5}") || !Arrays.asList("oracle-export-create", "oracle-export-inspect", "oracle-export-cleanup").contains(action)) throw new IllegalArgumentException();
            properties.setProperty("user", user); properties.setProperty("password", new String(password, StandardCharsets.UTF_8));
            Class.forName("com.oceanbase.jdbc.Driver"); stage = "CONNECT";
            try (Connection connection = DriverManager.getConnection("jdbc:oceanbase://" + host + ":" + port + "/?connectTimeout=5000&socketTimeout=10000", properties)) {
                if (!schema.equals(scalar(connection, "SELECT USER FROM DUAL"))) throw new IllegalArgumentException();
                operate(connection, action, run, schema);
            }
        } catch (Exception error) {
            int vendor = error instanceof SQLException ? ((SQLException) error).getErrorCode() : 0;
            System.out.println("{\"status\":\"BLOCKED\",\"code\":\"ORACLE_EXPORT_FIXTURE_UNAVAILABLE\",\"stage\":" + json(stage) + ",\"vendorCode\":" + vendor + "}");
            exit = 1;
        } finally {
            properties.clear(); if (password != null) Arrays.fill(password, (byte) 0);
        }
        if (exit != 0) System.exit(exit);
    }

    private static void operate(Connection connection, String action, String run, String schema) throws Exception {
        String prefix = "ODOO_" + run.substring(0,12).toUpperCase(Locale.ROOT) + "_", owner = prefix + "OWNER", table = prefix + "DATA";
        String[] names = new String[KINDS.length];
        for (int index=0; index<names.length; index++) names[index] = prefix + SUFFIXES[index];
        if (action.equals("oracle-export-create")) {
            stage = "COLLISION";
            if (localNameExists(connection, owner)) throw new IllegalArgumentException();
            for (int index=0; index<names.length; index++) if (localNameExists(connection,names[index]) || exists(connection,"PUBLIC_SYNONYM",names[index],schema)) throw new IllegalArgumentException();
            // 创建不使用 OR REPLACE，碰撞即失败；所有权行先持久化，以便部分失败也能精确清理。
            execute(connection, "CREATE TABLE " + owner + " (RUN_ID VARCHAR2(32) PRIMARY KEY)");
            try (PreparedStatement statement=connection.prepareStatement("INSERT INTO " + owner + " VALUES (?)")) { statement.setString(1,run); statement.executeUpdate(); } connection.commit();
            stage = "TABLE"; execute(connection,"CREATE TABLE " + table + " (ID NUMBER(10) PRIMARY KEY, LABEL VARCHAR2(32))");
            execute(connection,"INSERT INTO " + table + " VALUES (1,'alpha')"); execute(connection,"INSERT INTO " + table + " VALUES (2,'beta')"); connection.commit();
            LinkedHashMap<String,String> definitions = new LinkedHashMap<String,String>();
            definitions.put("VIEW", "CREATE VIEW " + prefix + "VIEW AS SELECT ID,LABEL FROM " + table);
            definitions.put("FUNCTION", "CREATE FUNCTION " + prefix + "FN RETURN NUMBER IS BEGIN RETURN 7; END;");
            definitions.put("PROCEDURE", "CREATE PROCEDURE " + prefix + "PROC IS BEGIN NULL; END;");
            definitions.put("PACKAGE", "CREATE PACKAGE " + prefix + "PKG AS FUNCTION FIXTURE_VALUE RETURN NUMBER; END;");
            definitions.put("TRIGGER", "CREATE TRIGGER " + prefix + "TRG BEFORE INSERT ON " + table + " FOR EACH ROW BEGIN :NEW.ID := :NEW.ID; END;");
            definitions.put("SYNONYM", "CREATE SYNONYM " + prefix + "SYN FOR " + table);
            definitions.put("PUBLIC_SYNONYM", "CREATE PUBLIC SYNONYM " + prefix + "PUB FOR " + schema + "." + table);
            definitions.put("TYPE", "CREATE TYPE " + prefix + "TYP AS OBJECT (ID NUMBER, LABEL VARCHAR2(32))");
            definitions.put("SEQUENCE", "CREATE SEQUENCE " + prefix + "SEQ START WITH 11 INCREMENT BY 2 NOCACHE");
            for (Map.Entry<String,String> definition : definitions.entrySet()) { stage=definition.getKey(); execute(connection,definition.getValue()); }
        }
        stage = "OWNER";
        if (!run.equals(scalar(connection,"SELECT RUN_ID FROM " + owner))) throw new IllegalArgumentException();
        if (action.equals("oracle-export-cleanup")) {
            // 先删依赖，再删类型与表；只删除所有权匹配的本轮精确名称，保留原有 Schema。
            for (String kind : new String[]{"PUBLIC_SYNONYM","SYNONYM","TRIGGER","PACKAGE","TYPE","FUNCTION","PROCEDURE","VIEW","SEQUENCE","TABLE"}) {
                int index=Arrays.asList(KINDS).indexOf(kind); stage="DROP_"+kind;
                if (exists(connection,kind,names[index],schema)) {
                    if (kind.equals("PUBLIC_SYNONYM") && !ownedPublicSynonym(connection,names[index],schema,table)) throw new IllegalArgumentException();
                    execute(connection,"DROP " + (kind.equals("PUBLIC_SYNONYM") ? "PUBLIC SYNONYM" : kind) + " " + names[index] + (kind.equals("TABLE") ? " PURGE" : ""));
                }
            }
            for (int index=0; index<names.length; index++) if (exists(connection,KINDS[index],names[index],schema)) throw new SQLException();
            execute(connection,"DROP TABLE " + owner + " PURGE");
            System.out.println("{\"status\":\"CLEANED\",\"ownerVerified\":true,\"allObjectsAbsent\":true}"); return;
        }
        stage = "INSPECT";
        StringBuilder objects=new StringBuilder("[");
        for (int index=0; index<names.length; index++) {
            if (!exists(connection,KINDS[index],names[index],schema)) throw new SQLException();
            if (KINDS[index].equals("PUBLIC_SYNONYM") && !ownedPublicSynonym(connection,names[index],schema,table)) throw new SQLException();
            if (index>0) objects.append(','); objects.append("{\"objectType\":").append(json(KINDS[index])).append(",\"name\":").append(json(names[index])).append('}');
        }
        objects.append(']');
        StringBuilder rows=new StringBuilder("["); int count=0;
        try (Statement statement=connection.createStatement(); ResultSet result=statement.executeQuery("SELECT ID,LABEL FROM " + table + " ORDER BY ID")) {
            while (result.next()) { if (++count>2) throw new SQLException(); if(count>1) rows.append(','); rows.append('[').append(result.getInt(1)).append(',').append(json(result.getString(2))).append(']'); }
        } rows.append(']'); if(count!=2) throw new SQLException();
        System.out.println("{\"status\":" + json(action.equals("oracle-export-create") ? "CREATED" : "INSPECTED") + ",\"ownerVerified\":true,\"objects\":" + objects + ",\"rows\":" + rows + "}");
    }

    private static boolean exists(Connection connection, String kind, String name, String schema) throws SQLException {
        boolean publicSynonym=kind.equals("PUBLIC_SYNONYM");
        String sql=publicSynonym ? "SELECT COUNT(*) FROM ALL_SYNONYMS WHERE (OWNER='PUBLIC' OR OWNER='__public') AND SYNONYM_NAME=?" : "SELECT COUNT(*) FROM USER_OBJECTS WHERE OBJECT_NAME=? AND OBJECT_TYPE=?";
        try (PreparedStatement statement=connection.prepareStatement(sql)) {
            statement.setQueryTimeout(5); statement.setString(1,name); if(!publicSynonym) statement.setString(2,kind);
            try (ResultSet result=statement.executeQuery()) { if(!result.next()) throw new SQLException(); return result.getInt(1)>0; }
        }
    }

    private static boolean localNameExists(Connection connection,String name) throws SQLException {
        try (PreparedStatement statement=connection.prepareStatement("SELECT COUNT(*) FROM USER_OBJECTS WHERE OBJECT_NAME=?")) {
            statement.setQueryTimeout(5); statement.setString(1,name);
            try (ResultSet result=statement.executeQuery()) { if(!result.next()) throw new SQLException(); return result.getInt(1)>0; }
        }
    }

    private static boolean ownedPublicSynonym(Connection connection,String name,String schema,String table) throws SQLException {
        try (PreparedStatement statement=connection.prepareStatement("SELECT TABLE_OWNER,TABLE_NAME FROM ALL_SYNONYMS WHERE (OWNER='PUBLIC' OR OWNER='__public') AND SYNONYM_NAME=?")) {
            statement.setQueryTimeout(5); statement.setString(1,name);
            try (ResultSet result=statement.executeQuery()) { return result.next() && schema.equals(result.getString(1)) && table.equals(result.getString(2)) && !result.next(); }
        }
    }

    private static String scalar(Connection connection, String sql) throws SQLException {
        try (Statement statement=connection.createStatement()) { statement.setQueryTimeout(5); try (ResultSet result=statement.executeQuery(sql)) { if(!result.next()) throw new SQLException(); String value=result.getString(1); if(result.next()) throw new SQLException(); return value; } }
    }
    private static void execute(Connection connection,String sql) throws SQLException { try(Statement statement=connection.createStatement()){statement.setQueryTimeout(10);statement.executeUpdate(sql);} }
    private static String text(DataInputStream input) throws Exception { int length=input.readInt();if(length<0||length>4096)throw new IllegalArgumentException();byte[] value=new byte[length];input.readFully(value);try{return new String(value,StandardCharsets.UTF_8);}finally{Arrays.fill(value,(byte)0);} }
    private static String json(String value) { StringBuilder out=new StringBuilder("\"");for(char character:value.toCharArray()){if(character=='"'||character=='\\')out.append('\\');if(character<32)out.append(String.format(Locale.ROOT,"\\u%04x",(int)character));else out.append(character);}return out.append('"').toString(); }
}
