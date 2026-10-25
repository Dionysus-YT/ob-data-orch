package com.obdataorch.jdbcprobe;

import java.io.ByteArrayInputStream;
import java.io.DataInputStream;
import java.io.IOException;
import java.nio.ByteBuffer;
import java.nio.CharBuffer;
import java.nio.charset.CodingErrorAction;
import java.nio.charset.StandardCharsets;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.Arrays;
import java.util.Locale;
import java.util.Properties;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/** 旁路独立固定只读入口；不接收任意 SQL，不试写、不提交、不运行官方工具。 */
public final class DirectLoadProbe {
    private static final String PASS="ACCESSIBLE", FAIL="NOT_ACCESSIBLE", UNKNOWN="UNAVAILABLE";
    private static final Pattern VERSION=Pattern.compile("[0-9]+\\.[0-9]+\\.[0-9]+(?:\\.[0-9]+)?");
    private static final Pattern ODP_VERSION=Pattern.compile("\\AObProxy-OceanBase +([0-9]+\\.[0-9]+\\.[0-9]+(?:\\.[0-9]+)?)(?:[-\\s]|\\z)");
    private static final Pattern GRANT=Pattern.compile("(?i)^GRANT\\s+([A-Z_ ,]+)\\s+ON\\s+(`(?:``|[^`])*`|\\*|[A-Za-z0-9_$]+)\\.(`(?:``|[^`])*`|\\*|[A-Za-z0-9_$]+)\\s+TO\\s+.+$");
    static final String TABLE_SQL="SELECT TABLE_TYPE FROM information_schema.TABLES WHERE TABLE_SCHEMA=? AND TABLE_NAME=?";
    static final String COLUMN_SQL="SELECT DATA_TYPE, EXTRA FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=? AND TABLE_NAME=? ORDER BY ORDINAL_POSITION";
    static final String INDEX_SQL="SELECT INDEX_NAME, NON_UNIQUE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=? AND TABLE_NAME=?";
    static final String ODP_SQL="SHOW PROXYINFO BINARY";
    private DirectLoadProbe() {}

    public static void main(String[] args) {
        Input input=null; Properties properties=new Properties(); int exit=0;
        try {
            if(args.length!=0)throw new IllegalArgumentException();
            input=Input.read();Class.forName("com.oceanbase.jdbc.Driver");
            properties.setProperty("user",new String(input.username,StandardCharsets.UTF_8));
            properties.setProperty("password",new String(input.password,StandardCharsets.UTF_8));
            String url="jdbc:oceanbase://"+input.host+":"+input.port+"/?connectTimeout=5000&socketTimeout=5000";
            try(Connection connection=DriverManager.getConnection(url,properties)){System.out.println(verify(connection,input).json());}
        }catch(ClassNotFoundException exception){exit=fail("DRIVER_UNAVAILABLE",13);
        }catch(SQLException exception){exit=fail("CONNECTION_FAILED",12);
        }catch(IOException|RuntimeException exception){exit=fail("INVALID_INPUT",11);
        }finally{properties.clear();if(input!=null)input.destroy();}
        if(exit!=0)System.exit(exit);
    }
    private static int fail(String code,int exit){System.out.println("{\"status\":\"FAILED\",\"code\":\""+code+"\"}");return exit;}

    /** 可读连接、权限、结构分别保留事实；任何异常仅收缩该事实，不返回驱动异常原文。 */
    static Observation verify(Connection connection,Input input) {
        Observation observed=new Observation();boolean mysql="MYSQL".equals(input.mode);
        try {
            if(mysql)connection.setCatalog(input.database);
            else {
                boolean found=false;int count=0;
                try(ResultSet rows=connection.getMetaData().getSchemas()) {while(rows.next()){if(++count>10000)return observed;if(input.database.equals(rows.getString("TABLE_SCHEM")))found=true;}}
                if(!found)return observed;
            }
            observed.database=PASS;
        }catch(SQLException|RuntimeException exception){return observed;}
        observed.observer=observerVersion(connection);
        // 官方 ODP 固定只读命令只在当前业务连接允许时提供版本；不请求 proxysys 凭据或猜测版本。
        if("PRIVATE_ODP".equals(input.scenario))observed.odp=odpVersion(connection);
        if(!mysql){DirectLoadOracleMetadata.verify(connection,input,observed);return observed;}
        observed.write=mysqlWriteAccess(connection,input);
        mysqlTable(connection,input,observed);
        return observed;
    }

    private static String observerVersion(Connection connection) {
        // OB_VERSION 返回 OB 版本；不解析可改写且含 MySQL 兼容前缀的 VERSION()。
        try(Statement statement=connection.createStatement()) {
            statement.setQueryTimeout(5);
            try(ResultSet rows=statement.executeQuery("SELECT OB_VERSION() FROM DUAL")) {
                if(!rows.next())return "";String value=rows.getString(1);
                if(value==null||value.length()>64||!VERSION.matcher(value).matches()||rows.next())return "";
                return value;
            }
        }catch(SQLException|RuntimeException exception){return "";}
    }

    private static String odpVersion(Connection connection) {
        try(Statement statement=connection.createStatement()) {
            statement.setQueryTimeout(5);
            try(ResultSet rows=statement.executeQuery(ODP_SQL)) {
                if(!rows.next()||!"binary info".equals(rows.getString(1)))return "";
                String info=rows.getString(2);
                if(info==null||info.length()>4096||rows.next())return "";
                Matcher match=ODP_VERSION.matcher(info);
                return match.find()?match.group(1):"";
            }
        }catch(SQLException|RuntimeException exception){return "";}
    }

    private static String mysqlWriteAccess(Connection connection,Input input) {
        boolean insert=false,drop=!input.truncate,unknown=false,any=false;
        try(Statement statement=connection.createStatement()) {
            statement.setQueryTimeout(5);int count=0;
            try(ResultSet rows=statement.executeQuery("SHOW GRANTS")) {
                while(rows.next()) {
                    if(++count>4096)return UNKNOWN;String text=rows.getString(1);
                    if(text==null||text.length()>16384)return UNKNOWN;
                    Matcher grant=GRANT.matcher(text);
                    if(!grant.matches()){if(!text.trim().matches("(?is)^GRANT\\s+.*"))return UNKNOWN;unknown=true;continue;}
                    any=true;String db=unquote(grant.group(2)),table=unquote(grant.group(3));
                    if(!"*".equals(db)&&!input.database.equals(db)&&(db.indexOf('%')>=0||db.indexOf('_')>=0||db.indexOf('\\')>=0))unknown=true;
                    if(("*".equals(grant.group(2))||input.database.equals(db))&&("*".equals(grant.group(3))||input.table.equals(table))) {
                        for(String name:grant.group(1).toUpperCase(Locale.ROOT).split(",")) {
                            name=name.trim();boolean all="ALL".equals(name)||"ALL PRIVILEGES".equals(name);
                            insert|=all||"INSERT".equals(name);drop|=all||"DROP".equals(name);
                        }
                    }
                }
            }
            if(!any&&!unknown)return UNKNOWN;
            if(!insert||!drop)return unknown?UNKNOWN:FAIL;
            // 4.3.5 普通业务账号还需读取这些固定系统视图；零行读取不返回系统数据。
            String[] views={"GV$OB_MEMSTORE","DBA_OB_MAJOR_COMPACTION","DBA_OB_LS_LOCATIONS","__all_virtual_proxy_schema"};
            for(String view:views)try(ResultSet rows=statement.executeQuery("SELECT 1 FROM oceanbase."+view+" WHERE 1=0")){if(rows.next())return UNKNOWN;}
            return PASS;
        }catch(SQLException|RuntimeException exception){return UNKNOWN;}
    }
    private static String unquote(String value){return value.startsWith("`")?value.substring(1,value.length()-1).replace("``","`"):value;}

    private static void mysqlTable(Connection connection,Input input,Observation o) {
        try(PreparedStatement statement=scoped(connection,TABLE_SQL,input);ResultSet rows=statement.executeQuery()) {
            // 信息表缺行可能是不可见，不能把它当成已证明不存在或空表。
            if(!rows.next()||!"BASE TABLE".equals(rows.getString(1))||rows.next())return;
            o.exists=Boolean.TRUE;
        }catch(SQLException|RuntimeException exception){return;}
        try(Statement statement=connection.createStatement()) {
            statement.setQueryTimeout(5);
            try(ResultSet rows=statement.executeQuery("SELECT 1 FROM `"+input.database+"`.`"+input.table+"` LIMIT 1")){o.empty=Boolean.valueOf(!rows.next());}
        }catch(SQLException|RuntimeException exception){o.empty=null;}
        try(PreparedStatement statement=scoped(connection,COLUMN_SQL,input);ResultSet rows=statement.executeQuery()) {
            boolean bit=false,binary=false,virtual=false,lob=false;int count=0;
            while(rows.next()) {
                if(++count>4096)return;String type=rows.getString(1),extra=rows.getString(2);
                if(type==null||extra==null||type.length()>64||extra.length()>256)return;
                type=type.toLowerCase(Locale.ROOT);String flags=extra.toLowerCase(Locale.ROOT);
                if(!type.matches("bit|binary|varbinary|tinyblob|blob|mediumblob|longblob|tinytext|text|mediumtext|longtext|char|varchar|tinyint|smallint|mediumint|int|integer|bigint|decimal|numeric|float|double|real|date|datetime|timestamp|time|year|enum|set|json"))return;
                if(!flags.isEmpty()&&!flags.equals("auto_increment")&&!flags.equals("stored generated")&&!flags.equals("virtual generated")&&!flags.equals("default_generated"))return;
                bit|="bit".equals(type);binary|="binary".equals(type)||"varbinary".equals(type);virtual|=flags.equals("virtual generated");lob|=type.endsWith("blob")||type.endsWith("text");
            }
            if(count>0){o.bit=Boolean.valueOf(bit);o.binary=Boolean.valueOf(binary);o.virtual=Boolean.valueOf(virtual);o.lob=Boolean.valueOf(lob);}
        }catch(SQLException|RuntimeException exception){o.bit=null;o.binary=null;o.virtual=null;o.lob=null;}
        try(PreparedStatement statement=scoped(connection,INDEX_SQL,input);ResultSet rows=statement.executeQuery()) {
            boolean index=false,unique=false;int count=0;
            while(rows.next()) {
                if(++count>4096)return;String name=rows.getString(1),nonUnique=rows.getString(2);
                if(name==null||name.isEmpty()||!("0".equals(nonUnique)||"1".equals(nonUnique)))return;
                index=true;unique|="0".equals(nonUnique);
            }
            o.index=Boolean.valueOf(index);o.unique=Boolean.valueOf(unique);
        }catch(SQLException|RuntimeException exception){o.index=null;o.unique=null;}
    }
    private static PreparedStatement scoped(Connection connection,String sql,Input input)throws SQLException {
        PreparedStatement statement=connection.prepareStatement(sql);
        try{statement.setQueryTimeout(5);statement.setString(1,input.database);statement.setString(2,input.table);return statement;}
        catch(SQLException|RuntimeException exception){statement.close();throw exception;}
    }

    /** 输出只包含固定枚举、严格数字版本和可空布尔，绝不拼接授权原文或数据行。 */
    static final class Observation {
        String database=UNKNOWN,write=UNKNOWN,observer="",odp="";
        Boolean exists,empty,bit,binary,virtual,index,lob,unique;
        String json(){return "{\"status\":\"SUCCESS\",\"databaseAccess\":\""+database+"\",\"writeAccess\":\""+write+"\",\"observerVersion\":\""+observer+"\",\"odpVersion\":\""+odp+"\",\"tableFacts\":{\"exists\":"+exists+",\"empty\":"+empty+",\"hasBIT\":"+bit+",\"hasBinary\":"+binary+",\"hasVirtualGenerated\":"+virtual+",\"hasIndex\":"+index+",\"hasLOB\":"+lob+",\"hasUniqueIndex\":"+unique+"}}";}
    }

    /** v11 有界二进制帧只允许单表标识符，秘密与原始输入在所有出口尽力清零。 */
    static final class Input {
        String host,mode,database,table,scenario;int port;boolean truncate;byte[] username,password;
        static Input read()throws IOException {
            byte[] raw=new byte[32769];Input result=new Input();int length=0;
            try {
                while(length<raw.length){int count=System.in.read(raw,length,raw.length-length);if(count<0)break;length+=count;}
                if(length>=raw.length)throw new IllegalArgumentException();
                DataInputStream stream=new DataInputStream(new ByteArrayInputStream(raw,0,length));
                if(stream.readInt()!=11)throw new IllegalArgumentException();result.host=text(stream,253);result.port=stream.readInt();
                result.username=value(stream,256);result.password=value(stream,4096);result.mode=text(stream,16);result.database=identifier(stream);result.table=identifier(stream);result.scenario=text(stream,32);
                int flags=stream.readInt();result.truncate=flags==1;
                if(!result.host.matches("[A-Za-z0-9][A-Za-z0-9.-]{0,252}")||result.port<1||result.port>65535||result.username.length==0||result.password.length==0||!("MYSQL".equals(result.mode)||"ORACLE".equals(result.mode))||!("CLOUD_ODP".equals(result.scenario)||"PRIVATE_ODP".equals(result.scenario)||"OBSERVER_DIRECT".equals(result.scenario))||flags<0||flags>1||stream.read()!=-1)throw new IllegalArgumentException();
                secret(result.username);secret(result.password);
                return result;
            }catch(IOException|RuntimeException exception){result.destroy();throw exception;}finally{Arrays.fill(raw,(byte)0);}
        }
        private static byte[] value(DataInputStream stream,int maximum)throws IOException {int size=stream.readInt();if(size<0||size>maximum)throw new IllegalArgumentException();byte[] value=new byte[size];try{stream.readFully(value);return value;}catch(IOException exception){Arrays.fill(value,(byte)0);throw exception;}}
        private static String text(DataInputStream stream,int maximum)throws IOException {byte[] value=value(stream,maximum);try{return StandardCharsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT).onUnmappableCharacter(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(value)).toString();}finally{Arrays.fill(value,(byte)0);}}
        private static String identifier(DataInputStream stream)throws IOException {String name=text(stream,256);if(name.isEmpty()||name.charAt(0)=='-')throw new IllegalArgumentException();for(int i=0;i<name.length();){int c=name.codePointAt(i);if(!Character.isLetterOrDigit(c)&&c!='_'&&c!='$'&&c!='-')throw new IllegalArgumentException();i+=Character.charCount(c);}return name;}
        private static void secret(byte[] value)throws IOException {CharBuffer decoded=StandardCharsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT).onUnmappableCharacter(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(value));try{while(decoded.hasRemaining())if(Character.isISOControl(decoded.get()))throw new IllegalArgumentException();}finally{if(decoded.hasArray())Arrays.fill(decoded.array(),(char)0);}}
        void destroy(){if(username!=null)Arrays.fill(username,(byte)0);if(password!=null)Arrays.fill(password,(byte)0);}
    }
}
