// Package applog gives SleepSwitch a single shared file logger. Windows
// builds use -H windowsgui, which detaches stdout/stderr, so we need a file
// on disk to debug what the app is doing in the wild.
package applog

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// Enabled is the compile-time switch for file logging. Flip to true and
// rebuild when you need to debug a release on a user machine.
const Enabled = false

var (
	once   sync.Once
	logger *log.Logger
	path   string
)

// Init wires up the package-level logger to a file under the user's cache
// directory (e.g. %LOCALAPPDATA%\<appName>\log.txt on Windows). Safe to call
// multiple times — only the first call has effect. When Enabled is false,
// the logger discards everything and no file is created.
func Init(appName string) {
	once.Do(func() {
		if !Enabled {
			logger = log.New(io.Discard, "", 0)
			return
		}
		cacheDir, err := os.UserCacheDir()
		if err != nil || cacheDir == "" {
			cacheDir = os.TempDir()
		}
		dir := filepath.Join(cacheDir, appName)
		_ = os.MkdirAll(dir, 0o755)
		path = filepath.Join(dir, "log.txt")

		var sink io.Writer = os.Stderr
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err == nil {
			sink = f
		}
		logger = log.New(sink, "", log.LstdFlags|log.Lmicroseconds)
	})
}

// L returns the shared logger. Falls back to the stdlib default if Init was
// never called, so callers don't have to nil-check.
func L() *log.Logger {
	if logger == nil {
		return log.Default()
	}
	return logger
}

// Path returns the absolute path to the log file (empty before Init).
func Path() string { return path }
