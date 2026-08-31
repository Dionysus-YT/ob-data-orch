$ErrorActionPreference = 'Stop'

# S0/S1 验证强制关闭所有真实连接、凭据探测、存储探测和工具执行开关。
$realExecutionVariables = @(
    'OB_DATA_ORCH_ENABLE_REAL_EXECUTION',
    'OB_DATA_ORCH_ENABLE_AGENT_JDBC_CONNECTION_TEST',
    'OB_DATA_ORCH_ENABLE_AGENT_EXPORT_PREFLIGHT',
    'OB_DATA_ORCH_ENABLE_AGENT_STORAGE_CONNECTIVITY_PROBE'
)
$originalValues = @{}
foreach ($name in $realExecutionVariables) {
    $originalValues[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
    [Environment]::SetEnvironmentVariable($name, 'false', 'Process')
}

try {
    # 这些包只使用临时 SQLite、合成身份、假 Agent、假工具和临时工作区。
    $packages = @(
        './internal/exportdomain',
        './internal/agentexec',
        './internal/agentexecution',
        './internal/agentwire',
        './internal/controlplane',
        './internal/integration',
        './internal/localmvp'
    )
    go test -count=1 $packages
    if ($LASTEXITCODE -ne 0) {
        throw "S0/S1 Export 合成验证失败，退出码 $LASTEXITCODE"
    }
    Write-Host 'S0/S1 Export 合成验证通过：未使用真实网络、凭据、用户数据库或 OBDUMPER。'
}
finally {
    foreach ($name in $realExecutionVariables) {
        $value = $originalValues[$name]
        if ($null -eq $value) {
            Remove-Item -LiteralPath "Env:$name" -ErrorAction SilentlyContinue
        } else {
            [Environment]::SetEnvironmentVariable($name, $value, 'Process')
        }
    }
}
