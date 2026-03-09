package main

import (
	"os"
	"runtime"
	"testing"
)

// setTestHome sets HOME (and USERPROFILE on Windows) for the test process.
func setTestHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", dir)
	}
}

// testHomeEnv returns env vars that set the home directory for a subprocess.
func testHomeEnv(dir string) []string {
	env := []string{"HOME=" + dir}
	if runtime.GOOS == "windows" {
		env = append(env, "USERPROFILE="+dir)
	}
	return env
}

// subprocEnv returns os.Environ() plus home dir and any extra vars for subprocess tests.
func subprocEnv(dir string, extra ...string) []string {
	env := append(os.Environ(), testHomeEnv(dir)...)
	return append(env, extra...)
}
