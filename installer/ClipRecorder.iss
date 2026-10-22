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
CloseApplications=no
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
Filename: "{app}\ClipRecorder.exe"; Description: "启动 ClipRecorder"; Flags: nowait postinstall

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

{ A tray app cannot be closed by closing its windows, so a running copy is
  asked to quit through its own command line, which lets an active recording
  finish saving. Versions without the switch are closed forcibly afterwards. }
function ClipRecorderRunning: Boolean;
var
  ExitCode: Integer;
  TaskFile: String;
  Output: AnsiString;
begin
  TaskFile := ExpandConstant('{tmp}\cliprecorder-tasklist.txt');
  DeleteFile(TaskFile);
  Exec(ExpandConstant('{cmd}'), '/c tasklist /NH /FI "IMAGENAME eq ClipRecorder.exe" > "' + TaskFile + '"',
    '', SW_HIDE, ewWaitUntilTerminated, ExitCode);
  Result := False;
  if FileExists(TaskFile) then
  begin
    if LoadStringFromFile(TaskFile, Output) then
      Result := Pos('ClipRecorder.exe', String(Output)) > 0;
    DeleteFile(TaskFile);
  end;
  Log('ClipRecorder running: ' + IntToStr(Ord(Result)) + ' (tasklist exit ' + IntToStr(ExitCode) + ')');
end;

procedure CloseRunningClipRecorder;
var
  ExitCode, Attempt: Integer;
  Exe: String;
begin
  Exe := ExpandConstant('{app}\ClipRecorder.exe');
  if not FileExists(Exe) then
    Exit;
  if not ClipRecorderRunning then
    Exit;
  if not WizardSilent then
    WizardForm.StatusLabel.Caption := '正在关闭正在运行的 ClipRecorder…';
  Log('Asking ' + Exe + ' to quit');
  Exec(Exe, '--quit', '', SW_HIDE, ewWaitUntilTerminated, ExitCode);
  for Attempt := 1 to 10 do
  begin
    Sleep(500);
    if not ClipRecorderRunning then
      Exit;
  end;
  Log('Forcing ClipRecorder to close');
  Exec(ExpandConstant('{cmd}'), '/c taskkill /F /IM ClipRecorder.exe >nul 2>&1',
    '', SW_HIDE, ewWaitUntilTerminated, ExitCode);
  Sleep(500);
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  CloseRunningClipRecorder;
  Result := '';
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssInstall then
    CloseRunningClipRecorder;
end;
