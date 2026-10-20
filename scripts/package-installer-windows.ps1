param([string]$Version = '')
$ErrorActionPreference = 'Stop'
if ($env:OS -ne 'Windows_NT') { throw 'Build the Windows installer on Windows.' }
$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$CanonicalVersion = (Get-Content -Raw (Join-Path $Root 'VERSION')).Trim()
if (-not $Version) { $Version = $CanonicalVersion }
if ($Version -cne $CanonicalVersion) { throw 'Installer version must match VERSION.' }
$Compiler = Join-Path ${env:ProgramFiles(x86)} 'Inno Setup 6\ISCC.exe'
if (-not (Test-Path -LiteralPath $Compiler)) { throw 'Inno Setup 6 is required (preinstalled on windows-2025 runners).' }
if (-not (Test-Path -LiteralPath (Join-Path $Root 'bin/pr-nanny.exe'))) { throw 'Build the production executable first.' }
$Dist = Join-Path $Root 'dist'
$Work = Join-Path $Dist ('.installer-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $Work | Out-Null
try {
  $Bootstrapper = Join-Path $Work 'MicrosoftEdgeWebview2Setup.exe'
  Invoke-WebRequest -Uri 'https://go.microsoft.com/fwlink/p/?LinkId=2124703' -OutFile $Bootstrapper
  $Signature = Get-AuthenticodeSignature -LiteralPath $Bootstrapper
  if ($Signature.Status -ne 'Valid' -or $Signature.SignerCertificate.Subject -notmatch '(^|,\s*)O=Microsoft Corporation(,|$)') {
    throw 'The WebView2 bootstrapper must have a valid Microsoft signature.'
  }
  $NumericVersion = ($Version -split '[-+]')[0]
  & $Compiler "/DAppVersion=$Version" "/DNumericVersion=$NumericVersion" "/DSourceRoot=$Root" "/DOutputDirectory=$Dist" "/DBootstrapper=$Bootstrapper" (Join-Path $Root 'build/windows/installer.iss')
  if ($LASTEXITCODE -ne 0) { throw 'Windows installer compilation failed.' }
  $Installer = Join-Path $Dist "pr-nanny-$Version-windows-x64-setup.exe"
  if (-not (Test-Path -LiteralPath $Installer)) { throw 'Expected installer was not created.' }
  $InstallerVersion = [Diagnostics.FileVersionInfo]::GetVersionInfo($Installer)
  if ($InstallerVersion.ProductVersion -cne $Version -or $InstallerVersion.FileVersion -cne "$NumericVersion.0") {
    throw 'Installer version resources do not match VERSION.'
  }
  $Hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $Installer).Hash.ToLowerInvariant()
  Set-Content -Encoding ascii -NoNewline -Path "$Installer.sha256" -Value "$Hash  $(Split-Path -Leaf $Installer)`n"
  Write-Output "Created $Installer"
} finally {
  Remove-Item -LiteralPath $Work -Recurse -Force
}
