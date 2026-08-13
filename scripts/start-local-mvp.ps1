[CmdletBinding()]
param(
    [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'

$repositoryRoot = Split-Path -Parent $PSScriptRoot
$runtimeDirectory = Join-Path $repositoryRoot 'var\local-mvp-tls'
$certificateScript = Join-Path $PSScriptRoot 'new-local-mvp-tls-certificate.ps1'
$agentPackageScript = Join-Path $PSScriptRoot 'build-local-mvp-agent-package.ps1'
$certificatePath = Join-Path $runtimeDirectory 'control-plane-cert.pem'
$keyPath = Join-Path $runtimeDirectory 'control-plane-key.pem'
$caPath = Join-Path $runtimeDirectory 'control-plane-ca.pem'
$controlPlanePath = Join-Path $runtimeDirectory 'control-plane-local-mvp.exe'
$controlPlaneOutput = Join-Path $runtimeDirectory 'control-plane-local-mvp.stdout.log'
$controlPlaneError = Join-Path $runtimeDirectory 'control-plane-local-mvp.stderr.log'
$webOutput = Join-Path $runtimeDirectory 'vite-local-mvp.stdout.log'
$webError = Join-Path $runtimeDirectory 'vite-local-mvp.stderr.log'
$viteEntrypoint = Join-Path $repositoryRoot 'web\node_modules\vite\bin\vite.js'

function Test-ListeningPort {
    param([int]$Port)

    return $null -ne (Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1)
}

function Get-ListeningProcess {
    param([int]$Port)

    $listener = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($null -eq $listener) {
        return $null
    }
    return Get-Process -Id $listener.OwningProcess -ErrorAction Stop
}

function Test-ExpectedControlPlaneProcess {
    param([System.Diagnostics.Process]$Process)

    if ($null -eq $Process) {
        return $false
    }
    try {
        return [string]::Equals($Process.Path, $controlPlanePath, [System.StringComparison]::OrdinalIgnoreCase)
    } catch {
        return $false
    }
}

function Test-ControlPlaneIsCurrent {
    param([System.Diagnostics.Process]$Process)

    if ($null -eq $Process) {
        return $false
    }
    try {
        # 本机证书或二进制在控制面启动后更新时，旧进程仍持有旧实现；必须重启，不能把仅能跳过校验的探测当作健康。
        $certificateWriteTime = (Get-Item -LiteralPath $certificatePath -ErrorAction Stop).LastWriteTimeUtc
        $binaryWriteTime = (Get-Item -LiteralPath $controlPlanePath -ErrorAction Stop).LastWriteTimeUtc
        return $Process.StartTime.ToUniversalTime() -ge $certificateWriteTime -and $Process.StartTime.ToUniversalTime() -ge $binaryWriteTime
    } catch {
        return $false
    }
}

function Test-LocalMVPControlPlane {
    try {
        $response = Invoke-WebRequest -Uri 'https://127.0.0.1:8080/api/v1/data-sources' -Method Get -UseBasicParsing -SkipCertificateCheck -ErrorAction Stop
        return $response.StatusCode -eq 200
    } catch {
        return $false
    }
}

function Test-LocalMVPWebProxy {
    try {
        $response = Invoke-WebRequest -Uri 'http://localhost:5173/api/v1/data-sources' -Method Get -UseBasicParsing -ErrorAction Stop
        return $response.StatusCode -eq 200
    } catch {
        return $false
    }
}

function Wait-ListeningPort {
    param([int]$Port, [string]$Name)

    for ($attempt = 0; $attempt -lt 30; $attempt++) {
        if (Test-ListeningPort -Port $Port) {
            return
        }
        Start-Sleep -Milliseconds 500
    }
    throw "$Name 未能在预期端口启动。请查看 $runtimeDirectory 下的日志。"
}

function Wait-PortClosed {
    param([int]$Port, [string]$Name)

    for ($attempt = 0; $attempt -lt 30; $attempt++) {
        if (-not (Test-ListeningPort -Port $Port)) {
            return
        }
        Start-Sleep -Milliseconds 500
    }
    throw "$Name 未能停止。"
}

New-Item -ItemType Directory -Path $runtimeDirectory -Force | Out-Null
$tlsFiles = @($certificatePath, $keyPath, $caPath)
$existingTLSFileCount = @($tlsFiles | Where-Object { Test-Path -LiteralPath $_ -PathType Leaf }).Count
if ($existingTLSFileCount -eq 0) {
    & $certificateScript
} elseif ($existingTLSFileCount -ne $tlsFiles.Count) {
    throw '本机 TLS 材料不完整。为保护既有 Agent 信任关系，启动脚本不会自动删除或轮换证书；请先恢复原证书、私钥和 CA。'
}
if (-not (Test-Path -LiteralPath $certificatePath -PathType Leaf) -or -not (Test-Path -LiteralPath $keyPath -PathType Leaf) -or -not (Test-Path -LiteralPath $caPath -PathType Leaf)) {
    throw '本机 TLS 证书准备失败。'
}

$certificate = $null
try {
    # 必须同时验证证书可解析且私钥与证书匹配；单参数 CreateFromPemFile 会把证书文件误作私钥文件。
    $certificate = [System.Security.Cryptography.X509Certificates.X509Certificate2]::CreateFromPemFile($certificatePath, $keyPath)
    if ($certificate.NotAfter.ToUniversalTime() -le [DateTime]::UtcNow) {
        throw '本机 TLS 证书已过期。为保护既有 Agent 信任关系，启动脚本不会自动轮换 CA；请执行受控证书轮换并同步更新 Agent。'
    }
} catch {
    if ($_.Exception.Message -like '本机 TLS 证书已过期*') {
        throw
    }
    throw '本机 TLS 证书或私钥无效。为保护既有 Agent 信任关系，启动脚本不会自动删除或轮换证书。'
} finally {
    if ($null -ne $certificate) {
        $certificate.Dispose()
    }
}

if ((Get-FileHash -Algorithm SHA256 -LiteralPath $certificatePath).Hash -ne (Get-FileHash -Algorithm SHA256 -LiteralPath $caPath).Hash) {
    throw '本机控制面证书与 Agent CA 不一致。启动脚本不会自动轮换信任材料。'
}
& $agentPackageScript -ControlPlaneCAFile $caPath

if (-not $SkipBuild) {
    Push-Location $repositoryRoot
    try {
        go build -trimpath -o $controlPlanePath ./cmd/control-plane
    } finally {
        Pop-Location
    }
}
if (-not (Test-Path -LiteralPath $controlPlanePath -PathType Leaf)) {
    throw '控制面二进制不存在。请不要使用 -SkipBuild，或先完成构建。'
}

if (Test-ListeningPort -Port 8080) {
    $existingControlPlane = Get-ListeningProcess -Port 8080
    if (-not (Test-ExpectedControlPlaneProcess -Process $existingControlPlane)) {
        throw '8080 端口已被非本系统控制面进程占用。'
    }
    if (-not (Test-ControlPlaneIsCurrent -Process $existingControlPlane) -or -not (Test-LocalMVPControlPlane)) {
        Stop-Process -Id $existingControlPlane.Id -ErrorAction Stop
        Wait-PortClosed -Port 8080 -Name '旧控制面'
    }
}

if (-not (Test-ListeningPort -Port 8080)) {
    $env:OB_DATA_ORCH_TLS_CERT_FILE = $certificatePath
    $env:OB_DATA_ORCH_TLS_KEY_FILE = $keyPath
    # 本机 MVP 仅对已通过预检查且由用户显式提交的固定单表 CSV 任务启用受控 OBDUMPER 启动，不开放任意命令、SQL 或路径操作。
    $env:OB_DATA_ORCH_ENABLE_AGENT_JDBC_CONNECTION_TEST = 'true'
    $env:OB_DATA_ORCH_ENABLE_REAL_EXECUTION = 'true'
    Start-Process -FilePath $controlPlanePath -ArgumentList '--local-mvp' -WorkingDirectory $repositoryRoot -WindowStyle Hidden -RedirectStandardOutput $controlPlaneOutput -RedirectStandardError $controlPlaneError | Out-Null
    Wait-ListeningPort -Port 8080 -Name '控制面'
}

if ((Test-ListeningPort -Port 5173) -and -not (Test-LocalMVPWebProxy)) {
    $existingWeb = Get-ListeningProcess -Port 5173
    $existingWebCommand = if ($null -eq $existingWeb) { '' } else { (Get-CimInstance Win32_Process -Filter "ProcessId = $($existingWeb.Id)" -ErrorAction SilentlyContinue).CommandLine }
    if ($null -eq $existingWeb -or $existingWeb.ProcessName -ne 'node' -or $existingWebCommand -notmatch 'vite') {
        throw '5173 端口已被非本系统页面服务进程占用。'
    }
    Stop-Process -Id $existingWeb.Id -ErrorAction Stop
    Wait-PortClosed -Port 5173 -Name '旧页面服务'
}

if (-not (Test-ListeningPort -Port 5173)) {
    # 页面代理只访问回环控制面；继承外部 HTTP(S) 代理会把本机 TLS 误发到代理端。
    foreach ($proxyVariable in @('HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy', 'NPM_CONFIG_PROXY', 'NPM_CONFIG_HTTPS_PROXY', 'npm_config_proxy', 'npm_config_https_proxy')) {
        Remove-Item -LiteralPath ("Env:" + $proxyVariable) -ErrorAction SilentlyContinue
    }
    $env:NO_PROXY = 'localhost,127.0.0.1,::1'
    $env:no_proxy = $env:NO_PROXY
    $env:NODE_EXTRA_CA_CERTS = $caPath
    if (-not (Test-Path -LiteralPath $viteEntrypoint -PathType Leaf)) {
        throw 'Vite 依赖未安装。请先在 web 目录执行 npm install。'
    }
    Start-Process -FilePath 'node.exe' -ArgumentList $viteEntrypoint, '--host', '127.0.0.1' -WorkingDirectory (Join-Path $repositoryRoot 'web') -WindowStyle Hidden -RedirectStandardOutput $webOutput -RedirectStandardError $webError | Out-Null
    Wait-ListeningPort -Port 5173 -Name '页面服务'
}

Write-Output '本机系统已启动： http://localhost:5173/data-sources'
