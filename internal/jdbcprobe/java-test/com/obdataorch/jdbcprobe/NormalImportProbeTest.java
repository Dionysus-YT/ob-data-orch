package com.obdataorch.jdbcprobe;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataOutputStream;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.lang.reflect.InvocationHandler;
import java.lang.reflect.Method;
import java.lang.reflect.Proxy;
import java.sql.Connection;
import java.sql.DatabaseMetaData;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/** 只用合成 JDBC 代理验证普通导入只读检查；任何额外 SQL 或写接口调用都失败。 */
public final class NormalImportProbeTest {
    private static final String COLUMNS="SELECT COLUMN_NAME, IS_NULLABLE, COLUMN_DEFAULT, EXTRA, TABLE_SCHEMA, TABLE_NAME, DATA_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION";
    public static void main(String[] args) throws Exception {
        NormalImportProbe.Input input=input("MYSQL","synthetic_db");
        String[] all={"GRANT ALL PRIVILEGES ON `synthetic_db`.* TO 'synthetic'@'%'"};
        expect(NormalImportProbe.verify(connection(input,all,columns(),false),input),"ACCESSIBLE","ACCESSIBLE","ACCESSIBLE");
        expect(NormalImportProbe.verify(connection(input,new String[]{"GRANT SELECT ON `synthetic_db`.* TO 'synthetic'@'%'"},columns(),false),input),"ACCESSIBLE","NOT_ACCESSIBLE","ACCESSIBLE");
        expect(NormalImportProbe.verify(connection(input,new String[]{"GRANT `synthetic_role` TO 'synthetic'@'%'"},columns(),false),input),"ACCESSIBLE","UNAVAILABLE","ACCESSIBLE");
        expect(NormalImportProbe.verify(connection(input,new String[]{"GRANT ALL PRIVILEGES ON `synthetic_db`.`*` TO 'synthetic'@'%'"},columns(),false),input),"ACCESSIBLE","NOT_ACCESSIBLE","ACCESSIBLE");
        input.tables.get(0).privileges=9;
        expect(NormalImportProbe.verify(connection(input,new String[]{"GRANT CREATE, INSERT ON `synthetic_db`.`synthetic_table` TO 'synthetic'@'%'"},columns(),false),input),"ACCESSIBLE","ACCESSIBLE","ACCESSIBLE");
        input.tables.get(0).privileges=1;input.tables.get(0).columns=1;
        expect(NormalImportProbe.verify(connection(input,all,columns(),false),input),"ACCESSIBLE","ACCESSIBLE","NOT_ACCESSIBLE");
        input.tables.get(0).columns=2;input.includes=Arrays.asList("missing","note");
        expect(NormalImportProbe.verify(connection(input,all,columns(),false),input),"ACCESSIBLE","ACCESSIBLE","NOT_ACCESSIBLE");
        input.includes=new ArrayList<String>();input.excludes=Arrays.asList("id");input.tables.get(0).columns=1;
        expect(NormalImportProbe.verify(connection(input,all,columns(),false),input),"ACCESSIBLE","ACCESSIBLE","ACCESSIBLE");
        input.excludes=new ArrayList<String>();input.tables.get(0).columns=2;
        expect(NormalImportProbe.verify(connection(input,all,new ArrayList<Map<String,Object>>(),false),input),"ACCESSIBLE","ACCESSIBLE","UNAVAILABLE");
        input.tables.get(0).targets=Arrays.asList("id","note");input.tables.get(0).required=Arrays.asList("id");
        expect(NormalImportProbe.verify(connection(input,all,new ArrayList<Map<String,Object>>(),false),input),"ACCESSIBLE","ACCESSIBLE","ACCESSIBLE");
        input.flags=11;input.tables.get(0).flags=3;
        expect(NormalImportProbe.verify(connection(input,all,new ArrayList<Map<String,Object>>(),false),input),"ACCESSIBLE","ACCESSIBLE","ACCESSIBLE");
        input.tables.get(0).targets=new ArrayList<String>();input.tables.get(0).required=new ArrayList<String>();
        expect(NormalImportProbe.verify(connection(input,all,columns(),false),input),"ACCESSIBLE","ACCESSIBLE","UNAVAILABLE");
        input.flags=3;input.tables.get(0).flags=1;
        expect(NormalImportProbe.verify(connection(input,all,columns(),true),input),"ACCESSIBLE","UNAVAILABLE","UNAVAILABLE");
        NormalImportProbe.Input oracle=input("ORACLE","SYNTHETIC_SCHEMA");
        expect(NormalImportProbe.verify(connection(oracle,new String[]{"CREATE SESSION"},columns(),false),oracle),"ACCESSIBLE","ACCESSIBLE","ACCESSIBLE");
        expect(NormalImportProbe.verify(connection(oracle,new String[]{"SYNTHETIC_PRIVILEGES_UNAVAILABLE"},columns(),false),oracle),"ACCESSIBLE","ACCESSIBLE","ACCESSIBLE");
        oracle.tables.get(0).privileges=8;
        expect(NormalImportProbe.verify(connection(oracle,new String[]{"CREATE TABLE"},columns(),false),oracle),"ACCESSIBLE","ACCESSIBLE","ACCESSIBLE");
        expect(NormalImportProbe.verify(connection(oracle,new String[]{"DBA"},columns(),false),oracle),"ACCESSIBLE","UNAVAILABLE","ACCESSIBLE");
        expect(NormalImportProbe.verify(connection(oracle,new String[]{"SYNTHETIC_PRIVILEGES_UNAVAILABLE"},columns(),false),oracle),"ACCESSIBLE","UNAVAILABLE","ACCESSIBLE");
        oracle.tables.get(0).privileges=1;
        oracle.database="OTHER_SCHEMA";
        expect(NormalImportProbe.verify(connection(oracle,new String[]{"CREATE SESSION"},columns(),false),oracle),"ACCESSIBLE","UNAVAILABLE","ACCESSIBLE");
        expect(NormalImportProbe.verify(connection(oracle,new String[]{"INSERT ANY TABLE"},columns(),false),oracle),"ACCESSIBLE","ACCESSIBLE","ACCESSIBLE");
        expect(NormalImportProbe.verify(connection(oracle,new String[]{"DBA"},columns(),false),oracle),"ACCESSIBLE","UNAVAILABLE","ACCESSIBLE");
        expect(NormalImportProbe.verify(connection(oracle,new String[]{"SYNTHETIC_PRIVILEGES_UNAVAILABLE"},columns(),false),oracle),"ACCESSIBLE","UNAVAILABLE","ACCESSIBLE");
        detailedReasonsAndPerTablePrivileges();
        typedExclusionAndObjects();
        fixedProtocolBoundary();
    }

    /** 固定 v11 帧拒绝旧版本、未知权限位、用途不符和尾随数据，且不依赖真实驱动。 */
    private static void fixedProtocolBoundary() throws Exception {
        InputStream previous=System.in;
        try {
            for(int variant=0;variant<5;variant++) {
                ByteArrayOutputStream bytes=new ByteArrayOutputStream();DataOutputStream data=new DataOutputStream(bytes);
                data.writeInt(variant==1?10:11);writeText(data,"synthetic.invalid");data.writeInt(2883);writeText(data,"synthetic-user");writeText(data,"synthetic-password");writeText(data,"MYSQL");writeText(data,"synthetic_db");
                data.writeInt(variant==3?1:3);data.writeInt(0);data.writeInt(0);data.writeInt(1);writeText(data,"synthetic_table");data.writeInt(2);data.writeInt(variant==2?64:1);data.writeInt(1);data.writeInt(0);data.writeInt(0);data.writeInt(0);data.writeInt(0);data.writeInt(0);
                if(variant==4)data.writeByte(1);
                System.setIn(new ByteArrayInputStream(bytes.toByteArray()));boolean rejected=false;NormalImportProbe.Input input=null;
                try {input=NormalImportProbe.Input.read();}catch(IllegalArgumentException expected){rejected=true;}finally{if(input!=null)input.destroy();}
                if(rejected!=(variant!=0))throw new AssertionError("固定 v11 协议边界不符合合成预期");
            }
        } finally {System.setIn(previous);}
    }
    private static void writeText(DataOutputStream data,String text)throws Exception{byte[] value=text.getBytes(StandardCharsets.UTF_8);data.writeInt(value.length);data.write(value);}

    /** 类型排除必须有目标类型事实；声明必须有对应权限和依赖，非默认角色不能补足权限。 */
    private static void typedExclusionAndObjects() {
        NormalImportProbe.Input input=input("MYSQL","synthetic_db");input.tables.get(0).columns=1;input.excludedTypes=Arrays.asList("BLOB");
        List<Map<String,Object>> typed=columns();typed.get(0).put("7","int");typed.get(1).put("7","blob");
        String[] all={"GRANT ALL PRIVILEGES ON *.* TO 'synthetic'@'%'"};
        expect(NormalImportProbe.verify(connection(input,all,typed,false),input),"ACCESSIBLE","ACCESSIBLE","ACCESSIBLE");
        typed.get(1).put("2","NO");
        expect(NormalImportProbe.verify(connection(input,all,typed,false),input),"ACCESSIBLE","ACCESSIBLE","NOT_ACCESSIBLE","VERIFIED","REQUIRED_COLUMN_MISSING");
        for(String kind:new String[]{"table-group","view","trigger","obj-user","function","procedure"}) {
            input=input("MYSQL","synthetic_db");input.tables.clear();input.flags=49;
            NormalImportProbe.Definition object=new NormalImportProbe.Definition();object.kind=kind;object.name="synthetic_object";object.reference="trigger".equals(kind)?"synthetic_table":"";input.objects.add(object);
            expect(NormalImportProbe.verify(connection(input,all,columns(),false),input),"ACCESSIBLE","ACCESSIBLE","ACCESSIBLE");
            String[] insufficient={"GRANT SELECT ON `synthetic_db`.* TO 'synthetic'@'%'"};
            expect(NormalImportProbe.verify(connection(input,insufficient,columns(),false),input),"ACCESSIBLE","UNAVAILABLE","ACCESSIBLE");
            if("table-group".equals(kind)||"obj-user".equals(kind))expect(NormalImportProbe.verify(connection(input,new String[]{"GRANT ALL PRIVILEGES ON `synthetic_db`.* TO 'synthetic'@'%'"},columns(),false),input),"ACCESSIBLE","UNAVAILABLE","ACCESSIBLE");
        }
        for(String kind:new String[]{"role","sequence","synonym","type","type-body","package","package-body"}) {
            input=input("ORACLE","SYNTHETIC_SCHEMA");input.tables.clear();input.flags=49;
            NormalImportProbe.Definition object=new NormalImportProbe.Definition();object.kind=kind;object.name="SYNTHETIC_OBJECT";object.reference=kind.endsWith("-body")?"SYNTHETIC_OBJECT":"synonym".equals(kind)?"SYNTHETIC_TABLE":"";input.objects.add(object);
            String privilege="role".equals(kind)?"CREATE ROLE":"sequence".equals(kind)?"CREATE SEQUENCE":"synonym".equals(kind)?"CREATE SYNONYM":kind.startsWith("type")?"CREATE TYPE":"CREATE PROCEDURE";
            String[] explicit={privilege};expect(NormalImportProbe.verify(connection(input,explicit,columns(),false),input),"ACCESSIBLE","ACCESSIBLE","ACCESSIBLE");
            String[] defaults={"DEFAULT_ROLE:synthetic_role:YES","ROLE_PRIV:synthetic_role:"+privilege};
            expect(NormalImportProbe.verify(connection(input,defaults,columns(),false),input),"ACCESSIBLE","ACCESSIBLE","ACCESSIBLE");
            String[] inactive={"DEFAULT_ROLE:synthetic_role:NO","ROLE_PRIV:synthetic_role:"+privilege};
            expect(NormalImportProbe.verify(connection(input,inactive,columns(),false),input),"ACCESSIBLE","UNAVAILABLE","ACCESSIBLE");
            String[] nested={"DEFAULT_ROLE:synthetic_role:YES","ROLE_PRIV:other_role:"+privilege};
            expect(NormalImportProbe.verify(connection(input,nested,columns(),false),input),"ACCESSIBLE","UNAVAILABLE","ACCESSIBLE");
            if(!object.reference.isEmpty())expect(NormalImportProbe.verify(connection(input,new String[]{privilege,"SYNTHETIC_DEPENDENCY_MISSING"},columns(),false),input),"ACCESSIBLE","UNAVAILABLE","ACCESSIBLE");
        }
    }

    private static void detailedReasonsAndPerTablePrivileges() {
        NormalImportProbe.Input input=input("MYSQL","synthetic_db");
        NormalImportProbe.Table definition=new NormalImportProbe.Table();definition.name="new_table";definition.flags=2;definition.privileges=8|32;definition.targets=Arrays.asList("id","note");input.tables.add(definition);input.flags=51;
        String[] scoped={"GRANT INSERT ON `synthetic_db`.`synthetic_table` TO 'synthetic'@'%'","GRANT CREATE, DROP ON `synthetic_db`.`new_table` TO 'synthetic'@'%'"};
        expect(NormalImportProbe.verify(connection(input,scoped,columns(),false),input),"ACCESSIBLE","ACCESSIBLE","ACCESSIBLE","VERIFIED","VERIFIED");
        String[] missingDrop={scoped[0],"GRANT CREATE ON `synthetic_db`.`new_table` TO 'synthetic'@'%'"};
        expect(NormalImportProbe.verify(connection(input,missingDrop,columns(),false),input),"ACCESSIBLE","NOT_ACCESSIBLE","ACCESSIBLE","DROP_REQUIRED","VERIFIED");
        String[] role={scoped[0],missingDrop[1],"GRANT `synthetic_role` TO 'synthetic'@'%'"};
        expect(NormalImportProbe.verify(connection(input,role,columns(),false),input),"ACCESSIBLE","UNAVAILABLE","ACCESSIBLE","GRANTS_UNSUPPORTED","VERIFIED");
        String[] unrecognized={scoped[0],scoped[1],"REVOKE INSERT ON `synthetic_db`.`synthetic_table` FROM 'synthetic'@'%'"};
        expect(NormalImportProbe.verify(connection(input,unrecognized,columns(),false),input),"ACCESSIBLE","UNAVAILABLE","ACCESSIBLE","GRANTS_UNSUPPORTED","VERIFIED");
        input.flags=19;input.tables.get(0).flags=3;
        expect(NormalImportProbe.verify(connection(input,scoped,columns(),false),input),"ACCESSIBLE","UNAVAILABLE","UNAVAILABLE","DDL_UNSUPPORTED","INPUT_DEFINITION_UNSUPPORTED");
        input=input("MYSQL","synthetic_db");String[] grants={"GRANT INSERT ON `synthetic_db`.* TO 'synthetic'@'%'"};
        expect(NormalImportProbe.verify(connection(input,grants,new ArrayList<Map<String,Object>>(),false),input),"ACCESSIBLE","ACCESSIBLE","UNAVAILABLE","VERIFIED","TARGET_STRUCTURE_MISSING");
        input.tables.get(0).columns=1;
        expect(NormalImportProbe.verify(connection(input,grants,columns(),false),input),"ACCESSIBLE","ACCESSIBLE","NOT_ACCESSIBLE","VERIFIED","COLUMN_COUNT_MISMATCH");
        input.includes=Arrays.asList("unknown_column");
        expect(NormalImportProbe.verify(connection(input,grants,columns(),false),input),"ACCESSIBLE","ACCESSIBLE","NOT_ACCESSIBLE","VERIFIED","COLUMN_NAME_MISMATCH");
        input.includes=new ArrayList<String>();input.excludes=Arrays.asList("id");List<Map<String,Object>> required=columns();required.get(0).put("4","");
        expect(NormalImportProbe.verify(connection(input,grants,required,false),input),"ACCESSIBLE","ACCESSIBLE","NOT_ACCESSIBLE","VERIFIED","REQUIRED_COLUMN_MISSING");
        expect(NormalImportProbe.verify(connection(input,grants,columns(),true),input),"ACCESSIBLE","UNAVAILABLE","UNAVAILABLE","GRANTS_QUERY_FAILED","TARGET_METADATA_UNAVAILABLE");
    }
    private static NormalImportProbe.Input input(String mode,String database) {
        NormalImportProbe.Input input=new NormalImportProbe.Input();input.mode=mode;input.database=database;input.flags=3;
        NormalImportProbe.Table table=new NormalImportProbe.Table();table.name="synthetic_table";table.columns=2;table.privileges=1;table.flags=1;input.tables.add(table);return input;
    }
    private static void expect(String[] actual,String... expected) {if(!Arrays.equals(expected.length==3?Arrays.copyOf(actual,3):actual,expected))throw new AssertionError("普通导入检查结果不符合合成预期："+Arrays.toString(actual));}
    private static List<Map<String,Object>> columns() {
        List<Map<String,Object>> result=new ArrayList<Map<String,Object>>();
        result.add(row("1","id","2","NO","3",null,"4","auto_increment","COLUMN_NAME","id","IS_GENERATEDCOLUMN","NO","IS_AUTOINCREMENT","YES","NULLABLE",DatabaseMetaData.columnNoNulls,"COLUMN_DEF",null));
        result.add(row("1","note","2","YES","3",null,"4","","COLUMN_NAME","note","IS_GENERATEDCOLUMN","NO","IS_AUTOINCREMENT","NO","NULLABLE",DatabaseMetaData.columnNullable,"COLUMN_DEF",null));return result;
    }
    private static Map<String,Object> row(Object... pairs) {Map<String,Object> value=new HashMap<String,Object>();for(int i=0;i<pairs.length;i+=2)value.put((String)pairs[i],pairs[i+1]);return value;}
    @SuppressWarnings("unchecked")
    private static <T>T proxy(Class<T> type,InvocationHandler handler){return (T)Proxy.newProxyInstance(NormalImportProbeTest.class.getClassLoader(),new Class<?>[]{type},handler);}
    private static Object idle(Method method) {if("close".equals(method.getName())||"setQueryTimeout".equals(method.getName()))return null;throw new AssertionError("只读探针调用了未允许的接口："+method.getName());}
    private static ResultSet rows(final List<Map<String,Object>> data) {
        return proxy(ResultSet.class,new InvocationHandler(){int index=-1;boolean wasNull;
            public Object invoke(Object target,Method method,Object[] args)throws Throwable {
                if("next".equals(method.getName()))return ++index<data.size();if("wasNull".equals(method.getName()))return wasNull;
                if("getString".equals(method.getName())||"getInt".equals(method.getName())) {String key=String.valueOf(args[0]);if(!data.get(index).containsKey(key))throw new SQLException("synthetic-unavailable");Object value=data.get(index).get(key);wasNull=value==null;return "getInt".equals(method.getName())?(value==null?0:((Number)value).intValue()):(value==null?null:String.valueOf(value));}
                return idle(method);
            }});
    }
    private static Connection connection(final NormalImportProbe.Input input,final String[] grants,final List<Map<String,Object>> columns,final boolean unavailable) {
        return proxy(Connection.class,new InvocationHandler(){
            public Object invoke(Object target,Method method,Object[] args)throws Throwable {
                String name=method.getName();
                if("setCatalog".equals(name)){if(!input.database.equals(args[0]))throw new AssertionError();return null;}
                if("createStatement".equals(name))return proxy(Statement.class,new InvocationHandler(){public Object invoke(Object target,Method method,Object[] args)throws Throwable {
                    if(!"executeQuery".equals(method.getName()))return idle(method);if(unavailable)throw new SQLException("synthetic-unavailable");String sql=(String)args[0];List<Map<String,Object>> data=new ArrayList<Map<String,Object>>();
                    if("SELECT SYS_CONTEXT('USERENV', 'CURRENT_USER') FROM DUAL".equals(sql)){data.add(row("1","SYNTHETIC_SCHEMA"));return rows(data);}
                    if("SELECT GRANTED_ROLE,DEFAULT_ROLE FROM USER_ROLE_PRIVS".equals(sql)){for(String value:grants)if(value.startsWith("DEFAULT_ROLE:")){String[] parts=value.split(":",3);data.add(row("1",parts[1],"2",parts[2]));}return rows(data);}
                    if("SELECT ROLE,PRIVILEGE FROM ROLE_SYS_PRIVS".equals(sql)){for(String value:grants)if(value.startsWith("ROLE_PRIV:")){String[] parts=value.split(":",3);data.add(row("1",parts[1],"2",parts[2]));}return rows(data);}
                    if(!"SHOW GRANTS".equals(sql)&&!"SELECT PRIVILEGE FROM USER_SYS_PRIVS".equals(sql))throw new AssertionError("探针传入了非固定查询");
                    if(Arrays.asList(grants).contains("SYNTHETIC_PRIVILEGES_UNAVAILABLE"))throw new SQLException("synthetic-unavailable");
                    for(String grant:grants)if(!grant.startsWith("DEFAULT_ROLE:")&&!grant.startsWith("ROLE_PRIV:"))data.add(row("1",grant));return rows(data);
                }});
                if("prepareStatement".equals(name)){
                    if("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=? AND TABLE_NAME=?".equals(args[0])||"SELECT COUNT(*) FROM ALL_OBJECTS WHERE OWNER=? AND OBJECT_NAME=? AND OBJECT_TYPE=? AND STATUS='VALID'".equals(args[0]))return proxy(PreparedStatement.class,new InvocationHandler(){public Object invoke(Object target,Method method,Object[] args)throws Throwable{if("setString".equals(method.getName()))return null;if("executeQuery".equals(method.getName()))return rows(Arrays.asList(row("1",Arrays.asList(grants).contains("SYNTHETIC_DEPENDENCY_MISSING")?0:1)));return idle(method);}});
                    if(!COLUMNS.equals(args[0]))throw new AssertionError("探针传入了非固定查询");return proxy(PreparedStatement.class,new InvocationHandler(){Map<Integer,String> binds=new HashMap<Integer,String>();public Object invoke(Object target,Method method,Object[] args)throws Throwable {
                    if("setString".equals(method.getName())){binds.put((Integer)args[0],(String)args[1]);return null;}
                    if("executeQuery".equals(method.getName())){if(unavailable)throw new SQLException("synthetic-unavailable");if(!input.database.equals(binds.get(1))||!"synthetic_table".equals(binds.get(2)))throw new AssertionError("元信息未绑定当前库表");List<Map<String,Object>> data=new ArrayList<Map<String,Object>>();for(Map<String,Object> column:columns){Map<String,Object> row=new HashMap<String,Object>(column);row.put("5",input.database);row.put("6","synthetic_table");data.add(row);}return rows(data);}return idle(method);
                }});}
                if("getMetaData".equals(name)){if("MYSQL".equals(input.mode))throw new AssertionError("MySQL 固定列查询不应依赖驱动的元数据通配符接口");return proxy(DatabaseMetaData.class,new InvocationHandler(){public Object invoke(Object target,Method method,Object[] args)throws Throwable {
                    if("getSearchStringEscape".equals(method.getName()))return "\\";
                    if("getSchemas".equals(method.getName()))return rows(Arrays.asList(row("TABLE_SCHEM",input.database)));
                    if("getColumns".equals(method.getName())){List<Map<String,Object>> data=new ArrayList<Map<String,Object>>();for(Map<String,Object> column:columns){Map<String,Object> value=new HashMap<String,Object>(column);value.put("TABLE_SCHEM",input.database);value.put("TABLE_NAME","synthetic_table");data.add(value);}return rows(data);}return idle(method);
                }});}
                return idle(method);
            }});
    }
}
