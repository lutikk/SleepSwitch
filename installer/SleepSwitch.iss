; SleepSwitch — Inno Setup installer script.
;
; Build prerequisites (run on Windows):
;   1. Build the executable:
;        go install fyne.io/tools/cmd/fyne@latest
;        fyne package -os windows -name SleepSwitch -appID com.luciferdennica.sleepswitch ^
;          -icon ..\assets\icon.png -src ..\cmd\sleepswitch -release
;      The command produces SleepSwitch.exe next to itself.
;   2. Place SleepSwitch.exe into ..\build\ (or adjust SourceExe below).
;   3. Compile this script with Inno Setup 6 (ISCC.exe):
;        "C:\Program Files (x86)\Inno Setup 6\ISCC.exe" SleepSwitch.iss
;      Resulting installer lands in ..\build\SleepSwitch-<version>-Setup.exe.

#define MyAppName       "SleepSwitch"
#define MyAppVersion    "1.0.5"
#define MyAppPublisher  "lutikk"
#define MyAppURL        "https://github.com/lutikk/SleepSwitch"
#define MyAppExeName    "SleepSwitch.exe"
#define SourceExe       "..\build\SleepSwitch.exe"
#define SourceLicense   "..\LICENSE"

[Setup]
; A fixed GUID identifies the application across versions; never change it for
; the same product, otherwise upgrades become side-by-side installs.
AppId={{A4A85F2C-7C8E-4F19-9F1D-9A2C9C2F1D02}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppVerName={#MyAppName} {#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}/releases
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
AllowNoIcons=yes
LicenseFile={#SourceLicense}
OutputDir=..\build
OutputBaseFilename={#MyAppName}-{#MyAppVersion}-Setup
Compression=lzma2/ultra
SolidCompression=yes
WizardStyle=modern
ArchitecturesInstallIn64BitMode=x64compatible
ArchitecturesAllowed=x64compatible
; Let the user choose between per-user and per-machine install at runtime.
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
UninstallDisplayIcon={app}\{#MyAppExeName}
UninstallDisplayName={#MyAppName}
MinVersion=10.0
DisableProgramGroupPage=yes
; In-app updater calls the installer with /VERYSILENT /CLOSEAPPLICATIONS
; /RESTARTAPPLICATIONS — these flags need CloseApplications=yes so Setup
; closes the running SleepSwitch.exe before overwriting it.
CloseApplications=yes
RestartApplications=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "russian"; MessagesFile: "compiler:Languages\Russian.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked
Name: "startup"; Description: "Запускать SleepSwitch при входе в Windows"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "{#SourceExe}"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceLicense}"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{group}\{cm:UninstallProgram,{#MyAppName}}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon
Name: "{userstartup}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: startup

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#MyAppName}}"; Flags: nowait postinstall skipifsilent
