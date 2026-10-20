; The installer runs per-user and never removes preferences or credentials.
[Setup]
AppId=io.github.alexcatdad.pr-nanny
AppName=PR Nanny
AppVersion={#AppVersion}
AppPublisher=alexcatdad
AppPublisherURL=https://github.com/alexcatdad/pr-nanny
DefaultDirName={localappdata}\Programs\PR Nanny
DefaultGroupName=PR Nanny
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible and not arm64
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
OutputDir={#OutputDirectory}
OutputBaseFilename=pr-nanny-{#AppVersion}-windows-x64-setup
SetupIconFile={#SourceRoot}\build\windows\icon.ico
UninstallDisplayIcon={app}\pr-nanny.exe
VersionInfoVersion={#NumericVersion}.0
VersionInfoProductVersion={#NumericVersion}.0
VersionInfoProductTextVersion={#AppVersion}
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
CloseApplications=yes
RestartApplications=no

[Files]
Source: "{#SourceRoot}\bin\pr-nanny.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceRoot}\LICENSE"; DestDir: "{app}"
Source: "{#Bootstrapper}"; DestName: "MicrosoftEdgeWebview2Setup.exe"; Flags: dontcopy

[Icons]
Name: "{group}\PR Nanny"; Filename: "{app}\pr-nanny.exe"

[Run]
Filename: "{app}\pr-nanny.exe"; Description: "Start PR Nanny"; Flags: nowait postinstall skipifsilent

[Code]
function HasWebView2: Boolean;
var
  Version: String;
  Key: String;
begin
  Key := 'Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}';
  Result := (RegQueryStringValue(HKLM32, Key, 'pv', Version) and
    (Version <> '') and (Version <> '0.0.0.0'));
  if not Result then
    Result := (RegQueryStringValue(HKCU32, Key, 'pv', Version) and
      (Version <> '') and (Version <> '0.0.0.0'));
  if not Result then
    Result := (RegQueryStringValue(HKCU64, Key, 'pv', Version) and
      (Version <> '') and (Version <> '0.0.0.0'));
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  ExitCode: Integer;
begin
  Result := '';
  if HasWebView2 then Exit;
  WizardForm.StatusLabel.Caption := 'Installing Microsoft Edge WebView2 Runtime...';
  ExtractTemporaryFile('MicrosoftEdgeWebview2Setup.exe');
  if not Exec(ExpandConstant('{tmp}\MicrosoftEdgeWebview2Setup.exe'),
    '/silent /install', '', SW_HIDE, ewWaitUntilTerminated, ExitCode) then
    Result := 'Could not start the WebView2 installer. Please retry setup.'
  else if (ExitCode <> 0) or not HasWebView2 then
    Result := 'WebView2 could not be installed. Check your internet connection and retry setup.';
end;
