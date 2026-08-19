param(
    [string]$Version = "0.11.0",
    [string]$CertificateFile = "",
    [string]$CertificatePassword = "",
    [string]$RiotAPIKeyCipher = ""
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$desktopRoot = Join-Path $projectRoot "desktop"
$backendRoot = Join-Path $desktopRoot "backend"
$backendOutput = Join-Path $backendRoot "loot-service.exe"

if ($Version -notmatch '^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$') {
    throw "Version must use semantic version syntax"
}

# 真实 Riot API Key 密文只能通过参数或环境变量在构建时临时注入，
# 绝不写入任何会被提交到 git 的文件。未提供时构建出的程序未配置 Key，
# 仅能用环境变量 RIOT_API_KEY 临时调试。
if (-not $RiotAPIKeyCipher -and $env:RIOT_API_KEY_CIPHER) {
    $RiotAPIKeyCipher = $env:RIOT_API_KEY_CIPHER
}
if ($RiotAPIKeyCipher -and $RiotAPIKeyCipher -match '["`]') {
    throw "RiotAPIKeyCipher must not contain quote characters"
}

Push-Location $projectRoot
try {
    New-Item -ItemType Directory -Force -Path $backendRoot | Out-Null
    $unformatted = @(gofmt -l .)
    if ($unformatted.Count -gt 0) { throw "Go files are not formatted: $($unformatted -join ', ')" }
    go test ./...
    go vet ./...
    node --check web/app.js
    node --check web/champions.js
    $webTests = @(Get-ChildItem (Join-Path $projectRoot "web\*.test.cjs") | ForEach-Object { $_.FullName })
    node --test $webTests
    node --check desktop/main.cjs
    node --check desktop/proxy-resolution.cjs
    $desktopTests = @(Get-ChildItem (Join-Path $desktopRoot "*.test.cjs") | ForEach-Object { $_.FullName })
    node --test $desktopTests

    $previousGoos = $env:GOOS
    $previousGoarch = $env:GOARCH
    $previousCgo = $env:CGO_ENABLED
    try {
        $env:GOOS = "windows"
        $env:GOARCH = "amd64"
        $env:CGO_ENABLED = "0"
        $ldflags = "-s -w -H=windowsgui -buildid= -X main.version=$Version"
        if ($RiotAPIKeyCipher) {
            $ldflags += " -X main.riotAPIKeyCipher=$RiotAPIKeyCipher"
        } else {
            Write-Warning "No RiotAPIKeyCipher supplied. This build has no embedded Riot API key (KR lookups need RIOT_API_KEY at runtime)."
        }
        go build -buildvcs=false -trimpath -ldflags $ldflags -o $backendOutput .
    } finally {
        $env:GOOS = $previousGoos
        $env:GOARCH = $previousGoarch
        $env:CGO_ENABLED = $previousCgo
    }

    $selfTest = Start-Process -FilePath $backendOutput -ArgumentList "--self-test" -Wait -PassThru
    if ($selfTest.ExitCode -ne 0) { throw "Windows backend self-test failed with exit code $($selfTest.ExitCode)" }

    $package = Get-Content -Raw (Join-Path $desktopRoot "package.json") | ConvertFrom-Json
    if ($package.version -ne $Version) {
        throw "Version mismatch: package.json is $($package.version), requested build is $Version"
    }

    if ($CertificateFile) {
        $env:CSC_LINK = $CertificateFile
        $env:CSC_KEY_PASSWORD = $CertificatePassword
    } else {
        Write-Warning "Desktop package is unsigned. SmartScreen may show Unknown publisher until a trusted certificate builds reputation."
    }

    Push-Location $desktopRoot
    try {
        if (Test-Path "package-lock.json") { npm ci } else { npm install }

        # electron-builder 对 portable 目标硬编码使用内置模板，无法通过配置替换。
        # 覆盖为定制模板：首次启动解压到本地缓存，之后直接秒开（见 desktop/nsis/portable.nsi）。
        $portableTemplate = Join-Path $desktopRoot "nsis\portable.nsi"
        $builderTemplate = Join-Path $desktopRoot "node_modules\app-builder-lib\templates\nsis\portable.nsi"
        if (-not (Test-Path $portableTemplate)) { throw "Custom portable NSIS template is missing: $portableTemplate" }
        if (-not (Test-Path $builderTemplate)) { throw "electron-builder portable template not found: $builderTemplate" }
        Copy-Item -Force $portableTemplate $builderTemplate

        # 便携版与安装版必须分两次调用打包：electron-builder 26 在同一次
        # 调用中构建两个 NSIS 系目标时，共享的应用包文件会被提前清理，
        # 导致安装版报 ENOENT 且便携版产物被截断。
        npm run pack:win
        npm run pack:win-setup
    } finally {
        Pop-Location
    }

    $unpackedDirectory = Join-Path $projectRoot "dist\desktop\win-unpacked"
    $zipArtifact = Join-Path $projectRoot "dist\desktop\Deep Legends.zip"
    if (-not (Test-Path $unpackedDirectory -PathType Container)) {
        throw "Packaged client directory is missing: $unpackedDirectory"
    }
    if (Test-Path $zipArtifact) { Remove-Item -Force $zipArtifact }
    Compress-Archive -Path (Join-Path $unpackedDirectory "*") -DestinationPath $zipArtifact -CompressionLevel Optimal
    Remove-Item -Recurse -Force $unpackedDirectory

    $setupArtifact = Join-Path $projectRoot "dist\desktop\Deep Legends Setup.exe"
    if (-not (Test-Path $setupArtifact)) {
        throw "Installer artifact is missing: $setupArtifact"
    }

    $artifacts = Get-ChildItem (Join-Path $projectRoot "dist\desktop") -File |
        Where-Object { $_.Name -in "Deep Legends.exe", "Deep Legends Setup.exe", "Deep Legends.zip" } |
        Sort-Object Name
    $hashLines = foreach ($artifact in $artifacts) {
        $hash = (Get-FileHash -Algorithm SHA256 $artifact.FullName).Hash.ToLowerInvariant()
        "$hash  $($artifact.Name)"
    }
    $hashLines | Set-Content -Encoding ascii (Join-Path $projectRoot "dist\desktop\SHA256SUMS.txt")
    Write-Host "Desktop build complete: $(Join-Path $projectRoot 'dist\desktop')"
} finally {
    Pop-Location
}
