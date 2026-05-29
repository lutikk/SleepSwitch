// Package wintray draws SleepSwitch as a Windows system-tray app — a single
// icon in the notification area with a right-click menu and an optional
// status window.
package wintray

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/lutikk/SleepSwitch/assets"
	"github.com/lutikk/SleepSwitch/internal/applog"
	"github.com/lutikk/SleepSwitch/internal/updater"
	"github.com/lutikk/SleepSwitch/internal/winpower"
)

const (
	appTitle = "SleepSwitch"
	appID    = "com.luciferdennica.sleepswitch"
)

var (
	windowIcon = fyne.NewStaticResource("icon.png", assets.Icon)
	// Trays on Windows 11 render best from a small square icon — feeding the
	// full 1024x1024 master can silently fail to register the systray entry.
	trayIcon = fyne.NewStaticResource("tray_icon.png", assets.TrayIcon)
)

// Run boots the Fyne app, installs the system tray menu and a hidden status
// window. Blocks until the user picks Quit.
func Run(version string) {
	applog.Init(appTitle)
	log := applog.L()
	log.Printf("=== SleepSwitch %s starting (log: %s) ===", version, applog.Path())

	a := app.NewWithID(appID)
	a.SetIcon(windowIcon)

	w := a.NewWindow(appTitle)
	w.SetIcon(windowIcon)
	w.Resize(fyne.NewSize(320, 220))
	w.SetFixedSize(true)
	w.CenterOnScreen()
	w.SetCloseIntercept(func() { w.Hide() })

	t := &tray{app: a, win: w, version: version}
	t.buildWindow()
	t.installTray()
	t.refresh()

	// Always show the window on launch so the user has a visible UI even if
	// the systray icon fails to register (some Win11 builds throttle tray
	// registration). Closing the window hides it back into the tray.
	w.Show()

	// Win11 hides all new tray icons in the overflow flyout by default. Try
	// to flip IsPromoted=1 in the registry so ours sits next to the clock.
	// Runs in a goroutine — polls until Windows has registered the icon.
	go promoteTrayIcon()

	// Auto-check for updates in the background a few seconds after launch.
	// If a newer version is published on GitHub, prompt the user; on confirm
	// we download the silent installer, run it, and exit so it can patch
	// our binary in place.
	go t.autoCheckUpdates()

	a.Run()
}

type tray struct {
	app       fyne.App
	win       fyne.Window
	version   string
	state     winpower.State
	statusDot *canvas.Circle
	statusLbl *widget.Label
	subLbl    *widget.Label
	toggleBtn *widget.Button
	menu      *fyne.Menu
	preventMI *fyne.MenuItem
	allowMI   *fyne.MenuItem
}

func (t *tray) buildWindow() {
	title := canvas.NewText(appTitle, color.NRGBA{R: 0xEA, G: 0xEA, B: 0xF2, A: 0xFF})
	title.TextSize = 22
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter

	t.statusDot = canvas.NewCircle(color.NRGBA{R: 0x6B, G: 0x72, B: 0x80, A: 0xFF})
	dot := container.New(&fixed{w: 14, h: 14}, t.statusDot)

	t.statusLbl = widget.NewLabelWithStyle("…", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	t.subLbl = widget.NewLabelWithStyle("проверяю состояние", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	row := container.New(layout.NewHBoxLayout(), layout.NewSpacer(), dot, t.statusLbl, layout.NewSpacer())

	t.toggleBtn = widget.NewButton("…", t.onToggle)
	t.toggleBtn.Importance = widget.HighImportance

	updateBtn := widget.NewButton("Проверить обновления", func() { go t.checkUpdatesManual() })

	footer := widget.NewLabelWithStyle("v"+t.version, fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	var secondaryRow fyne.CanvasObject = updateBtn
	if applog.Enabled {
		logBtn := widget.NewButton("Открыть лог", openLogFile)
		secondaryRow = container.New(layout.NewGridLayout(2), logBtn, updateBtn)
	}

	content := container.NewVBox(
		container.NewPadded(title),
		row,
		t.subLbl,
		layout.NewSpacer(),
		container.NewPadded(t.toggleBtn),
		container.NewPadded(secondaryRow),
		footer,
	)
	t.win.SetContent(container.NewPadded(content))
}

func openLogFile() {
	path := applog.Path()
	if path == "" {
		return
	}
	applog.L().Printf("openLogFile: %s", path)
	cmd := exec.Command("notepad.exe", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	_ = cmd.Start()
}

func (t *tray) installTray() {
	desk, ok := t.app.(desktop.App)
	if !ok {
		// No tray support — fall back to showing the window.
		t.win.Show()
		return
	}

	t.preventMI = fyne.NewMenuItem("Запретить сон", func() { go t.toggleTo(true) })
	t.allowMI = fyne.NewMenuItem("Разрешить сон", func() { go t.toggleTo(false) })
	showMI := fyne.NewMenuItem("Показать окно", func() { t.win.Show(); t.win.RequestFocus() })
	updateMI := fyne.NewMenuItem("Проверить обновления", func() { go t.checkUpdatesManual() })

	items := []*fyne.MenuItem{t.preventMI, t.allowMI, fyne.NewMenuItemSeparator(), showMI, updateMI}
	if applog.Enabled {
		items = append(items, fyne.NewMenuItem("Открыть лог", openLogFile))
	}
	t.menu = fyne.NewMenu(appTitle, items...)
	desk.SetSystemTrayMenu(t.menu)
	desk.SetSystemTrayIcon(trayIcon)
}

func (t *tray) refresh() {
	state, err := winpower.Current()
	if err != nil {
		applog.L().Printf("tray.refresh: winpower.Current error: %v", err)
		t.subLbl.SetText(fmt.Sprintf("ошибка: %v", err))
		return
	}
	applog.L().Printf("tray.refresh: state=%v", state)
	t.applyState(state)
}

func (t *tray) applyState(state winpower.State) {
	t.state = state
	switch state {
	case winpower.Allowed:
		t.statusDot.FillColor = color.NRGBA{R: 0x22, G: 0xC5, B: 0x5E, A: 0xFF}
		t.statusLbl.SetText("Сон разрешён")
		t.subLbl.SetText("при простое или закрытии крышки система уйдёт в сон")
		t.toggleBtn.SetText("Запретить сон")
	case winpower.Prevented:
		t.statusDot.FillColor = color.NRGBA{R: 0xF5, G: 0x9E, B: 0x0B, A: 0xFF}
		t.statusLbl.SetText("Сон запрещён")
		t.subLbl.SetText("при закрытии крышки ноутбук продолжит работать")
		t.toggleBtn.SetText("Разрешить сон")
	default:
		t.statusDot.FillColor = color.NRGBA{R: 0x6B, G: 0x72, B: 0x80, A: 0xFF}
		t.statusLbl.SetText("Неизвестно")
		t.subLbl.SetText("powercfg вернул неожиданный ответ")
		t.toggleBtn.SetText("Запретить сон")
	}
	t.statusDot.Refresh()
	if t.preventMI != nil {
		t.preventMI.Disabled = (state == winpower.Prevented)
		t.allowMI.Disabled = (state == winpower.Allowed)
		if t.menu != nil {
			t.menu.Refresh()
		}
	}
}

func (t *tray) onToggle() {
	prevent := t.state != winpower.Prevented
	go t.toggleTo(prevent)
}

func (t *tray) toggleTo(prevent bool) {
	fyne.Do(func() {
		t.toggleBtn.Disable()
		t.subLbl.SetText("применяю изменения…")
	})
	err := winpower.Set(prevent)
	time.Sleep(150 * time.Millisecond)
	fyne.Do(func() {
		t.toggleBtn.Enable()
		if err != nil && !winpower.IsUserCanceled(err) {
			dialog.ShowError(fmt.Errorf("не удалось изменить настройку: %w", err), t.win)
		}
		t.refresh()
	})
}

// autoCheckUpdates polls GitHub once on startup (after a short delay so the
// network is up and the UI is interactive) and prompts the user if there's a
// newer release.
func (t *tray) autoCheckUpdates() {
	time.Sleep(5 * time.Second)
	t.checkUpdates(false)
}

// checkUpdatesManual is the entry point for the "Проверить обновления"
// button/menu item — same flow as auto, but always reports the outcome
// (including "уже актуальная версия").
func (t *tray) checkUpdatesManual() {
	t.checkUpdates(true)
}

func (t *tray) checkUpdates(notifyIfNone bool) {
	log := applog.L()
	log.Printf("checkUpdates: manualNotify=%v", notifyIfNone)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	avail, err := updater.CheckWindows(ctx, t.version)
	if err != nil {
		log.Printf("checkUpdates: error: %v", err)
		if notifyIfNone {
			fyne.Do(func() {
				dialog.ShowError(fmt.Errorf("не удалось проверить обновления: %w", err), t.win)
			})
		}
		return
	}
	if avail == nil {
		log.Printf("checkUpdates: up to date")
		if notifyIfNone {
			fyne.Do(func() {
				dialog.ShowInformation("Обновления", "У тебя уже последняя версия — "+t.version+".", t.win)
			})
		}
		return
	}
	log.Printf("checkUpdates: new version %s available at %s", avail.Version, avail.ExeURL)

	fyne.Do(func() {
		msg := fmt.Sprintf("Доступна версия %s. Скачать и установить сейчас?\nПриложение перезапустится автоматически.", avail.Version)
		dialog.NewConfirm("Доступно обновление", msg, func(yes bool) {
			if !yes {
				return
			}
			go t.applyUpdate(avail)
		}, t.win).Show()
	})
}

func (t *tray) applyUpdate(avail *updater.WindowsAvailable) {
	log := applog.L()
	fyne.Do(func() { t.subLbl.SetText("скачиваю обновление…") })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := updater.ApplyWindows(ctx, avail.ExeURL, t.version); err != nil {
		log.Printf("applyUpdate: error: %v", err)
		fyne.Do(func() {
			dialog.ShowError(fmt.Errorf("не удалось установить обновление: %w", err), t.win)
			t.refresh()
		})
		return
	}
	log.Printf("applyUpdate: installer launched, exiting")
	// Give the installer a moment to spawn and start its UAC dance, then
	// exit so it can replace our binary.
	time.Sleep(800 * time.Millisecond)
	os.Exit(0)
}

type fixed struct{ w, h float32 }

func (f *fixed) MinSize(_ []fyne.CanvasObject) fyne.Size { return fyne.NewSize(f.w, f.h) }
func (f *fixed) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Resize(fyne.NewSize(f.w, f.h))
		o.Move(fyne.NewPos((size.Width-f.w)/2, (size.Height-f.h)/2))
	}
}
