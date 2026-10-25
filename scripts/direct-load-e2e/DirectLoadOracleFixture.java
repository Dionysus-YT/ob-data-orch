import java.io.DataInputStream;
import java.nio.charset.StandardCharsets;
import java.sql.*;
import java.util.*;

/** 旁路测试专用固定 Oracle 夹具；不进入产品，不创建用户、授权或接管存量表。 */
public final class DirectLoadOracleFixture {
    static String text(DataInputStream input) throws Exception {
        int size=input.readInt();if(size<0||size>4096)throw new IllegalArgumentException();
        byte[] bytes=new byte[size];input.readFully(bytes);try{return new String(bytes,StandardCharsets.UTF_8);}finally{Arrays.fill(bytes,(byte)0);}
    }
    static String scalar(Connection connection,String sql) throws Exception {
        try(Statement statement=connection.createStatement()){statement.setQueryTimeout(5);try(ResultSet rows=statement.executeQuery(sql)){if(!rows.next())throw new SQLException();String value=rows.getString(1);if(value==null||rows.next())throw new SQLException();return value;}}
    }
    static int tableCount(Connection connection,String name) throws Exception {
        try(PreparedStatement statement=connection.prepareStatement("SELECT COUNT(*) FROM USER_TABLES WHERE TABLE_NAME=?")){statement.setQueryTimeout(5);statement.setString(1,name);try(ResultSet rows=statement.executeQuery()){if(!rows.next())throw new SQLException();return rows.getInt(1);}}
    }
    static void execute(Connection connection,String sql) throws Exception {
        try(Statement statement=connection.createStatement()){statement.setQueryTimeout(10);statement.executeUpdate(sql);}
    }
    static String json(String value) {
        StringBuilder output=new StringBuilder("\"");for(char c:value.toCharArray()){if(c=='"'||c=='\\')output.append('\\').append(c);else if(c<32)output.append(String.format(Locale.ROOT,"\\u%04x",(int)c));else output.append(c);}return output.append('"').toString();
    }
    public static void main(String[] args) {
        Properties properties=new Properties();byte[] password=null;int exit=0;
        try {
            if(args.length!=0)throw new IllegalArgumentException();DataInputStream input=new DataInputStream(System.in);
            if(!"DLO1".equals(text(input)))throw new IllegalArgumentException();
            String action=text(input),run=text(input),schema=text(input),host=text(input),port=text(input),username=text(input);
            int size=input.readInt();if(size<1||size>4096)throw new IllegalArgumentException();password=new byte[size];input.readFully(password);if(input.read()!=-1)throw new IllegalArgumentException();
            if(!run.matches("[a-f0-9]{32}")||!schema.matches("[A-Z_][A-Z0-9_$#]{0,127}")||!host.matches("[A-Za-z0-9][A-Za-z0-9.-]{0,252}")||!port.matches("[0-9]{1,5}")||!Arrays.asList("direct-oracle-probe","direct-oracle-create","direct-oracle-inspect","direct-oracle-cleanup").contains(action))throw new IllegalArgumentException();
            Class.forName("com.oceanbase.jdbc.Driver");properties.setProperty("user",username);properties.setProperty("password",new String(password,StandardCharsets.UTF_8));
            try(Connection connection=DriverManager.getConnection("jdbc:oceanbase://"+host+":"+port+"/?connectTimeout=5000&socketTimeout=10000",properties)) {
                if(!schema.equals(scalar(connection,"SELECT USER FROM DUAL")))throw new IllegalArgumentException();
                if(action.equals("direct-oracle-probe")) {
                    String version="";try{String value=scalar(connection,"SELECT OB_VERSION() FROM DUAL");if(value.matches("[0-9]+\\.[0-9]+\\.[0-9]+(?:\\.[0-9]+)?"))version=value;}catch(SQLException ignored){}
                    System.out.println("{\"status\":\"PROBED\",\"mode\":\"ORACLE\",\"currentUserMatched\":true,\"observerVersion\":"+json(version)+",\"arbitrarySqlEnabled\":false}");
                } else operate(connection,action,run);
            }
        } catch(Exception exception){System.out.println("{\"status\":\"BLOCKED\",\"code\":\"ORACLE_DIRECT_FIXTURE_UNAVAILABLE\"}");exit=1;}
        finally {properties.clear();if(password!=null)Arrays.fill(password,(byte)0);}
        if(exit!=0)System.exit(exit);
    }
    private static void operate(Connection connection,String action,String run) throws Exception {
        String prefix="DLVO_"+run.substring(0,12).toUpperCase(Locale.ROOT)+"_",owner=prefix+"OWNER",table=prefix+"BASIC";
        if(action.equals("direct-oracle-create")) {
            if(tableCount(connection,owner)!=0||tableCount(connection,table)!=0)throw new IllegalArgumentException();
            execute(connection,"CREATE TABLE "+owner+" (RUN_ID VARCHAR2(32) PRIMARY KEY)");
            try(PreparedStatement statement=connection.prepareStatement("INSERT INTO "+owner+" VALUES (?)")){statement.setString(1,run);statement.executeUpdate();}connection.commit();
            execute(connection,"CREATE TABLE "+table+" (ID NUMBER(10) PRIMARY KEY, LABEL VARCHAR2(64), AMOUNT NUMBER(12,4))");
            System.out.println("{\"status\":\"CREATED\",\"ownerVerified\":true,\"table\":"+json(table)+"}");return;
        }
        if(!run.equals(scalar(connection,"SELECT RUN_ID FROM "+owner)))throw new IllegalArgumentException();
        if(action.equals("direct-oracle-cleanup")) {
            if(tableCount(connection,table)==1)execute(connection,"DROP TABLE "+table+" PURGE");
            execute(connection,"DROP TABLE "+owner+" PURGE");
            if(tableCount(connection,owner)!=0||tableCount(connection,table)!=0)throw new SQLException();
            System.out.println("{\"status\":\"CLEANED\",\"ownerVerified\":true,\"tablesAbsent\":true}");return;
        }
        StringBuilder data=new StringBuilder("[");int count=0;
        try(Statement statement=connection.createStatement()){statement.setQueryTimeout(5);try(ResultSet rows=statement.executeQuery("SELECT ID,LABEL,AMOUNT FROM "+table+" ORDER BY ID")){while(rows.next()){if(++count>100)throw new SQLException();if(count>1)data.append(',');data.append('[');for(int i=1;i<=3;i++){if(i>1)data.append(',');String value=rows.getString(i);data.append(value==null?"null":json(value));}data.append(']');}}}data.append(']');
        StringBuilder columns=new StringBuilder("[");int ordinal=0;
        try(PreparedStatement statement=connection.prepareStatement("SELECT COLUMN_NAME,DATA_TYPE,DATA_PRECISION,DATA_SCALE,NULLABLE FROM USER_TAB_COLUMNS WHERE TABLE_NAME=? ORDER BY COLUMN_ID")){statement.setQueryTimeout(5);statement.setString(1,table);try(ResultSet rows=statement.executeQuery()){while(rows.next()){if(++ordinal>16)throw new SQLException();if(ordinal>1)columns.append(',');columns.append('[');for(int i=1;i<=5;i++){if(i>1)columns.append(',');String value=rows.getString(i);columns.append(value==null?"null":json(value));}columns.append(']');}}}columns.append(']');
        int primaryKeys=Integer.parseInt(scalar(connection,"SELECT COUNT(*) FROM USER_CONSTRAINTS WHERE TABLE_NAME='"+table+"' AND CONSTRAINT_TYPE='P'"));
        System.out.println("{\"status\":\"INSPECTED\",\"ownerVerified\":true,\"rows\":"+data+",\"columns\":"+columns+",\"primaryKeys\":"+primaryKeys+"}");
    }
}
