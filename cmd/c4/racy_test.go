package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/internal/racy"
	"github.com/Avalanche-io/c4/scan"
)

// useRacyClock installs a deterministic wall clock for the racy rule:
// Now returns the fake time and Sleep advances it.
func useRacyClock(t *testing.T, start time.Time, recheck func(round, pending int)) *time.Time {
	t.Helper()
	now := start
	oldNow, oldSleep, oldRecheck := racy.Now, racy.Sleep, racy.Recheck
	racy.Now = func() time.Time { return now }
	racy.Sleep = func(d time.Duration) { now = now.Add(d) }
	racy.Recheck = recheck
	t.Cleanup(func() {
		racy.Now, racy.Sleep, racy.Recheck = oldNow, oldSleep, oldRecheck
	})
	return &now
}

func writeAt(t *testing.T, path, content string, mtime time.Time) os.FileInfo {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

// `c4 id <file>` applies the racy rule: a same-size rewrite inside the
// file's mtime second is captured, a file that never settles gets a null
// timestamp, and a stable file is untouched.
func TestIdentifyFileRacy(t *testing.T) {
	T := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "HEAD")
	v2 := c4.Identify(strings.NewReader("v2-bbbb"))

	// Rewritten once inside the racy second: the new content is recorded.
	info := writeAt(t, path, "v1-aaaa", T)
	useRacyClock(t, T, func(round, n int) {
		if round == 1 {
			writeAt(t, path, "v2-bbbb", T)
		}
	})
	e := identifyFile(path, info, scan.ModeFull, false, racy.Start())
	if e.C4ID != v2 || !e.Timestamp.Equal(T) {
		t.Fatalf("got %s %v, want v2 %s at %v", e.C4ID, e.Timestamp, v2, T)
	}

	// Never leaves its racy second: null timestamp.
	info = writeAt(t, path, "v1-aaaa", T)
	var now *time.Time
	now = useRacyClock(t, T, func(round, n int) {
		writeAt(t, path, "v2-bbbb", racy.Second(*now).Add(time.Second))
	})
	e = identifyFile(path, info, scan.ModeFull, false, racy.Start())
	if !e.Timestamp.Equal(c4m.NullTimestamp()) {
		t.Fatalf("unsettled file timestamp %v, want null", e.Timestamp)
	}

	// Stable: no re-check, no wait.
	old := T.Add(-time.Hour)
	info = writeAt(t, path, "v1-aaaa", old)
	now = useRacyClock(t, T, func(round, n int) {
		t.Fatalf("stable file re-checked")
	})
	e = identifyFile(path, info, scan.ModeFull, false, racy.Start())
	if !e.Timestamp.Equal(old) || e.C4ID != c4.Identify(strings.NewReader("v1-aaaa")) || !now.Equal(T) {
		t.Fatalf("stable file changed: %s %v (clock moved %v)", e.C4ID, e.Timestamp, now.Sub(T))
	}
}
