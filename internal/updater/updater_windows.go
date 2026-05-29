//go:build windows

package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// WindowsAvailable describes a Windows release waiting to be installed.
type WindowsAvailable struct {
	Version string
	ExeURL  string
	PageURL string
}

// CheckWindows hits GitHub Releases and returns a non-nil result if there's a
// newer version published with an `*-Setup.exe` asset. nil/nil = up to date.
func CheckWindows(ctx context.Context, currentVersion string) (*WindowsAvailable, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "SleepSwitch/"+currentVersion)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github вернул %s", resp.Status)
	}

	var info releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	latest := strings.TrimPrefix(info.TagName, "v")
	if latest == "" {
		return nil, errors.New("в релизе нет tag_name")
	}
	if compareVersions(latest, currentVersion) <= 0 {
		return nil, nil
	}

	var exeURL string
	for _, a := range info.Assets {
		name := strings.ToLower(a.Name)
		if strings.HasSuffix(name, "-setup.exe") || strings.Contains(name, "setup.exe") {
			exeURL = a.BrowserDownloadURL
			break
		}
	}
	if exeURL == "" {
		return nil, errors.New("в релизе нет Setup.exe ассета")
	}
	return &WindowsAvailable{Version: latest, ExeURL: exeURL, PageURL: info.HTMLURL}, nil
}

// ApplyWindows downloads the installer to %TEMP%, launches it with the
// silent + close-running-app flags, and returns. The caller must exit
// immediately so the installer can overwrite the running binary.
func ApplyWindows(ctx context.Context, exeURL, currentVersion string) error {
	path, err := downloadInstaller(ctx, exeURL, currentVersion)
	if err != nil {
		return fmt.Errorf("скачать инсталлер: %w", err)
	}

	// /VERYSILENT — no UI; /SUPPRESSMSGBOXES — no info popups;
	// /CLOSEAPPLICATIONS — Setup will close the running SleepSwitch.exe;
	// /RESTARTAPPLICATIONS — relaunch it after install.
	cmd := exec.Command(path, "/VERYSILENT", "/SUPPRESSMSGBOXES", "/CLOSEAPPLICATIONS", "/RESTARTAPPLICATIONS", "/NORESTART")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("запустить инсталлер: %w", err)
	}
	return nil
}

func downloadInstaller(ctx context.Context, url, ua string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "SleepSwitch/"+ua)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %s", resp.Status)
	}

	f, err := os.CreateTemp("", "sleepswitch-setup-*.exe")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	f.Close()
	return f.Name(), nil
}
