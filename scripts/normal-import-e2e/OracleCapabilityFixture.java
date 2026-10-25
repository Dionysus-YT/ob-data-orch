import java.io.DataInputStream;
import java.nio.charset.StandardCharsets;
import java.sql.*;
import java.util.*;

/** 测试专用固定夹具，不进入产品 JAR；只操作带 run 所有权的有限 Oracle 对象。 */
public final class OracleCapabilityFixture {
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
            if(!Arrays.asList("create","inspect","cleanup").contains(action.replace("import-oracle-capabilities-","")))throw new IllegalArgumentException();
            Class.forName("com.oceanbase.jdbc.Driver");p.setProperty("user",username);p.setProperty("password",new String(password,StandardCharsets.UTF_8));
            try(Connection c=DriverManager.getConnection("jdbc:oceanbase://"+host+":"+port+"/?connectTimeout=5000&socketTimeout=10000",p)) {
                String current=scalar(c,"SELECT USER FROM DUAL");
                if(!schema.equals(current))throw new IllegalArgumentException();
                operate(c,action,run);
            }
        } catch(Exception e){System.out.println("{\"status\":\"BLOCKED\",\"code\":\"ORACLE_FIXED_FIXTURE_UNAVAILABLE\"}");code=1;}
        finally {p.clear();if(password!=null)Arrays.fill(password,(byte)0);}
        if(code!=0)System.exit(code);
    }
    private static void execute(Connection c,String sql) throws Exception {
        try(Statement s=c.createStatement()){s.setQueryTimeout(10);s.executeUpdate(sql);}
    }
    private static int count(Connection c,String sql,String value) throws Exception {
        try(PreparedStatement s=c.prepareStatement(sql)){s.setString(1,value);try(ResultSet r=s.executeQuery()){if(!r.next())throw new SQLException();return r.getInt(1);}}
    }
    private static void operate(Connection c,String action,String run) throws Exception {
        String prefix="NIVC_"+run.substring(0,12).toUpperCase(Locale.ROOT)+"_",owner=prefix+"OWNER";
        String[] keys={"role","sequence","synonym","type","type_body","package","package_body","trigger"};
        String[] suffix={"ROLE","SEQ","SYN","TYP","TB","PKG","PB","TRG"};
        String[] kinds={"ROLE","SEQUENCE","SYNONYM","TYPE","TYPE BODY","PACKAGE","PACKAGE BODY","TRIGGER"};
        if(action.endsWith("-create")) {
            // 在所有权表中记录全局角色初始不存在；禁止接管同名对象。
            if(count(c,"SELECT COUNT(*) FROM DBA_ROLES WHERE ROLE=?",prefix+"ROLE")!=0)throw new IllegalArgumentException();
            execute(c,"CREATE TABLE "+owner+" (RUN_ID VARCHAR2(32) PRIMARY KEY)");
            try(PreparedStatement s=c.prepareStatement("INSERT INTO "+owner+" VALUES (?)")){s.setString(1,run);s.executeUpdate();}c.commit();
            execute(c,"CREATE TABLE "+prefix+"BASE (ID NUMBER PRIMARY KEY)");
            execute(c,"CREATE TYPE "+prefix+"TB AS OBJECT (ID NUMBER, MEMBER FUNCTION VALUE1 RETURN NUMBER)");
            execute(c,"CREATE PACKAGE "+prefix+"PB AS PROCEDURE P; END;");
            System.out.println("{\"status\":\"CREATED\",\"ownerVerified\":true}");return;
        }
        if(!run.equals(scalar(c,"SELECT RUN_ID FROM "+owner)))throw new IllegalArgumentException();
        if(action.endsWith("-cleanup")) {
            if(count(c,"SELECT COUNT(*) FROM DBA_ROLES WHERE ROLE=?",prefix+"ROLE")==1)execute(c,"DROP ROLE "+prefix+"ROLE");
            String[] drops={"PACKAGE PB","PACKAGE PKG","TYPE TB","TYPE TYP","SYNONYM SYN","SEQUENCE SEQ","TABLE BASE"};
            String[] objectKinds={"PACKAGE","PACKAGE","TYPE","TYPE","SYNONYM","SEQUENCE","TABLE"};
            for(int i=0;i<drops.length;i++){String[] d=drops[i].split(" ");if(count(c,"SELECT COUNT(*) FROM USER_OBJECTS WHERE OBJECT_NAME=?",prefix+d[1])>0)execute(c,"DROP "+objectKinds[i]+" "+prefix+d[1]+(i==6?" PURGE":""));}
            execute(c,"DROP TABLE "+owner+" PURGE");
            int remaining=0;
            for(String name:new String[]{"OWNER","BASE","SEQ","SYN","TYP","TB","PKG","PB","TRG"})remaining+=count(c,"SELECT COUNT(*) FROM USER_OBJECTS WHERE OBJECT_NAME=?",prefix+name);
            boolean absent=remaining==0&&count(c,"SELECT COUNT(*) FROM DBA_ROLES WHERE ROLE=?",prefix+"ROLE")==0;
            System.out.println("{\"status\":\"CLEANED\",\"ownerVerified\":true,\"objectsAbsent\":"+absent+"}");return;
        }
        StringBuilder out=new StringBuilder("{\"status\":\"INSPECTED\",\"ownerVerified\":true,\"records\":[");
        for(int i=0;i<keys.length;i++) {
            String name=prefix+suffix[i];boolean exists=false,matched=false;
            if(i==0){
                exists=count(c,"SELECT COUNT(*) FROM DBA_ROLES WHERE ROLE=?",name)==1;
                // Oracle 自动把新角色授予创建者；只允许该一条原生 ADMIN OPTION 关系，角色自身必须无权限。
                matched=exists&&count(c,"SELECT COUNT(*) FROM DBA_ROLE_PRIVS WHERE GRANTED_ROLE=?",name)==1
                    &&count(c,"SELECT COUNT(*) FROM DBA_ROLE_PRIVS WHERE GRANTED_ROLE=? AND GRANTEE=USER AND ADMIN_OPTION='YES'",name)==1
                    &&count(c,"SELECT COUNT(*) FROM DBA_SYS_PRIVS WHERE GRANTEE=?",name)==0
                    &&count(c,"SELECT COUNT(*) FROM DBA_TAB_PRIVS WHERE GRANTEE=?",name)==0
                    &&count(c,"SELECT COUNT(*) FROM DBA_ROLE_PRIVS WHERE GRANTEE=?",name)==0;
            }
            else {try(PreparedStatement s=c.prepareStatement("SELECT STATUS FROM USER_OBJECTS WHERE OBJECT_NAME=? AND OBJECT_TYPE=?")){s.setString(1,name);s.setString(2,kinds[i]);try(ResultSet r=s.executeQuery()){exists=r.next();matched=exists&&"VALID".equals(r.getString(1))&&!r.next();}}}
            if(exists&&i==1)try(PreparedStatement s=c.prepareStatement("SELECT INCREMENT_BY,CACHE_SIZE FROM USER_SEQUENCES WHERE SEQUENCE_NAME=?")){s.setString(1,name);try(ResultSet r=s.executeQuery()){matched&=r.next()&&r.getInt(1)==1&&r.getInt(2)==0;}}
            if(exists&&i==2)matched&="0".equals(scalar(c,"SELECT COUNT(*) FROM "+name));
            if(exists&&i==3)matched&=count(c,"SELECT COUNT(*) FROM USER_TYPE_ATTRS WHERE TYPE_NAME=? AND ATTR_NAME='ID' AND ATTR_TYPE_NAME='NUMBER'",name)==1;
            if(exists&&i==4)matched&="1".equals(scalar(c,"SELECT V.T.VALUE1() FROM (SELECT "+name+"(1) T FROM DUAL) V"));
            if(exists&&i==6)try(CallableStatement s=c.prepareCall("BEGIN "+name+".P; END;")){s.execute();}
            if(exists&&i==7)matched&=count(c,"SELECT COUNT(*) FROM USER_TRIGGERS WHERE TRIGGER_NAME=? AND STATUS='ENABLED' AND TRIGGERING_EVENT='INSERT' AND TABLE_NAME='"+prefix+"BASE'",name)==1;
            if(i>0)out.append(',');out.append("{\"key\":\"").append(keys[i]).append("\",\"exists\":").append(exists).append(",\"contentMatched\":").append(matched).append(",\"structureMatched\":").append(matched).append('}');
        }
        System.out.println(out.append("]}").toString());
    }
}
