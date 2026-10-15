# svcman installer for Windows — Author: Konstantinos Patronas <kpatronas@gmail.com>
#
# Downloads the release binary for this CPU from GitHub, checks it against SHA256SUMS,
# installs it as svcman.exe and adds the folder to your user PATH (no admin needed).
#
#   irm https://raw.githubusercontent.com/kpatr1981/svcman/main/install.ps1 | iex
#   .\install.ps1                         install to %LOCALAPPDATA%\Programs\svcman
#   .\install.ps1 -Dir C:\tools           another folder
#   .\install.ps1 -Version v0.1.0         a specific release (default: latest)
#   .\install.ps1 -Uninstall              remove it and its PATH entry

param(
    [string]$Dir = (Join-Path $env:LOCALAPPDATA 'Programs\svcman'),
    [string]$Version = 'latest',
    [switch]$Uninstall
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'   # Invoke-WebRequest is very slow with the progress bar
$repo = 'kpatr1981/svcman'
$target = Join-Path $Dir 'svcman.exe'
$onWindows = $env:OS -eq 'Windows_NT'

function Set-UserPath([string]$folder, [bool]$add) {
    if (-not $onWindows) { return }
    $cur = [Environment]::GetEnvironmentVariable('Path', 'User')
    $parts = @($cur -split ';' | Where-Object { $_ -and ($_.TrimEnd('\') -ne $folder.TrimEnd('\')) })
    if ($add) { $parts += $folder }
    [Environment]::SetEnvironmentVariable('Path', ($parts -join ';'), 'User')
}

if ($Uninstall) {
    if (Test-Path $target) { Remove-Item $target -Force; Write-Host "removed $target" }
    else { Write-Host "svcman is not installed in $Dir" }
    if ((Test-Path $Dir) -and -not (Get-ChildItem $Dir -Force)) { Remove-Item $Dir }
    Set-UserPath $Dir $false
    return
}

# Windows PowerShell 5.1 may default to TLS 1.0, which GitHub refuses.
try { [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12 } catch {}

$arch = 'amd64'
try {
    if ([Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq 'Arm64') { $arch = 'arm64' }
} catch {
    if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64' -or $env:PROCESSOR_ARCHITEW6432 -eq 'ARM64') { $arch = 'arm64' }
}
$asset = "svcman-windows-$arch.exe"
if ($Version -eq 'latest') { $base = "https://github.com/$repo/releases/latest/download" }
else { $base = "https://github.com/$repo/releases/download/$Version" }

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("svcman-" + [Guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    Write-Host "downloading $asset ($Version)"
    Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile (Join-Path $tmp $asset)
    Invoke-WebRequest -UseBasicParsing -Uri "$base/SHA256SUMS" -OutFile (Join-Path $tmp 'SHA256SUMS')

    $want = (Get-Content (Join-Path $tmp 'SHA256SUMS') | Where-Object { ($_ -split '\s+')[1] -eq $asset } |
             ForEach-Object { ($_ -split '\s+')[0] }) | Select-Object -First 1
    $got = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $asset)).Hash.ToLower()
    if (-not $want -or $want.ToLower() -ne $got) { throw "checksum mismatch for $asset (want '$want', got '$got')" }
    Write-Host "checksum ok"

    New-Item -ItemType Directory -Path $Dir -Force | Out-Null
    Move-Item -Force (Join-Path $tmp $asset) $target
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
Write-Host "installed $target"

Set-UserPath $Dir $true
if ($onWindows -and (($env:Path -split ';') -notcontains $Dir)) { $env:Path = "$env:Path;$Dir" }   # this session too
if ($onWindows) { & $target --version | Select-Object -First 1 }

if (-not (Get-Command ssh -ErrorAction SilentlyContinue)) {
    Write-Host "note: svcman needs the OpenSSH client: Settings > System > Optional features > OpenSSH Client"
}
if ($onWindows) { Write-Host "Open a new terminal for the PATH change to apply everywhere." }
