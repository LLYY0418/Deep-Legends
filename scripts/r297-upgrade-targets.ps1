# Dot-sourced by r206-real-upgrade-windows.ps1; these paths belong to its CI fixture.
function Test-R297UpgradeTargets {
    param([string]$Candidate, [array]$Published)
    # go test runs from backend/, so export an absolute native installer path.
    $Candidate = (Resolve-Path -LiteralPath $Candidate).Path
    $savedInstall = $script:install
    $defaultCopy = Join-Path $env:LOCALAPPDATA 'Programs/Deep Legends/Deep Legends.exe'
    $defaultBefore = if (Test-Path -LiteralPath $defaultCopy) { (Get-FileHash -LiteralPath $defaultCopy -Algorithm SHA256).Hash } else { $null }
    $results = @()
    try {
        Stop-InstalledApp
        foreach ($oldBuild in $Published) {
            foreach ($stale in @($false,$true)) {
                $label = $oldBuild.version + '-' + $(if ($stale) { 'stale-registration' } else { 'custom-directory' })
                $script:install = Join-Path $root ('Games/Deep Legends-' + $label)
                $script:appDirectories += $script:install
                New-Item -ItemType Directory -Force $script:install | Out-Null
                Run-Setup $oldBuild.file -Version $oldBuild.version -PublishedLegacy
                Stop-InstalledApp
                $sentinel = Join-Path $script:install 'r297-original-directory.txt'
                # The installer may replace its own files; this verifies the old directory survives.
                Set-Content -LiteralPath $sentinel -Value 'r297-original-directory' -Encoding utf8
                $staleRegistrationWrites = 0
                if ($stale) {
                    foreach ($view in @([Microsoft.Win32.RegistryView]::Registry64,[Microsoft.Win32.RegistryView]::Registry32)) {
                        $base = [Microsoft.Win32.RegistryKey]::OpenBaseKey([Microsoft.Win32.RegistryHive]::CurrentUser,$view)
                        $parent = $base.OpenSubKey('Software\Microsoft\Windows\CurrentVersion\Uninstall')
                        try {
                            foreach ($name in $parent.GetSubKeyNames()) {
                                $key = $parent.OpenSubKey($name,$true)
                                try {
                                    # Only the registration whose uninstaller is inside this fixture.
                                    $command = [string]$key.GetValue('UninstallString','')
                                    if ($key.GetValue('DisplayName','') -eq 'Deep Legends' -and $command.Contains($script:install)) {
                                        $key.SetValue('InstallLocation',(Join-Path $root 'r297-nonexistent-directory'))
                                        $staleRegistrationWrites++
                                    }
                                } finally { $key.Dispose() }
                            }
                        } finally { if ($parent) { $parent.Dispose() }; $base.Dispose() }
                    }
                }
                if ($stale -and $staleRegistrationWrites -eq 0) { throw "R297 stale-registration fixture did not alter an owned key ($label)" }
                $env:R297_TARGET_INSTALL = $script:install
                $env:R297_TARGET_SETUP = $Candidate
                $script:lastSetupDeadline = (Get-Date).AddMinutes(3)
                & go test ./backend -count=1 -run '^TestR297WindowsInstalledTargetHandoff$' *> (Join-Path $evidence ('r297-target-' + $label + '.log'))
                if ($LASTEXITCODE -ne 0) { throw "R297 native handoff failed: $label" }
                Wait-R265InstallerLaunch
                Assert-InstalledVersion $candidateVersion
                foreach ($file in @('Deep Legends.exe','Uninstall Deep Legends.exe','resources/app.asar','resources/app.asar.unpacked/backend/loot-service.exe')) {
                    if (-not (Test-Path -LiteralPath (Join-Path $script:install $file))) { throw "R297 original directory missing $file ($label)" }
                }
                $defaultAfter = if (Test-Path -LiteralPath $defaultCopy) { (Get-FileHash -LiteralPath $defaultCopy -Algorithm SHA256).Hash } else { $null }
                if ($defaultAfter -ne $defaultBefore) { throw "R297 upgrade created or changed default copy ($label)" }
                $results += @{ old_version=$oldBuild.version; scenario=$label; original_directory_intact=$true; default_copy_unchanged=$true; native_handoff=$true; stale_registration_requested=$stale; stale_registration_writes=$staleRegistrationWrites }
                Stop-InstalledApp
            }
        }
        $results | ConvertTo-Json -Depth 5 | Set-Content (Join-Path $evidence 'r297-upgrade-targets.json') -Encoding utf8
    } finally {
        Remove-Item Env:R297_TARGET_INSTALL,Env:R297_TARGET_SETUP -ErrorAction SilentlyContinue
        $script:install = $savedInstall
    }
}
