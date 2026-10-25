package com.obdataorch.jdbcprobe;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataOutputStream;
import java.io.InputStream;
import java.lang.reflect.InvocationHandler;
import java.lang.reflect.Method;
import java.lang.reflect.Proxy;
import java.nio.charset.StandardCharsets;
import java.sql.Connection;
import java.sql.DatabaseMetaData;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.Arrays;

/** 仅使用代理 JDBC 与内存二进制帧；任何未列明 SQL、试写或提交都会直接失败。 */
public final class DirectLoadProbeTest {
    private DirectLoadProbeTest() {}
    public static void main(String[] args)throws Exception {
        Fixture f=new Fixture();DirectLoadProbe.Input input=input(false);
        DirectLoadProbe.Observation o=DirectLoadProbe.verify(f.connection(),input);
        require("ACCESSIBLE".equals(o.database)&&"ACCESSIBLE".equals(o.write)&&"4.3.5.2".equals(o.observer)&&Boolean.TRUE.equals(o.empty)&&Boolean.FALSE.equals(o.bit)&&Boolean.FALSE.equals(o.binary)&&Boolean.FALSE.equals(o.virtual)&&Boolean.FALSE.equals(o.index),"基础事实未独立取得");
        require(o.json().contains("\"odpVersion\":\"\"")&&!o.json().contains("synthetic-user"),"未知版本或秘密边界失效");
        input.scenario="PRIVATE_ODP";o=DirectLoadProbe.verify(f.connection(),input);
        require("4.3.5".equals(o.odp)&&o.json().contains("\"odpVersion\":\"4.3.5\"")&&!o.json().contains("synthetic-build"),"ODP 固定版本未读取或泄露了原始构建信息");
        f.proxyDenied=true;require("".equals(DirectLoadProbe.verify(f.connection(),input).odp),"拒绝 ODP 版本查询后未保持未知");f.proxyDenied=false;
        for(String[][] proxyInfo:new String[][][]{{},{ {"other info","ObProxy-OceanBase 4.3.5"} },{ {"binary info","MySQL 5.7 compatible with 4.3.5"} },{ {"binary info","ObProxy-OceanBase 4.3"} },{ {"binary info","ObProxy-OceanBase 4.3.5unexpected"} },{ {"binary info","ObProxy-OceanBase 4.3.5"},{"binary info","ObProxy-OceanBase 9.9.9"} }}) {
            f.proxyInfo=proxyInfo;require("".equals(DirectLoadProbe.verify(f.connection(),input).odp),"异常 ODP 返回被推测为版本");
        }
        f.proxyInfo=new String[][]{{"binary info","ObProxy-OceanBase 4.3.5.1-synthetic-build\nversion:RELEASE"}};
        require("4.3.5.1".equals(DirectLoadProbe.verify(f.connection(),input).odp),"四段 ODP 版本未保留");
        input.scenario="OBSERVER_DIRECT";require("".equals(DirectLoadProbe.verify(f.connection(),input).odp),"直连场景错误借用 ODP 元信息");
        input.truncate=true;o=DirectLoadProbe.verify(f.connection(),input);require("NOT_ACCESSIBLE".equals(o.write),"TRUNCATE 未要求 DROP");
        f.grants=new String[][]{{"GRANT INSERT, DROP ON `synthetic_db`.`synthetic_table` TO synthetic_identity"}};
        require("ACCESSIBLE".equals(DirectLoadProbe.verify(f.connection(),input).write),"精确目标授权未识别");
        f.grants=new String[][]{{"GRANT synthetic_role TO synthetic_identity"}};
        require("UNAVAILABLE".equals(DirectLoadProbe.verify(f.connection(),input).write),"未展开角色被伪装为授权通过");
        f.grants=new String[][]{{"GRANT ALL PRIVILEGES ON *.* TO synthetic_identity"}};f.viewsDenied=true;
        require("UNAVAILABLE".equals(DirectLoadProbe.verify(f.connection(),input).write),"系统视图权限缺失被放行");f.viewsDenied=false;
        String[][][] columns={{{"bit",""}},{{"binary",""}},{{"varchar","VIRTUAL GENERATED"}}};
        for(int i=0;i<columns.length;i++){f.columns=columns[i];o=DirectLoadProbe.verify(f.connection(),input);require(Boolean.TRUE.equals(i==0?o.bit:i==1?o.binary:o.virtual),"受限结构未保留可信事实");}
        f.columns=new String[][]{{"unrecognized_type",""}};o=DirectLoadProbe.verify(f.connection(),input);require(o.bit==null&&o.binary==null&&o.virtual==null&&o.lob==null,"未知类型被当成不含受限结构");
        f.columns=new String[][]{{"blob",""}};f.indexes=new String[][]{{"synthetic_index","0"}};f.empty=false;
        o=DirectLoadProbe.verify(f.connection(),input);require(Boolean.TRUE.equals(o.lob)&&Boolean.TRUE.equals(o.index)&&Boolean.TRUE.equals(o.unique)&&Boolean.FALSE.equals(o.empty),"增量模式依赖事实错误");
        f.tableVisible=false;o=DirectLoadProbe.verify(f.connection(),input);require(o.exists==null&&o.empty==null,"不可见目标被误判为空表");
        f.tableVisible=true;f.metadataDenied=true;o=DirectLoadProbe.verify(f.connection(),input);require(o.exists==null&&"ACCESSIBLE".equals(o.database),"结构缺权限污染连接或放行");
        f.metadataDenied=false;f.version="5.7-OceanBase-v4.3.5.2";require("".equals(DirectLoadProbe.verify(f.connection(),input).observer),"非固定版本返回被猜测解析");
        input.destroy();for(byte b:input.password)require(b==0,"秘密未清零");
        protocol();DirectLoadOracleMetadataTest.run();System.out.println("DirectLoadProbe synthetic tests passed");
    }
    private static void protocol()throws Exception {
        byte[] good=frame(11,"synthetic_table",0,false);InputStream previous=System.in;
        try {
            System.setIn(new ByteArrayInputStream(good));DirectLoadProbe.Input parsed=DirectLoadProbe.Input.read();require("OBSERVER_DIRECT".equals(parsed.scenario),"固定协议场景丢失");parsed.destroy();
            byte[] malformed=good.clone();malformed[4+4+"synthetic.invalid".getBytes(StandardCharsets.UTF_8).length+4+4]=(byte)0xff;
            byte[][] bad={frame(10,"synthetic_table",0,false),frame(11,"db.table",0,false),frame(11,"x`",0,false),frame(11,"synthetic_table",2,false),frame(11,"synthetic_table",0,true),Arrays.copyOf(good,good.length-1),Arrays.copyOf(good,good.length+1),malformed};
            for(byte[] raw:bad){System.setIn(new ByteArrayInputStream(raw));boolean rejected=false;try{DirectLoadProbe.Input.read();}catch(Exception expected){rejected=true;}require(rejected,"不规范二进制输入未拒绝");}
        }finally{System.setIn(previous);}
    }
    private static byte[] frame(int version,String table,int flags,boolean badSecret)throws Exception {
        ByteArrayOutputStream bytes=new ByteArrayOutputStream();DataOutputStream out=new DataOutputStream(bytes);out.writeInt(version);text(out,"synthetic.invalid");out.writeInt(2881);text(out,"synthetic-user");text(out,badSecret?"synthetic\npassword":"synthetic-password");text(out,"MYSQL");text(out,"synthetic_db");text(out,table);text(out,"OBSERVER_DIRECT");out.writeInt(flags);out.flush();return bytes.toByteArray();
    }
    private static void text(DataOutputStream out,String value)throws Exception {byte[] bytes=value.getBytes(StandardCharsets.UTF_8);out.writeInt(bytes.length);out.write(bytes);}
    private static DirectLoadProbe.Input input(boolean truncate){DirectLoadProbe.Input i=new DirectLoadProbe.Input();i.host="synthetic.invalid";i.port=2881;i.username="synthetic-user".getBytes(StandardCharsets.UTF_8);i.password="synthetic-password".getBytes(StandardCharsets.UTF_8);i.mode="MYSQL";i.database="synthetic_db";i.table="synthetic_table";i.scenario="OBSERVER_DIRECT";i.truncate=truncate;return i;}
    private static void require(boolean condition,String message){if(!condition)throw new AssertionError(message);}
    private static Object proxy(Class<?> type,InvocationHandler handler){return Proxy.newProxyInstance(DirectLoadProbeTest.class.getClassLoader(),new Class<?>[]{type},handler);}
    private static ResultSet rows(final String[][] values){return(ResultSet)proxy(ResultSet.class,new InvocationHandler(){int index=-1;public Object invoke(Object p,Method m,Object[] a){String n=m.getName();if(n.equals("next"))return ++index<values.length;if(n.equals("getString"))return values[index][a[0] instanceof Integer?((Integer)a[0])-1:0];if(n.equals("close"))return null;throw new AssertionError("意外结果集操作");}});}
    static final class Fixture {
        String[][] grants={{"GRANT INSERT ON *.* TO synthetic_identity"}},columns={{"int",""}},indexes={};
        String version="4.3.5.2";boolean empty=true,tableVisible=true,metadataDenied=false,viewsDenied=false;
        String[][] proxyInfo={{"binary info","ObProxy-OceanBase 4.3.5-synthetic-build\nversion:RELEASE"}};boolean proxyDenied=false;
        Connection connection(){return(Connection)proxy(Connection.class,new InvocationHandler(){public Object invoke(Object p,Method m,Object[] a)throws Exception{String n=m.getName();if(n.equals("setCatalog")||n.equals("close"))return null;if(n.equals("createStatement"))return statement(null);if(n.equals("prepareStatement"))return statement((String)a[0]);if(n.equals("getMetaData"))return proxy(DatabaseMetaData.class,new InvocationHandler(){public Object invoke(Object p,Method m,Object[] a){if(m.getName().equals("getSchemas"))return rows(new String[][]{{"synthetic_db"}});throw new AssertionError("意外元信息操作");}});throw new AssertionError("意外连接操作或试写");}});}
        Object statement(final String prepared){return proxy(prepared==null?Statement.class:PreparedStatement.class,new InvocationHandler(){public Object invoke(Object p,Method m,Object[] a)throws Exception {String n=m.getName();if(n.equals("close")||n.equals("setQueryTimeout"))return null;if(n.equals("setString")){require(((Integer)a[0])==1?"synthetic_db".equals(a[1]):"synthetic_table".equals(a[1]),"查询未绑定冻结标识符");return null;}if(!n.equals("executeQuery"))throw new AssertionError("意外写入或语句操作");String sql=prepared==null?(String)a[0]:prepared;
            if(sql.equals("SELECT OB_VERSION() FROM DUAL"))return rows(new String[][]{{version}});
            if(sql.equals(DirectLoadProbe.ODP_SQL)){if(proxyDenied)throw new SQLException("synthetic");return rows(proxyInfo);}
            if(sql.equals("SHOW GRANTS"))return rows(grants);
            if(sql.matches("SELECT 1 FROM oceanbase\\.(GV\\$OB_MEMSTORE|DBA_OB_MAJOR_COMPACTION|DBA_OB_LS_LOCATIONS|__all_virtual_proxy_schema) WHERE 1=0")){if(viewsDenied)throw new SQLException("synthetic");return rows(new String[][]{});}
            if(sql.equals(DirectLoadProbe.TABLE_SQL)){if(metadataDenied)throw new SQLException("synthetic");return rows(tableVisible?new String[][]{{"BASE TABLE"}}:new String[][]{});}
            if(sql.equals(DirectLoadProbe.COLUMN_SQL))return rows(columns);if(sql.equals(DirectLoadProbe.INDEX_SQL))return rows(indexes);
            if(sql.equals("SELECT 1 FROM `synthetic_db`.`synthetic_table` LIMIT 1"))return rows(empty?new String[][]{}:new String[][]{{"1"}});
            throw new AssertionError("固定查询集合之外的 SQL");
        }});}
    }
}
