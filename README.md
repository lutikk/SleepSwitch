# SleepSwitch

A tiny macOS app for **MacBooks** to allow or prevent system sleep when the lid is closed — one clear switch.

---

## English

### What It Does

SleepSwitch is a one-button GUI on top of the macOS `pmset` `disablesleep` setting:

- **Prevent Sleep** runs `pmset -a disablesleep 1` — your Mac stays awake even with the lid closed.
- **Allow Sleep** runs `pmset -a disablesleep 0` — your Mac goes back to its normal sleep schedule.

By default macOS asks for an administrator password on every toggle. Click **Set Up Passwordless** once — SleepSwitch writes a NOPASSWD rule into `/etc/sudoers.d/sleepswitch` (validated via `visudo`), and every subsequent toggle is instant, no password. You can revert from the same button at any time.

SleepSwitch also checks GitHub Releases at startup and via the **Check for Updates** button. If a newer version is available, it downloads the DMG, replaces itself, and relaunches — no manual reinstall.

### Who It's For

SleepSwitch is for anyone who needs the Mac to keep working while they walk away, and is tired of typing `sudo pmset -a disablesleep 1` every time.

- **Vibe-coders** — leave Claude Code, Cursor, Codex, Aider or any AI agent grinding through a long task overnight without the Mac dozing off mid-prompt.
- **ML / data folks** — local model training, fine-tuning, embeddings, big inference jobs that run for hours.
- **Heavy downloaders** — Steam / Epic game installs, huge model weights from Hugging Face, torrents, multi-GB Docker pulls.
- **Render / build engineers** — long video exports (Final Cut, DaVinci), 3D renders (Blender), Xcode/Gradle builds, CI runs on a self-hosted runner.
- **Streamers and presenters** — OBS, Zoom, Keynote sessions where a sleeping display is the worst-case scenario.
- **Home-server users** — Mac mini or old MacBook running as a local web/dev/Plex server with the lid shut.
- **Audio listeners** — Spotify, Apple Music, podcasts with the lid closed and headphones on.
- **Backup junkies** — full Time Machine, iCloud, Arq, Backblaze syncs that need uninterrupted hours.

In short: anyone who has ever cursed at a Mac for sleeping at the worst possible moment.

### Install

1. Download the latest `SleepSwitch-X.Y.Z.dmg` from the [Releases](https://github.com/lutikk/SleepSwitch/releases) page (universal binary — runs on Apple Silicon and Intel).
2. Open the DMG and drag **SleepSwitch** to the **Applications** folder.
3. First launch: right-click `SleepSwitch.app` in `/Applications` → **Open** (Gatekeeper warning, since the app is not Apple-signed).
4. Click the toggle. Enter your admin password when macOS asks.
5. Optional: click **Set Up Passwordless** to skip the password prompt on every future toggle.

### Build From Source

Requires Go 1.22+ and the Fyne toolchain (uses Cgo + system OpenGL on macOS).

```bash
go install fyne.io/tools/cmd/fyne@latest
fyne package -os darwin -name SleepSwitch --appID com.luciferdennica.sleepswitch -icon assets/icon.png -src ./cmd/sleepswitch
```

For a universal binary (Apple Silicon + Intel):

```bash
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 CC="clang -arch arm64"   go build -o build/ss-arm64 ./cmd/sleepswitch
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 CC="clang -arch x86_64"  go build -o build/ss-amd64 ./cmd/sleepswitch
lipo -create -output build/SleepSwitch.app/Contents/MacOS/sleepswitch build/ss-arm64 build/ss-amd64
```

### Windows

On Windows SleepSwitch lives in the system tray. Right-click the icon to **Prevent Sleep** / **Allow Sleep**, or open the status window. The toggle calls `powercfg /setacvalueindex SCHEME_CURRENT SUB_BUTTONS LIDACTION` for both AC and DC — equivalent to changing **Control Panel → Power Options → Choose what closing the lid does**. Windows will ask for elevation (UAC) on every toggle in this release; a Task Scheduler-based passwordless mode is planned.

Build on Windows:

```cmd
go install fyne.io/tools/cmd/fyne@latest
fyne package -os windows -name SleepSwitch -appID com.luciferdennica.sleepswitch ^
  -icon assets\icon.png -src .\cmd\sleepswitch -release
move SleepSwitch.exe build\
```

Then compile the installer with Inno Setup 6:

```cmd
"C:\Program Files (x86)\Inno Setup 6\ISCC.exe" installer\SleepSwitch.iss
```

The resulting `SleepSwitch-1.0.2-Setup.exe` lands in `build\` and offers a Welcome → License → Folder → Tasks (desktop shortcut, launch on login) → Install → Finish wizard, plus a proper entry in **Programs and Features**.

### License

MIT — see [LICENSE](LICENSE).

---

## Русский

Маленькое macOS-приложение для **MacBook'ов**: одной кнопкой разрешает или запрещает уход в сон при закрытой крышке.

### Что Это

SleepSwitch — это однокнопочный GUI над командой `pmset disablesleep`:

- **Запретить сон** — выполняет `pmset -a disablesleep 1`. Mac не засыпает даже с закрытой крышкой.
- **Разрешить сон** — выполняет `pmset -a disablesleep 0`. Возвращает обычный режим сна.

По умолчанию macOS спрашивает пароль администратора на каждое переключение. Кнопка **Настроить без пароля** один раз записывает правило в `/etc/sudoers.d/sleepswitch` (с валидацией через `visudo`), после чего любое переключение работает мгновенно, без пароля. Откатить можно той же кнопкой в любой момент.

SleepSwitch также сам проверяет GitHub Releases при старте и по кнопке **Проверить обновления**. Если есть новая версия — скачивает DMG, заменяет себя на свежий бандл и перезапускается. Никакой ручной переустановки.

### Кому Подойдёт

SleepSwitch — для всех, кому нужно, чтобы Mac продолжал работать, пока хозяин занят чем-то другим, и кому надоело каждый раз набирать `sudo pmset -a disablesleep 1`.

- **Вайбкодеры** — оставить Claude Code, Cursor, Codex, Aider или любой AI-агент молотить большую задачу на ночь, не боясь, что Mac уснёт посреди генерации.
- **ML / data-инженеры** — локальный тренинг моделей, fine-tuning, расчёт эмбеддингов, тяжёлый инференс на часы.
- **Качающие много** — установки Steam / Epic, веса моделей с Hugging Face, торренты, многогигабайтные docker pull'ы.
- **Видео и 3D** — длинные экспорты в Final Cut и DaVinci, рендеры в Blender, тяжёлые сборки Xcode/Gradle, локальный CI.
- **Стримеры и докладчики** — OBS, Zoom, Keynote — когда уснувший экран это худший из сценариев.
- **Домашний сервер** — Mac mini или старый MacBook в роли web/dev/Plex-сервера с закрытой крышкой.
- **Слушать музыку** — Spotify, Apple Music, подкасты с закрытой крышкой и наушниками.
- **Бэкап-маньяки** — полный Time Machine, iCloud, Arq, Backblaze, которым нужны часы без перерыва.

Если коротко: всем, кто хоть раз матерился на Mac за то, что тот уснул в самый неподходящий момент.

### Установка

1. Скачай свежий `SleepSwitch-X.Y.Z.dmg` со страницы [Releases](https://github.com/lutikk/SleepSwitch/releases) (universal binary — работает и на Apple Silicon, и на Intel).
2. Открой DMG и перетащи **SleepSwitch** в папку **Applications**.
3. Первый запуск: правый клик по `SleepSwitch.app` в `/Applications` → **Открыть** (Gatekeeper ругается, потому что приложение не подписано Apple).
4. Нажми кнопку. Введи пароль администратора, когда macOS попросит.
5. По желанию: нажми **Настроить без пароля**, чтобы дальше переключать сон без диалога пароля.

### Сборка Из Исходников

Требуется Go 1.22+ и тулчейн Fyne (Cgo + системный OpenGL на macOS).

```bash
go install fyne.io/tools/cmd/fyne@latest
fyne package -os darwin -name SleepSwitch --appID com.luciferdennica.sleepswitch -icon assets/icon.png -src ./cmd/sleepswitch
```

Universal binary (Apple Silicon + Intel):

```bash
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 CC="clang -arch arm64"   go build -o build/ss-arm64 ./cmd/sleepswitch
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 CC="clang -arch x86_64"  go build -o build/ss-amd64 ./cmd/sleepswitch
lipo -create -output build/SleepSwitch.app/Contents/MacOS/sleepswitch build/ss-arm64 build/ss-amd64
```

### Windows

На Windows SleepSwitch живёт в системном трее. Правый клик по иконке → **Запретить сон / Разрешить сон**, либо «Показать окно» для статуса. Кнопка вызывает `powercfg /setacvalueindex SCHEME_CURRENT SUB_BUTTONS LIDACTION` для AC и DC — это ровно то же, что **Панель управления → Электропитание → Действие при закрытии крышки**. В этом релизе Windows показывает UAC на каждое переключение; беспарольный режим через Task Scheduler — в планах.

Сборка на Windows:

```cmd
go install fyne.io/tools/cmd/fyne@latest
fyne package -os windows -name SleepSwitch -appID com.luciferdennica.sleepswitch ^
  -icon assets\icon.png -src .\cmd\sleepswitch -release
move SleepSwitch.exe build\
```

Затем установщик через Inno Setup 6:

```cmd
"C:\Program Files (x86)\Inno Setup 6\ISCC.exe" installer\SleepSwitch.iss
```

В `build\` появится `SleepSwitch-1.0.2-Setup.exe` с мастером Welcome → Лицензия → Папка → Опции (ярлык на рабочем столе, автозапуск при входе) → Установка → Готово, и нормальной записью в **Программы и компоненты**.

### Лицензия

MIT — см. [LICENSE](LICENSE).
