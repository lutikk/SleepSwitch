// Package updater checks GitHub Releases for a newer SleepSwitch.app and
// applies it in place by replacing the running .app bundle, then relaunching.
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
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const releasesURL = "https://api.github.com/repos/lutikk/SleepSwitch/releases/latest"

type Available struct {
	Version string
	DMGURL  string
	PageURL string
}

type releaseInfo struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// Check returns a non-nil Available when GitHub has a release newer than
// currentVersion. nil/nil means "up to date".
func Check(ctx context.Context, currentVersion string) (*Available, error) {
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

	var dmg string
	for _, a := range info.Assets {
		if strings.HasSuffix(strings.ToLower(a.Name), ".dmg") {
			dmg = a.BrowserDownloadURL
			break
		}
	}
	if dmg == "" {
		return nil, errors.New("в релизе нет .dmg файла")
	}
	return &Available{Version: latest, DMGURL: dmg, PageURL: info.HTMLURL}, nil
}

// Apply downloads the DMG, mounts it, replaces the running bundle with the new
// one and starts it via `open -n`. Caller is expected to exit after a
// successful return so the old process steps aside.
func Apply(ctx context.Context, dmgURL, currentVersion string) error {
	bundle, err := currentBundlePath()
	if err != nil {
		return fmt.Errorf("не нашёл текущий бандл: %w", err)
	}

	dmgPath, err := downloadToTemp(ctx, dmgURL, currentVersion)
	if err != nil {
		return fmt.Errorf("скачать DMG: %w", err)
	}
	defer os.Remove(dmgPath)

	mountPoint, err := mountDMG(dmgPath)
	if err != nil {
		return fmt.Errorf("смонтировать DMG: %w", err)
	}
	defer detachDMG(mountPoint)

	srcApp, err := findAppInDir(mountPoint)
	if err != nil {
		return fmt.Errorf("в DMG нет .app: %w", err)
	}
	if err := replaceBundle(srcApp, bundle); err != nil {
		return fmt.Errorf("обновить %s: %w", bundle, err)
	}
	if err := exec.Command("open", "-n", bundle).Start(); err != nil {
		return fmt.Errorf("перезапустить: %w", err)
	}
	return nil
}

func compareVersions(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		ai, bi := 0, 0
		if i < len(as) {
			ai, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bi, _ = strconv.Atoi(bs[i])
		}
		if ai != bi {
			if ai > bi {
				return 1
			}
			return -1
		}
	}
	return 0
}

func currentBundlePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	// /.../SleepSwitch.app/Contents/MacOS/sleepswitch -> /.../SleepSwitch.app
	dir := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if !strings.HasSuffix(dir, ".app") {
		return "", fmt.Errorf("не выглядит как .app: %s", dir)
	}
	return dir, nil
}

func downloadToTemp(ctx context.Context, url, ua string) (string, error) {
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

	f, err := os.CreateTemp("", "sleepswitch-*.dmg")
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

func mountDMG(path string) (string, error) {
	out, err := exec.Command("hdiutil", "attach", "-nobrowse", "-readonly", "-plist", path).Output()
	if err != nil {
		return "", err
	}
	idx := strings.Index(string(out), "/Volumes/")
	if idx < 0 {
		return "", errors.New("не нашёл точку монтирования в выводе hdiutil")
	}
	rest := string(out)[idx:]
	end := strings.IndexAny(rest, "<\n")
	if end < 0 {
		end = len(rest)
	}
	return strings.TrimSpace(rest[:end]), nil
}

func detachDMG(mountPoint string) {
	_ = exec.Command("hdiutil", "detach", mountPoint, "-quiet").Run()
}

func findAppInDir(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", errors.New("не нашёл .app в смонтированном DMG")
}

// replaceBundle swaps the .app at dst for the one at src. We move the old
// bundle aside first so a failed ditto can be rolled back. If the user lacks
// permission to write to dst's parent (rare on /Applications for admins), we
// fall back to osascript-with-admin.
func replaceBundle(src, dst string) error {
	tmpOld := dst + ".old"
	_ = os.RemoveAll(tmpOld)

	if err := exec.Command("/bin/mv", dst, tmpOld).Run(); err != nil {
		return privilegedReplace(src, dst)
	}
	if err := exec.Command("/usr/bin/ditto", src, dst).Run(); err != nil {
		_ = exec.Command("/bin/mv", tmpOld, dst).Run()
		return err
	}
	_ = os.RemoveAll(tmpOld)
	return nil
}

func privilegedReplace(src, dst string) error {
	script := fmt.Sprintf(
		`do shell script "/bin/rm -rf %[2]s && /usr/bin/ditto %[1]s %[2]s" with administrator privileges with prompt "SleepSwitch обновляется и хочет записать в %[2]s."`,
		shellQuote(src), shellQuote(dst),
	)
	return exec.Command("osascript", "-e", script).Run()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}