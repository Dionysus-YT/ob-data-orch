import java.io.DataInputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.*;
import java.nio.file.attribute.BasicFileAttributes;
import java.sql.*;
import java.util.*;

/** Oracle 真实执行的小型所有权夹具；只处理当前用户中由随机 run 派生的固定表和输入目录。 */
public final class DirectLoadOracleExecutionFixture {
    private static final String[] SUFFIX={"BASIC","SHARDS","SEPARATOR","INCREMENTAL","TRUNCATE","THREAD","PARALLEL","RAW","REPLACE","LOB"};
    public static void main(String[] args) {
        Properties properties=new Properties();byte[] password=null;int exit=0;
        try {
            if(args.length!=0)throw new IllegalArgumentException();DataInputStream input=new DataInputStream(System.in);
            if(!"DLO1".equals(DirectLoadOracleFixture.text(input)))throw new IllegalArgumentException();
            String action=DirectLoadOracleFixture.text(input),run=DirectLoadOracleFixture.text(input),schema=DirectLoadOracleFixture.text(input),host=DirectLoadOracleFixture.text(input),port=DirectLoadOracleFixture.text(input),username=DirectLoadOracleFixture.text(input);
            int size=input.readInt();if(size<1||size>4096)throw new IllegalArgumentException();password=new byte[size];input.readFully(password);if(input.read()!=-1)throw new IllegalArgumentException();
            if(!run.matches("[a-f0-9]{32}")||!schema.matches("[A-Z_][A-Z0-9_$#]{0,127}")||!host.matches("[A-Za-z0-9][A-Za-z0-9.-]{0,252}")||!port.matches("[0-9]{1,5}")||Integer.parseInt(port)<1||Integer.parseInt(port)>65535||!Arrays.asList("direct-oracle-execution-create","direct-oracle-execution-inspect","direct-oracle-execution-cleanup").contains(action))throw new IllegalArgumentException();
            Class.forName("com.oceanbase.jdbc.Driver");properties.setProperty("user",username);properties.setProperty("password",new String(password,StandardCharsets.UTF_8));
            try(Connection connection=DriverManager.getConnection("jdbc:oceanbase://"+host+":"+port+"/?connectTimeout=5000&socketTimeout=10000",properties)) {
                if(!schema.equals(DirectLoadOracleFixture.scalar(connection,"SELECT USER FROM DUAL")))throw new IllegalArgumentException();
                operate(connection,action,run,schema);
            }
        }catch(Exception exception){System.out.println("{\"status\":\"BLOCKED\",\"code\":\"ORACLE_EXECUTION_FIXTURE_UNAVAILABLE\"}");exit=1;}
        finally{properties.clear();if(password!=null)Arrays.fill(password,(byte)0);}
        if(exit!=0)System.exit(exit);
    }
    private static void operate(Connection connection,String action,String run,String schema)throws Exception {
        String prefix="DLVO_"+run.substring(0,12).toUpperCase(Locale.ROOT)+"_",owner=prefix+"OWNER";
        Path parent=Paths.get("ob-data").toAbsolutePath().normalize(),root=parent.resolve("dlvt-oracle-"+run.substring(0,12));
        if(!parent.toRealPath().equals(parent)||!root.getParent().equals(parent))throw new IllegalArgumentException();
        if(action.endsWith("-create")) {
            if(Files.exists(root)||DirectLoadOracleFixture.tableCount(connection,owner)!=0)throw new IllegalArgumentException();
            for(String suffix:SUFFIX)if(DirectLoadOracleFixture.tableCount(connection,prefix+suffix)!=0)throw new IllegalArgumentException();
            DirectLoadOracleFixture.execute(connection,"CREATE TABLE "+owner+" (RUN_ID VARCHAR2(32) PRIMARY KEY)");
            try(PreparedStatement statement=connection.prepareStatement("INSERT INTO "+owner+" VALUES (?)")){statement.setString(1,run);statement.executeUpdate();}connection.commit();
            for(String suffix:SUFFIX){String definition=suffix.equals("RAW")?"(ID NUMBER(10) PRIMARY KEY, VALUE1 RAW(16))":suffix.equals("LOB")?"(ID NUMBER(10) PRIMARY KEY, PAYLOAD CLOB)":"(ID NUMBER(10) PRIMARY KEY, LABEL VARCHAR2(64), AMOUNT NUMBER(12,4))";DirectLoadOracleFixture.execute(connection,"CREATE TABLE "+prefix+suffix+" "+definition);}
            for(String suffix:new String[]{"INCREMENTAL","TRUNCATE","REPLACE"})DirectLoadOracleFixture.execute(connection,"INSERT INTO "+prefix+suffix+" VALUES (9,'original',9.0000)");
            DirectLoadOracleFixture.execute(connection,"INSERT INTO "+prefix+"LOB VALUES (9,'original')");
            DirectLoadOracleFixture.execute(connection,"CREATE INDEX "+prefix+"REPLACE_I ON "+prefix+"REPLACE (LABEL)");connection.commit();
            Files.createDirectory(root);Files.write(root.resolve(".owner.json"),("{\"runId\":\""+run+"\"}").getBytes(StandardCharsets.UTF_8),StandardOpenOption.CREATE_NEW);
            for(String suffix:SUFFIX){Path directory=Files.createDirectory(root.resolve(suffix.toLowerCase(Locale.ROOT)));String table=prefix+suffix;
                if(suffix.equals("SHARDS")){write(directory,table+".0.csv","1,alpha,12.3400\n");write(directory,table+".1.csv","2,中文,-0.0100\n");}
                else write(directory,table+".csv",suffix.equals("RAW")?"1,AB\n":suffix.equals("LOB")?"1,alpha\n":suffix.equals("SEPARATOR")?"1|alpha|12.3400\n2|中文|-0.0100\n":"1,alpha,12.3400\n2,中文,-0.0100\n");
            }
            System.out.println("{\"status\":\"CREATED\",\"ownerVerified\":true,\"tables\":10}");return;
        }
        if(!run.equals(DirectLoadOracleFixture.scalar(connection,"SELECT RUN_ID FROM "+owner)))throw new IllegalArgumentException();
        if(action.endsWith("-cleanup")) {
            // 删除前同时核对精确绝对目录与独立文件标记；遍历不跟随链接，不按名称或年龄接管其他目录。
            if(Files.exists(root)&&(Files.isSymbolicLink(root)||Files.size(root.resolve(".owner.json"))>128||!new String(Files.readAllBytes(root.resolve(".owner.json")),StandardCharsets.UTF_8).equals("{\"runId\":\""+run+"\"}")))throw new IllegalArgumentException();
            for(String suffix:SUFFIX)if(DirectLoadOracleFixture.tableCount(connection,prefix+suffix)==1)DirectLoadOracleFixture.execute(connection,"DROP TABLE "+prefix+suffix+" PURGE");
            DirectLoadOracleFixture.execute(connection,"DROP TABLE "+owner+" PURGE");
            for(String suffix:SUFFIX)if(DirectLoadOracleFixture.tableCount(connection,prefix+suffix)!=0)throw new SQLException();
            if(DirectLoadOracleFixture.tableCount(connection,owner)!=0)throw new SQLException();
            if(Files.exists(root))Files.walkFileTree(root,EnumSet.noneOf(FileVisitOption.class),32,new SimpleFileVisitor<Path>(){
                private void scope(Path target)throws IOException{if(!target.toAbsolutePath().normalize().startsWith(root))throw new IOException();}
                public FileVisitResult visitFile(Path file,BasicFileAttributes attributes)throws IOException{scope(file);Files.delete(file);return FileVisitResult.CONTINUE;}
                public FileVisitResult postVisitDirectory(Path directory,IOException error)throws IOException{scope(directory);if(error!=null)throw error;Files.delete(directory);return FileVisitResult.CONTINUE;}
            });
            System.out.println("{\"status\":\"CLEANED\",\"ownerVerified\":true,\"tablesAbsent\":true,\"inputAbsent\":"+!Files.exists(root)+"}");return;
        }
        StringBuilder output=new StringBuilder("{\"status\":\"INSPECTED\",\"ownerVerified\":true,\"tables\":{");
        for(int index=0;index<SUFFIX.length;index++){if(index>0)output.append(',');String table=prefix+SUFFIX[index];output.append(DirectLoadOracleFixture.json(SUFFIX[index])).append(':').append(describe(connection,table,schema));}
        output.append("},\"uncontrolledRuntimeFiles\":[");int count=0;
        if(Files.exists(root))try(java.util.stream.Stream<Path> files=Files.walk(root)){Iterator<Path> iterator=files.iterator();while(iterator.hasNext()){Path file=iterator.next();if(Files.isRegularFile(file)&&file.toString().matches("(?i).*\\.(log|info|warn|error)$")){if(++count>1024)throw new IOException();if(count>1)output.append(',');output.append(DirectLoadOracleFixture.json(root.relativize(file).toString()));}}}
        System.out.println(output.append("]}").toString());
    }
    private static void write(Path directory,String name,String content)throws IOException{Files.write(directory.resolve(name),content.getBytes(StandardCharsets.UTF_8),StandardOpenOption.CREATE_NEW);}
    private static String query(Connection connection,String sql,String table)throws Exception {
        try(PreparedStatement statement=connection.prepareStatement(sql)){statement.setQueryTimeout(5);statement.setString(1,table);try(ResultSet rows=statement.executeQuery()){return rows(rows);}}
    }
    private static String rows(ResultSet rows)throws Exception {
        StringBuilder result=new StringBuilder("[");int count=0,columns=rows.getMetaData().getColumnCount();if(columns<1||columns>16)throw new SQLException();
        while(rows.next()){if(++count>100)throw new SQLException();if(count>1)result.append(',');result.append('[');for(int i=1;i<=columns;i++){if(i>1)result.append(',');String value=rows.getString(i);if(value!=null&&value.length()>4096)throw new SQLException();result.append(value==null?"null":DirectLoadOracleFixture.json(value));}result.append(']');}return result.append(']').toString();
    }
    private static String describe(Connection connection,String table,String schema)throws Exception {
        String data;try(Statement statement=connection.createStatement()){statement.setQueryTimeout(5);try(ResultSet result=statement.executeQuery("SELECT * FROM "+table+" ORDER BY ID")){data=rows(result);}}
        String columns=query(connection,"SELECT COLUMN_NAME,DATA_TYPE,DATA_LENGTH,DATA_PRECISION,DATA_SCALE,NULLABLE,CHAR_LENGTH,VIRTUAL_COLUMN,HIDDEN_COLUMN FROM USER_TAB_COLS WHERE TABLE_NAME=? ORDER BY COLUMN_ID",table);
        String indexes=query(connection,"SELECT I.UNIQUENESS,I.STATUS,C.COLUMN_NAME,C.COLUMN_POSITION FROM USER_INDEXES I JOIN USER_IND_COLUMNS C ON I.INDEX_NAME=C.INDEX_NAME WHERE I.TABLE_NAME=? ORDER BY I.UNIQUENESS,C.COLUMN_POSITION,C.COLUMN_NAME",table);
        String constraints=query(connection,"SELECT C.CONSTRAINT_TYPE,K.COLUMN_NAME,K.POSITION FROM USER_CONSTRAINTS C LEFT JOIN USER_CONS_COLUMNS K ON C.CONSTRAINT_NAME=K.CONSTRAINT_NAME AND C.TABLE_NAME=K.TABLE_NAME WHERE C.TABLE_NAME=? ORDER BY C.CONSTRAINT_TYPE,K.POSITION,K.COLUMN_NAME",table);
        String ddl=null;try(PreparedStatement statement=connection.prepareStatement("SELECT DBMS_METADATA.GET_DDL('TABLE',?,?) FROM DUAL")){statement.setQueryTimeout(5);statement.setString(1,table);statement.setString(2,schema);try(ResultSet result=statement.executeQuery()){if(result.next()){String value=result.getString(1);if(value!=null&&value.length()<=65536&&!result.next())ddl=value;}}}catch(SQLException ignored){}
        return "{\"rows\":"+data+",\"structure\":{\"columns\":"+columns+",\"indexes\":"+indexes+",\"constraints\":"+constraints+",\"ddl\":"+(ddl==null?"null":DirectLoadOracleFixture.json(ddl))+"}}";
    }
}
