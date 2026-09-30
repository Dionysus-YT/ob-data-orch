$ErrorActionPreference = 'Stop'
$OutputEncoding = [Text.UTF8Encoding]::new($false)
$repository = Split-Path -Parent $PSScriptRoot
$executable = Join-Path $repository 'var/dev/control-plane/control-plane.exe'
$secret = Read-Host '设置开发环境 admin 密码（至少 8 个字符）' -AsSecureString
$pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secret)
try {
    $plain = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)
    "https://127.0.0.1:18443`n$plain" | & $executable -initialize
    if ($LASTEXITCODE -ne 0) { throw '开发环境初始化失败。' }
} finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)
    $plain = $null
    $secret.Dispose()
}
