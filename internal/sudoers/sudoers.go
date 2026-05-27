// Package sudoers manages /etc/sudoers.d/sleepswitch — a NOPASSWD rule for
// `pmset -a disablesleep` so the user does not have to type a password on
// every toggle.
package sudoers

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strings"
)

const Path = "/etc/sudoers.d/sleepswitch"

var safeUsername = regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`)

func rule() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	if !safeUsername.MatchString(u.Username) {
		return "", fmt.Errorf("неподдерживаемое имя пользователя: %q", u.Username)
	}
	return fmt.Sprintf("%s ALL=(ALL) NOPASSWD: /usr/bin/pmset -a disablesleep *\n", u.Username), nil
}

// Configured probes whether `sudo -n pmset -a disablesleep 0` would succeed
// without prompting. We can't read /etc/sudoers.d/sleepswitch directly
// because it is mode 0440, root-only.
func Configured() bool {
	cmd := exec.Command("sudo", "-n", "-l", "/usr/bin/pmset", "-a", "disablesleep", "0")
	return cmd.Run() == nil
}

// Install writes the NOPASSWD rule via osascript-with-admin. visudo validates
// the file before it goes live, so a broken rule cannot lock the user out.
func Install() error {
	body, err := rule()
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp("", "sleepswitch-sudoers-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(body); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	script := fmt.Sprintf(
		`do shell script "/usr/sbin/visudo -cf %[1]s && /bin/mv %[1]s %[2]s && /usr/sbin/chown root:wheel %[2]s && /bin/chmod 440 %[2]s" with administrator privileges with prompt "SleepSwitch хочет один раз настроить переключение без пароля."`,
		shellQuote(tmpPath), shellQuote(Path),
	)
	return runOsascript(script)
}

func Remove() error {
	script := fmt.Sprintf(
		`do shell script "/bin/rm -f %s" with administrator privileges with prompt "SleepSwitch удаляет правило беспарольного pmset."`,
		shellQuote(Path),
	)
	return runOsascript(script)
}

func runOsascript(script string) error {
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

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}