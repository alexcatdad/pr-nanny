param([Parameter(Mandatory=$true)][string]$Installer)
$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:OS -ne 'Windows_NT') { throw 'Run installer acceptance only on a disposable Windows Actions runner.' }
$UninstallKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\io.github.alexcatdad.pr-nanny_is1'
$Data = Join-Path ([Environment]::GetFolderPath('ApplicationData')) 'pr-nanny'
if ((Test-Path $UninstallKey) -or (Test-Path $Data)) { throw 'Refusing to alter existing PR Nanny installation or data.' }
$Work = Join-Path $env:RUNNER_TEMP ('pr-nanny-setup-' + [guid]::NewGuid().ToString('N'))
$Target = Join-Path $Work 'Application with spaces'
$Group = 'PR Nanny Setup Test ' + [guid]::NewGuid().ToString('N')
$ShortcutFolder = Join-Path ([Environment]::GetFolderPath('StartMenu')) "Programs\$Group"
New-Item -ItemType Directory -Path $Work, $Data | Out-Null
Set-Content -LiteralPath (Join-Path $Data 'preserve-test.txt') -Value 'synthetic-preference'
$Installed = $false
try {
  for ($Attempt = 0; $Attempt -lt 2; $Attempt++) {
    $Arguments = @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/SP-', "/DIR=`"$Target`"", "/GROUP=`"$Group`"")
    $Process = Start-Process -FilePath (Resolve-Path $Installer).Path -ArgumentList $Arguments -PassThru
    if (-not $Process.WaitForExit(120000)) { $Process.Kill(); throw 'Installer timed out.' }
    if ($Process.ExitCode -ne 0) { throw "Installer exited with $($Process.ExitCode)." }
    $Installed = $true
    $Binary = Join-Path $Target 'pr-nanny.exe'
    if (-not (Test-Path $Binary) -or -not (Test-Path $UninstallKey)) { throw 'Installer did not install/register the app.' }
    $Shortcut = (New-Object -ComObject WScript.Shell).CreateShortcut((Join-Path $ShortcutFolder 'PR Nanny.lnk'))
    if ($Shortcut.TargetPath -ne $Binary) { throw 'Start Menu shortcut targets the wrong executable.' }
    $ExpectedHash = (Get-FileHash -Algorithm SHA256 (Join-Path $PSScriptRoot '../bin/pr-nanny.exe')).Hash
    if ((Get-FileHash -Algorithm SHA256 $Binary).Hash -cne $ExpectedHash) { throw 'Installed binary differs from the production binary.' }
  }
  $Smoke = Start-Process -FilePath $Binary -ArgumentList '--smoke-test' -PassThru
  if (-not $Smoke.WaitForExit(90000)) { $Smoke.Kill(); throw 'Installed native IPC smoke timed out.' }
  if ($Smoke.ExitCode -ne 0) { throw 'Installed native IPC smoke failed.' }
  $Removal = Start-Process -FilePath (Join-Path $Target 'unins000.exe') -ArgumentList '/VERYSILENT /SUPPRESSMSGBOXES /NORESTART' -PassThru
  if (-not $Removal.WaitForExit(60000)) { $Removal.Kill(); throw 'Uninstaller timed out.' }
  if ($Removal.ExitCode -ne 0 -or (Test-Path $Binary) -or (Test-Path $UninstallKey) -or (Test-Path $ShortcutFolder)) { throw 'Uninstaller left application files, registration or shortcuts.' }
  $Installed = $false
  if ((Get-Content -Raw (Join-Path $Data 'preserve-test.txt')).Trim() -cne 'synthetic-preference') { throw 'Uninstaller changed user data.' }
  Write-Output 'Windows setup, reinstall, installed IPC and uninstall acceptance passed; user data preserved.'
} finally {
  if ($Installed -and (Test-Path (Join-Path $Target 'unins000.exe'))) {
    Start-Process -FilePath (Join-Path $Target 'unins000.exe') -ArgumentList '/VERYSILENT /SUPPRESSMSGBOXES /NORESTART' -Wait
  }
  Remove-Item -LiteralPath $Work, $Data -Recurse -Force -ErrorAction SilentlyContinue
}
