package com.obdataorch.jdbcprobe;

import java.sql.SQLException;
import java.sql.SQLNonTransientConnectionException;
import java.sql.SQLRecoverableException;
import java.sql.SQLTimeoutException;
import java.sql.SQLTransientConnectionException;

public final class ConnectionProbeObjectAccessClassificationTest {
    private ConnectionProbeObjectAccessClassificationTest() {
    }

    public static void main(String[] args) {
        assertClassification(new SQLException("synthetic", "42000", 1142), "NOT_ACCESSIBLE");
        assertClassification(new SQLException("synthetic", "42S02", 1146), "NOT_ACCESSIBLE");
        assertClassification(new SQLException("synthetic"), "NOT_ACCESSIBLE");
        assertClassification(new SQLTransientConnectionException("synthetic"), "UNAVAILABLE");
        assertClassification(new SQLNonTransientConnectionException("synthetic"), "UNAVAILABLE");
        assertClassification(new SQLRecoverableException("synthetic"), "UNAVAILABLE");
        assertClassification(new SQLTimeoutException("synthetic"), "UNAVAILABLE");
        assertClassification(new SQLException("synthetic", "08S01"), "UNAVAILABLE");
        assertClassification(new SQLException("synthetic", "HYT00"), "UNAVAILABLE");

        SQLException chained = new SQLException("synthetic", "42000", 1142);
        chained.setNextException(new SQLNonTransientConnectionException("synthetic"));
        assertClassification(chained, "UNAVAILABLE");
    }

    private static void assertClassification(SQLException exception, String expected) {
        String actual = ConnectionProbe.classifyObjectSQLException(exception);
        if (!expected.equals(actual)) {
            throw new AssertionError("对象访问异常分类错误：期望 " + expected + "，实际 " + actual);
        }
    }
}
