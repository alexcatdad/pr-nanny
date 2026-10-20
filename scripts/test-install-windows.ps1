param([Parameter(Mandatory=$true)][string]$Archive)
$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:OS -ne 'Windows_NT') { throw 'Shortcut installation probe requires a disposable Windows Actions runner.' }
$Folder = Join-Path ([Environment]::GetFolderPath('StartMenu')) 'Programs\PR Nanny'
if (Test-Path -LiteralPath $Folder) { throw 'Refusing to alter an existing PR Nanny shortcut folder.' }
$Work = Join-Path $env:RUNNER_TEMP ('pr-nanny-install-' + [guid]::NewGuid().ToString('N'))
try {
  Expand-Archive -LiteralPath $Archive -DestinationPath $Work
  & (Join-Path $Work 'install.ps1')
  $ShortcutPath = Join-Path $Folder 'PR Nanny.lnk'
  if (-not (Test-Path -LiteralPath $ShortcutPath)) { throw 'Installer did not create a Start Menu shortcut.' }
  $Shell = New-Object -ComObject WScript.Shell
  $Shortcut = $Shell.CreateShortcut($ShortcutPath)
  if ($Shortcut.TargetPath -ne (Join-Path $Work 'pr-nanny.exe')) { throw 'Shortcut targets the wrong executable.' }
  & (Join-Path $Work 'uninstall.ps1')
  if (Test-Path -LiteralPath $Folder) { throw 'Uninstaller did not remove the shortcut folder.' }
  Write-Output 'Windows package install/uninstall passed.'
} finally {
  if (Test-Path -LiteralPath $Folder) { Remove-Item -LiteralPath $Folder -Recurse -Force }
  if (Test-Path -LiteralPath $Work) { Remove-Item -LiteralPath $Work -Recurse -Force }
}
