import java.io.DataInputStream;
import java.nio.charset.StandardCharsets;
import java.sql.*;
import java.util.*;

/** 测试专用固定夹具，不进入产品 JAR；只操作带 run 所有权的三个 Oracle 表。 */
public final class OracleRemainingFixture {
    private static String text(DataInputStream in) throws Exception {
        int n=in.readInt();if(n<0||n>4096)throw new IllegalArgumentException();byte[] b=new byte[n];in.readFully(b);try{return new String(b,StandardCharsets.UTF_8);}finally{Arrays.fill(b,(byte)0);}
    }
    private static String scalar(Connection c,String sql) throws Exception {
        try(Statement s=c.createStatement()){s.setQueryTimeout(5);try(ResultSet r=s.executeQuery(sql)){if(!r.next())throw new SQLException();String v=r.getString(1);if(v==null||r.next())throw new SQLException();return v;}}
    }
    private static boolean queryWorks(Connection c,String sql) {try{scalar(c,sql);return true;}catch(Exception e){return false;}}
    public static void main(String[] args) {
        Properties p=new Properties();byte[] password=null;int code=0;
        try {
            if(args.length!=0)throw new IllegalArgumentException();
            DataInputStream in=new DataInputStream(System.in);
            if(!"NIVR1".equals(text(in)))throw new IllegalArgumentException();
            String action=text(in),run=text(in),schema=text(in),host=text(in),port=text(in),username=text(in);
            int n=in.readInt();if(n<=0||n>4096)throw new IllegalArgumentException();password=new byte[n];in.readFully(password);if(in.read()!=-1)throw new IllegalArgumentException();
            if(!run.matches("[a-f0-9]{32}")||!schema.matches("[A-Za-z_][A-Za-z0-9_$#]{0,127}")||!host.matches("[A-Za-z0-9.:-]{1,253}")||!port.matches("[0-9]{1,5}"))throw new IllegalArgumentException();
            if(!Arrays.asList("probe","create","inspect","cleanup").contains(action.replace("import-oracle-remaining-","")))throw new IllegalArgumentException();
            Class.forName("com.oceanbase.jdbc.Driver");p.setProperty("user",username);p.setProperty("password",new String(password,StandardCharsets.UTF_8));
            try(Connection c=DriverManager.getConnection("jdbc:oceanbase://"+host+":"+port+"/?connectTimeout=5000&socketTimeout=10000",p)) {
                String current=scalar(c,"SELECT USER FROM DUAL");
                if(!schema.equals(current))throw new IllegalArgumentException();
                if(action.endsWith("-probe")) {
                    boolean context=queryWorks(c,"SELECT SYS_CONTEXT('USERENV', 'CURRENT_USER') FROM DUAL");
                    boolean privs=false,create=false;int count=0;
                    try(Statement s=c.createStatement();ResultSet r=s.executeQuery("SELECT PRIVILEGE FROM SESSION_PRIVS")){while(r.next()){if(++count>4096)throw new SQLException();create|="CREATE TABLE".equals(r.getString(1));}privs=true;}catch(SQLException e){}
                    boolean direct=false,directCreate=false;int directCount=0;
                    try(Statement s=c.createStatement();ResultSet r=s.executeQuery("SELECT PRIVILEGE FROM USER_SYS_PRIVS")){while(r.next()){if(++directCount>4096)throw new SQLException();directCreate|="CREATE TABLE".equals(r.getString(1));}direct=true;}catch(SQLException e){}
                    boolean activeRolesReadable=false;int activeRoles=0;
                    try(Statement s=c.createStatement();ResultSet r=s.executeQuery("SELECT ROLE FROM SESSION_ROLES")){while(r.next()){if(++activeRoles>256)throw new SQLException();}activeRolesReadable=true;}catch(SQLException e){}
                    boolean defaultRolesReadable=false,rolePrivilegesReadable=false,roleCreate=false;int defaults=0;Set<String> roles=new HashSet<String>();
                    try(Statement s=c.createStatement();ResultSet r=s.executeQuery("SELECT GRANTED_ROLE,DEFAULT_ROLE FROM USER_ROLE_PRIVS")){while(r.next()){if("YES".equals(r.getString(2))){roles.add(r.getString(1));defaults++;}if(defaults>256)throw new SQLException();}defaultRolesReadable=true;}catch(SQLException e){}
                    try(Statement s=c.createStatement();ResultSet r=s.executeQuery("SELECT ROLE,PRIVILEGE FROM ROLE_SYS_PRIVS")){int seen=0;while(r.next()){if(++seen>4096)throw new SQLException();if(roles.contains(r.getString(1)))roleCreate|="CREATE TABLE".equals(r.getString(2));}rolePrivilegesReadable=true;}catch(SQLException e){}
                    boolean roleContext=true;
                    for(String role:roles)try(PreparedStatement s=c.prepareStatement("SELECT SYS_CONTEXT('SYS_SESSION_ROLES', ?) FROM DUAL")){s.setString(1,role);try(ResultSet r=s.executeQuery()){roleContext&=r.next()&&"TRUE".equals(r.getString(1));}}catch(SQLException e){roleContext=false;}
                    System.out.println("{\"defaultRoleSessionContextVerified\":"+(!roles.isEmpty()&&roleContext)+"}");
                    System.out.println("{\"defaultRolesReadable\":"+defaultRolesReadable+",\"defaultRoleCount\":"+defaults+",\"rolePrivilegesReadable\":"+rolePrivilegesReadable+",\"defaultRoleCreateTable\":"+roleCreate+"}");
                    System.out.println("{\"status\":\"ORACLE_DIAGNOSED\",\"schemaOwnerMatched\":true,\"builtinSys\":"+"SYS".equals(current)+",\"sysContextSupported\":"+context+",\"sessionPrivilegesReadable\":"+privs+",\"createTablePrivilege\":"+create+",\"directPrivilegesReadable\":"+direct+",\"directCreateTablePrivilege\":"+directCreate+",\"activeRolesReadable\":"+activeRolesReadable+",\"activeRoleCount\":"+activeRoles+"}");
                } else operate(c,action,run);
            }
        } catch(Exception e){System.out.println("{\"status\":\"BLOCKED\",\"code\":\"ORACLE_FIXED_FIXTURE_UNAVAILABLE\"}");code=1;}
        finally {p.clear();if(password!=null)Arrays.fill(password,(byte)0);}
        if(code!=0)System.exit(code);
    }
    private static void operate(Connection c,String action,String run) throws Exception {
        String prefix="NIVR_"+run.substring(0,12).toUpperCase(Locale.ROOT)+"_",owner=prefix+"OWNER";
        String[] suffix={"DATE","TIMESTAMP","TZ"},types={"DATE","TIMESTAMP(6)","TIMESTAMP(6) WITH TIME ZONE"};
        if(action.endsWith("-create")) {
            // 不接管已存在对象；所有对象名称由本轮随机标识派生，失败保留固定清理入口。
            try(Statement s=c.createStatement()){s.executeUpdate("CREATE TABLE "+owner+" (RUN_ID VARCHAR2(32) PRIMARY KEY)");}
            try(PreparedStatement s=c.prepareStatement("INSERT INTO "+owner+" VALUES (?)")){s.setString(1,run);s.executeUpdate();}
            c.commit();
            for(int i=0;i<3;i++)try(Statement s=c.createStatement()){s.executeUpdate("CREATE TABLE "+prefix+suffix[i]+" (ID NUMBER PRIMARY KEY, VALUE1 "+types[i]+")");}
            System.out.println("{\"status\":\"CREATED\",\"ownerVerified\":true,\"tableCount\":3}");return;
        }
        if(!run.equals(scalar(c,"SELECT RUN_ID FROM "+owner)))throw new IllegalArgumentException();
        if(action.endsWith("-cleanup")) {
            for(String name:suffix)try(Statement s=c.createStatement()){s.executeUpdate("DROP TABLE "+prefix+name+" PURGE");}
            try(Statement s=c.createStatement()){s.executeUpdate("DROP TABLE "+owner+" PURGE");}
            try(PreparedStatement s=c.prepareStatement("SELECT COUNT(*) FROM USER_TABLES WHERE TABLE_NAME IN (?,?,?,?)")){for(int i=0;i<3;i++)s.setString(i+1,prefix+suffix[i]);s.setString(4,owner);try(ResultSet r=s.executeQuery()){if(!r.next()||r.getInt(1)!=0)throw new SQLException();}}
            System.out.println("{\"status\":\"CLEANED\",\"ownerVerified\":true,\"tablesAbsent\":true}");return;
        }
        String[] formats={"YYYY-MM-DD HH24:MI:SS","YYYY-MM-DD HH24:MI:SS.FF6","YYYY-MM-DD HH24:MI:SS.FF6 TZH:TZM"};
        String[] expected={"2026-10-20 12:34:56","2026-10-20 12:34:56.123456","2026-10-20 12:34:56.123456 +08:00"};
        StringBuilder out=new StringBuilder("{\"status\":\"INSPECTED\",\"ownerVerified\":true,\"records\":[");
        for(int i=0;i<3;i++) {
            int rows=0;boolean content=true;
            try(Statement s=c.createStatement();ResultSet r=s.executeQuery("SELECT ID,TO_CHAR(VALUE1,'"+formats[i]+"') FROM "+prefix+suffix[i]+" ORDER BY ID")){while(r.next()){rows++;content&=r.getInt(1)==1&&expected[i].equals(r.getString(2));}}
            int columns=0;boolean structure=true;
            try(PreparedStatement s=c.prepareStatement("SELECT COLUMN_NAME,DATA_TYPE,NULLABLE FROM USER_TAB_COLUMNS WHERE TABLE_NAME=? ORDER BY COLUMN_ID")){s.setString(1,prefix+suffix[i]);try(ResultSet r=s.executeQuery()){while(r.next()){columns++;structure&=columns==1?"ID".equals(r.getString(1))&&"NUMBER".equals(r.getString(2))&&"N".equals(r.getString(3)):columns==2&&"VALUE1".equals(r.getString(1))&&types[i].equals(r.getString(2));}}}
            try(PreparedStatement s=c.prepareStatement("SELECT COUNT(*) FROM USER_CONSTRAINTS C JOIN USER_CONS_COLUMNS K ON C.CONSTRAINT_NAME=K.CONSTRAINT_NAME AND C.TABLE_NAME=K.TABLE_NAME WHERE C.TABLE_NAME=? AND C.CONSTRAINT_TYPE='P' AND K.COLUMN_NAME='ID' AND K.POSITION=1")){s.setString(1,prefix+suffix[i]);try(ResultSet r=s.executeQuery()){structure&=r.next()&&r.getInt(1)==1;}}
            if(i>0)out.append(',');out.append("{\"key\":\"").append(suffix[i]).append("\",\"actualRows\":").append(rows).append(",\"contentMatched\":").append(rows==1&&content).append(",\"structureMatched\":").append(columns==2&&structure).append('}');
        }
        System.out.println(out.append("]}").toString());
    }
}
