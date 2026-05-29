// Package winpower toggles Windows sleep behavior in a way that works for
// both desktops and laptops, regardless of OEM-hidden power settings or the
// system locale.
//
// We manage two settings in tandem:
//   - SUB_SLEEP/STANDBYIDLE — system sleep idle timeout (seconds, 0 = never).
//     Always present and always writable on every Windows install.
//   - SUB_BUTTONS/LIDACTION — what happens on lid close (0 do nothing, 1
//     sleep, 2 hibernate, 3 shutdown). Often hidden by OEMs; we touch it on
//     a best-effort basis.
//
// Reads use the Win32 PowrProf API directly — no parsing of localized
// powercfg text. Writes go through an elevated powershell wrapper because
// modifying HKLM-rooted power schemes needs admin.
//
// As belt-and-suspenders, while "prevent sleep" is active we also hold a
// SetThreadExecutionState flag so the system stays awake even if writing
// the registry value silently failed (group policy, locked-down corporate
// images etc).
package winpower

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/lutikk/SleepSwitch/internal/applog"
)

// CREATE_NO_WINDOW prevents the child console process from flashing a window
// when the parent is a GUI app (Fyne builds with -H windowsgui).
const createNoWindow = 0x08000000

func hiddenCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd
}

type State int

const (
	Allowed State = iota
	Prevented
	Unknown
)

// GUIDs are stable across all Windows versions since Vista. The plain (no
// braces) string forms are what powercfg expects on its command line —
// powershell interprets `{...}` as a ScriptBlock, so braced GUIDs silently
// turn into no-ops when passed through `Start-Process -ArgumentList`.
const (
	subSleepStr           = "238c9fa8-0aad-41ed-83f4-97be242c8f20"
	settingStandbyIdleStr = "29f6c1db-86da-48c5-9fdb-f2b67b1f44da"
	subButtonsStr         = "4f971e89-eebd-4455-a8de-9e59040e7347"
	settingLidActionStr   = "5ca83367-6e45-459f-a27b-476b1d01c936"
)

var (
	subSleep           = mustGUID("{" + subSleepStr + "}")
	settingStandbyIdle = mustGUID("{" + settingStandbyIdleStr + "}")
	subButtons         = mustGUID("{" + subButtonsStr + "}")
	settingLidAction   = mustGUID("{" + settingLidActionStr + "}")
)

func mustGUID(s string) windows.GUID {
	g, err := windows.GUIDFromString(s)
	if err != nil {
		panic(err)
	}
	return g
}

// Lazy-loaded PowrProf entry points. NewLazySystemDLL resolves at first use,
// so the binary still loads on systems where the DLL is missing — we'll just
// error out gracefully.
var (
	powrprof                   = windows.NewLazySystemDLL("powrprof.dll")
	kernel32                   = windows.NewLazySystemDLL("kernel32.dll")
	procPowerGetActiveScheme   = powrprof.NewProc("PowerGetActiveScheme")
	procPowerReadACValueIndex  = powrprof.NewProc("PowerReadACValueIndex")
	procPowerReadDCValueIndex  = powrprof.NewProc("PowerReadDCValueIndex")
	procSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
	procLocalFree              = kernel32.NewProc("LocalFree")
)

// SetThreadExecutionState flags.
const (
	esContinuous       uint32 = 0x80000000
	esSystemRequired   uint32 = 0x00000001
	esDisplayRequired  uint32 = 0x00000002
)

// Default sleep timeouts to restore when we have no saved backup (e.g. user
// installs SleepSwitch on a system already configured to never sleep).
const (
	defaultStandbyAC uint32 = 1800 // 30 min on AC
	defaultStandbyDC uint32 = 900  // 15 min on battery
	defaultLidAC     uint32 = 1    // sleep
	defaultLidDC     uint32 = 1    // sleep
)

const (
	backupRegPath = `Software\SleepSwitch\Backup`
	stateRegPath  = `Software\SleepSwitch\State`
)

func getActiveScheme() (windows.GUID, error) {
	var ptr *windows.GUID
	ret, _, _ := procPowerGetActiveScheme.Call(0, uintptr(unsafe.Pointer(&ptr)))
	if ret != 0 {
		return windows.GUID{}, fmt.Errorf("PowerGetActiveScheme rc=%d", ret)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(ptr)))
	return *ptr, nil
}

func readIndex(proc *windows.LazyProc, scheme, sub, setting *windows.GUID) (uint32, error) {
	var v uint32
	ret, _, _ := proc.Call(
		0,
		uintptr(unsafe.Pointer(scheme)),
		uintptr(unsafe.Pointer(sub)),
		uintptr(unsafe.Pointer(setting)),
		uintptr(unsafe.Pointer(&v)),
	)
	if ret != 0 {
		return 0, fmt.Errorf("rc=%d", ret)
	}
	return v, nil
}

// Current returns the sleep state. Source of truth, in priority order:
//  1. STANDBYIDLE read via Win32 API — accurate, what the OS actually has.
//  2. Last user intent stored in HKCU — accurate to what the user wanted,
//     used when the API read fails (locked-down systems, missing DLL etc).
//  3. Allowed — the safe default.
func Current() (State, error) {
	log := applog.L()
	scheme, err := getActiveScheme()
	if err != nil {
		log.Printf("winpower.Current: getActiveScheme failed: %v — falling back to stored intent", err)
		return intentState(), nil
	}
	log.Printf("winpower.Current: active scheme = %s", scheme.String())

	sbAC, errAC := readIndex(procPowerReadACValueIndex, &scheme, &subSleep, &settingStandbyIdle)
	sbDC, errDC := readIndex(procPowerReadDCValueIndex, &scheme, &subSleep, &settingStandbyIdle)
	log.Printf("winpower.Current: STANDBYIDLE AC=%d (err=%v) DC=%d (err=%v)", sbAC, errAC, sbDC, errDC)
	if errAC != nil || errDC != nil {
		log.Printf("winpower.Current: API read failed — falling back to stored intent")
		return intentState(), nil
	}

	// Lid action — best-effort log, doesn't affect state decision.
	lidAC, lidACerr := readIndex(procPowerReadACValueIndex, &scheme, &subButtons, &settingLidAction)
	lidDC, lidDCerr := readIndex(procPowerReadDCValueIndex, &scheme, &subButtons, &settingLidAction)
	log.Printf("winpower.Current: LIDACTION AC=%d (err=%v) DC=%d (err=%v)", lidAC, lidACerr, lidDC, lidDCerr)

	if sbAC == 0 && sbDC == 0 {
		return Prevented, nil
	}
	return Allowed, nil
}

// Set toggles the sleep settings. On "prevent" we back up the current values
// to HKCU and write zeros; on "allow" we restore from backup (or fall back
// to sensible defaults if no backup exists yet).
func Set(prevent bool) error {
	log := applog.L()
	log.Printf("winpower.Set: prevent=%v", prevent)

	// Persist user intent first — ES flag and intent-based fallback will work
	// even if powercfg write fails afterwards.
	saveIntent(prevent)
	holdExecutionState(prevent)

	scheme, err := getActiveScheme()
	if err != nil {
		log.Printf("winpower.Set: getActiveScheme failed: %v — relying on ES flag only", err)
		return nil
	}

	var sbAC, sbDC, lidAC, lidDC uint32

	if prevent {
		// Snapshot current state before zeroing it, so we can restore later.
		sbAC, _ = readIndex(procPowerReadACValueIndex, &scheme, &subSleep, &settingStandbyIdle)
		sbDC, _ = readIndex(procPowerReadDCValueIndex, &scheme, &subSleep, &settingStandbyIdle)
		lidAC, _ = readIndex(procPowerReadACValueIndex, &scheme, &subButtons, &settingLidAction)
		lidDC, _ = readIndex(procPowerReadDCValueIndex, &scheme, &subButtons, &settingLidAction)

		// Don't overwrite backup with zeros if we're already in a prevented
		// state (e.g. user toggles twice). Defaults will rescue us on restore.
		if sbAC != 0 || sbDC != 0 {
			saveBackup(sbAC, sbDC, lidAC, lidDC)
		}
		if err := applyPowercfg(0, 0, 0, 0); err != nil {
			log.Printf("winpower.Set: applyPowercfg failed: %v — ES flag still active", err)
			return err
		}
	} else {
		var ok bool
		sbAC, sbDC, lidAC, lidDC, ok = loadBackup()
		if !ok {
			sbAC, sbDC = defaultStandbyAC, defaultStandbyDC
			lidAC, lidDC = defaultLidAC, defaultLidDC
		}
		log.Printf("winpower.Set: restoring STANDBYIDLE AC=%d DC=%d LIDACTION AC=%d DC=%d (fromBackup=%v)",
			sbAC, sbDC, lidAC, lidDC, ok)
		if err := applyPowercfg(sbAC, sbDC, lidAC, lidDC); err != nil {
			log.Printf("winpower.Set: applyPowercfg failed: %v — ES flag cleared, OS values unchanged", err)
			return err
		}
	}
	return nil
}

// saveIntent persists what the user asked for, regardless of whether we
// could actually write the OS settings.
func saveIntent(prevent bool) {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, stateRegPath, registry.SET_VALUE)
	if err != nil {
		applog.L().Printf("winpower.saveIntent: open key failed: %v", err)
		return
	}
	defer k.Close()
	var v uint32
	if prevent {
		v = 1
	}
	_ = k.SetDWordValue("PreventIntent", v)
}

func intentState() State {
	k, err := registry.OpenKey(registry.CURRENT_USER, stateRegPath, registry.QUERY_VALUE)
	if err != nil {
		return Allowed
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("PreventIntent")
	if err != nil {
		return Allowed
	}
	if v == 1 {
		return Prevented
	}
	return Allowed
}

// applyPowercfg runs powercfg under UAC elevation. We can't write power
// scheme values from a non-admin process — Microsoft locks the writes to
// HKLM. We stage a .bat file in %TEMP%, launch it elevated, redirect its
// output to a separate log file, then read that file back so the operator
// can see exactly what powercfg said.
func applyPowercfg(sbAC, sbDC, lidAC, lidDC uint32) error {
	log := applog.L()

	tmpDir := os.TempDir()
	batPath := filepath.Join(tmpDir, "sleepswitch-apply.bat")
	logPath := filepath.Join(tmpDir, "sleepswitch-apply.log")

	bat := fmt.Sprintf("@echo off\r\n"+
		"echo === SleepSwitch apply %s === > %q\r\n"+
		"powercfg /setacvalueindex SCHEME_CURRENT %[2]s %[3]s %[4]d >> %[1]q 2>&1\r\n"+
		"powercfg /setdcvalueindex SCHEME_CURRENT %[2]s %[3]s %[5]d >> %[1]q 2>&1\r\n"+
		"powercfg /setacvalueindex SCHEME_CURRENT %[6]s %[7]s %[8]d >> %[1]q 2>&1\r\n"+
		"powercfg /setdcvalueindex SCHEME_CURRENT %[6]s %[7]s %[9]d >> %[1]q 2>&1\r\n"+
		"powercfg /setactive SCHEME_CURRENT >> %[1]q 2>&1\r\n"+
		"echo === done === >> %[1]q\r\n",
		logPath, subSleepStr, settingStandbyIdleStr, sbAC, sbDC,
		subButtonsStr, settingLidActionStr, lidAC, lidDC,
	)
	if err := os.WriteFile(batPath, []byte(bat), 0o644); err != nil {
		return fmt.Errorf("write bat: %w", err)
	}
	_ = os.Remove(logPath)
	log.Printf("winpower.applyPowercfg: bat=%s logCapture=%s", batPath, logPath)

	// Elevated launch via Start-Process -Verb RunAs -Wait. cmd.exe runs our
	// .bat which has its own stdout redirection inside.
	ps := fmt.Sprintf(
		"$p = Start-Process -FilePath cmd.exe -ArgumentList '/c',%q -Verb RunAs -WindowStyle Hidden -Wait -PassThru; exit $p.ExitCode",
		batPath,
	)
	cmd := hiddenCmd("powershell.exe", "-NoProfile", "-Command", ps)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()

	capture, _ := os.ReadFile(logPath)
	log.Printf("winpower.applyPowercfg: exit=%v stderr=%q powercfg-output:\n%s",
		err, strings.TrimSpace(stderr.String()), strings.TrimSpace(string(capture)))

	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

// holdExecutionState flips the per-process "keep system awake" flag. When
// prevent=true we set ES_CONTINUOUS|ES_SYSTEM_REQUIRED|ES_DISPLAY_REQUIRED;
// when false we clear it back to ES_CONTINUOUS only. The flag is per-thread
// but ES_CONTINUOUS makes it persist until explicitly cleared (or process
// exit), so we call this from the UI goroutine and it sticks.
func holdExecutionState(prevent bool) {
	flags := esContinuous
	if prevent {
		flags |= esSystemRequired | esDisplayRequired
	}
	ret, _, _ := procSetThreadExecutionState.Call(uintptr(flags))
	applog.L().Printf("winpower.holdExecutionState: prevent=%v flags=0x%x prev=0x%x", prevent, flags, ret)
}

func saveBackup(sbAC, sbDC, lidAC, lidDC uint32) {
	log := applog.L()
	k, _, err := registry.CreateKey(registry.CURRENT_USER, backupRegPath, registry.SET_VALUE)
	if err != nil {
		log.Printf("winpower.saveBackup: open key failed: %v", err)
		return
	}
	defer k.Close()
	_ = k.SetDWordValue("StandbyAC", sbAC)
	_ = k.SetDWordValue("StandbyDC", sbDC)
	_ = k.SetDWordValue("LidAC", lidAC)
	_ = k.SetDWordValue("LidDC", lidDC)
	log.Printf("winpower.saveBackup: stored STANDBY AC=%d DC=%d LID AC=%d DC=%d", sbAC, sbDC, lidAC, lidDC)
}

func loadBackup() (sbAC, sbDC, lidAC, lidDC uint32, ok bool) {
	log := applog.L()
	k, err := registry.OpenKey(registry.CURRENT_USER, backupRegPath, registry.QUERY_VALUE)
	if err != nil {
		log.Printf("winpower.loadBackup: open key failed: %v", err)
		return 0, 0, 0, 0, false
	}
	defer k.Close()
	getU32 := func(name string) (uint32, bool) {
		v, _, err := k.GetIntegerValue(name)
		if err != nil {
			return 0, false
		}
		return uint32(v), true
	}
	var okSb, okLid bool
	sbAC, okSb = getU32("StandbyAC")
	sbDC, ok2 := getU32("StandbyDC")
	okSb = okSb && ok2
	lidAC, okLid = getU32("LidAC")
	lidDC, ok3 := getU32("LidDC")
	okLid = okLid && ok3

	// Standby is required, lid is optional (some OEMs lack it entirely).
	if !okSb {
		return 0, 0, 0, 0, false
	}
	if !okLid {
		lidAC, lidDC = defaultLidAC, defaultLidDC
	}
	return sbAC, sbDC, lidAC, lidDC, true
}

func IsUserCanceled(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "canceled by the user") ||
		strings.Contains(msg, "отменена пользователем") ||
		strings.Contains(msg, "the user refused") ||
		strings.Contains(msg, "0x800704c7")
}
