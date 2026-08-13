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
[System.IO.File]::WriteAllText($stagingRegisterLauncherPath, "@echo off`r`ncd /d `"%~dp0`"`r`nset `"OB_DATA_ORCH_ENABLE_AGENT_JDBC_CONNECTION_TEST=true`"`r`nset `"OB_DATA_ORCH_ENABLE_AGENT_EXPORT_PREFLIGHT=true`"`r`nset `"OB_DATA_ORCH_ENABLE_REAL_EXECUTION=true`"`r`nagent.exe -register`r`n", [System.Text.UTF8Encoding]::new($false))
[System.IO.File]::WriteAllText($stagingStartLauncherPath, "@echo off`r`ncd /d `"%~dp0`"`r`nset `"OB_DATA_ORCH_ENABLE_AGENT_JDBC_CONNECTION_TEST=true`"`r`nset `"OB_DATA_ORCH_ENABLE_AGENT_EXPORT_PREFLIGHT=true`"`r`nset `"OB_DATA_ORCH_ENABLE_REAL_EXECUTION=true`"`r`nagent.exe`r`n", [System.Text.UTF8Encoding]::new($false))
New-Item -ItemType Directory -Path $webAssetDirectory -Force | Out-Null
Compress-Archive -LiteralPath $stagingAgentExecutable, $stagingCAPath, $stagingConfigPath, $stagingRegisterLauncherPath, $stagingStartLauncherPath -DestinationPath $archivePath -Force

$runningCurrentAgent = Get-Process -Name 'agent' -ErrorAction SilentlyContinue | Where-Object {
    try { [string]::Equals($_.Path, $agentExecutable, [System.StringComparison]::OrdinalIgnoreCase) } catch { $false }
} | Select-Object -First 1
if ($null -eq $runningCurrentAgent) {
    Copy-Item -LiteralPath $stagingAgentExecutable, $stagingCAPath, $stagingConfigPath, $stagingRegisterLauncherPath, $stagingStartLauncherPath -Destination $OutputDirectory -Force
} else {
    Write-Warning '当前本机 Agent 正在运行，未覆盖其目录；请停止旧 Agent 后从页面下载并解压最新 ZIP，再启动 Agent。'
}

Write-Output "已生成 Windows 本机 Agent 包：$OutputDirectory"
Write-Output "已生成页面下载文件：$archivePath"
Write-Output '首次使用双击“首次注册并启动Agent.cmd”，粘贴页面注册码后保持窗口运行；后续双击“启动Agent.cmd”。两个脚本都会启用固定 JDBC 连接测试、六项导出预检查和受控真实执行；OBDUMPER 仅会在页面预检查通过并显式提交任务后启动。'
