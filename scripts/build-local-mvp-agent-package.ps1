[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$ControlPlaneCAFile,
    # 默认输出与 Agent 实际部署目录保持一致，避免 var 下并存多份 agent mvp 副本。
    [string]$OutputDirectory = (Join-Path (Split-Path -Parent $PSScriptRoot) 'var\ob-data-orch-agent-windows-amd64')
)

$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path -Parent $PSScriptRoot
if (-not [System.IO.Path]::IsPathFullyQualified($ControlPlaneCAFile) -or -not (Test-Path -LiteralPath $ControlPlaneCAFile -PathType Leaf)) {
    throw 'ControlPlaneCAFile 必须是现有的绝对常规文件路径。'
}
if (-not [System.IO.Path]::IsPathFullyQualified($OutputDirectory)) {
    throw 'OutputDirectory 必须是绝对路径。'
}

New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
$agentExecutable = Join-Path $OutputDirectory 'agent.exe'
$caTarget = Join-Path $OutputDirectory 'control-plane-ca.pem'
$configPath = Join-Path $OutputDirectory 'agent-config.json'
$registerLauncherPath = Join-Path $OutputDirectory '首次注册并启动Agent.cmd'
$startLauncherPath = Join-Path $OutputDirectory '启动Agent.cmd'
$webAssetDirectory = Join-Path $repositoryRoot 'var\local-mvp-web-assets'
$archivePath = Join-Path $webAssetDirectory 'ob-data-orch-agent-windows-amd64.zip'
$stagingDirectory = Join-Path $webAssetDirectory 'agent-package-staging'
$stagingAgentExecutable = Join-Path $stagingDirectory 'agent.exe'
$stagingCAPath = Join-Path $stagingDirectory 'control-plane-ca.pem'
$stagingConfigPath = Join-Path $stagingDirectory 'agent-config.json'
$stagingRegisterLauncherPath = Join-Path $stagingDirectory '首次注册并启动Agent.cmd'
$stagingStartLauncherPath = Join-Path $stagingDirectory '启动Agent.cmd'
New-Item -ItemType Directory -Path $stagingDirectory -Force | Out-Null
# 本机二进制变更流程（AGENTS.md 6.1）：先删除旧 staging 二进制，再构建新的，避免覆盖失败或残留旧副本。
Remove-Item -LiteralPath $stagingAgentExecutable -Force -ErrorAction SilentlyContinue
Push-Location $repositoryRoot
try {
    go build -trimpath -o $stagingAgentExecutable ./cmd/agent
} finally {
    Pop-Location
}
Copy-Item -LiteralPath $ControlPlaneCAFile -Destination $stagingCAPath -Force
$config = [ordered]@{
    formatVersion = 'agent-bundle-config-v1'
    controlPlaneUrl = 'https://127.0.0.1:8080'
    controlPlaneCaFile = 'control-plane-ca.pem'
    stateDirectory = 'agent-security'
}
[System.IO.File]::WriteAllText($stagingConfigPath, ($config | ConvertTo-Json -Compress), [System.Text.UTF8Encoding]::new($false))
[System.IO.File]::WriteAllText($stagingRegisterLauncherPath, "@echo off`r`ncd /d `"%~dp0`"`r`nset `"OB_DATA_ORCH_ENABLE_AGENT_JDBC_CONNECTION_TEST=true`"`r`nset `"OB_DATA_ORCH_ENABLE_AGENT_EXPORT_PREFLIGHT=true`"`r`nset `"OB_DATA_ORCH_ENABLE_REAL_EXECUTION=true`"`r`nset `"OB_DATA_ORCH_ENABLE_AGENT_STORAGE_CONNECTIVITY_PROBE=true`"`r`nagent.exe -register`r`n", [System.Text.UTF8Encoding]::new($false))
[System.IO.File]::WriteAllText($stagingStartLauncherPath, "@echo off`r`ncd /d `"%~dp0`"`r`nset `"OB_DATA_ORCH_ENABLE_AGENT_JDBC_CONNECTION_TEST=true`"`r`nset `"OB_DATA_ORCH_ENABLE_AGENT_EXPORT_PREFLIGHT=true`"`r`nset `"OB_DATA_ORCH_ENABLE_REAL_EXECUTION=true`"`r`nset `"OB_DATA_ORCH_ENABLE_AGENT_STORAGE_CONNECTIVITY_PROBE=true`"`r`nagent.exe`r`n", [System.Text.UTF8Encoding]::new($false))
New-Item -ItemType Directory -Path $webAssetDirectory -Force | Out-Null
Compress-Archive -LiteralPath $stagingAgentExecutable, $stagingCAPath, $stagingConfigPath, $stagingRegisterLauncherPath, $stagingStartLauncherPath -DestinationPath $archivePath -Force

$runningCurrentAgent = Get-Process -Name 'agent' -ErrorAction SilentlyContinue | Where-Object {
    try { [string]::Equals($_.Path, $agentExecutable, [System.StringComparison]::OrdinalIgnoreCase) } catch { $false }
} | Select-Object -First 1
if ($null -ne $runningCurrentAgent) {
    # 运行中的旧 Agent 会锁定二进制；按本机二进制变更流程停旧 → 删旧 → 安装新 → 按原开关重启。
    if (-not (Test-Path -LiteralPath $caTarget -PathType Leaf) -or
        (Get-FileHash -Algorithm SHA256 -LiteralPath $caTarget).Hash -ne (Get-FileHash -Algorithm SHA256 -LiteralPath $ControlPlaneCAFile).Hash) {
        throw '当前本机 Agent 正在运行且信任的 CA 与控制面不一致。请先停止该 Agent，再重新运行启动脚本以更新受控 Agent 包；不要删除 agent-security 身份目录。'
    }
    Stop-Process -Id $runningCurrentAgent.Id -ErrorAction Stop
    # 等待旧进程完全退出并释放文件句柄；杀进程后的短暂窗口内文件仍可能被占用，删除做短重试。
    Wait-Process -Id $runningCurrentAgent.Id -Timeout 15 -ErrorAction SilentlyContinue
    $agentRemoved = $false
    for ($attempt = 0; $attempt -lt 40; $attempt++) {
        Remove-Item -LiteralPath $agentExecutable -Force -ErrorAction SilentlyContinue
        if (-not (Test-Path -LiteralPath $agentExecutable -PathType Leaf)) {
            $agentRemoved = $true
            break
        }
        Start-Sleep -Milliseconds 500
    }
    if (-not $agentRemoved) {
        throw '旧 Agent 二进制仍被占用，无法删除。请确认常驻 Agent 已完全退出且无其他进程占用后重试。'
    }
    Copy-Item -LiteralPath $stagingAgentExecutable, $stagingCAPath, $stagingConfigPath, $stagingRegisterLauncherPath, $stagingStartLauncherPath -Destination $OutputDirectory -Force
    # 按打包启动脚本的原开关重启常驻 Agent（身份目录 agent-security 原样保留，不删除、不改写）。
    $env:OB_DATA_ORCH_ENABLE_AGENT_JDBC_CONNECTION_TEST = 'true'
    $env:OB_DATA_ORCH_ENABLE_AGENT_EXPORT_PREFLIGHT = 'true'
    $env:OB_DATA_ORCH_ENABLE_REAL_EXECUTION = 'true'
    $env:OB_DATA_ORCH_ENABLE_AGENT_STORAGE_CONNECTIVITY_PROBE = 'true'
    Start-Process -FilePath $agentExecutable -WorkingDirectory $OutputDirectory -WindowStyle Hidden | Out-Null
    Start-Sleep -Seconds 2
    $restartedAgent = Get-Process -Name 'agent' -ErrorAction SilentlyContinue | Where-Object {
        try { [string]::Equals($_.Path, $agentExecutable, [System.StringComparison]::OrdinalIgnoreCase) } catch { $false }
    } | Select-Object -First 1
    if ($null -eq $restartedAgent) {
        throw '新 Agent 二进制安装后未能启动。请检查 ' + $OutputDirectory + ' 下的 agent.stdout.log/agent.stderr.log。'
    }
    Write-Output "已按本机二进制变更流程更新并重启常驻 Agent（PID $($restartedAgent.Id)，最新构建）。"
} else {
    # 无运行中的常驻 Agent：先删除旧二进制再安装新包，不覆盖失败、不残留旧副本。
    Remove-Item -LiteralPath $agentExecutable -Force -ErrorAction SilentlyContinue
    Copy-Item -LiteralPath $stagingAgentExecutable, $stagingCAPath, $stagingConfigPath, $stagingRegisterLauncherPath, $stagingStartLauncherPath -Destination $OutputDirectory -Force
}

Write-Output "已生成 Windows 本机 Agent 包：$OutputDirectory"
Write-Output "已生成页面下载文件：$archivePath"
Write-Output '首次使用双击“首次注册并启动Agent.cmd”，粘贴页面注册码后保持窗口运行；后续双击“启动Agent.cmd”。两个脚本都会启用固定 JDBC 连接测试、六项导出预检查和受控真实执行；OBDUMPER 仅会在页面预检查通过并显式提交任务后启动。'
