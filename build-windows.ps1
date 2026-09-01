param(
    [string]$Version = "0.11.2",
    [string]$OutputDirectory = "",
    [string]$CertificateThumbprint = "",
    [string]$TimestampUrl = "http://timestamp.digicert.com",
    [string]$RiotAPIKeyCipher = ""
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
if ($Version -notmatch '^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$') {
    throw "Version must use semantic version syntax, for example 0.3.0 or 0.3.0-beta.1"
}

# 真实 Riot API Key 只能通过参数、环境变量或本机专属的 riot_key.local.txt
# 在构建时临时注入，绝不写入任何会被提交到 git 的文件（riot_key.local.*
# 已在 .gitignore 中排除）。
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
$dist = if ($OutputDirectory) {
    if ([System.IO.Path]::IsPathRooted($OutputDirectory)) {
        [System.IO.Path]::GetFullPath($OutputDirectory)
    } else {
        [System.IO.Path]::GetFullPath((Join-Path $projectRoot $OutputDirectory))
    }
} else {
    Join-Path $projectRoot "dist"
}
$versionedName = "Deep Legends.exe"
$stableName = "Deep Legends.exe"
$versionedOutput = Join-Path $dist $versionedName
$stableOutput = Join-Path $dist $stableName
$zipName = "Deep Legends.zip"
$zipOutput = Join-Path $dist $zipName

Push-Location $projectRoot
try {
    New-Item -ItemType Directory -Force -Path $dist | Out-Null
    $unformatted = @(gofmt -l .)
    if ($unformatted.Count -gt 0) {
        throw "Go files are not formatted: $($unformatted -join ', ')"
    }

    go test ./...
    go vet ./...

    $previousGoos = $env:GOOS
    $previousGoarch = $env:GOARCH
    $previousCgo = $env:CGO_ENABLED
    $sourceFingerprint = (& node (Join-Path $projectRoot "desktop\source-fingerprint.cjs")).Trim()
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
        go build -buildvcs=false -trimpath -ldflags $ldflags -o $versionedOutput .
    } finally {
        $env:GOOS = $previousGoos
        $env:GOARCH = $previousGoarch
        $env:CGO_ENABLED = $previousCgo
    }

    & node (Join-Path $projectRoot "desktop\verify-build-fingerprint.cjs") $versionedOutput $sourceFingerprint
    if ($LASTEXITCODE -ne 0) { throw "Windows executable build fingerprint verification failed" }

    if ($CertificateThumbprint) {
        $signTool = (Get-Command signtool.exe -ErrorAction Stop).Source
        & $signTool sign /sha1 $CertificateThumbprint /fd SHA256 /tr $TimestampUrl /td SHA256 $versionedOutput
        if ($LASTEXITCODE -ne 0) { throw "signtool failed with exit code $LASTEXITCODE" }
        $signature = Get-AuthenticodeSignature $versionedOutput
        if ($signature.Status -ne "Valid") { throw "Authenticode signature is not valid: $($signature.Status)" }
    } else {
        Write-Warning "No certificate thumbprint supplied. The build is unsigned and SmartScreen may show Unknown publisher."
    }

    if ($versionedOutput -ne $stableOutput) { Copy-Item -Force $versionedOutput $stableOutput }
    Compress-Archive -Force -Path @($versionedOutput, (Join-Path $projectRoot "README.md")) -DestinationPath $zipOutput

    $artifacts = @($versionedOutput, $zipOutput) | Select-Object -Unique
    $hashLines = foreach ($artifact in $artifacts) {
        $hash = (Get-FileHash -Algorithm SHA256 $artifact).Hash.ToLowerInvariant()
        "$hash  $(Split-Path -Leaf $artifact)"
    }
    $hashLines | Set-Content -Encoding ascii (Join-Path $dist "SHA256SUMS.txt")

    $selfTestProcess = Start-Process -FilePath $versionedOutput -ArgumentList "-self-test" -Wait -PassThru
    if ($selfTestProcess.ExitCode -ne 0) { throw "Windows executable self-test failed with exit code $($selfTestProcess.ExitCode)" }
    Write-Host "Build complete: $versionedOutput"
    Write-Host "Package: $zipOutput"
} finally {
    Pop-Location
}
