param(
    [string]$Version = "",
    [string]$CertificateFile = "",
    [string]$CertificatePassword = "",
    [string]$RiotAPIKeyCipher = "",
    [ValidateSet("private", "public")][string]$KeyMode = "private"
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$desktopRoot = Join-Path $projectRoot "desktop"
$backendRoot = Join-Path $desktopRoot "backend"
$backendOutput = Join-Path $backendRoot "loot-service.exe"

$package = Get-Content -Raw -Encoding UTF8 (Join-Path $desktopRoot "package.json") | ConvertFrom-Json
if (-not $Version) { $Version = $package.version }

if ($Version -notmatch '^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$') {
    throw "Version must use semantic version syntax"
}
if ($package.version -ne $Version) {
    throw "Version mismatch: package.json is $($package.version), requested build is $Version"
}
$artifactSuffix = if ($KeyMode -eq "public") { "-public" } else { "" }
$setupArtifact = "Deep Legends Setup $Version$artifactSuffix.exe"
$checksumArtifact = "SHA256SUMS$artifactSuffix.txt"

# public 构建不读取个人凭据。private 构建保留本地注入方式。
if ($KeyMode -eq "public") {
    if ($RiotAPIKeyCipher) { throw "Public builds cannot accept RiotAPIKeyCipher" }
    Write-Host "Public build: no embedded Riot API key; KR queries require RIOT_API_KEY at runtime."
} else {
    # 真实 Riot API Key 只能通过参数、环境变量或本机专属的 riot_key.local.txt
    # 在构建时临时注入，绝不写入任何会被提交到 git 的文件（riot_key.local.*
    # 已在 .gitignore 中排除）。private 模式缺少 Key 时立即停止构建。
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
                $RiotAPIKeyCipher = (go run ./backend -encrypt-riot-key $plainKey | Select-Object -Last 1).Trim()
                if ($LASTEXITCODE -ne 0) { throw "Riot key encryption failed" }
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
}

$previousKeyMode = $env:DEEP_LEGENDS_KEY_MODE
Push-Location $projectRoot
try {
    $env:DEEP_LEGENDS_KEY_MODE = $KeyMode
    Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $projectRoot "dist\desktop\release-build.json")
    Get-ChildItem (Join-Path $projectRoot "dist\desktop") -File -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -match '^Deep Legends( Setup)?( ([0-9a-f]{12}|[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?)(-public)?)?\.(exe|zip)$' -or $_.Name -match '^SHA256SUMS(-public)?\.txt$' } |
        Remove-Item -Force
    $staleUnpacked = Join-Path $projectRoot "dist\desktop\win-unpacked"
    if (Test-Path $staleUnpacked) { Remove-Item -Recurse -Force $staleUnpacked }
    New-Item -ItemType Directory -Force -Path $backendRoot | Out-Null
    # Match relative paths so a hidden ancestor of the checkout does not hide source.
    $goSources = @(Get-ChildItem -Recurse -File -Filter '*.go' | Where-Object { $_.FullName.Substring($projectRoot.Length) -notmatch '[\\/](\.[^\\/]+|node_modules|vendor|dist)[\\/]' } | ForEach-Object { $_.FullName })
    if ($goSources.Count -eq 0) { throw "No project Go sources found" }
    $unformatted = @(gofmt -l $goSources)
    if ($LASTEXITCODE -ne 0) { throw "Go formatting check failed" }
    if ($unformatted.Count -gt 0) { throw "Go files are not formatted: $($unformatted -join ', ')" }
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw "Root tests failed" }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw "Root vet failed" }
    Push-Location (Join-Path $projectRoot "installer")
    try {
        go test ./...
        if ($LASTEXITCODE -ne 0) { throw "Installer tests failed" }
        go vet ./...
        if ($LASTEXITCODE -ne 0) { throw "Installer vet failed" }
    } finally {
        Pop-Location
    }

    # Web and desktop tests import jsdom from desktop/node_modules.
    Push-Location $desktopRoot
    try {
        if (Test-Path "package-lock.json") { npm ci } else { npm install }
        if ($LASTEXITCODE -ne 0) { throw "Desktop dependencies install failed" }
    } finally {
        Pop-Location
    }

    node --check backend/web/app.js
    if ($LASTEXITCODE -ne 0) { throw "backend/web/app.js syntax check failed" }
    node --check backend/web/champions.js
    if ($LASTEXITCODE -ne 0) { throw "backend/web/champions.js syntax check failed" }
    $webTests = @(Get-ChildItem (Join-Path $projectRoot "backend\web\*.test.cjs") | ForEach-Object { $_.FullName })
    node --test $webTests
    if ($LASTEXITCODE -ne 0) { throw "Web tests failed" }
    node --check desktop/main.cjs
    if ($LASTEXITCODE -ne 0) { throw "desktop/main.cjs syntax check failed" }
    node --check desktop/proxy-resolution.cjs
    if ($LASTEXITCODE -ne 0) { throw "desktop/proxy-resolution.cjs syntax check failed" }
    $desktopTests = @(Get-ChildItem (Join-Path $desktopRoot "*.test.cjs") | ForEach-Object { $_.FullName })
    node --test $desktopTests
    if ($LASTEXITCODE -ne 0) { throw "Desktop tests failed" }

    $previousGoos = $env:GOOS
    $previousGoarch = $env:GOARCH
    $previousCgo = $env:CGO_ENABLED
    $sourceFingerprint = (& node (Join-Path $desktopRoot "source-fingerprint.cjs")).Trim()
    if ($LASTEXITCODE -ne 0) { throw "Source fingerprint calculation failed" }
    if ($sourceFingerprint -notmatch '^[0-9a-f]{12}$') { throw "Invalid source fingerprint: $sourceFingerprint" }
    try {
        $env:GOOS = "windows"
        $env:GOARCH = "amd64"
        $env:CGO_ENABLED = "0"
        $ldflags = "-s -w -H=windowsgui -buildid= -X main.version=$Version -X main.buildFingerprint=$sourceFingerprint -X main.riotAPIKey= -X main.riotAPIKeyCipher=$RiotAPIKeyCipher"
        go build -buildvcs=false -trimpath -ldflags $ldflags -o $backendOutput ./backend
        if ($LASTEXITCODE -ne 0) { throw "Backend build failed" }
    } finally {
        $env:GOOS = $previousGoos
        $env:GOARCH = $previousGoarch
        $env:CGO_ENABLED = $previousCgo
    }

    & node (Join-Path $desktopRoot "verify-build-fingerprint.cjs") $backendOutput $sourceFingerprint
    if ($LASTEXITCODE -ne 0) { throw "Backend build fingerprint verification failed" }
    $selfTestArgs = "--self-test"
    if ($KeyMode -eq "private") { $selfTestArgs += " --self-check-riot-key" }
    $selfTest = Start-Process -FilePath $backendOutput -ArgumentList $selfTestArgs -Wait -PassThru
    if ($selfTest.ExitCode -ne 0) { throw "Windows backend self-test failed with exit code $($selfTest.ExitCode)" }

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
        # 文件名使用版本号；诊断与构建核验仍保留源码指纹。
        $env:DEEP_LEGENDS_FINGERPRINT = $sourceFingerprint
        $builderSignArgs = @()
        if (-not $CertificateFile) { $builderSignArgs = @("--config.win.signExecutable=false") }
        if ($KeyMode -eq "public") { $builderSignArgs += '--config.nsis.artifactName=Deep Legends Setup ${version}-public.${ext}' }

        # 只构建 setup；beforePack 默认使用 7z level 3，保留原有解压方式。
        & node (Join-Path $projectRoot "installer\build-shell.cjs") --uninstall $Version
        if ($LASTEXITCODE -ne 0) { throw "Uninstaller shell build failed" }
        npm run pack:win-setup -- $builderSignArgs
        if ($LASTEXITCODE -ne 0) { throw "NSIS packaging failed" }
        Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $desktopRoot "uninstall-shell.exe")
        & node (Join-Path $projectRoot "installer\build-shell.cjs") $Version $sourceFingerprint
        if ($LASTEXITCODE -ne 0) { throw "Installer shell build failed" }
    } finally {
        Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $desktopRoot "uninstall-shell.exe")
        Pop-Location
    }

    $unpackedDirectory = Join-Path $projectRoot "dist\desktop\win-unpacked"
    if (-not (Test-Path $unpackedDirectory -PathType Container)) {
        throw "Packaged client directory is missing: $unpackedDirectory"
    }
    $packagedAsar = Join-Path $unpackedDirectory "resources\app.asar"
    & node (Join-Path $desktopRoot "verify-packaged-runtime.cjs") $packagedAsar
    if ($LASTEXITCODE -ne 0) { throw "Packaged desktop runtime verification failed" }
    $packagedBackend = Join-Path $unpackedDirectory "resources\app.asar.unpacked\backend\loot-service.exe"
    & node (Join-Path $desktopRoot "verify-build-fingerprint.cjs") $packagedBackend $sourceFingerprint
    if ($LASTEXITCODE -ne 0) { throw "Packaged backend build fingerprint verification failed" }
    & node (Join-Path $desktopRoot "release-build.cjs") $sourceFingerprint
    if ($LASTEXITCODE -ne 0) { throw "Release build verification failed" }
    Remove-Item -Recurse -Force $unpackedDirectory

    $artifacts = Get-ChildItem (Join-Path $projectRoot "dist\desktop") -File |
        Where-Object { $_.Name -eq $setupArtifact } |
        Sort-Object Name
    $hashLines = foreach ($artifact in $artifacts) {
        $hash = (Get-FileHash -Algorithm SHA256 $artifact.FullName).Hash.ToLowerInvariant()
        "$hash  $($artifact.Name)"
    }
    $hashLines | Set-Content -Encoding ascii (Join-Path $projectRoot "dist\desktop\$checksumArtifact")
    Write-Host "Desktop build complete: $(Join-Path $projectRoot 'dist\desktop')"
} finally {
    $env:DEEP_LEGENDS_KEY_MODE = $previousKeyMode
    Pop-Location
}
