package com.obdataorch.jdbcprobe;

import java.sql.*;
import java.util.*;
import java.util.regex.*;

/** 非表对象的固定只读权限与依赖检查；不接收或执行定义 SQL。 */
final class ObjectImportPrivileges {
    private static final Set<String> KINDS=new HashSet<String>(Arrays.asList("table-group","view","trigger","obj-user","role","sequence","synonym","type","type-body","package","package-body","function","procedure"));
    private static final Pattern GRANT=Pattern.compile("(?i)^GRANT\\s+([A-Z_ ,]+)\\s+ON\\s+(`(?:``|[^`])*`|\\*|[A-Za-z0-9_$]+)\\.(`(?:``|[^`])*`|\\*|[A-Za-z0-9_$]+)\\s+TO\\s+.+$");
    private ObjectImportPrivileges() {}
    static boolean validKind(String kind){return KINDS.contains(kind);}
    private static NormalImportProbe.Check pass(){return new NormalImportProbe.Check("ACCESSIBLE","VERIFIED");}
    private static NormalImportProbe.Check unknown(){return new NormalImportProbe.Check("UNAVAILABLE","DDL_UNSUPPORTED");}
    static NormalImportProbe.Check verify(Connection c,NormalImportProbe.Input input) {
        try {return "MYSQL".equals(input.mode)?mysql(c,input):oracle(c,input);}catch(SQLException|RuntimeException e){return unknown();}
    }
    private static final class Grant {
        String database,target;Set<String> privileges=new HashSet<String>();boolean all;
        boolean allows(String privilege){return all||privileges.contains(privilege);}
    }
    private static String unquote(String text){return text.startsWith("`")?text.substring(1,text.length()-1).replace("``","`"):text;}
    private static NormalImportProbe.Check mysql(Connection c,NormalImportProbe.Input input)throws SQLException {
        List<Grant> grants=new ArrayList<Grant>();
        try(Statement s=c.createStatement()){s.setQueryTimeout(5);try(ResultSet r=s.executeQuery("SHOW GRANTS")){int count=0;while(r.next()){
            if(++count>4096)return unknown();String text=r.getString(1);if(text==null||text.length()>16384)return unknown();Matcher match=GRANT.matcher(text);
            if(!match.matches()){if(!text.trim().matches("(?is)^GRANT\\s+.*"))return unknown();continue;}
            Grant grant=new Grant();grant.database=unquote(match.group(2));grant.target=unquote(match.group(3));
            for(String privilege:match.group(1).toUpperCase(Locale.ROOT).split(",")){privilege=privilege.trim();grant.all|="ALL PRIVILEGES".equals(privilege)||"ALL".equals(privilege);grant.privileges.add(privilege);}grants.add(grant);
        }}}
        for(NormalImportProbe.Definition object:input.objects){
            String required;boolean global=false;String target=object.name;
            if("table-group".equals(object.kind)){required="CREATE";global=true;}
            else if("obj-user".equals(object.kind)){required="CREATE USER";global=true;}
            else if("view".equals(object.kind))required="CREATE VIEW";
            else if("trigger".equals(object.kind)){required="TRIGGER";target=object.reference;}
            else if("function".equals(object.kind)||"procedure".equals(object.kind))required="CREATE ROUTINE";
            else return unknown();
            boolean allowed=false;
            for(Grant grant:grants){boolean root="*".equals(grant.database)&&"*".equals(grant.target);boolean scope=root||!global&&input.database.equals(grant.database)&&("*".equals(grant.target)||target.equals(grant.target));allowed|=scope&&grant.allows(required);}
            if(!allowed)return unknown();
            if(!object.reference.isEmpty()){
                try(PreparedStatement s=c.prepareStatement("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=? AND TABLE_NAME=?")){s.setQueryTimeout(5);s.setString(1,input.database);s.setString(2,object.reference);try(ResultSet r=s.executeQuery()){if(!r.next()||r.getInt(1)!=1||r.next())return unknown();}}
            }
        }
        return pass();
    }
    /** 新建连接没有 SET ROLE；只展开登录默认启用角色的直接系统权限，不推断非默认或嵌套角色。 */
    static Set<String> oraclePrivileges(Connection c)throws SQLException {
        Set<String> result=new HashSet<String>();
        try(Statement s=c.createStatement()){s.setQueryTimeout(5);try(ResultSet r=s.executeQuery("SELECT PRIVILEGE FROM USER_SYS_PRIVS")){int count=0;while(r.next()){if(++count>4096)throw new SQLException();String value=r.getString(1);if(value==null||value.length()>128)throw new SQLException();result.add(value);}}}
        Set<String> defaults=new HashSet<String>();
        try {
            try(Statement s=c.createStatement()){s.setQueryTimeout(5);try(ResultSet r=s.executeQuery("SELECT GRANTED_ROLE,DEFAULT_ROLE FROM USER_ROLE_PRIVS")){int count=0;while(r.next()){if(++count>256)throw new SQLException();String role=r.getString(1),enabled=r.getString(2);if(role==null||role.length()>256||!("YES".equals(enabled)||"NO".equals(enabled)))throw new SQLException();if("YES".equals(enabled))defaults.add(role);}}}
            if(!defaults.isEmpty())try(Statement s=c.createStatement()){s.setQueryTimeout(5);try(ResultSet r=s.executeQuery("SELECT ROLE,PRIVILEGE FROM ROLE_SYS_PRIVS")){int count=0;Set<String> observed=new HashSet<String>();while(r.next()){if(++count>4096)throw new SQLException();String role=r.getString(1),privilege=r.getString(2);if(role==null||privilege==null||privilege.length()>128)throw new SQLException();if(defaults.contains(role))observed.add(privilege);}result.addAll(observed);}}
        } catch(SQLException e){/* 角色事实不完整时仅保留此前已证明的直接权限，不能部分放行。 */}
        return result;
    }
    private static NormalImportProbe.Check oracle(Connection c,NormalImportProbe.Input input)throws SQLException {
        String current;
        try(Statement s=c.createStatement()){s.setQueryTimeout(5);try(ResultSet r=s.executeQuery("SELECT SYS_CONTEXT('USERENV', 'CURRENT_USER') FROM DUAL")){if(!r.next())return unknown();current=r.getString(1);if(current==null||r.next())return unknown();}}
        // 非表声明首版只支持所属 Schema；跨 Schema 的对象/角色组合仍失败关闭。
        if(!input.database.equals(current))return unknown();
        Set<String> privileges=oraclePrivileges(c);
        for(NormalImportProbe.Definition object:input.objects){
            String required;
            if("role".equals(object.kind))required="CREATE ROLE";
            else if("sequence".equals(object.kind))required="CREATE SEQUENCE";
            else if("synonym".equals(object.kind))required="CREATE SYNONYM";
            else if("type".equals(object.kind)||"type-body".equals(object.kind))required="CREATE TYPE";
            else if("package".equals(object.kind)||"package-body".equals(object.kind)||"function".equals(object.kind)||"procedure".equals(object.kind))required="CREATE PROCEDURE";
            else if("view".equals(object.kind))required="CREATE VIEW";
            else if("trigger".equals(object.kind))required="CREATE TRIGGER";
            else return unknown();
            if(!privileges.contains(required))return unknown();
            if(!object.reference.isEmpty()){
                String type="type-body".equals(object.kind)?"TYPE":"package-body".equals(object.kind)?"PACKAGE":"TABLE";
                try(PreparedStatement s=c.prepareStatement("SELECT COUNT(*) FROM ALL_OBJECTS WHERE OWNER=? AND OBJECT_NAME=? AND OBJECT_TYPE=? AND STATUS='VALID'")){s.setQueryTimeout(5);s.setString(1,input.database);s.setString(2,object.reference.toUpperCase(Locale.ROOT));s.setString(3,type);try(ResultSet r=s.executeQuery()){if(!r.next()||r.getInt(1)!=1||r.next())return unknown();}}
            }
        }
        return pass();
    }
}
