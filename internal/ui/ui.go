// Package ui builds the SleepSwitch window and wires user actions to the
// pmset/sudoers/updater packages.
package ui

import (
	"context"
	"fmt"
	"image/color"
	"net/url"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/lutikk/SleepSwitch/internal/pmset"
	"github.com/lutikk/SleepSwitch/internal/sudoers"
	"github.com/lutikk/SleepSwitch/internal/updater"
)

const (
	appTitle = "SleepSwitch"
	appID    = "com.luciferdennica.sleepswitch"
)

// Run creates the Fyne app, wires it to a window and blocks until the user
// closes it. version is shown in the footer and sent as User-Agent.
func Run(version string) {
	a := app.NewWithID(appID)
	a.Settings().SetTheme(&sleepTheme{})

	w := a.NewWindow(appTitle)
	w.Resize(fyne.NewSize(380, 340))
	w.SetFixedSize(true)
	w.CenterOnScreen()

	u := &window{app: a, win: w, version: version}
	u.build()
	u.refresh()

	go u.checkForUpdate(false)

	w.ShowAndRun()
}

type window struct {
	app     fyne.App
	win     fyne.Window
	version string

	statusDot   *canvas.Circle
	statusText  *widget.Label
	subtitle    *widget.Label
	toggle      *widget.Button
	refreshBtn  *widget.Button
	passlessBtn *widget.Button
	updateBtn   *widget.Button
	footer      *widget.Label
}

func (u *window) build() {
	title := canvas.NewText(appTitle, color.NRGBA{R: 0xEA, G: 0xEA, B: 0xF2, A: 0xFF})
	title.TextSize = 26
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter

	u.statusDot = canvas.NewCircle(color.NRGBA{R: 0x6B, G: 0x72, B: 0x80, A: 0xFF})
	dotBox := container.New(&fixedSize{w: 14, h: 14}, u.statusDot)

	u.statusText = widget.NewLabelWithStyle("…", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	u.subtitle = widget.NewLabelWithStyle("проверяю состояние", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	statusRow := container.New(layout.NewHBoxLayout(),
		layout.NewSpacer(), dotBox, u.statusText, layout.NewSpacer())

	u.toggle = widget.NewButton("…", u.onToggle)
	u.toggle.Importance = widget.HighImportance

	u.refreshBtn = widget.NewButtonWithIcon("Обновить статус", theme.ViewRefreshIcon(), u.refresh)
	u.passlessBtn = widget.NewButtonWithIcon("Настроить без пароля", theme.SettingsIcon(), u.onPassless)
	u.updateBtn = widget.NewButtonWithIcon("Проверить обновления", theme.DownloadIcon(), func() { go u.checkForUpdate(true) })

	u.footer = widget.NewLabelWithStyle("v"+u.version, fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	rowSecondary := container.New(layout.NewGridLayout(2), u.passlessBtn, u.updateBtn)

	content := container.NewVBox(
		container.NewPadded(title),
		statusRow,
		u.subtitle,
		layout.NewSpacer(),
		container.NewPadded(u.toggle),
		container.NewPadded(u.refreshBtn),
		container.NewPadded(rowSecondary),
		u.footer,
	)

	u.win.SetContent(container.NewPadded(content))
}

func (u *window) refresh() {
	state, err := pmset.Current()
	if err != nil {
		dialog.ShowError(fmt.Errorf("не удалось прочитать настройку сна: %w", err), u.win)
		return
	}
	u.applyState(state)
	u.refreshPasslessButton()
}

func (u *window) applyState(state pmset.State) {
	switch state {
	case pmset.Allowed:
		u.statusDot.FillColor = color.NRGBA{R: 0x22, G: 0xC5, B: 0x5E, A: 0xFF}
		u.statusText.SetText("Сон разрешён")
		u.subtitle.SetText("Mac засыпает по расписанию системы")
		u.toggle.SetText("Запретить сон")
		u.toggle.SetIcon(theme.MediaPauseIcon())
	case pmset.Prevented:
		u.statusDot.FillColor = color.NRGBA{R: 0xF5, G: 0x9E, B: 0x0B, A: 0xFF}
		u.statusText.SetText("Сон запрещён")
		u.subtitle.SetText("Mac не уйдёт в сон, пока не разрешите")
		u.toggle.SetText("Разрешить сон")
		u.toggle.SetIcon(theme.MediaPlayIcon())
	default:
		u.statusDot.FillColor = color.NRGBA{R: 0x6B, G: 0x72, B: 0x80, A: 0xFF}
		u.statusText.SetText("Неизвестно")
		u.subtitle.SetText("не удалось определить текущий режим")
		u.toggle.SetText("Запретить сон")
		u.toggle.SetIcon(theme.MediaPauseIcon())
	}
	u.statusDot.Refresh()
}

func (u *window) refreshPasslessButton() {
	if sudoers.Configured() {
		u.passlessBtn.SetText("Отключить «без пароля»")
		u.passlessBtn.SetIcon(theme.LogoutIcon())
	} else {
		u.passlessBtn.SetText("Настроить без пароля")
		u.passlessBtn.SetIcon(theme.SettingsIcon())
	}
}

func (u *window) onToggle() {
	state, err := pmset.Current()
	if err != nil {
		dialog.ShowError(err, u.win)
		return
	}
	prevent := state != pmset.Prevented

	u.toggle.Disable()
	u.refreshBtn.Disable()
	u.subtitle.SetText("применяю изменения…")

	go func() {
		var setErr error
		if sudoers.Configured() {
			setErr = pmset.SetDisabledSilent(prevent)
			if setErr != nil {
				// Sudoers might have been removed externally — fall back.
				setErr = pmset.SetDisabledWithAdmin(prevent)
			}
		} else {
			setErr = pmset.SetDisabledWithAdmin(prevent)
		}
		time.Sleep(150 * time.Millisecond)

		fyne.Do(func() {
			u.toggle.Enable()
			u.refreshBtn.Enable()
			if setErr != nil && !pmset.IsUserCanceled(setErr) {
				dialog.ShowError(fmt.Errorf("не удалось изменить настройку: %w", setErr), u.win)
			}
			u.refresh()
		})
	}()
}

func (u *window) onPassless() {
	if sudoers.Configured() {
		dialog.ShowConfirm("Отключить «без пароля»",
			"Удалить /etc/sudoers.d/sleepswitch? После этого каждое переключение снова потребует пароль.",
			func(ok bool) {
				if !ok {
					return
				}
				go func() {
					err := sudoers.Remove()
					fyne.Do(func() {
						if err != nil && !pmset.IsUserCanceled(err) {
							dialog.ShowError(err, u.win)
						}
						u.refreshPasslessButton()
					})
				}()
			}, u.win)
		return
	}
	dialog.ShowConfirm("Настроить без пароля",
		"SleepSwitch добавит правило в /etc/sudoers.d/sleepswitch, чтобы переключать сон без пароля. Потребуется один раз ввести пароль администратора. Продолжить?",
		func(ok bool) {
			if !ok {
				return
			}
			go func() {
				err := sudoers.Install()
				fyne.Do(func() {
					if err != nil && !pmset.IsUserCanceled(err) {
						dialog.ShowError(err, u.win)
					}
					u.refreshPasslessButton()
				})
			}()
		}, u.win)
}

func (u *window) checkForUpdate(interactive bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	avail, err := updater.Check(ctx, u.version)
	if err != nil {
		if interactive {
			fyne.Do(func() {
				dialog.ShowError(fmt.Errorf("проверка обновлений: %w", err), u.win)
			})
		}
		return
	}
	if avail == nil {
		if interactive {
			fyne.Do(func() {
				dialog.ShowInformation("Обновлений нет", "У вас установлена последняя версия v"+u.version+".", u.win)
			})
		}
		return
	}

	fyne.Do(func() {
		u.promptUpdate(avail)
	})
}

func (u *window) promptUpdate(avail *updater.Available) {
	d := dialog.NewConfirm(
		"Доступна версия v"+avail.Version,
		"Установлена v"+u.version+". Скачать и установить v"+avail.Version+"?\nПо завершении SleepSwitch перезапустится.",
		func(ok bool) {
			if !ok {
				if avail.PageURL != "" {
					if pageURL, parseErr := url.Parse(avail.PageURL); parseErr == nil {
						_ = u.app.OpenURL(pageURL)
					}
				}
				return
			}
			go u.applyUpdate(avail)
		}, u.win)
	d.SetConfirmText("Обновить")
	d.SetDismissText("Открыть страницу релиза")
	d.Show()
}

func (u *window) applyUpdate(avail *updater.Available) {
	fyne.Do(func() {
		u.toggle.Disable()
		u.refreshBtn.Disable()
		u.passlessBtn.Disable()
		u.updateBtn.Disable()
		u.subtitle.SetText("скачиваю обновление…")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := updater.Apply(ctx, avail.DMGURL, u.version)

	fyne.Do(func() {
		u.toggle.Enable()
		u.refreshBtn.Enable()
		u.passlessBtn.Enable()
		u.updateBtn.Enable()
		if err != nil {
			u.subtitle.SetText("обновление не удалось")
			dialog.ShowError(err, u.win)
			return
		}
		u.subtitle.SetText("перезапускаюсь…")
	})
	if err == nil {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}
}