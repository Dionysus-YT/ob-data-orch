param(
    [Parameter(Mandatory = $true)]
    [string]$JdkHome,
    [string]$OutputPath = "internal/jdbcprobe/assets/ob-data-orch-jdbc-probe.jar"
)

$ErrorActionPreference = "Stop"
$javac = Join-Path $JdkHome "bin/javac.exe"
$jar = Join-Path $JdkHome "bin/jar.exe"
$java = Join-Path $JdkHome "bin/java.exe"
if (-not (Test-Path -LiteralPath $javac) -or -not (Test-Path -LiteralPath $jar) -or -not (Test-Path -LiteralPath $java)) {
    throw "未找到 Java 8 JDK 的 javac.exe、jar.exe 或 java.exe"
}

$sourceRoot = Join-Path $PSScriptRoot "../internal/jdbcprobe/java"
$testSourceRoot = Join-Path $PSScriptRoot "../internal/jdbcprobe/java-test"
$outputAbsolute = Join-Path $PSScriptRoot (Join-Path ".." $OutputPath)
$outputDirectory = Split-Path -Parent $outputAbsolute
$classes = Join-Path ([System.IO.Path]::GetTempPath()) ("ob-data-orch-jdbc-probe-" + [guid]::NewGuid().ToString("N"))
$testClasses = Join-Path ([System.IO.Path]::GetTempPath()) ("ob-data-orch-jdbc-probe-test-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $classes -Force | Out-Null
New-Item -ItemType Directory -Path $testClasses -Force | Out-Null
New-Item -ItemType Directory -Path $outputDirectory -Force | Out-Null
try {
    $sources = @(Get-ChildItem -LiteralPath $sourceRoot -Recurse -File -Filter '*.java' | Select-Object -ExpandProperty FullName)
    if ($sources.Count -eq 0) {
        throw "未找到 JDBC 探针 Java 源码"
    }
    $compilerArguments = @("-encoding", "UTF-8", "-source", "8", "-target", "8", "-d", $classes) + $sources
    & $javac @compilerArguments
    if ($LASTEXITCODE -ne 0) {
        throw "JDBC 探针编译失败"
    }
    & $jar cf $outputAbsolute -C $classes .
    if ($LASTEXITCODE -ne 0) {
        throw "JDBC 探针 JAR 打包失败"
    }
    $testSources = @(Get-ChildItem -LiteralPath $testSourceRoot -Recurse -File -Filter '*.java' | Select-Object -ExpandProperty FullName)
    if ($testSources.Count -eq 0) {
        throw "未找到 JDBC 探针 Java 测试源码"
    }
    $testCompilerArguments = @("-encoding", "UTF-8", "-source", "8", "-target", "8", "-cp", $classes, "-d", $testClasses) + $testSources
    & $javac @testCompilerArguments
    if ($LASTEXITCODE -ne 0) {
        throw "JDBC 探针 Java 测试编译失败"
    }
    $testClasspath = $classes + [System.IO.Path]::PathSeparator + $testClasses
    & $java -cp $testClasspath com.obdataorch.jdbcprobe.ConnectionProbeObjectAccessClassificationTest
    if ($LASTEXITCODE -ne 0) {
        throw "JDBC 探针 Java 测试失败"
    }
} finally {
    Remove-Item -LiteralPath $classes -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $testClasses -Recurse -Force -ErrorAction SilentlyContinue
}
