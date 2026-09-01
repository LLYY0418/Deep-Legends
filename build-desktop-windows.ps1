param(
    [string]$Version = "0.11.2",
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

# 真实 Riot API Key 只能通过参数、环境变量或本机专属的 riot_key.local.txt
# 在构建时临时注入，绝不写入任何会被提交到 git 的文件（riot_key.local.*
# 已在 .gitignore 中排除）。三者都没有时构建出的程序未配置 Key，仅能用
# 环境变量 RIOT_API_KEY 临时调试。
if (-not $RiotAPIKeyCipher -and $env:RIOT_API_KEY_CIPHER) {
    $RiotAPIKeyCipher = $env:RIOT_API_KEY_CIPHER
}
$localKeyFile = Join-Path $projectRoot "riot_key.local.txt"
if (-not $RiotAPIKeyCipher -and (Test-Path $localKeyFile)) {
    $plainKey = (Get-Content $localKeyFile | Where-Object { $_ -and ($_.Trim() -notmatch '^#') } | Select-Object -First 1)
    if ($plainKey) { $plainKey = $plainKey.Trim() }
    if ($plainKey) {
        Push-Location $projectRoot
        try {
            $RiotAPIKeyCipher = (go run . -encrypt-riot-key $plainKey | Select-Object -Last 1).Trim()
        } finally {
            Pop-Location
        }
        if (-not $RiotAPIKeyCipher) { throw "Failed to encrypt the key from riot_key.local.txt" }
        Write-Host "Riot API key loaded from riot_key.local.txt (not tracked by git) and encrypted for this build."
    }
}
if ($RiotAPIKeyCipher -and $RiotAPIKeyCipher -match '["`]') {
    throw "RiotAPIKeyCipher must not contain quote characters"
}
if (-not $RiotAPIKeyCipher) {
    throw "Riot API key is required for desktop builds. Set RIOT_API_KEY_CIPHER, pass -RiotAPIKeyCipher, or provide riot_key.local.txt."
}

Push-Location $projectRoot
try {
    Get-ChildItem (Join-Path $projectRoot "dist\desktop") -File -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -match '^Deep Legends( Setup)?( [0-9a-f]{12})?\.(exe|zip)$' -or $_.Name -eq "SHA256SUMS.txt" } |
        Remove-Item -Force
    $staleUnpacked = Join-Path $projectRoot "dist\desktop\win-unpacked"
    if (Test-Path $staleUnpacked) { Remove-Item -Recurse -Force $staleUnpacked }
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
    $sourceFingerprint = (& node (Join-Path $desktopRoot "source-fingerprint.cjs")).Trim()
    if ($sourceFingerprint -notmatch '^[0-9a-f]{12}$') { throw "Invalid source fingerprint: $sourceFingerprint" }
    try {
        $env:GOOS = "windows"
        $env:GOARCH = "amd64"
        $env:CGO_ENABLED = "0"
        $ldflags = "-s -w -H=windowsgui -buildid= -X main.version=$Version -X main.buildFingerprint=$sourceFingerprint"
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

    & node (Join-Path $desktopRoot "verify-build-fingerprint.cjs") $backendOutput $sourceFingerprint
    if ($LASTEXITCODE -ne 0) { throw "Backend build fingerprint verification failed" }
    $selfTest = Start-Process -FilePath $backendOutput -ArgumentList "--self-test --self-check-riot-key" -Wait -PassThru
    if ($selfTest.ExitCode -ne 0) { throw "Windows backend self-test failed with exit code $($selfTest.ExitCode)" }

    $package = Get-Content -Raw (Join-Path $desktopRoot "package.json") | ConvertFrom-Json
    if ($package.version -ne $Version) {
        throw "Version mismatch: package.json is $($package.version), requested build is $Version"
    }

    if ($CertificateFile) {
        $env:CSC_LINK = $CertificateFile
        $env:CSC_KEY_PASSWORD = $CertificatePassword
    } else {
        Remove-Item Env:CSC_LINK -ErrorAction SilentlyContinue
        Remove-Item Env:CSC_KEY_PASSWORD -ErrorAction SilentlyContinue
        Write-Warning "Desktop package is unsigned. SmartScreen may show Unknown publisher until a trusted certificate builds reputation."
    }

    Push-Location $desktopRoot
    try {
        if (Test-Path "package-lock.json") { npm ci } else { npm install }

        $templateOutput = (& node (Join-Path $desktopRoot "apply-portable-template.cjs")).Trim()
        if ($templateOutput -notmatch 'fingerprint=([0-9a-f]{12})$') { throw "Portable template did not return a fingerprint: $templateOutput" }
        # 产物名与设置页展示同一个源码指纹；portable 内部缓存使用后端指纹。
        $env:DEEP_LEGENDS_FINGERPRINT = $sourceFingerprint
        $builderSignArgs = @()
        if (-not $CertificateFile) { $builderSignArgs = @("--config.win.signAndEditExecutable=false", "--config.win.signExecutable=false") }

        # 便携版与安装版必须分两次调用打包：electron-builder 26 在同一次
        # 调用中构建两个 NSIS 系目标时，共享的应用包文件会被提前清理，
        # 导致安装版报 ENOENT 且便携版产物被截断。
        npm run pack:win -- $builderSignArgs
        npm run pack:win-setup -- $builderSignArgs
    } finally {
        Pop-Location
    }

    $unpackedDirectory = Join-Path $projectRoot "dist\desktop\win-unpacked"
    $zipArtifact = Join-Path $projectRoot "dist\desktop\Deep Legends $env:DEEP_LEGENDS_FINGERPRINT.zip"
    if (-not (Test-Path $unpackedDirectory -PathType Container)) {
        throw "Packaged client directory is missing: $unpackedDirectory"
    }
    $packagedBackend = Join-Path $unpackedDirectory "resources\app.asar.unpacked\backend\loot-service.exe"
    & node (Join-Path $desktopRoot "verify-build-fingerprint.cjs") $packagedBackend $sourceFingerprint
    if ($LASTEXITCODE -ne 0) { throw "Packaged backend build fingerprint verification failed" }
    if (Test-Path $zipArtifact) { Remove-Item -Force $zipArtifact }
    Compress-Archive -Path (Join-Path $unpackedDirectory "*") -DestinationPath $zipArtifact -CompressionLevel Optimal
    Remove-Item -Recurse -Force $unpackedDirectory

    $artifacts = Get-ChildItem (Join-Path $projectRoot "dist\desktop") -File |
        Where-Object { $_.Name -match '^Deep Legends( Setup)? [0-9a-f]{12}\.(exe|zip)$' } |
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
