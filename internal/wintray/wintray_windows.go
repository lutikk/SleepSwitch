// Package wintray draws SleepSwitch as a Windows system-tray app — a single
// icon in the notification area with a right-click menu and an optional
// status window.
package wintray

import (
	"fmt"
	"image/color"
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
	"github.com/lutikk/SleepSwitch/internal/winpower"
)

const (
	appTitle = "SleepSwitch"
	appID    = "com.luciferdennica.sleepswitch"
)

var trayIcon = fyne.NewStaticResource("icon.png", assets.Icon)

// Run boots the Fyne app, installs the system tray menu and a hidden status
// window. Blocks until the user picks Quit.
func Run(version string) {
	a := app.NewWithID(appID)
	a.SetIcon(trayIcon)

	w := a.NewWindow(appTitle)
	w.SetIcon(trayIcon)
	w.Resize(fyne.NewSize(320, 220))
	w.SetFixedSize(true)
	w.CenterOnScreen()
	w.SetCloseIntercept(func() { w.Hide() })

	t := &tray{app: a, win: w, version: version}
	t.buildWindow()
	t.installTray()
	t.refresh()

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

	footer := widget.NewLabelWithStyle("v"+t.version+" — иконка живёт в трее", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	content := container.NewVBox(
		container.NewPadded(title),
		row,
		t.subLbl,
		layout.NewSpacer(),
		container.NewPadded(t.toggleBtn),
		footer,
	)
	t.win.SetContent(container.NewPadded(content))
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

	t.menu = fyne.NewMenu(appTitle, t.preventMI, t.allowMI, fyne.NewMenuItemSeparator(), showMI)
	desk.SetSystemTrayMenu(t.menu)
	desk.SetSystemTrayIcon(trayIcon)
}

func (t *tray) refresh() {
	state, err := winpower.Current()
	if err != nil {
		t.subLbl.SetText("не удалось прочитать настройку")
		return
	}
	t.applyState(state)
}

func (t *tray) applyState(state winpower.State) {
	t.state = state
	switch state {
	case winpower.Allowed:
		t.statusDot.FillColor = color.NRGBA{R: 0x22, G: 0xC5, B: 0x5E, A: 0xFF}
		t.statusLbl.SetText("Сон разрешён")
		t.subLbl.SetText("при закрытии крышки Mac уйдёт в сон")
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

type fixed struct{ w, h float32 }

func (f *fixed) MinSize(_ []fyne.CanvasObject) fyne.Size { return fyne.NewSize(f.w, f.h) }
func (f *fixed) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Resize(fyne.NewSize(f.w, f.h))
		o.Move(fyne.NewPos((size.Width-f.w)/2, (size.Height-f.h)/2))
	}
}
