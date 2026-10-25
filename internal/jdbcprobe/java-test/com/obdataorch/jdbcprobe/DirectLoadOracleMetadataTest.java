package com.obdataorch.jdbcprobe;

import java.lang.reflect.*;
import java.nio.charset.StandardCharsets;
import java.sql.*;
import java.util.*;

/** Oracle 固定查询合成测试；任何未列明 SQL、试写、提交或未绑定目标均立即失败。 */
final class DirectLoadOracleMetadataTest {
    private DirectLoadOracleMetadataTest() {}
    static void run() {
        Fixture fixture=new Fixture();DirectLoadProbe.Input input=input();DirectLoadProbe.Observation observed=DirectLoadProbe.verify(fixture.connection(),input);
        require("ACCESSIBLE".equals(observed.database)&&"ACCESSIBLE".equals(observed.write)&&"4.4.2.3".equals(observed.observer)&&Boolean.TRUE.equals(observed.exists)&&Boolean.TRUE.equals(observed.empty),"Oracle 所属 Schema 事实不完整");
        require(Boolean.FALSE.equals(observed.bit)&&Boolean.FALSE.equals(observed.binary)&&Boolean.FALSE.equals(observed.virtual)&&Boolean.FALSE.equals(observed.lob)&&Boolean.TRUE.equals(observed.index)&&Boolean.TRUE.equals(observed.unique),"Oracle 普通列与唯一索引事实不完整");
        input.truncate=true;require("ACCESSIBLE".equals(DirectLoadProbe.verify(fixture.connection(),input).write),"所属 Schema 既有表清空被错误依赖系统 ANY 权限");
        fixture.owner="OTHER_SCHEMA";observed=DirectLoadProbe.verify(fixture.connection(),input);require("UNAVAILABLE".equals(observed.write)&&observed.exists==null,"跨 Schema 被借用所有者或角色权限");fixture.owner="SYNTHETIC_SCHEMA";
        fixture.ownerDenied=true;observed=DirectLoadProbe.verify(fixture.connection(),input);require("UNAVAILABLE".equals(observed.write)&&observed.exists==null,"未知当前用户被放行");fixture.ownerDenied=false;
        fixture.viewsDenied=true;observed=DirectLoadProbe.verify(fixture.connection(),input);require("UNAVAILABLE".equals(observed.write)&&Boolean.TRUE.equals(observed.exists),"系统视图未知被放行或污染独立表事实");fixture.viewsDenied=false;
        fixture.visible=false;observed=DirectLoadProbe.verify(fixture.connection(),input);require(observed.exists==null&&observed.empty==null,"字典缺行被误认成不存在或空表");fixture.visible=true;
        fixture.metadataDenied=true;observed=DirectLoadProbe.verify(fixture.connection(),input);require("ACCESSIBLE".equals(observed.database)&&observed.exists==null,"字典拒绝污染连接或放行结构");fixture.metadataDenied=false;
        fixture.emptyDenied=true;require(DirectLoadProbe.verify(fixture.connection(),input).empty==null,"未知表空性被当成空表");fixture.emptyDenied=false;
        fixture.empty=false;require(Boolean.FALSE.equals(DirectLoadProbe.verify(fixture.connection(),input).empty),"非空 Oracle 表事实丢失");
        for(String type:new String[]{"RAW","LONG RAW","BLOB"}){fixture.columns=new String[][]{{type,"NO","NO"}};require(Boolean.TRUE.equals(DirectLoadProbe.verify(fixture.connection(),input).binary),"Oracle 二进制未保留限制事实");}
        fixture.columns=new String[][]{{"CLOB","NO","NO"}};require(Boolean.TRUE.equals(DirectLoadProbe.verify(fixture.connection(),input).lob),"Oracle LOB 未保留事实");
        fixture.columns=new String[][]{{"NUMBER","YES","NO"}};require(Boolean.TRUE.equals(DirectLoadProbe.verify(fixture.connection(),input).virtual),"Oracle 虚拟列未保留事实");
        for(String[][] columns:new String[][][]{{{"UNKNOWN_OBJECT","NO","NO"}},{{"NUMBER","NO","YES"}},{{"NUMBER","UNKNOWN","NO"}},{{null,"NO","NO"}},new String[][]{}}){fixture.columns=columns;observed=DirectLoadProbe.verify(fixture.connection(),input);require(observed.bit==null&&observed.binary==null&&observed.virtual==null&&observed.lob==null,"未知 Oracle 列被视为安全结构");}
        fixture.columns=new String[4097][3];for(int i=0;i<fixture.columns.length;i++)fixture.columns[i]=new String[]{"NUMBER","NO","NO"};require(DirectLoadProbe.verify(fixture.connection(),input).bit==null,"超预算列被放行");
        fixture.columns=new String[][]{{"TIMESTAMP(6) WITH TIME ZONE","NO","NO"}};require(Boolean.FALSE.equals(DirectLoadProbe.verify(fixture.connection(),input).binary),"固定 Oracle 时间类型未识别");
        for(String[][] indexes:new String[][][]{{{"SYNTHETIC_INDEX","UNKNOWN","VALID"}},{{"SYNTHETIC_INDEX","UNIQUE","UNUSABLE"}}}){fixture.indexes=indexes;observed=DirectLoadProbe.verify(fixture.connection(),input);require(observed.index==null&&observed.unique==null,"未知或不可用索引被视为不存在");}
        fixture.indexes=new String[][]{};observed=DirectLoadProbe.verify(fixture.connection(),input);require(Boolean.FALSE.equals(observed.index)&&Boolean.FALSE.equals(observed.unique),"无索引事实错误");input.destroy();
    }
    private static DirectLoadProbe.Input input(){DirectLoadProbe.Input value=new DirectLoadProbe.Input();value.host="synthetic.invalid";value.port=2881;value.mode="ORACLE";value.database="SYNTHETIC_SCHEMA";value.table="SYNTHETIC_TABLE";value.scenario="OBSERVER_DIRECT";value.username="SYNTHETIC_USER@synthetic".getBytes(StandardCharsets.UTF_8);value.password="synthetic-only".getBytes(StandardCharsets.UTF_8);return value;}
    private static void require(boolean value,String message){if(!value)throw new AssertionError(message);}
    private static Object proxy(Class<?> type,InvocationHandler handler){return Proxy.newProxyInstance(DirectLoadOracleMetadataTest.class.getClassLoader(),new Class<?>[]{type},handler);}
    private static ResultSet rows(final String[][] values){return(ResultSet)proxy(ResultSet.class,new InvocationHandler(){int position=-1;public Object invoke(Object p,Method m,Object[] a){if(m.getName().equals("next"))return ++position<values.length;if(m.getName().equals("getString"))return values[position][a[0] instanceof Integer?(Integer)a[0]-1:0];if(m.getName().equals("close"))return null;throw new AssertionError("意外 Oracle 结果集操作");}});}
    private static final class Fixture {
        String owner="SYNTHETIC_SCHEMA";boolean ownerDenied=false,viewsDenied=false,visible=true,metadataDenied=false,empty=true,emptyDenied=false;
        String[][] columns={{"NUMBER","NO","NO"},{"VARCHAR2","NO","NO"}},indexes={{"SYNTHETIC_PK","UNIQUE","VALID"}};
        Connection connection(){return(Connection)proxy(Connection.class,(p,m,a)->{switch(m.getName()){case "close":return null;case "createStatement":return statement(null);case "prepareStatement":return statement((String)a[0]);case "getMetaData":return proxy(DatabaseMetaData.class,(p2,m2,a2)->{if(m2.getName().equals("getSchemas"))return rows(new String[][]{{"SYNTHETIC_SCHEMA"}});throw new AssertionError("意外 Oracle 元信息操作");});default:throw new AssertionError("意外 Oracle 连接或试写操作");}});}
        Object statement(final String prepared){return proxy(prepared==null?Statement.class:PreparedStatement.class,new InvocationHandler(){Map<Integer,String> parameters=new HashMap<Integer,String>();public Object invoke(Object p,Method m,Object[] a)throws Exception{
            String name=m.getName();if(name.equals("close")||name.equals("setQueryTimeout"))return null;if(name.equals("setString")){parameters.put((Integer)a[0],(String)a[1]);return null;}if(!name.equals("executeQuery"))throw new AssertionError("Oracle 探针尝试执行非查询");
            String sql=prepared==null?(String)a[0]:prepared;
            if(prepared!=null)require("SYNTHETIC_SCHEMA".equals(parameters.get(1))&&"SYNTHETIC_TABLE".equals(parameters.get(2))&&parameters.size()==2,"Oracle 查询没有绑定唯一目标");
            if(sql.equals("SELECT OB_VERSION() FROM DUAL"))return rows(new String[][]{{"4.4.2.3"}});
            if(sql.equals(DirectLoadOracleMetadata.OWNER_SQL)){if(ownerDenied)throw new SQLException("synthetic");return rows(new String[][]{{owner}});}
            if(sql.matches("SELECT 1 FROM SYS\\.(GV\\$OB_MEMSTORE|DBA_OB_MAJOR_COMPACTION|DBA_OB_LS_LOCATIONS|all_virtual_proxy_schema) WHERE 1=0")){if(viewsDenied)throw new SQLException("synthetic");return rows(new String[][]{});}
            if(sql.equals(DirectLoadOracleMetadata.TABLE_SQL)){if(metadataDenied)throw new SQLException("synthetic");return rows(visible?new String[][]{{"SYNTHETIC_TABLE"}}:new String[][]{});}
            if(sql.equals(DirectLoadOracleMetadata.COLUMN_SQL))return rows(columns);if(sql.equals(DirectLoadOracleMetadata.INDEX_SQL))return rows(indexes);
            if(sql.equals("SELECT 1 FROM \"SYNTHETIC_SCHEMA\".\"SYNTHETIC_TABLE\" WHERE ROWNUM <= 1")){if(emptyDenied)throw new SQLException("synthetic");return rows(empty?new String[][]{}:new String[][]{{"1"}});}
            throw new AssertionError("固定 Oracle 只读查询集合之外的 SQL");
        }});}
    }
}
