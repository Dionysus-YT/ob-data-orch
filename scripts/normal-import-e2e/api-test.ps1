param([switch]$Gates, [switch]$Repair, [switch]$Closeout, [switch]$Remaining, [switch]$RemainingCloseout, [switch]$Capabilities, [switch]$CapabilitiesCloseout, [switch]$CapabilityNegatives, [switch]$CapabilityRetry)
$ErrorActionPreference = 'Stop'
$workspace = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$nodePath = (Get-Command node -ErrorAction Stop).Source
$secret = Read-Host '请输入现有 admin 密码（仅用于本机 API 测试，不回显）' -AsSecureString
$pointer = [IntPtr]::Zero
$characters = $null
$passwordBytes = $null
$process = $null
try {
    $pointer = [Runtime.InteropServices.Marshal]::SecureStringToGlobalAllocUnicode($secret)
    $characters = New-Object char[] $secret.Length
    for ($index = 0; $index -lt $characters.Length; $index++) {
        $characters[$index] = [char][Runtime.InteropServices.Marshal]::ReadInt16($pointer, $index * 2)
    }
    $passwordBytes = [System.Text.Encoding]::UTF8.GetBytes($characters)
    $start = [System.Diagnostics.ProcessStartInfo]::new()
    $start.FileName = $nodePath
    $start.WorkingDirectory = $workspace
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardInput = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    $start.ArgumentList.Add((Join-Path $PSScriptRoot 'api-runner.mjs'))
    $start.ArgumentList.Add($(if ($Capabilities) { '--capabilities' } elseif ($CapabilityRetry) { '--capabilities-retry' } elseif ($CapabilityNegatives) { '--capabilities-negatives' } elseif ($CapabilitiesCloseout) { '--capabilities-closeout' } elseif ($Gates) { '--gates' } elseif ($Repair) { '--repair' } elseif ($Closeout) { '--closeout' } elseif ($Remaining) { '--remaining' } elseif ($RemainingCloseout) { '--remaining-closeout' } else { '--execute' }))
    $process = [System.Diagnostics.Process]::Start($start)
    # 同时读取两个管道，隐藏子进程也能保留固定进度/错误码，避免缓冲区阻塞。
    $outputTask = $process.StandardOutput.ReadToEndAsync()
    $errorTask = $process.StandardError.ReadToEndAsync()
    # 密码只经匿名标准输入；不进入参数、环境、文件、命令历史或标准输出。
    $process.StandardInput.BaseStream.Write($passwordBytes, 0, $passwordBytes.Length)
    $process.StandardInput.Close()
    [Array]::Clear($passwordBytes, 0, $passwordBytes.Length)
    [Array]::Clear($characters, 0, $characters.Length)
    [Runtime.InteropServices.Marshal]::ZeroFreeGlobalAllocUnicode($pointer)
    $pointer = [IntPtr]::Zero
    $secret.Dispose()
    $secret = $null
    $process.WaitForExit()
    $safeOutput = $outputTask.GetAwaiter().GetResult()
    $safeErrors = $errorTask.GetAwaiter().GetResult()
    if ($safeOutput) { Write-Output $safeOutput }
    if ($safeErrors) { Write-Output $safeErrors }
    if ($process.ExitCode -ne 0) { throw 'API 测试未完成；按固定错误码和本轮回执核对，不重复提交。' }
} finally {
    if ($passwordBytes) { [Array]::Clear($passwordBytes, 0, $passwordBytes.Length) }
    if ($characters) { [Array]::Clear($characters, 0, $characters.Length) }
    if ($pointer -ne [IntPtr]::Zero) { [Runtime.InteropServices.Marshal]::ZeroFreeGlobalAllocUnicode($pointer) }
    if ($secret) { $secret.Dispose() }
    if ($process) { $process.Dispose() }
}
