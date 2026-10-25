param(
    [Parameter(Mandatory = $true)][ValidatePattern('^artifacts[\\/]direct-load-e2e-[0-9]{8}-[a-f0-9]{12}$')][string]$RunDirectory,
    [switch]$Standalone,
    [switch]$Oracle,
    [switch]$Smoke
)
$ErrorActionPreference = 'Stop'
if ($Smoke -and -not ($Standalone -or $Oracle)) { throw 'Smoke 仅适用于 Standalone 或 Oracle 真实回归。' }
if ($Oracle -and $Standalone) { throw 'Oracle 与 Standalone 必须分批运行。' }
$workspace = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$nodePath = (Get-Command node -ErrorAction Stop).Source
$secret = Read-Host '请输入现有 admin 密码（仅用于本机旁路 API 测试，不回显）' -AsSecureString
$pointer = [IntPtr]::Zero
$characters = $null
$passwordBytes = $null
$process = $null
try {
    $pointer = [Runtime.InteropServices.Marshal]::SecureStringToGlobalAllocUnicode($secret)
    $characters = New-Object char[] $secret.Length
    for ($index = 0; $index -lt $characters.Length; $index++) { $characters[$index] = [char][Runtime.InteropServices.Marshal]::ReadInt16($pointer, $index * 2) }
    $passwordBytes = [System.Text.Encoding]::UTF8.GetBytes($characters)
    $start = [System.Diagnostics.ProcessStartInfo]::new()
    $start.FileName = $nodePath
    $start.WorkingDirectory = $workspace
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardInput = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    $runnerName = if ($Oracle) { 'oracle-execution-api.mjs' } elseif ($Standalone) { 'standalone-api.mjs' } else { 'api-runner.mjs' }
    $start.ArgumentList.Add((Join-Path $PSScriptRoot $runnerName))
    $start.ArgumentList.Add($RunDirectory)
    if ($Smoke) { $start.ArgumentList.Add('--smoke') }
    $process = [System.Diagnostics.Process]::Start($start)
    $outputTask = $process.StandardOutput.ReadToEndAsync()
    $errorTask = $process.StandardError.ReadToEndAsync()
    # 密码仅通过匿名 stdin 传递，发送后立即清零；不进入参数、文件、环境或输出。
    $process.StandardInput.BaseStream.Write($passwordBytes, 0, $passwordBytes.Length)
    $process.StandardInput.Close()
    [Array]::Clear($passwordBytes, 0, $passwordBytes.Length)
    [Array]::Clear($characters, 0, $characters.Length)
    [Runtime.InteropServices.Marshal]::ZeroFreeGlobalAllocUnicode($pointer)
    $pointer = [IntPtr]::Zero
    $secret.Dispose()
    $secret = $null
    Write-Host '正在运行 API 测试；结果写入本轮 artifacts 回执。'
    $process.WaitForExit()
    $safeOutput = $outputTask.GetAwaiter().GetResult()
    $safeErrors = $errorTask.GetAwaiter().GetResult()
    if ($safeOutput) { Write-Output $safeOutput }
    if ($safeErrors) { Write-Output $safeErrors }
    if ($process.ExitCode -ne 0) { throw 'API 测试未完成，请核对本轮回执；不要重放该批次。' }
} finally {
    if ($passwordBytes) { [Array]::Clear($passwordBytes, 0, $passwordBytes.Length) }
    if ($characters) { [Array]::Clear($characters, 0, $characters.Length) }
    if ($pointer -ne [IntPtr]::Zero) { [Runtime.InteropServices.Marshal]::ZeroFreeGlobalAllocUnicode($pointer) }
    if ($secret) { $secret.Dispose() }
    if ($process) { $process.Dispose() }
}
