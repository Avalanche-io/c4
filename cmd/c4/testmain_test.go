package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// realHome is the runner's own home directory, recorded before TestMain
// replaces it, so a test can check that nothing the suite runs touches it.
var realHome string

// TestMain gives every c4 process these tests start a home of its own, so
// no test reads or writes the real ~/.c4 of whoever runs the suite. The
// store is found from the home directory, and Go's os.UserHomeDir reads
// HOME on Unix but USERPROFILE on Windows: a test that set only HOME
// stored into the runner's real profile on Windows, and the no-store test
// that ran after it then found that store (Windows CI, 2026-11-11). Tests
// still set C4_STORE, HOME or USERPROFILE themselves where they need to;
// their settings come later in the environment and win.
func TestMain(m *testing.M) {
	realHome, _ = os.UserHomeDir()
	dir, err := os.MkdirTemp("", "c4-cli-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// The go command that buildC4 runs finds its caches from the home
	// directory too: pin them to where they are now before moving it.
	if out, err := exec.Command("go", "env", "GOCACHE", "GOMODCACHE", "GOPATH").Output(); err == nil {
		vals := strings.Split(strings.TrimSpace(string(out)), "\n")
		for i, k := range []string{"GOCACHE", "GOMODCACHE", "GOPATH"} {
			if i < len(vals) && strings.TrimSpace(vals[i]) != "" {
				os.Setenv(k, strings.TrimSpace(vals[i]))
			}
		}
	}
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	os.Unsetenv("C4_STORE")
	os.Unsetenv("C4_STORE_AUTO")

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
