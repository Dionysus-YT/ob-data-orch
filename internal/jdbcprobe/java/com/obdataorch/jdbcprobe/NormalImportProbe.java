package com.obdataorch.jdbcprobe;

import java.io.ByteArrayInputStream;
import java.io.DataInputStream;
import java.io.IOException;
import java.nio.ByteBuffer;
import java.nio.charset.CodingErrorAction;
import java.nio.charset.StandardCharsets;
import java.sql.Connection;
import java.sql.DatabaseMetaData;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Properties;
import java.util.Set;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/** 普通导入固定只读探针；只接受结构标识符和权限位，不接受路径、SQL、URL 或驱动。 */
public final class NormalImportProbe {
    private static final String PASS="ACCESSIBLE", FAIL="NOT_ACCESSIBLE", UNKNOWN="UNAVAILABLE";
    private static final int INSERT=1, UPDATE=2, DELETE=4, CREATE=8, ALTER=16, DROP=32;
    private static final Pattern GRANT=Pattern.compile("(?i)^GRANT\\s+([A-Z_ ,]+)\\s+ON\\s+(`(?:``|[^`])*`|\\*|[A-Za-z0-9_$]+)\\.(`(?:``|[^`])*`|\\*|[A-Za-z0-9_$]+)\\s+TO\\s+.+$");
    private NormalImportProbe() {}

    public static void main(String[] args) {
        Input input=null; Properties properties=new Properties();int exit=0;
        try {
            if(args.length!=0) throw new IllegalArgumentException();
            input=Input.read(); Class.forName("com.oceanbase.jdbc.Driver");
            properties.setProperty("user",new String(input.username,StandardCharsets.UTF_8));
            properties.setProperty("password",new String(input.password,StandardCharsets.UTF_8));
            String url="jdbc:oceanbase://"+input.host+":"+input.port+"/?connectTimeout=5000&socketTimeout=5000";
            try(Connection connection=DriverManager.getConnection(url,properties)) {
                String[] result=verify(connection,input);
                System.out.println("{\"status\":\"SUCCESS\",\"databaseAccess\":\""+result[0]+"\",\"writeAccess\":\""+result[1]+"\",\"inputMapping\":\""+result[2]+"\",\"writeReason\":\""+result[3]+"\",\"mappingReason\":\""+result[4]+"\"}");
            }
        } catch(ClassNotFoundException exception) { exit=fail("DRIVER_UNAVAILABLE",13);
        } catch(SQLException exception) { exit=fail("CONNECTION_FAILED",12);
        } catch(IOException|RuntimeException exception) { exit=fail("INVALID_INPUT",11);
        } finally { properties.clear();if(input!=null)input.destroy(); }
        if(exit!=0)System.exit(exit);
    }

    private static int fail(String code,int exit) { System.out.println("{\"status\":\"FAILED\",\"code\":\""+code+"\"}");return exit; }

    /** 连接、权限与映射分别投影；任一元信息不可判定时保持未知，不执行试写或文件 SQL。 */
    static String[] verify(Connection connection,Input input) {
        boolean mysql="MYSQL".equals(input.mode);
        try {
            if(mysql) connection.setCatalog(input.database);
            else {
                boolean found=false;int count=0;
                try(ResultSet schemas=connection.getMetaData().getSchemas()) {while(schemas.next()){if(++count>10000)return blocked(UNKNOWN);if(input.database.equals(schemas.getString("TABLE_SCHEM")))found=true;}}
                if(!found)return blocked(FAIL);
            }
        } catch(SQLException exception) {return blocked(UNKNOWN);}
        Check write;
        if((input.flags&16)!=0&&(input.flags&32)==0)write=unknown("DDL_UNSUPPORTED");
        else if(input.tables.isEmpty()&&input.objects.isEmpty())write=unknown("INPUT_CONTEXT_UNAVAILABLE");
        else write=mysql?mysqlPrivileges(connection,input):oraclePrivileges(connection,input);
        if(PASS.equals(write.status)&&!input.objects.isEmpty())write=ObjectImportPrivileges.verify(connection,input);
        Check mapping=(input.flags&2)==0?verified():((input.flags&1)==0||input.tables.isEmpty()?unknown("INPUT_UNSUPPORTED"):mapping(connection,input,mysql));
        return new String[]{PASS,write.status,mapping.status,write.reason,mapping.reason};
    }

    private static String[] blocked(String database){return new String[]{database,UNKNOWN,UNKNOWN,"DATABASE_UNAVAILABLE","DATABASE_UNAVAILABLE"};}
    static final class Check {final String status,reason;Check(String status,String reason){this.status=status;this.reason=reason;}}
    private static Check verified(){return new Check(PASS,"VERIFIED");}
    private static Check unknown(String reason){return new Check(UNKNOWN,reason);}
    private static Check rejected(String reason){return new Check(FAIL,reason);}
    private static String missingPrivilege(int missing){
        int[] bits={INSERT,UPDATE,DELETE,CREATE,ALTER,DROP};String[] names={"INSERT_REQUIRED","UPDATE_REQUIRED","DELETE_REQUIRED","CREATE_REQUIRED","ALTER_REQUIRED","DROP_REQUIRED"};
        for(int i=0;i<bits.length;i++)if((missing&bits[i])!=0)return names[i];
        throw new IllegalArgumentException();
    }

    private static Check mysqlPrivileges(Connection connection,Input input) {
        List<Grant> grants=new ArrayList<Grant>();boolean incomplete=false;
        try(Statement statement=connection.createStatement()) {
            statement.setQueryTimeout(5);int count=0;
            // SHOW GRANTS 由程序固定，结果仅在当前进程内解释，不返回账号、授权文本或异常原文。
            try(ResultSet rows=statement.executeQuery("SHOW GRANTS")) {
                while(rows.next()) {
                    if(++count>4096)return unknown("GRANTS_UNSUPPORTED");String text=rows.getString(1);
                    if(text==null||text.length()>16384)return unknown("GRANTS_UNSUPPORTED");
                    Matcher match=GRANT.matcher(text);if(!match.matches()){
                        // 未展开的正向 GRANT 只能补充权限；其他未知语句不能按可忽略的授权处理。
                        if(!text.trim().matches("(?is)^GRANT\\s+.*"))return unknown("GRANTS_UNSUPPORTED");
                        incomplete=true;continue;
                    }
                    int mask=privileges(match.group(1));
                    boolean global="*".equals(match.group(2)),allTables="*".equals(match.group(3));
                    grants.add(new Grant(unquote(match.group(2)),unquote(match.group(3)),mask,global,allTables));
                }
            }
        } catch(SQLException|RuntimeException exception) {return unknown("GRANTS_QUERY_FAILED");}
        if(grants.isEmpty()&&!incomplete)return unknown("GRANTS_QUERY_FAILED");
        for(Table table:input.tables) {
            int available=0;boolean uncertain=incomplete;
            for(Grant grant:grants){
                if(!grant.global&&!input.database.equals(grant.database)&&(grant.database.indexOf('%')>=0||grant.database.indexOf('_')>=0||grant.database.indexOf('\\')>=0))uncertain=true;
                if((grant.global||input.database.equals(grant.database))&&(grant.allTables||table.name.equals(grant.table)))available|=grant.mask;
            }
            int missing=table.privileges&~available;
            if(missing!=0) {
                if(uncertain)return unknown("GRANTS_UNSUPPORTED");
                return rejected(missingPrivilege(missing));
            }
        }
        return verified();
    }

    private static int privileges(String names) {
        int result=0;for(String name:names.toUpperCase(Locale.ROOT).split(",")) {
            name=name.trim();if("ALL PRIVILEGES".equals(name)||"ALL".equals(name))return 63;
            if("INSERT".equals(name))result|=INSERT;if("UPDATE".equals(name))result|=UPDATE;if("DELETE".equals(name))result|=DELETE;
            if("CREATE".equals(name))result|=CREATE;if("ALTER".equals(name))result|=ALTER;if("DROP".equals(name))result|=DROP;
        }return result;
    }
    private static String unquote(String name) {return name.startsWith("`")?name.substring(1,name.length()-1).replace("``","`"):name;}

    private static Check oraclePrivileges(Connection connection,Input input) {
        Set<String> privileges=new HashSet<String>();String user;
        try {
            try(Statement identity=connection.createStatement()) {
                identity.setQueryTimeout(5);
                try(ResultSet current=identity.executeQuery("SELECT SYS_CONTEXT('USERENV', 'CURRENT_USER') FROM DUAL")) {if(!current.next())return unknown("SCHEMA_PRIVILEGES_UNAVAILABLE");user=current.getString(1);if(user==null||current.next())return unknown("SCHEMA_PRIVILEGES_UNAVAILABLE");}
            }
        } catch(SQLException|RuntimeException exception) {return unknown("SCHEMA_PRIVILEGES_UNAVAILABLE");}
        boolean owner=input.database.equals(user),needsSystem=!owner;
        for(Table table:input.tables)needsSystem|=(table.privileges&CREATE)!=0;
        if(!needsSystem)return verified();
        try {
            try(Statement statement=connection.createStatement()) {
                statement.setQueryTimeout(5);int count=0;
                // 所属 Schema 的既有表操作来自对象所有权；仅 CREATE 或跨 Schema 操作需要系统权限事实。
                // OceanBase Oracle 模式使用当前用户直接授权视图，不依赖未实现的 SESSION_PRIVS，也不展开角色。
                try(ResultSet rows=statement.executeQuery("SELECT PRIVILEGE FROM USER_SYS_PRIVS")) {while(rows.next()){if(++count>4096)return unknown("SCHEMA_PRIVILEGES_UNAVAILABLE");String privilege=rows.getString(1);if(privilege==null)return unknown("SCHEMA_PRIVILEGES_UNAVAILABLE");privileges.add(privilege);}}
            }
        } catch(SQLException|RuntimeException exception) {return unknown("SCHEMA_PRIVILEGES_UNAVAILABLE");}
        String[] names={"INSERT ANY TABLE","UPDATE ANY TABLE","DELETE ANY TABLE","CREATE ANY TABLE","ALTER ANY TABLE","DROP ANY TABLE"};
        int[] masks={INSERT,UPDATE,DELETE,CREATE,ALTER,DROP};
        for(Table table:input.tables)for(int i=0;i<masks.length;i++)if((table.privileges&masks[i])!=0) {
            if(owner&&(masks[i]!=CREATE||privileges.contains("CREATE TABLE")))continue;
            // 不推断跨 Schema 的对象授权；没有 ANY 权限时仍可能存在对象级授权，保持未知。
            if(!privileges.contains(names[i]))return unknown("SCHEMA_PRIVILEGES_UNAVAILABLE");
        }
        return verified();
    }

    private static Check mapping(Connection connection,Input input,boolean mysql) {
        try {
            // MySQL 使用参数绑定的固定查询，无需依赖驱动可能未实现的元数据通配符接口。
            DatabaseMetaData metadata=mysql?null:connection.getMetaData();String escape=mysql?null:metadata.getSearchStringEscape();
            for(Table table:input.tables) {
                if((table.flags&1)==0)continue;
                if((table.flags&2)!=0&&table.targets.isEmpty())return unknown("INPUT_DEFINITION_UNSUPPORTED");
                String tablePattern=mysql?null:pattern(table.name,escape),schemaPattern=mysql?null:pattern(input.database,escape);
                if(!mysql&&(tablePattern==null||schemaPattern==null))return unknown("TARGET_METADATA_UNAVAILABLE");
                List<String> columns=new ArrayList<String>();Set<String> required=new HashSet<String>(),seenColumns=new HashSet<String>();
                if((input.flags&8)!=0&&(table.flags&2)!=0) {
                    if(table.targets.isEmpty())return unknown("INPUT_DEFINITION_UNSUPPORTED");
                    columns.addAll(table.targets);required.addAll(table.required);
                } else if(mysql) {
                    try(PreparedStatement query=connection.prepareStatement("SELECT COLUMN_NAME, IS_NULLABLE, COLUMN_DEFAULT, EXTRA, TABLE_SCHEMA, TABLE_NAME, DATA_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION")) {
                        query.setQueryTimeout(5);query.setString(1,input.database);query.setString(2,table.name);
                        try(ResultSet rows=query.executeQuery()) {int observedColumns=0;while(rows.next()) {
                            if(++observedColumns>4096||!input.database.equals(rows.getString(5))||!table.name.equals(rows.getString(6)))return unknown("TARGET_METADATA_UNAVAILABLE");
                            String name=rows.getString(1),nullable=rows.getString(2),value=rows.getString(3),extra=rows.getString(4);
                            if(name==null||extra==null||!seenColumns.add(name)||!("YES".equals(nullable)||"NO".equals(nullable)))return unknown("TARGET_METADATA_UNAVAILABLE");
                            String attributes=extra.toUpperCase(Locale.ROOT);
                            if(attributes.contains("VIRTUAL GENERATED")||attributes.contains("STORED GENERATED"))continue;
                            if("NO".equals(nullable)&&value==null&&!attributes.contains("AUTO_INCREMENT"))required.add(name);
                            if(!input.excludedTypes.isEmpty()){String type=rows.getString(7);if(type==null)return unknown("TARGET_METADATA_UNAVAILABLE");if(input.excludedTypes.contains(type.toUpperCase(Locale.ROOT)))continue;}
                            columns.add(name);
                        }}
                    }
                } else try(ResultSet rows=metadata.getColumns(null,schemaPattern,tablePattern,"%")) {
                    int observedColumns=0;while(rows.next()) {
                        if(++observedColumns>4096||!input.database.equals(rows.getString("TABLE_SCHEM"))||!table.name.equals(rows.getString("TABLE_NAME")))return unknown("TARGET_METADATA_UNAVAILABLE");
                        String name=rows.getString("COLUMN_NAME");if(name==null||!seenColumns.add(name))return unknown("TARGET_METADATA_UNAVAILABLE");
                        String generated="";try{generated=rows.getString("IS_GENERATEDCOLUMN");}catch(SQLException unsupported){return unknown("TARGET_METADATA_UNAVAILABLE");}
                        if("YES".equalsIgnoreCase(generated))continue;
                        String auto=rows.getString("IS_AUTOINCREMENT");String value=rows.getString("COLUMN_DEF");int nullable=rows.getInt("NULLABLE");
                        if(rows.wasNull())return unknown("TARGET_METADATA_UNAVAILABLE");
                        if(nullable!=DatabaseMetaData.columnNoNulls&&nullable!=DatabaseMetaData.columnNullable)return unknown("TARGET_METADATA_UNAVAILABLE");
                        if(nullable==DatabaseMetaData.columnNoNulls&&value==null&&!"YES".equalsIgnoreCase(auto))required.add(name);
                        if(!input.excludedTypes.isEmpty()){String type=rows.getString("TYPE_NAME");if(type==null)return unknown("TARGET_METADATA_UNAVAILABLE");type=type.toUpperCase(Locale.ROOT);int precision=type.indexOf('(');if(precision>=0)type=type.substring(0,precision);if(input.excludedTypes.contains(type))continue;}
                        columns.add(name);
                    }
                }
                if(columns.isEmpty()) {columns.addAll(table.targets);required.addAll(table.required);}
                if(columns.isEmpty())return unknown("TARGET_STRUCTURE_MISSING");
                List<String> target=new ArrayList<String>(columns);
                if(!input.includes.isEmpty()) {if(!columns.containsAll(input.includes))return rejected("COLUMN_NAME_MISMATCH");target=new ArrayList<String>(input.includes);}
                else if(!input.excludes.isEmpty()) {if(!columns.containsAll(input.excludes))return rejected("COLUMN_NAME_MISMATCH");target.removeAll(input.excludes);}
                else if(!table.names.isEmpty()) {if(!columns.containsAll(table.names))return (input.flags&4)!=0?unknown("AUTO_COLUMNS_UNSUPPORTED"):rejected("COLUMN_NAME_MISMATCH");target=new ArrayList<String>(table.names);}
                if(table.columns!=target.size())return rejected("COLUMN_COUNT_MISMATCH");
                if(!target.containsAll(required))return rejected("REQUIRED_COLUMN_MISSING");
            }
            return verified();
        } catch(SQLException|RuntimeException exception) {return unknown("TARGET_METADATA_UNAVAILABLE");}
    }
    private static String pattern(String value,String escape) {if(escape==null||escape.isEmpty())return value.indexOf('%')>=0||value.indexOf('_')>=0?null:value;return value.replace(escape,escape+escape).replace("%",escape+"%").replace("_",escape+"_");}

    static final class Table {String name;int columns,privileges,flags;List<String> names=new ArrayList<String>(),targets=new ArrayList<String>(),required=new ArrayList<String>();}
    static final class Definition {String kind,name,reference;}
    static final class Grant {final String database,table;final int mask;final boolean global,allTables;Grant(String database,String table,int mask,boolean global,boolean allTables){this.database=database;this.table=table;this.mask=mask;this.global=global;this.allTables=allTables;}}
    static final class Input {
        String host,mode,database;int port,flags;byte[] username,password;
        List<String> includes=new ArrayList<String>(),excludes=new ArrayList<String>();List<Table> tables=new ArrayList<Table>();
        List<String> excludedTypes=new ArrayList<String>();List<Definition> objects=new ArrayList<Definition>();
        static Input read() throws IOException {
            byte[] raw=new byte[1024*1024+1];Input result=new Input();int length=0;
            try {
                while(length<raw.length){int count=System.in.read(raw,length,raw.length-length);if(count<0)break;length+=count;}
                if(length>=raw.length)throw new IllegalArgumentException();
                DataInputStream stream=new DataInputStream(new ByteArrayInputStream(raw,0,length));
                if(stream.readInt()!=11)throw new IllegalArgumentException();result.host=text(stream,253);result.port=stream.readInt();
                result.username=value(stream,256);result.password=value(stream,4096);result.mode=text(stream,16);result.database=identifier(stream);
                for(byte value:result.username)if(value==10||value==13)throw new IllegalArgumentException();for(byte value:result.password)if(value==10||value==13)throw new IllegalArgumentException();
                result.flags=stream.readInt();result.includes=names(stream);result.excludes=names(stream);
                int count=stream.readInt();if(count<0||count>1024)throw new IllegalArgumentException();Set<String> seen=new HashSet<String>();
                for(int i=0;i<count;i++){Table table=new Table();table.name=identifier(stream);table.columns=stream.readInt();table.privileges=stream.readInt();table.flags=stream.readInt();table.names=names(stream);table.targets=names(stream);table.required=names(stream);if(table.columns<0||table.columns>4096||table.privileges<0||table.privileges>63||table.flags<1||table.flags>3||table.privileges==0&&((result.flags&16)==0||(result.flags&32)!=0)||!seen.add(table.name)||!table.targets.containsAll(table.required)||(table.flags&2)==0&&!table.targets.isEmpty()||(result.flags&32)!=0&&(table.flags&2)!=0&&table.targets.isEmpty())throw new IllegalArgumentException();result.tables.add(table);}
                result.excludedTypes=names(stream);for(String type:result.excludedTypes)if(!Arrays.asList("BLOB","TINYBLOB","MEDIUMBLOB","LONGBLOB","CLOB","TEXT","TINYTEXT","MEDIUMTEXT","LONGTEXT","RAW","BINARY","VARBINARY","CHAR","VARCHAR","VARCHAR2","NUMBER","DECIMAL","INT","INTEGER","DATE","TIMESTAMP").contains(type))throw new IllegalArgumentException();
                int objectCount=stream.readInt();if(objectCount<0||objectCount+count>1024)throw new IllegalArgumentException();Set<String> objectKeys=new HashSet<String>();
                for(int i=0;i<objectCount;i++){Definition object=new Definition();object.kind=text(stream,32);object.name=identifier(stream);object.reference=text(stream,256);if(!ObjectImportPrivileges.validKind(object.kind)||!object.name.matches("[A-Za-z_][A-Za-z_0-9$]{0,127}")||!object.reference.isEmpty()&&!object.reference.matches("[A-Za-z_][A-Za-z_0-9$]{0,127}")||!objectKeys.add(object.kind+":"+object.name))throw new IllegalArgumentException();result.objects.add(object);}
                boolean hasData=false,hasDefinition=!result.objects.isEmpty();for(Table table:result.tables){hasData|=(table.flags&1)!=0;hasDefinition|=(table.flags&2)!=0;}
                if((result.flags&2)==0&&hasData||(result.flags&16)==0&&hasDefinition||((result.flags&1)!=0&&(result.flags&2)!=0&&!hasData)||((result.flags&32)!=0&&(result.flags&16)!=0&&!hasDefinition))throw new IllegalArgumentException();
                if(!result.host.matches("[A-Za-z0-9][A-Za-z0-9.-]{0,252}")||result.port<1||result.port>65535||result.username.length==0||result.password.length==0||!("MYSQL".equals(result.mode)||"ORACLE".equals(result.mode))||result.flags<0||result.flags>63||!result.includes.isEmpty()&&!result.excludes.isEmpty()||stream.read()!=-1)throw new IllegalArgumentException();
                return result;
            } catch(IOException|RuntimeException exception){result.destroy();throw exception;} finally {Arrays.fill(raw,(byte)0);}
        }
        private static byte[] value(DataInputStream stream,int maximum)throws IOException {int size=stream.readInt();if(size<0||size>maximum)throw new IllegalArgumentException();byte[] value=new byte[size];try{stream.readFully(value);return value;}catch(IOException exception){Arrays.fill(value,(byte)0);throw exception;}}
        private static String text(DataInputStream stream,int maximum)throws IOException {byte[] data=value(stream,maximum);try{return StandardCharsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT).onUnmappableCharacter(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(data)).toString();}finally{Arrays.fill(data,(byte)0);}}
        private static String identifier(DataInputStream stream)throws IOException {String name=text(stream,256);if(name.isEmpty()||!name.equals(name.trim())||name.indexOf('*')>=0||name.indexOf(',')>=0)throw new IllegalArgumentException();for(int i=0;i<name.length();i++)if(Character.isISOControl(name.charAt(i)))throw new IllegalArgumentException();return name;}
        private static List<String> names(DataInputStream stream)throws IOException {int count=stream.readInt();if(count<0||count>4096)throw new IllegalArgumentException();List<String> result=new ArrayList<String>();Set<String> seen=new HashSet<String>();for(int i=0;i<count;i++){String name=identifier(stream);if(!seen.add(name))throw new IllegalArgumentException();result.add(name);}return result;}
        void destroy(){if(username!=null)Arrays.fill(username,(byte)0);if(password!=null)Arrays.fill(password,(byte)0);}
    }
}
