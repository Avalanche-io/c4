package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4/store"
)

// With no store configured, c4 asks whether to create one only when
// someone can answer. stdin at /dev/null (cron, launchd, an agent) is a
// character device, so it passes the terminal check, but its read ends at
// once with nothing read. That is no answer, and it must create nothing.
// Through 1.0.26 the empty read counted as Enter, and c4 id -s silently
// created ~/.c4/store and a config pointing at it.

func TestIDStoreWithStdinAtDevNullCreatesNoStore(t *testing.T) {
	if fi, err := os.Stat(os.DevNull); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		t.Skipf("%s is not a character device here", os.DevNull)
	}
	bin := buildC4(t)
	root := t.TempDir()
	home := filepath.Join(root, "home")
	os.MkdirAll(home, 0o755)
	dir := filepath.Join(root, "tree")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644)

	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()
	cmd := exec.Command(bin, "id", "-s", dir)
	cmd.Env = childEnv(map[string]string{"HOME": home, "USERPROFILE": home})
	cmd.Stdin = devnull
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	if err == nil {
		t.Errorf("c4 id -s with stdin at %s exited 0, want non-zero:\n%s", os.DevNull, stderr.String())
	}
	if _, ok := err.(*exec.ExitError); err != nil && !ok {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(stderr.String(), "C4_STORE") {
		t.Errorf("stderr does not say how to configure a store:\n%s", stderr.String())
	}
	if strings.Contains(stderr.String(), "stored:") {
		t.Errorf("stderr claims content was stored:\n%s", stderr.String())
	}
	if entries, _ := os.ReadDir(home); len(entries) != 0 {
		t.Errorf("a store or config was created under HOME with nobody to answer: %v", entries)
	}
}

func TestStorePromptAnswers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  string
		create bool
	}{
		{"end of input, nothing read", "", false},
		{"Enter", "\n", true},
		{"y, then end of input", "y", true},
		{"yes", "yes\n", true},
		{"n", "n\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			setenvForTest(t, "HOME", home)
			setenvForTest(t, "USERPROFILE", home)

			s := openOrSetupStoreFrom(strings.NewReader(tc.input), true)
			_, statErr := os.Stat(store.DefaultStorePath())
			if tc.create {
				if s == nil || statErr != nil {
					t.Errorf("answer %q: store %v, %s: %v; want the default store created", tc.input, s, store.DefaultStorePath(), statErr)
				}
				return
			}
			if s != nil {
				t.Errorf("answer %q opened a store, want none", tc.input)
			}
			if entries, _ := os.ReadDir(home); len(entries) != 0 {
				t.Errorf("answer %q created %v under HOME, want nothing", tc.input, entries)
			}
		})
	}
}

// setenvForTest sets key for one test and restores it afterwards. t.Setenv
// needs Go 1.17, and this line builds with 1.16. TestMain has already
// unset C4_STORE.
func setenvForTest(t *testing.T, key, val string) {
	t.Helper()
	old, had := os.LookupEnv(key)
	os.Setenv(key, val)
	t.Cleanup(func() {
		if had {
			os.Setenv(key, old)
		} else {
			os.Unsetenv(key)
		}
	})
}
