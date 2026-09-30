param([switch]$Agent, [switch]$RealExecution)
$ErrorActionPreference = 'Stop'
# 开发进程留在当前终端，退出时由监视器正常关闭，不安装系统服务。
$runner = Join-Path $PSScriptRoot 'dev.mjs'
$runnerArguments = @($(if ($Agent) { 'agent' } else { 'control-plane' }))
if ($RealExecution) { $runnerArguments += '--real-execution' }
node $runner @runnerArguments
if ($LASTEXITCODE -ne 0) { throw '开发进程已退出，请检查上方错误。' }
