param([Parameter(Mandatory=$true)][string]$Archive)
$ErrorActionPreference = 'Stop'
$Version = (Get-Content -Raw -LiteralPath (Join-Path $PSScriptRoot '../VERSION')).Trim()
$Archive = (Resolve-Path $Archive).Path
$ChecksumFile = "$Archive.sha256"
if (-not (Test-Path -LiteralPath $ChecksumFile)) { throw "Missing SHA256 sidecar: $ChecksumFile" }
$Expected = ((Get-Content -Raw -LiteralPath $ChecksumFile).Trim() -split '\s+')[0].ToLowerInvariant()
if ($Expected -notmatch '^[0-9a-f]{64}$') { throw 'Invalid SHA256 sidecar.' }
$Actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $Archive).Hash.ToLowerInvariant()
if ($Actual -ne $Expected) { throw 'Archive SHA256 does not match.' }

Add-Type -AssemblyName System.IO.Compression.FileSystem
$Zip = [System.IO.Compression.ZipFile]::OpenRead($Archive)
$Temp = Join-Path ([IO.Path]::GetTempPath()) ('pr-nanny-verify-' + [guid]::NewGuid().ToString('N'))
try {
  $Names = @($Zip.Entries | ForEach-Object { $_.FullName })
  foreach ($Name in $Names) {
    if ([IO.Path]::IsPathRooted($Name) -or $Name -match '(^|[/\\])\.\.([/\\]|$)') { throw "Unsafe ZIP entry: $Name" }
  }
  foreach ($Required in @('pr-nanny.exe','LICENSE','README.txt','install.ps1','uninstall.ps1')) {
    if ($Names -notcontains $Required) { throw "Missing archive entry: $Required" }
  }
  [IO.Directory]::CreateDirectory($Temp) | Out-Null
  [System.IO.Compression.ZipFile]::ExtractToDirectory($Archive, $Temp)
  $Binary = Join-Path $Temp 'pr-nanny.exe'
  $Stream = [IO.File]::OpenRead($Binary)
  try {
    $Reader = [IO.BinaryReader]::new($Stream)
    if ($Reader.ReadUInt16() -ne 0x5A4D) { throw 'Executable is not a PE file.' }
    $Stream.Position = 0x3c
    $PEOffset = $Reader.ReadInt32()
    $Stream.Position = $PEOffset
    if ($Reader.ReadUInt32() -ne 0x00004550) { throw 'PE signature is invalid.' }
    if ($Reader.ReadUInt16() -ne 0x8664) { throw 'Executable is not Windows x64.' }
    $Stream.Position = $PEOffset + 24 + 68
    if ($Reader.ReadUInt16() -ne 2) { throw 'Executable is not a Windows GUI application.' }
  } finally { $Stream.Dispose() }
  $ResourceVersion = [Diagnostics.FileVersionInfo]::GetVersionInfo($Binary)
  if ($ResourceVersion.ProductVersion -cne $Version) { throw 'Windows ProductVersion does not match VERSION.' }
  $CoreVersion = ($Version -split '[-+]')[0] + '.0'
  if ($ResourceVersion.FileVersion -cne $CoreVersion) { throw 'Windows FileVersion does not match the numeric release version.' }
  $BuildInfo = (& go version -m $Binary 2>&1 | Out-String)
  if ($LASTEXITCODE -ne 0) { throw 'Unable to read Go executable build metadata.' }
  if ($BuildInfo -notmatch '-tags=production') { throw 'Production build tag is missing.' }
  if ($BuildInfo -match '(?i)-tags=[^\s]*(mcp|devtools|server)') { throw 'Forbidden development build tag found.' }
  $VersionOutput = Join-Path $Temp 'version.txt'
  $VersionProcess = Start-Process -FilePath $Binary -ArgumentList '--version' -RedirectStandardOutput $VersionOutput -PassThru
  if (-not $VersionProcess.WaitForExit(10000)) {
    $VersionProcess.Kill()
    throw 'Version probe timed out.'
  }
  if ($VersionProcess.ExitCode -ne 0 -or -not (Get-Content -Raw -LiteralPath $VersionOutput).StartsWith("PR Nanny $Version (")) {
    throw 'Runtime version does not match VERSION.'
  }
  Write-Host 'Windows package checks passed.'
} finally {
  $Zip.Dispose()
  if (Test-Path -LiteralPath $Temp) { Remove-Item -LiteralPath $Temp -Recurse -Force }
}
