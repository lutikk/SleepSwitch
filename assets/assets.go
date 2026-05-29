// Package assets embeds static resources (icons, etc.) bundled with
// SleepSwitch so they can be referenced from any OS-specific build.
package assets

import _ "embed"

//go:embed icon.png
var Icon []byte

//go:embed tray_icon.png
var TrayIcon []byte
