$ErrorActionPreference = 'Stop'

$Repo = 'casosorg/casos'
$Version = if ($env:CASOS_VERSION) { $env:CASOS_VERSION } else { 'latest' }
$InstallDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'CasOS\bin' }

if ($Version -eq 'latest') {
    Write-Host 'Fetching latest CasOS release...'
    $Version = (Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest").tag_name
    if (-not $Version) { throw 'Could not resolve the latest release.' }
}
if ($Version -notmatch '^v[0-9A-Za-z._-]+$') { throw "Invalid release version: $Version" }

$Architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
$ArchName = switch ($Architecture) {
    'X64' { 'amd64' }
    'Arm64' { 'arm64' }
    default { throw "Unsupported architecture: $Architecture" }
}

$Filename = "casos_windows_${ArchName}.exe"
$ReleaseUrl = "https://github.com/$Repo/releases/download/$Version"
$TempDir = Join-Path $env:TEMP "casos_install_$([guid]::NewGuid().ToString('N'))"
New-Item -ItemType Directory -Path $TempDir | Out-Null

try {
    $ExePath = Join-Path $TempDir $Filename
    $ChecksumsPath = Join-Path $TempDir 'SHA256SUMS'
    Write-Host "Downloading CasOS $Version..."
    Invoke-WebRequest -Uri "$ReleaseUrl/$Filename" -OutFile $ExePath -UseBasicParsing
    Invoke-WebRequest -Uri "$ReleaseUrl/SHA256SUMS" -OutFile $ChecksumsPath -UseBasicParsing

    $ChecksumLine = Get-Content $ChecksumsPath | Where-Object { $_ -match "^[0-9a-fA-F]{64}\s+\*?$([regex]::Escape($Filename))$" } | Select-Object -First 1
    if (-not $ChecksumLine) { throw "Release checksum for $Filename was not found." }
    $Expected = ($ChecksumLine -split '\s+')[0].ToLowerInvariant()
    $Actual = (Get-FileHash -Algorithm SHA256 -Path $ExePath).Hash.ToLowerInvariant()
    if ($Actual -ne $Expected) { throw 'Download checksum verification failed.' }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $InstalledExe = Join-Path $InstallDir 'casos.exe'
    $PendingExe = Join-Path $InstallDir 'casos.new.exe'
    Copy-Item -Path $ExePath -Destination $PendingExe -Force
    Move-Item -Path $PendingExe -Destination $InstalledExe -Force
} finally {
    Remove-Item -Recurse -Force $TempDir -ErrorAction SilentlyContinue
}

$UserPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
$PathEntries = @($UserPath -split ';' | Where-Object { $_ })
if ($PathEntries -notcontains $InstallDir) {
    $NewUserPath = (@($PathEntries) + $InstallDir) -join ';'
    [Environment]::SetEnvironmentVariable('PATH', $NewUserPath, 'User')
    Write-Host "Added $InstallDir to your user PATH."
}
$env:PATH = "$env:PATH;$InstallDir"

Write-Host "CasOS $Version installed at $InstalledExe"
Write-Host 'Starting CasOS at http://127.0.0.1:9000 ...'
& $InstalledExe
