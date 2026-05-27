package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

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