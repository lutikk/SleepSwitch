// Package pmset wraps the macOS `pmset` command for reading and changing the
// SleepDisabled flag.
package pmset

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type State int

const (
	Allowed State = iota
	Prevented
	Unknown
)

var sleepDisabledRe = regexp.MustCompile(`(?mi)^\s*SleepDisabled\s+(\d+)\s*$`)

func Current() (State, error) {
	out, err := exec.Command("pmset", "-g").CombinedOutput()
	if err != nil {
		return Unknown, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	m := sleepDisabledRe.FindSubmatch(out)
	if len(m) < 2 {
		return Allowed, nil
	}
	v, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return Unknown, err
	}
	if v == 1 {
		return Prevented, nil
	}
	return Allowed, nil
}

// SetDisabledSilent runs `sudo -n pmset -a disablesleep <v>`. It only succeeds
// when NOPASSWD is configured for the current user (see internal/sudoers).
func SetDisabledSilent(prevent bool) error {
	cmd := exec.Command("sudo", "-n", "/usr/bin/pmset", "-a", "disablesleep", flag(prevent))
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

// SetDisabledWithAdmin runs pmset via `osascript ... with administrator
// privileges`, which surfaces the system password dialog.
func SetDisabledWithAdmin(prevent bool) error {
	script := fmt.Sprintf(`do shell script "pmset -a disablesleep %s" with administrator privileges`, flag(prevent))
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

func IsUserCanceled(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "User canceled") ||
		strings.Contains(msg, "(-128)") ||
		strings.Contains(msg, "Пользователь отменил")
}

func flag(prevent bool) string {
	if prevent {
		return "1"
	}
	return "0"
}