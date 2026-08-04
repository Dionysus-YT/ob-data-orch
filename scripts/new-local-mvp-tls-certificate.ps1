[CmdletBinding()]
param(
    [string]$OutputDirectory = (Join-Path (Split-Path -Parent $PSScriptRoot) 'var\local-mvp-tls')
)

$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path -Parent $PSScriptRoot
if (-not [System.IO.Path]::IsPathFullyQualified($OutputDirectory)) {
    throw 'OutputDirectory 必须是绝对路径。'
}

$certificatePath = Join-Path $OutputDirectory 'control-plane-cert.pem'
$privateKeyPath = Join-Path $OutputDirectory 'control-plane-key.pem'
$caPath = Join-Path $OutputDirectory 'control-plane-ca.pem'
foreach ($path in @($certificatePath, $privateKeyPath, $caPath)) {
    if (Test-Path -LiteralPath $path) {
        throw "本机 TLS 文件已存在：$path。请显式选择新的输出目录，不覆盖已有材料。"
    }
}

New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
$rsa = [System.Security.Cryptography.RSA]::Create(2048)
try {
    $request = [System.Security.Cryptography.X509Certificates.CertificateRequest]::new(
        'CN=OB Data Orch Local MVP',
        $rsa,
        [System.Security.Cryptography.HashAlgorithmName]::SHA256,
        [System.Security.Cryptography.RSASignaturePadding]::Pkcs1
    )
    $request.CertificateExtensions.Add([System.Security.Cryptography.X509Certificates.X509BasicConstraintsExtension]::new($false, $false, 0, $true))
    $request.CertificateExtensions.Add([System.Security.Cryptography.X509Certificates.X509KeyUsageExtension]::new(
        [System.Security.Cryptography.X509Certificates.X509KeyUsageFlags]::DigitalSignature -bor [System.Security.Cryptography.X509Certificates.X509KeyUsageFlags]::KeyEncipherment,
        $true
    ))
    $san = [System.Security.Cryptography.X509Certificates.SubjectAlternativeNameBuilder]::new()
    $san.AddIpAddress([System.Net.IPAddress]::Loopback)
    $san.AddDnsName('localhost')
    $request.CertificateExtensions.Add($san.Build())
    $certificate = $request.CreateSelfSigned([System.DateTimeOffset]::UtcNow.AddMinutes(-5), [System.DateTimeOffset]::UtcNow.AddDays(7))
    $utf8 = [System.Text.UTF8Encoding]::new($false)
    [System.IO.File]::WriteAllText($certificatePath, $certificate.ExportCertificatePem(), $utf8)
    [System.IO.File]::WriteAllText($privateKeyPath, $rsa.ExportPkcs8PrivateKeyPem(), $utf8)
    [System.IO.File]::WriteAllText($caPath, $certificate.ExportCertificatePem(), $utf8)

    # 本机测试私钥只授予当前 Windows 账户访问，CA 文件保持可复制到 Agent 包。
    $acl = Get-Acl -LiteralPath $privateKeyPath
    $acl.SetAccessRuleProtection($true, $false)
    $currentUser = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
    $rule = [System.Security.AccessControl.FileSystemAccessRule]::new($currentUser, [System.Security.AccessControl.FileSystemRights]::FullControl, [System.Security.AccessControl.AccessControlType]::Allow)
    $acl.SetAccessRule($rule)
    Set-Acl -LiteralPath $privateKeyPath -AclObject $acl
} finally {
    $rsa.Dispose()
}

Write-Output "已生成仅用于回环 TLS 联调的证书材料：$OutputDirectory"
Write-Output "控制面证书：$certificatePath"
Write-Output "Agent CA 文件：$caPath"
