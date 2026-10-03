package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

// A c4m patch chain whose patch section holds an entry at depth 1
// directly under a depth-0 file. Every verb that resolves the chain used
// to panic (exit 2, Go stack trace); each must now exit 1 with a message
// naming the entry.
func TestMalformedDepthChainExitsCleanly(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()

	idOf := func(s string) string { return c4.Identify(strings.NewReader(s)).String() }
	good := "-rw-r--r-- 2026-01-01T00:00:00Z 2 a.txt " + idOf("a") + "\n" +
		"drwxr-xr-x 2026-01-01T00:00:00Z 2 d/ -\n" +
		"  -rw-r--r-- 2026-01-01T00:00:00Z 2 b.txt " + idOf("b") + "\n"
	bad := good +
		idOf(good) + "\n" +
		"-rw-r--r-- 2026-01-01T00:00:00Z 2 z.txt " + idOf("z") + "\n" +
		"  -rw-r--r-- 2026-01-01T00:00:00Z 2 y.txt " + idOf("y") + "\n"

	goodPath := filepath.Join(dir, "good.c4m")
	badPath := filepath.Join(dir, "bad.c4m")
	if err := os.WriteFile(goodPath, []byte(good), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badPath, []byte(bad), 0644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "dest")
	if err := os.Mkdir(dest, 0755); err != nil {
		t.Fatal(err)
	}

	cases := [][]string{
		{"patch", badPath},
		{"patch", badPath, dest},
		{"patch", goodPath, badPath, goodPath},
		{"log", badPath},
		{"diff", badPath, goodPath},
		{"diff", goodPath, badPath},
		{"split", badPath, "2", filepath.Join(dir, "before.c4m"), filepath.Join(dir, "after.c4m")},
		{"merge", badPath, goodPath},
		{"paths", badPath},
		{"explain", "patch", badPath, dest},
	}
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store")}
	for _, args := range cases {
		_, stderr, code := runC4WithEnv(t, bin, env, args...)
		name := strings.Join(args[:1], " ")
		if strings.Contains(stderr, "panic") || strings.Contains(stderr, "goroutine") {
			t.Errorf("c4 %s panicked:\n%s", name, stderr)
			continue
		}
		if code != 1 {
			t.Errorf("c4 %v: exit %d, want 1; stderr: %s", args, code, stderr)
		}
		if !strings.Contains(stderr, "y.txt") || !strings.Contains(stderr, "depth 1") {
			t.Errorf("c4 %v: message should name the entry and its depth; got: %s", args, stderr)
		}
	}

	// The destination was not touched.
	if names, _ := os.ReadDir(dest); len(names) != 0 {
		t.Errorf("dest modified by a failed patch: %v", names)
	}
}
