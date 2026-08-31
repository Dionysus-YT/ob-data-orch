#!/usr/bin/env sh
set -eu

# S0/S1 验证强制关闭所有真实连接、凭据探测、存储探测和工具执行开关。
OB_DATA_ORCH_ENABLE_REAL_EXECUTION=false \
OB_DATA_ORCH_ENABLE_AGENT_JDBC_CONNECTION_TEST=false \
OB_DATA_ORCH_ENABLE_AGENT_EXPORT_PREFLIGHT=false \
OB_DATA_ORCH_ENABLE_AGENT_STORAGE_CONNECTIVITY_PROBE=false \
go test -count=1 \
  ./internal/exportdomain \
  ./internal/agentexec \
  ./internal/agentexecution \
  ./internal/agentwire \
  ./internal/controlplane \
  ./internal/integration \
  ./internal/localmvp

printf '%s\n' 'S0/S1 Export 合成验证通过：未使用真实网络、凭据、用户数据库或 OBDUMPER。'
