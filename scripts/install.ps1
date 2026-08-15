# CasOS one-step installer, adapted from OpenAgent's Apache-2.0 installer.
# Usage:
#   irm https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.ps1 | iex
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.ps1))) -Uninstall
#
# Optional environment variables:
#   CASOS_VERSION  release tag such as v1.32.0 (default: latest)
#   CASOS_REPOSITORY GitHub release repository (default: casosorg/casos)
#   INSTALL_DIR    binary directory (default: $env:LOCALAPPDATA\CasOS\bin)

# ValueFromRemainingArguments catches the "--uninstall" spelling that anyone
# arriving from the Bash installer will reach for first; PowerShell would
# otherwise reject it as an unexpected positional argument.
param(
    [switch] $Uninstall,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]] $ExtraArgs
)

$ErrorActionPreference = 'Stop'

foreach ($arg in @($ExtraArgs)) {
    switch -Regex ($arg) {
        '^--?uninstall$' { $Uninstall = $true }
        '^$'             { }
        default          { throw "Unknown option: $arg" }
    }
}

# Windows PowerShell 5.1 on .NET Framework 4.6 and earlier defaults to SSL 3.0 /
# TLS 1.0, which github.com refuses, so the download fails before it starts. Add
# TLS 1.2 only when the process carries an explicit protocol list that lacks it.
# SystemDefault (0) means the OS picks the protocol and already prefers TLS
# 1.2/1.3; overwriting it would disable TLS 1.3 rather than fix anything.
$TlsProtocol = [Net.ServicePointManager]::SecurityProtocol
if ($TlsProtocol -ne 0 -and -not ($TlsProtocol -band [Net.SecurityProtocolType]::Tls12)) {
    [Net.ServicePointManager]::SecurityProtocol = $TlsProtocol -bor [Net.SecurityProtocolType]::Tls12
}

$Repo = if ($env:CASOS_REPOSITORY) { $env:CASOS_REPOSITORY } else { 'casosorg/casos' }
$Version = if ($env:CASOS_VERSION) { $env:CASOS_VERSION } else { 'latest' }
$InstallDir = if ($env:INSTALL_DIR) {
    if ($env:INSTALL_DIR.Contains([System.IO.Path]::PathSeparator)) {
        throw 'INSTALL_DIR must not contain a PATH separator.'
    }
    [System.IO.Path]::GetFullPath($env:INSTALL_DIR)
} else {
    Join-Path $env:LOCALAPPDATA 'CasOS\bin'
}

if ($Uninstall) {
    $InstalledExe = Join-Path $InstallDir 'casos.exe'
    $Found = $false

    if (Test-Path $InstalledExe) {
        $RunningCasOS = Get-CimInstance Win32_Process -Filter "Name = 'casos.exe'" -ErrorAction SilentlyContinue |
            Where-Object { $_.ExecutablePath -and ([System.IO.Path]::GetFullPath($_.ExecutablePath) -eq $InstalledExe) }
        if ($RunningCasOS) {
            throw 'CasOS is currently running. Exit CasOS, then run the uninstaller again.'
        }
        Remove-Item -Path $InstalledExe -Force
        Write-Host "Removed $InstalledExe"
        $Found = $true
    } else {
        Write-Host "No CasOS binary at $InstalledExe"
    }

    # Drop only the install directory itself. Under the default layout its
    # parent is %LOCALAPPDATA%\CasOS, which is the data directory.
    if ((Test-Path $InstallDir) -and -not (Get-ChildItem -Force -Path $InstallDir)) {
        Remove-Item -Path $InstallDir -Force
    }

    $UserPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
    $PathEntries = @($UserPath -split ';' | Where-Object { $_ })
    $KeptEntries = @($PathEntries | Where-Object { $_.TrimEnd('\') -ne $InstallDir.TrimEnd('\') })
    if ($KeptEntries.Count -ne $PathEntries.Count) {
        [Environment]::SetEnvironmentVariable('PATH', ($KeptEntries -join ';'), 'User')
        Write-Host "Removed $InstallDir from your user PATH."
        $Found = $true
    }
    $env:PATH = (@($env:PATH -split ';' | Where-Object { $_ -and $_.TrimEnd('\') -ne $InstallDir.TrimEnd('\') }) -join ';')

    if (-not $Found) { Write-Host 'Nothing to uninstall.' }

    $DataDir = if ($env:LOCALAPPDATA) { Join-Path $env:LOCALAPPDATA 'CasOS' } else { $null }
    if ($DataDir -and (Test-Path $DataDir)) {
        Write-Host ''
        Write-Host 'Your CasOS data was left in place at:'
        Write-Host "  $DataDir"
        Write-Host 'It holds the SQLite databases, the control-plane TLS material and the'
        Write-Host 'key that decrypts stored SSH credentials. Delete it only if you are'
        Write-Host 'certain you want that state gone:'
        Write-Host "  Remove-Item -Recurse -Force '$DataDir'"
    }
    return
}

if ($Repo -notmatch '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$') { throw "Invalid GitHub repository: $Repo" }

if ($Version -eq 'latest') {
    $ReleaseUrl = "https://github.com/$Repo/releases/latest/download"
} else {
    if ($Version -notmatch '^v[0-9A-Za-z._-]+$') { throw "Invalid release version: $Version" }
    $ReleaseUrl = "https://github.com/$Repo/releases/download/$Version"
}

$Architecture = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$ArchName = switch ($Architecture) {
    'AMD64' { 'x86_64' }
    'ARM64' { 'arm64' }
    default { throw "Unsupported architecture: $Architecture" }
}

$Filename = "casos_windows_${ArchName}.zip"
$TempDir = Join-Path $env:TEMP "casos_install_$([guid]::NewGuid().ToString('N'))"
$PendingExe = $null
New-Item -ItemType Directory -Path $TempDir | Out-Null

try {
    $ArchivePath = Join-Path $TempDir $Filename
    $ChecksumsPath = Join-Path $TempDir 'SHA256SUMS'
    Write-Host "Downloading CasOS $Version..."
    Invoke-WebRequest -Uri "$ReleaseUrl/$Filename" -OutFile $ArchivePath -UseBasicParsing
    Invoke-WebRequest -Uri "$ReleaseUrl/SHA256SUMS" -OutFile $ChecksumsPath -UseBasicParsing

    $ChecksumLine = Get-Content $ChecksumsPath |
        Where-Object { $_ -match "^[0-9a-fA-F]{64}\s+\*?$([regex]::Escape($Filename))$" } |
        Select-Object -First 1
    if (-not $ChecksumLine) { throw "Release checksum for $Filename was not found." }
    $Expected = ($ChecksumLine -split '\s+')[0].ToLowerInvariant()
    $Actual = (Get-FileHash -Algorithm SHA256 -Path $ArchivePath).Hash.ToLowerInvariant()
    if ($Actual -ne $Expected) { throw 'Download checksum verification failed.' }

    # The release ships the executable compressed, which cuts the download to
    # about a third. The archive holds that one file and nothing else.
    $ExtractDir = Join-Path $TempDir 'extracted'
    Expand-Archive -Path $ArchivePath -DestinationPath $ExtractDir -Force
    $ExePath = Join-Path $ExtractDir 'casos.exe'
    if (-not (Test-Path $ExePath)) { throw "$Filename did not contain casos.exe." }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $InstallDir = [System.IO.Path]::GetFullPath($InstallDir)
    $InstalledExe = Join-Path $InstallDir 'casos.exe'
    $PendingExe = Join-Path $InstallDir 'casos.new.exe'

    if (Test-Path $InstalledExe) {
        $RunningCasOS = Get-CimInstance Win32_Process -Filter "Name = 'casos.exe'" -ErrorAction SilentlyContinue |
            Where-Object { $_.ExecutablePath -and ([System.IO.Path]::GetFullPath($_.ExecutablePath) -eq $InstalledExe) }
        if ($RunningCasOS) {
            throw 'CasOS is currently running. Exit CasOS, then run the installer again to upgrade.'
        }
    }

    Copy-Item -Path $ExePath -Destination $PendingExe -Force
    Move-Item -Path $PendingExe -Destination $InstalledExe -Force
}
finally {
    Remove-Item -Recurse -Force $TempDir -ErrorAction SilentlyContinue
    if ($PendingExe) { Remove-Item -Force $PendingExe -ErrorAction SilentlyContinue }
}

$UserPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
$PathEntries = @($UserPath -split ';' | Where-Object { $_ })
if ($PathEntries -notcontains $InstallDir) {
    [Environment]::SetEnvironmentVariable('PATH', (@($PathEntries) + $InstallDir) -join ';', 'User')
    Write-Host "Added $InstallDir to your user PATH."
}
$env:PATH = "$env:PATH;$InstallDir"

Write-Host "CasOS $Version installed at $InstalledExe"
Write-Host "Run 'casos' and open http://localhost:9000"
