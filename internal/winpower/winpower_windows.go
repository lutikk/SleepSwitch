// Package winpower toggles the Windows "Lid close action" power setting via
// powercfg. The "prevent sleep" mode sets the action to 0 (Do nothing) for
// both AC and DC; "allow sleep" sets it back to 1 (Sleep) — the Windows
// default for laptops.
package winpower

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

// SUB_BUTTONS / LIDACTION GUIDs are stable across Windows versions.
const (
	subButtons = "4f971e89-eebd-4455-a8de-9e59040e7347"
	lidAction  = "5ca83367-6e45-459f-a27b-476b1d01c936"
)

var lidIndexRe = regexp.MustCompile(`(?i)Current\s+(AC|DC)\s+Power\s+Setting\s+Index:\s*0x([0-9a-f]+)`)

func Current() (State, error) {
	out, err := exec.Command("powercfg", "/q", "SCHEME_CURRENT", subButtons, lidAction).CombinedOutput()
	if err != nil {
		return Unknown, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	matches := lidIndexRe.FindAllStringSubmatch(string(out), -1)
	if len(matches) < 2 {
		return Unknown, errors.New("не нашёл AC/DC LIDACTION в выводе powercfg")
	}
	ac, dc := -1, -1
	for _, m := range matches {
		v, err := strconv.ParseInt(m[2], 16, 32)
		if err != nil {
			return Unknown, err
		}
		switch strings.ToUpper(m[1]) {
		case "AC":
			ac = int(v)
		case "DC":
			dc = int(v)
		}
	}
	// "Prevent sleep" only when both adapter and battery are set to Do Nothing.
	if ac == 0 && dc == 0 {
		return Prevented, nil
	}
	return Allowed, nil
}

// Set switches the Lid close action and re-applies the active scheme. powercfg
// itself can fail without UAC, so we wrap the calls in `Start-Process -Verb
// runAs` — Windows pops one elevation prompt per toggle.
func Set(prevent bool) error {
	value := "1" // sleep
	if prevent {
		value = "0" // do nothing
	}
	cmdLine := fmt.Sprintf(
		"powercfg /setacvalueindex SCHEME_CURRENT %[1]s %[2]s %[3]s; "+
			"powercfg /setdcvalueindex SCHEME_CURRENT %[1]s %[2]s %[3]s; "+
			"powercfg /setactive SCHEME_CURRENT",
		subButtons, lidAction, value,
	)
	ps := fmt.Sprintf(
		"Start-Process -FilePath powershell.exe -ArgumentList '-NoProfile','-Command','%s' -Verb runAs -WindowStyle Hidden -Wait",
		strings.ReplaceAll(cmdLine, "'", "''"),
	)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", ps)
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
	msg := strings.ToLower(err.Error())
	// "The operation was canceled by the user" / Russian variants surface in
	// stderr when the user dismisses the UAC prompt.
	return strings.Contains(msg, "canceled by the user") ||
		strings.Contains(msg, "отменена пользователем") ||
		strings.Contains(msg, "the user refused") ||
		strings.Contains(msg, "0x800704c7")
}
