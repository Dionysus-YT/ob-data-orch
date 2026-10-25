package com.obdataorch.jdbcprobe;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.Arrays;
import java.util.HashSet;
import java.util.Set;

/** Oracle 旁路固定只读事实；仅确认当前用户所属 Schema，不展开角色或跨 Schema 授权。 */
final class DirectLoadOracleMetadata {
    static final String OWNER_SQL="SELECT SYS_CONTEXT('USERENV', 'CURRENT_USER') FROM DUAL";
    static final String TABLE_SQL="SELECT TABLE_NAME FROM ALL_TABLES WHERE OWNER=? AND TABLE_NAME=?";
    static final String COLUMN_SQL="SELECT DATA_TYPE,VIRTUAL_COLUMN,HIDDEN_COLUMN FROM ALL_TAB_COLS WHERE OWNER=? AND TABLE_NAME=? ORDER BY COLUMN_ID";
    static final String INDEX_SQL="SELECT INDEX_NAME,UNIQUENESS,STATUS FROM ALL_INDEXES WHERE TABLE_OWNER=? AND TABLE_NAME=?";
    private static final Set<String> TYPES=new HashSet<String>(Arrays.asList("NUMBER","FLOAT","BINARY_FLOAT","BINARY_DOUBLE","CHAR","VARCHAR2","NCHAR","NVARCHAR2","DATE","CLOB","NCLOB","BLOB","RAW","LONG RAW"));
    private DirectLoadOracleMetadata() {}

    static void verify(Connection connection,DirectLoadProbe.Input input,DirectLoadProbe.Observation observed) {
        // 所属 Schema 的既有表操作源于对象所有权；未知身份与跨 Schema 不借用 ANY/角色权限。
        try(Statement statement=connection.createStatement()) {
            statement.setQueryTimeout(5);
            try(ResultSet rows=statement.executeQuery(OWNER_SQL)) {
                if(!rows.next()||!input.database.equals(rows.getString(1))||rows.next())return;
            }
        }catch(SQLException|RuntimeException exception){return;}
        observed.write=requiredViews(connection);
        table(connection,input,observed);
    }

    private static String requiredViews(Connection connection) {
        // 4.3.5 官方 Oracle 权限清单使用 SYS.all_virtual_proxy_schema，不能套用 MySQL 的双下划线名称。
        String[] views={"GV$OB_MEMSTORE","DBA_OB_MAJOR_COMPACTION","DBA_OB_LS_LOCATIONS","all_virtual_proxy_schema"};
        try(Statement statement=connection.createStatement()) {
            statement.setQueryTimeout(5);
            for(String view:views)try(ResultSet rows=statement.executeQuery("SELECT 1 FROM SYS."+view+" WHERE 1=0")){if(rows.next())return "UNAVAILABLE";}
            return "ACCESSIBLE";
        }catch(SQLException|RuntimeException exception){return "UNAVAILABLE";}
    }

    private static void table(Connection connection,DirectLoadProbe.Input input,DirectLoadProbe.Observation observed) {
        try(PreparedStatement statement=scoped(connection,TABLE_SQL,input);ResultSet rows=statement.executeQuery()) {
            // 字典缺行或多行都不能证明目标存在，也不能据此假设空表。
            if(!rows.next()||!input.table.equals(rows.getString(1))||rows.next())return;
            observed.exists=Boolean.TRUE;
        }catch(SQLException|RuntimeException exception){return;}
        try(Statement statement=connection.createStatement()) {
            statement.setQueryTimeout(5);
            try(ResultSet rows=statement.executeQuery("SELECT 1 FROM \""+input.database+"\".\""+input.table+"\" WHERE ROWNUM <= 1")){observed.empty=Boolean.valueOf(!rows.next());}
        }catch(SQLException|RuntimeException exception){observed.empty=null;}
        try(PreparedStatement statement=scoped(connection,COLUMN_SQL,input);ResultSet rows=statement.executeQuery()) {
            boolean binary=false,virtual=false,lob=false;int count=0;
            while(rows.next()) {
                if(++count>4096)return;
                String type=rows.getString(1),generated=rows.getString(2),hidden=rows.getString(3);
                if(type==null||type.length()>128||!("YES".equals(generated)||"NO".equals(generated))||!"NO".equals(hidden))return;
                // 隐藏列、对象类型和未知字典值保持未知；不把未识别的类型当成安全结构。
                if(!TYPES.contains(type)&&!type.matches("TIMESTAMP(?:\\([0-9]\\))?(?: WITH (?:LOCAL )?TIME ZONE)?"))return;
                binary|="RAW".equals(type)||"LONG RAW".equals(type)||"BLOB".equals(type);
                virtual|="YES".equals(generated);lob|="CLOB".equals(type)||"NCLOB".equals(type)||"BLOB".equals(type);
            }
            if(count>0){observed.bit=Boolean.FALSE;observed.binary=Boolean.valueOf(binary);observed.virtual=Boolean.valueOf(virtual);observed.lob=Boolean.valueOf(lob);}
        }catch(SQLException|RuntimeException exception){observed.bit=null;observed.binary=null;observed.virtual=null;observed.lob=null;}
        try(PreparedStatement statement=scoped(connection,INDEX_SQL,input);ResultSet rows=statement.executeQuery()) {
            boolean index=false,unique=false;int count=0;
            while(rows.next()) {
                if(++count>4096)return;String name=rows.getString(1),uniqueness=rows.getString(2),status=rows.getString(3);
                if(name==null||name.isEmpty()||name.length()>128||!("UNIQUE".equals(uniqueness)||"NONUNIQUE".equals(uniqueness))||!"VALID".equals(status))return;
                index=true;unique|="UNIQUE".equals(uniqueness);
            }
            observed.index=Boolean.valueOf(index);observed.unique=Boolean.valueOf(unique);
        }catch(SQLException|RuntimeException exception){observed.index=null;observed.unique=null;}
    }

    private static PreparedStatement scoped(Connection connection,String sql,DirectLoadProbe.Input input)throws SQLException {
        PreparedStatement statement=connection.prepareStatement(sql);
        try{statement.setQueryTimeout(5);statement.setString(1,input.database);statement.setString(2,input.table);return statement;}
        catch(SQLException|RuntimeException exception){statement.close();throw exception;}
    }
}
