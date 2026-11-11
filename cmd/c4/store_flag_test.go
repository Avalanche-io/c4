package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// -s with no content store configured and stdin not a terminal refuses
// with a clear error and a non-zero exit, before scanning: nothing on
// stdout, no store created. Through 1.0.23 it silently scanned
// without storing and exited 0 (reported by the fleet lead). With a
// store it stores, as always.
func TestStoreFlagRefusesWithoutAStore(t *testing.T) {
	bin := buildC4(t)
	root := t.TempDir()
	dir := filepath.Join(root, "t")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home")
	os.MkdirAll(home, 0o755)
	// HOME is empty (no ~/.c4/config), C4_STORE is unset: no store.
	env := map[string]string{"HOME": home, "C4_STORE": ""}

	out, stderr, code := runC4WithStdinEnv(t, bin, env, "y\n", "id", "-s", dir)
	if code == 0 {
		t.Fatalf("-s without a store exited 0:\n%s%s", out, stderr)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("-s without a store wrote to stdout:\n%s", out)
	}
	if !strings.Contains(stderr, "C4_STORE") || strings.Contains(stderr, "stored:") {
		t.Fatalf("stderr does not say how to configure a store, or claims a store:\n%s", stderr)
	}
	if entries, _ := os.ReadDir(home); len(entries) != 0 {
		t.Fatalf("a store was created under HOME without being asked: %v", entries)
	}

	env["C4_STORE"] = filepath.Join(root, "store")
	out, stderr, code = runC4WithStdinEnv(t, bin, env, "", "id", "-s", "-q", dir)
	if code != 0 || !strings.Contains(stderr, "stored:") { // on 1.0.x the stored ID is narrated on stderr
		t.Fatalf("-s with a store did not store (exit %d):\n%s%s", code, out, stderr)
	}
}

// runC4WithStdinEnv runs the binary with stdin supplied and extra
// environment, so stdin is a pipe rather than a terminal.
func runC4WithStdinEnv(t *testing.T, bin string, env map[string]string, stdin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = childEnv(env)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("exec error: %v", err)
		}
	}
	return stdout.String(), stderr.String(), code
}
