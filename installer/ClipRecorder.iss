#ifndef AppVersion
  #define AppVersion "1.1.0"
#endif

[Setup]
AppId={{F74C4059-8C9F-45B3-9B52-1EE8CD3853DB}
AppName=ClipRecorder
AppVersion={#AppVersion}
AppPublisher=ClipRecorder
DefaultDirName={localappdata}\Programs\ClipRecorder
DefaultGroupName=ClipRecorder
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
OutputDir=..\dist
OutputBaseFilename=ClipRecorder-Setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
UninstallDisplayIcon={app}\ClipRecorder.exe
CloseApplications=yes
CloseApplicationsFilter=ClipRecorder.exe
RestartApplications=no
SetupLogging=yes

[Languages]
Name: "chinesesimp"; MessagesFile: "ChineseSimplified.isl"

[Files]
Source: "..\dist\ClipRecorder.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\ClipRecorder"; Filename: "{app}\ClipRecorder.exe"; WorkingDir: "{app}"
Name: "{autodesktop}\ClipRecorder"; Filename: "{app}\ClipRecorder.exe"; WorkingDir: "{app}"

[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "ClipRecorder"; ValueData: """{app}\ClipRecorder.exe"" --startup"; Flags: uninsdeletevalue; Check: ShouldEnableAutoStart
Root: HKCU; Subkey: "Software\ClipRecorder"; ValueType: dword; ValueName: "AutoStart"; ValueData: "1"; Flags: createvalueifdoesntexist uninsdeletevalue uninsdeletekeyifempty

[Run]
Filename: "{app}\ClipRecorder.exe"; Description: "启动 ClipRecorder"; Flags: nowait postinstall skipifsilent

[Code]
function ShouldEnableAutoStart: Boolean;
var
  Enabled: Cardinal;
begin
  if RegQueryDWordValue(HKCU, 'Software\ClipRecorder', 'AutoStart', Enabled) then
    Result := Enabled <> 0
  else
    Result := True;
end;
