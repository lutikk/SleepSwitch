package main

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const appName = "SleepSwitch"

type sleepState int

const (
	sleepAllowed sleepState = iota
	sleepPrevented
	sleepUnknown
)

type ui struct {
	win        fyne.Window
	statusDot  *canvas.Circle
	statusText *widget.Label
	subtitle   *widget.Label
	toggle     *widget.Button
	refreshBtn *widget.Button
}

func main() {
	a := app.NewWithID("com.luciferdennica.sleepswitch")
	a.Settings().SetTheme(&sleepTheme{})

	w := a.NewWindow(appName)
	w.Resize(fyne.NewSize(380, 300))
	w.SetFixedSize(true)
	w.CenterOnScreen()

	u := &ui{win: w}
	u.build()
	u.refresh()

	w.ShowAndRun()
}

func (u *ui) build() {
	title := canvas.NewText("SleepSwitch", color.NRGBA{R: 0xEA, G: 0xEA, B: 0xF2, A: 0xFF})
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

	u.refreshBtn = widget.NewButtonWithIcon("Обновить", theme.ViewRefreshIcon(), u.refresh)

	buttons := container.New(layout.NewGridLayout(1), u.toggle, u.refreshBtn)

	content := container.NewVBox(
		container.NewPadded(title),
		statusRow,
		u.subtitle,
		layout.NewSpacer(),
		container.NewPadded(buttons),
	)

	u.win.SetContent(container.NewPadded(content))
}

func (u *ui) refresh() {
	state, err := currentState()
	if err != nil {
		dialog.ShowError(fmt.Errorf("не удалось прочитать настройку сна: %w", err), u.win)
		return
	}
	u.applyState(state)
}

func (u *ui) applyState(state sleepState) {
	switch state {
	case sleepAllowed:
		u.statusDot.FillColor = color.NRGBA{R: 0x22, G: 0xC5, B: 0x5E, A: 0xFF}
		u.statusText.SetText("Сон разрешён")
		u.subtitle.SetText("Mac засыпает по расписанию системы")
		u.toggle.SetText("Запретить сон")
		u.toggle.SetIcon(theme.MediaPauseIcon())
	case sleepPrevented:
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

func (u *ui) onToggle() {
	state, err := currentState()
	if err != nil {
		dialog.ShowError(err, u.win)
		return
	}
	prevent := state != sleepPrevented

	u.toggle.Disable()
	u.refreshBtn.Disable()
	u.subtitle.SetText("применяю изменения…")

	go func() {
		err := setDisableSleep(prevent)
		time.Sleep(150 * time.Millisecond)

		fyne.Do(func() {
			u.toggle.Enable()
			u.refreshBtn.Enable()
			if err != nil && !isCancel(err) {
				dialog.ShowError(fmt.Errorf("не удалось изменить настройку: %w", err), u.win)
			}
			u.refresh()
		})
	}()
}

func currentState() (sleepState, error) {
	out, err := exec.Command("pmset", "-g").CombinedOutput()
	if err != nil {
		return sleepUnknown, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	re := regexp.MustCompile(`(?mi)^\s*SleepDisabled\s+(\d+)\s*$`)
	match := re.FindSubmatch(out)
	if len(match) < 2 {
		return sleepAllowed, nil
	}
	value, err := strconv.Atoi(string(match[1]))
	if err != nil {
		return sleepUnknown, err
	}
	if value == 1 {
		return sleepPrevented, nil
	}
	return sleepAllowed, nil
}

func setDisableSleep(prevent bool) error {
	value := "0"
	if prevent {
		value = "1"
	}
	script := fmt.Sprintf(`do shell script "pmset -a disablesleep %s" with administrator privileges`, value)
	cmd := exec.Command("osascript", "-e", script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

func isCancel(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "User canceled") ||
		strings.Contains(msg, "(-128)") ||
		strings.Contains(msg, "Пользователь отменил")
}

type fixedSize struct{ w, h float32 }

func (f *fixedSize) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(f.w, f.h)
}

func (f *fixedSize) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Resize(fyne.NewSize(f.w, f.h))
		o.Move(fyne.NewPos((size.Width-f.w)/2, (size.Height-f.h)/2))
	}
}

type sleepTheme struct{}

func (sleepTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 0x14, G: 0x17, B: 0x21, A: 0xFF}
	case theme.ColorNameButton:
		return color.NRGBA{R: 0x1F, G: 0x24, B: 0x33, A: 0xFF}
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 0x1A, G: 0x1E, B: 0x29, A: 0xFF}
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 0x6D, G: 0x8B, B: 0xFF, A: 0xFF}
	case theme.ColorNameForeground:
		return color.NRGBA{R: 0xEA, G: 0xEA, B: 0xF2, A: 0xFF}
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 0x6B, G: 0x72, B: 0x80, A: 0xFF}
	case theme.ColorNameHover:
		return color.NRGBA{R: 0x2A, G: 0x30, B: 0x40, A: 0xFF}
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 0x1A, G: 0x1E, B: 0x29, A: 0xFF}
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 0x60}
	}
	return theme.DefaultTheme().Color(name, theme.VariantDark)
}

func (sleepTheme) Font(s fyne.TextStyle) fyne.Resource     { return theme.DefaultTheme().Font(s) }
func (sleepTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }
func (sleepTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNamePadding:
		return 8
	case theme.SizeNameInnerPadding:
		return 10
	case theme.SizeNameText:
		return 14
	}
	return theme.DefaultTheme().Size(n)
}