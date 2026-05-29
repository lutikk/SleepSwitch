package wintray

import (
	"os"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/lutikk/SleepSwitch/internal/applog"
)

// Windows 11 (especially 22H2+) hides all newly-registered tray icons in the
// overflow flyout by default. We promote ours by flipping IsPromoted=1 in
// HKCU\Control Panel\NotifyIconSettings\<hash>, which is the key Windows
// reads to decide whether an icon is "always shown" or hidden in overflow.
//
// The entry doesn't exist until Windows sees our Shell_NotifyIcon call, so
// we poll the registry for our executable path and update when found.

const notifyIconSettingsPath = `Control Panel\NotifyIconSettings`

// promoteTrayIcon runs in a goroutine after the app has had a chance to
// register its tray icon. Polls until our entry shows up (or times out),
// then writes IsPromoted=1 and broadcasts a settings change.
func promoteTrayIcon() {
	log := applog.L()
	exePath, err := os.Executable()
	if err != nil {
		log.Printf("promoteTrayIcon: os.Executable failed: %v", err)
		return
	}
	log.Printf("promoteTrayIcon: searching for %s", exePath)

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if updated := tryUpdatePromoted(exePath); updated {
			log.Printf("promoteTrayIcon: IsPromoted=1 written, broadcasting")
			broadcastSettingChange()
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Printf("promoteTrayIcon: timed out — entry not found in HKCU\\%s", notifyIconSettingsPath)
}

func tryUpdatePromoted(targetExe string) bool {
	log := applog.L()
	root, err := registry.OpenKey(registry.CURRENT_USER, notifyIconSettingsPath, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		// Key doesn't exist on Win10 or older Win11 builds — nothing to do.
		return false
	}
	defer root.Close()

	subs, err := root.ReadSubKeyNames(-1)
	if err != nil {
		return false
	}

	for _, name := range subs {
		sk, err := registry.OpenKey(registry.CURRENT_USER, notifyIconSettingsPath+`\`+name, registry.QUERY_VALUE|registry.SET_VALUE)
		if err != nil {
			continue
		}
		exe, _, err := sk.GetStringValue("ExecutablePath")
		if err != nil {
			sk.Close()
			continue
		}
		if strings.EqualFold(exe, targetExe) {
			log.Printf("promoteTrayIcon: match in subkey %s -> %s", name, exe)
			if err := sk.SetDWordValue("IsPromoted", 1); err != nil {
				log.Printf("promoteTrayIcon: SetDWordValue failed: %v", err)
				sk.Close()
				return false
			}
			sk.Close()
			return true
		}
		sk.Close()
	}
	return false
}

func broadcastSettingChange() {
	user32 := windows.NewLazySystemDLL("user32.dll")
	procSendMessageTimeoutW := user32.NewProc("SendMessageTimeoutW")

	const (
		HWND_BROADCAST     uintptr = 0xFFFF
		WM_SETTINGCHANGE   uintptr = 0x001A
		SMTO_ABORTIFHUNG   uintptr = 0x0002
	)

	section, _ := windows.UTF16PtrFromString("TraySettings")
	var result uintptr
	procSendMessageTimeoutW.Call(
		HWND_BROADCAST,
		WM_SETTINGCHANGE,
		0,
		uintptr(unsafe.Pointer(section)),
		SMTO_ABORTIFHUNG,
		1000,
		uintptr(unsafe.Pointer(&result)),
	)
}
