param(
  [string]$Version = ''
)
$ErrorActionPreference = 'Stop'
$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$SourceVersion = (Get-Content -Raw -Path (Join-Path $Root 'VERSION')).Trim()
$Numeric = '(0|[1-9][0-9]*)'
$PrereleaseId = '(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)'
$BuildId = '[0-9A-Za-z-]+'
$SemVer = "^$Numeric\.$Numeric\.$Numeric(-$PrereleaseId(\.$PrereleaseId)*)?(\+$BuildId(\.$BuildId)*)?$"
if ($SourceVersion -notmatch $SemVer) { throw 'VERSION must contain one valid SemVer value.' }
if (-not $Version) { $Version = $SourceVersion }
if ($Version -cne $SourceVersion) { throw "Requested version does not match VERSION ($SourceVersion)." }
if ($env:OS -ne 'Windows_NT' -or $env:PROCESSOR_ARCHITECTURE -notin @('AMD64','ARM64')) { throw 'Run Windows packaging on a Windows host.' }
if ($env:PROCESSOR_ARCHITECTURE -ne 'AMD64') { throw 'Windows x64 packaging requires an x64 host.' }
$WebView = @(
  'HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients',
  'HKLM:\SOFTWARE\Microsoft\EdgeUpdate\Clients'
) | Where-Object { Test-Path $_ } | ForEach-Object { Get-ChildItem $_ -ErrorAction SilentlyContinue } | ForEach-Object { Get-ItemProperty $_.PSPath -ErrorAction SilentlyContinue } | Where-Object { $_.name -like '*WebView2 Runtime*' }
if (-not $WebView) {
  Write-Warning 'The target Windows machine must have the Microsoft Edge WebView2 Runtime installed.'
}

$Frontend = Join-Path $Root 'frontend'
if ($env:PR_NANNY_PREBUILT_FRONTEND -eq '1') {
  if (-not (Test-Path (Join-Path $Frontend 'dist/index.html'))) { throw 'Prebuilt frontend is missing frontend/dist/index.html.' }
} else {
  Push-Location $Frontend
  try { & pnpm run build; if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed.' } }
  finally { Pop-Location }
}

$BinDir = Join-Path $Root 'bin'
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
$Binary = Join-Path $BinDir 'pr-nanny.exe'
Push-Location $Root
try {
  & go -C tools/windowsresources run . -root ../..
  if ($LASTEXITCODE -ne 0) { throw 'Windows resource generation failed.' }
  $env:CGO_ENABLED = '1'
  $env:GOOS = 'windows'
  $env:GOARCH = 'amd64'
  & go build -tags production -trimpath -ldflags "-H windowsgui -s -w -X main.version=$Version" -o $Binary .
  if ($LASTEXITCODE -ne 0) { throw 'Windows production build failed.' }
}
finally { Pop-Location }

$Dist = Join-Path $Root 'dist'
New-Item -ItemType Directory -Force -Path $Dist | Out-Null
$Stage = Join-Path $Dist ('.package-' + [guid]::NewGuid().ToString('N'))
$Payload = Join-Path $Stage "pr-nanny-$Version-windows-x64"
New-Item -ItemType Directory -Force -Path $Payload | Out-Null
try {
  Copy-Item $Binary (Join-Path $Payload 'pr-nanny.exe')
  Copy-Item (Join-Path $Root 'LICENSE') (Join-Path $Payload 'LICENSE')
  @"
PR Nanny $Version for Windows x64

Requirements: Windows 10/11 x64 and Microsoft Edge WebView2 Runtime.
Run pr-nanny.exe. GitHub CLI authentication is supported; a personal access
token, when used, is stored in Windows Credential Manager.
Run install.ps1 to create a current-user Start Menu shortcut. Run
uninstall.ps1 to remove that shortcut. Settings and credentials are preserved.
This prerelease is unsigned; Windows may show a security prompt.
"@ | Set-Content -Encoding utf8 (Join-Path $Payload 'README.txt')
  @'
$ErrorActionPreference = 'Stop'
$Here = Split-Path -Parent $MyInvocation.MyCommand.Path
$Start = [Environment]::GetFolderPath('StartMenu')
$Folder = Join-Path $Start 'Programs\PR Nanny'
New-Item -ItemType Directory -Force -Path $Folder | Out-Null
$Shell = New-Object -ComObject WScript.Shell
$Shortcut = $Shell.CreateShortcut((Join-Path $Folder 'PR Nanny.lnk'))
$Shortcut.TargetPath = Join-Path $Here 'pr-nanny.exe'
$Shortcut.WorkingDirectory = $Here
$Shortcut.Save()
Write-Output 'Installed the current-user Start Menu shortcut.'
'@ | Set-Content -Encoding utf8 (Join-Path $Payload 'install.ps1')
  @'
$ErrorActionPreference = 'Stop'
$Folder = Join-Path ([Environment]::GetFolderPath('StartMenu')) 'Programs\PR Nanny'
Remove-Item -LiteralPath $Folder -Recurse -Force -ErrorAction SilentlyContinue
Write-Output 'Removed the current-user Start Menu shortcut. Settings and credentials were kept.'
'@ | Set-Content -Encoding utf8 (Join-Path $Payload 'uninstall.ps1')
  $Archive = Join-Path $Dist "pr-nanny-$Version-windows-x64.zip"
  if (Test-Path $Archive) { Remove-Item -LiteralPath $Archive -Force }
  Compress-Archive -Path (Join-Path $Payload '*') -DestinationPath $Archive
  $Hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $Archive).Hash.ToLowerInvariant()
  Set-Content -Encoding ascii -NoNewline -Path "$Archive.sha256" -Value "$Hash  $(Split-Path -Leaf $Archive)`n"
  Write-Output "Created $Archive"
}
finally { Remove-Item -LiteralPath $Stage -Recurse -Force -ErrorAction SilentlyContinue }

& (Join-Path $PSScriptRoot 'package-installer-windows.ps1') -Version $Version
