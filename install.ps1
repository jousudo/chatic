# Copyright (c) 2026 Chatic Contributors
# Licensed under the Apache License, Version 2.0.
#
# One-line installer (Windows PowerShell):
#   irm https://raw.githubusercontent.com/jousudo/chatic/main/install.ps1 | iex
#
# Downloads the latest binary from GitHub Releases, installs it under %LOCALAPPDATA%
# and prepares the data folder. FFmpeg (optional, audio only) is auto-installed via
# winget when available. Set $env:CHATIC_SKIP_FFMPEG=1 to skip that step.

$ErrorActionPreference = 'Stop'

$Repo    = 'jousudo/chatic'
$Project = 'chatic'
$Bin     = 'chatic.exe'
$InstallDir = Join-Path $env:LOCALAPPDATA 'Programs\chatic'
$DataDir    = Join-Path $env:LOCALAPPDATA 'chatic'

# 1. Architecture (mapped to the GoReleaser names).
$arch = if ([Environment]::Is64BitOperatingSystem) {
    if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
} else { throw '32-bit Windows is not supported.' }

# 2. Version (latest release unless $env:CHATIC_VERSION/$env:TUTOR_VERSION is set).
$version = $env:CHATIC_VERSION
if (-not $version) { $version = $env:TUTOR_VERSION }
if (-not $version) {
    Write-Host 'Finding the latest version...'
    $rel = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
    $version = $rel.tag_name
    if (-not $version) { throw 'Could not find the latest version (has the first release been published yet?).' }
}
$verNum = $version.TrimStart('v')

# 3. Download and extract the release .zip.
$archive = "${Project}_${verNum}_windows_${arch}.zip"
$url = "https://github.com/$Repo/releases/download/$version/$archive"
$tmp = Join-Path $env:TEMP ("chatic-" + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $tmp | Out-Null
try {
    Write-Host "Downloading $archive ($version)..."
    $zipPath = Join-Path $tmp $archive
    Invoke-WebRequest -Uri $url -OutFile $zipPath
    Expand-Archive -Path $zipPath -DestinationPath $tmp -Force

    if (-not (Test-Path (Join-Path $tmp $Bin))) { throw "Binary '$Bin' not found inside the package." }

    # 4. Install the binary.
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Copy-Item -Force (Join-Path $tmp $Bin) (Join-Path $InstallDir $Bin)

    # 5. Prepare the data folder and the .env.
    New-Item -ItemType Directory -Force -Path (Join-Path $DataDir 'storage') | Out-Null
    $envFile = Join-Path $DataDir '.env'
    $envSample = Join-Path $tmp '.env.example'
    if ((-not (Test-Path $envFile)) -and (Test-Path $envSample)) {
        Copy-Item $envSample $envFile
    }
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

# 6. Best-effort FFmpeg (optional; only for audio) via winget. Never fatal.
$ffmpegStatus = 'skipped'
if (Get-Command ffmpeg -ErrorAction SilentlyContinue) {
    $ffmpegStatus = 'already installed'
} elseif ($env:CHATIC_SKIP_FFMPEG -eq '1') {
    $ffmpegStatus = 'skipped (CHATIC_SKIP_FFMPEG=1)'
} elseif (Get-Command winget -ErrorAction SilentlyContinue) {
    Write-Host 'Installing FFmpeg via winget (optional, for audio)...'
    try {
        winget install --silent --accept-source-agreements --accept-package-agreements --id Gyan.FFmpeg | Out-Null
        $ffmpegStatus = 'installed via winget (restart the terminal so it lands on PATH)'
    } catch {
        $ffmpegStatus = 'winget install failed - run: winget install Gyan.FFmpeg'
    }
} else {
    $ffmpegStatus = 'not installed (optional) - run: winget install Gyan.FFmpeg'
}

Write-Host ''
Write-Host "OK: Chatic $version installed at: $InstallDir\$Bin"
Write-Host '------------------------------------------------------------'
Write-Host 'To start:'
Write-Host "     Set-Location `"$DataDir`"; & `"$InstallDir\$Bin`""
Write-Host '   Then open http://localhost:3030/admin, create the password and scan the QR.'
Write-Host "Audio (voice/TTS): FFmpeg $ffmpegStatus."
