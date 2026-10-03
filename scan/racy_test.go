package scan

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/internal/racy"
)

// fakeClock installs a deterministic wall clock: Now returns the fake
// time, Sleep advances it. Recheck is installed as given. Everything is
// restored when the test ends.
type fakeClock struct{ now time.Time }

func useFakeClock(t *testing.T, start time.Time, recheck func(round, pending int)) *fakeClock {
	t.Helper()
	fc := &fakeClock{now: start}
	oldNow, oldSleep, oldRecheck := racy.Now, racy.Sleep, racy.Recheck
	racy.Now = func() time.Time { return fc.now }
	racy.Sleep = func(d time.Duration) { fc.now = fc.now.Add(d) }
	racy.Recheck = recheck
	t.Cleanup(func() {
		racy.Now, racy.Sleep, racy.Recheck = oldNow, oldSleep, oldRecheck
	})
	return fc
}

// racyT is the fake scan start: every fixture stamped with it is racy.
var racyT = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

// stableT is well before racyT: fixtures stamped with it are stable.
var stableT = time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)

// writeStamped writes content with fixed permissions and mtime.
func writeStamped(t *testing.T, path, content string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// stampDirs gives every directory under root fixed permissions and mtime,
// so a fixture's listing is reproducible across machines and umasks.
func stampDirs(t *testing.T, root string, mtime time.Time) {
	t.Helper()
	var dirs []string
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() {
			dirs = append(dirs, p)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		os.Chmod(dirs[i], 0755)
		if err := os.Chtimes(dirs[i], mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
}

func entryByName(t *testing.T, m *c4m.Manifest, name string) *c4m.Entry {
	t.Helper()
	for _, e := range m.Entries {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("no entry %q in manifest", name)
	return nil
}

func idOf(s string) c4.ID {
	return c4.Identify(strings.NewReader(s))
}

// A file whose mtime second is at or after the scan's start is racy and
// is re-checked; a file stamped in the past is not.
func TestRacyFileDetected(t *testing.T) {
	dir := t.TempDir()
	writeStamped(t, filepath.Join(dir, "old.txt"), "stable", stableT)
	writeStamped(t, filepath.Join(dir, "ref"), "v1-aaaa", racyT)
	stampDirs(t, dir, stableT)

	var rounds, pending []int
	useFakeClock(t, racyT, func(round, n int) {
		rounds = append(rounds, round)
		pending = append(pending, n)
	})

	if _, err := NewGeneratorWithOptions(WithMaxConcurrency(1)).GenerateFromPath(dir); err != nil {
		t.Fatal(err)
	}
	if len(rounds) != 1 || pending[0] != 1 {
		t.Fatalf("recheck rounds=%v pending=%v, want one round with 1 racy file", rounds, pending)
	}
}

// A same-size rewrite inside the racy file's mtime second, made after the
// walk read it, is captured: the recorded ID is the new content's, and the
// enclosing directories are re-identified to match a fresh scan.
func TestRacyRewriteCaptured(t *testing.T) {
	dir := t.TempDir()
	ref := filepath.Join(dir, "sub", "deep", "ref")
	writeStamped(t, ref, "v1-aaaa", racyT)
	writeStamped(t, filepath.Join(dir, "sub", "other"), "x", stableT)
	stampDirs(t, dir, stableT)

	useFakeClock(t, racyT, func(round, n int) {
		if round == 1 {
			// Same size, same mtime second: invisible to size+mtime trust.
			writeStamped(t, ref, "v2-bbbb", racyT)
			stampDirs(t, dir, stableT)
		}
	})

	m, err := NewGeneratorWithOptions(WithMaxConcurrency(1)).GenerateFromPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	e := entryByName(t, m, "ref")
	if e.C4ID != idOf("v2-bbbb") {
		t.Fatalf("recorded ID %s, want v2 %s (v1 is %s)", e.C4ID, idOf("v2-bbbb"), idOf("v1-aaaa"))
	}
	if !e.Timestamp.Equal(racyT) {
		t.Fatalf("timestamp %v, want %v", e.Timestamp, racyT)
	}

	// The tree is now quiet: a fresh scan (clock well past the stamps)
	// must produce byte-identical output, directories included.
	useFakeClock(t, racyT.Add(time.Hour), nil)
	fresh, err := NewGeneratorWithOptions(WithMaxConcurrency(1)).GenerateFromPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c4m.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	want, err := c4m.Marshal(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("settled scan differs from fresh scan:\n got:\n%s\nwant:\n%s", got, want)
	}
}

// A file still being rewritten inside its mtime second after every
// re-check round is recorded with a null timestamp, and its ancestors'
// sizes and IDs reflect that null.
func TestRacyUnsettledGetsNullTimestamp(t *testing.T) {
	for _, mode := range []ScanMode{ModeFull, ModeMetadata} {
		dir := t.TempDir()
		ref := filepath.Join(dir, "sub", "ref")
		writeStamped(t, ref, "v0-0000", racyT)
		stampDirs(t, dir, stableT)

		var fc *fakeClock
		var rounds int
		fc = useFakeClock(t, racyT, func(round, n int) {
			rounds = round
			// Rewrite stamped with the second the re-check will run in:
			// the file never leaves its racy window.
			writeStamped(t, ref, "v"+string(rune('0'+round))+"-zzzz", racy.Second(fc.now).Add(time.Second))
			stampDirs(t, dir, stableT)
		})

		m, err := NewGeneratorWithOptions(WithMode(mode), WithMaxConcurrency(1)).GenerateFromPath(dir)
		if err != nil {
			t.Fatal(err)
		}
		if rounds != racy.Rounds {
			t.Fatalf("mode %d: %d rounds, want %d", mode, rounds, racy.Rounds)
		}
		e := entryByName(t, m, "ref")
		if !e.Timestamp.Equal(c4m.NullTimestamp()) {
			t.Fatalf("mode %d: timestamp %v, want null", mode, e.Timestamp)
		}
		if mode == ModeFull {
			if e.C4ID != idOf("v3-zzzz") {
				t.Fatalf("ID %s, want the last observed content %s", e.C4ID, idOf("v3-zzzz"))
			}
			// The parent directory is identified by its listing, which now
			// carries the null timestamp.
			sub := entryByName(t, m, "sub/")
			want := c4.Identify(strings.NewReader(e.Canonical() + "\n"))
			if sub.C4ID != want {
				t.Fatalf("sub/ ID %s, want %s from its settled listing", sub.C4ID, want)
			}
			if wantSize := e.Size + int64(len(e.Canonical())+1); sub.Size != wantSize {
				t.Fatalf("sub/ size %d, want %d", sub.Size, wantSize)
			}
		}
	}
}

// A tree not modified during the scan is untouched by the racy rule: no
// waiting, no re-check, and the same bytes 1.0.17 produced (the expected
// ID was computed by the released c4 1.0.17 on this exact fixture).
func TestRacyStableTreeUnchanged(t *testing.T) {
	dir := t.TempDir()
	writeStamped(t, filepath.Join(dir, "a.txt"), "alpha\n", stableT)
	writeStamped(t, filepath.Join(dir, "sub", "b.txt"), "bravo bravo\n", stableT)
	writeStamped(t, filepath.Join(dir, "sub", "deep", "c.bin"), "charlie", stableT)
	stampDirs(t, dir, stableT)

	fc := useFakeClock(t, racyT, func(round, n int) {
		t.Fatalf("stable tree triggered a racy re-check (round %d, %d files)", round, n)
	})
	m, err := NewGeneratorWithOptions().GenerateFromPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !fc.now.Equal(racyT) {
		t.Fatalf("stable scan waited %v", fc.now.Sub(racyT))
	}
	// The golden listing below was produced by 1.0.17 on a Unix filesystem.
	// Windows reports different permission bits, so directory IDs differ
	// there for reasons unrelated to the racy rule.
	if runtime.GOOS == "windows" {
		return
	}
	// `c4 id <fixture>` output from c4 1.0.17.
	const want = "-rw-r--r-- 2020-01-02T03:04:05Z 6 a.txt c42yayFpQ5CKfAPUUjuE1atmYkUQQNRnfbhD5ftJNwSECL2PwPDHerxHp5KnKxLbHxm4DTiJvnKDDhs19t5uHDUFCa\n" +
		"drwxr-xr-x 2020-01-02T03:04:05Z 415 sub/ c43bBdHuBSSuLnvc9w7U3WMH2MZyP1CrXQZkdoGEYujJhAEEVWy76LbD8YopaNNRgdp8EX5CMagXP33M7KQwQpLEMX\n" +
		"  -rw-r--r-- 2020-01-02T03:04:05Z 12 b.txt c45u5yhtKadxthyxxR6ftBqqZ5XhzNXzfEvJsjDkNc1feEznW749xumA73nyMdCaeKsv4foBRgxKjXgvd7FvGwXaZB\n" +
		"  drwxr-xr-x 2020-01-02T03:04:05Z 138 deep/ c419t5BYJttxLiRmznFFe4DFJncyLQ9dFx63fWgACBjJx3YJ6GTD3FaU4sXniVFcFtEWhQS885baSHM9p5FJi8hb66\n" +
		"    -rw-r--r-- 2020-01-02T03:04:05Z 7 c.bin c44erLietE8C1iKmQ3y4ENqA9g82Exdkoxox3KEHops2ux5MTsuMjfbFRvUPsPdi9Pxc3C2MRvLxWT8eFw5XKbRQGw\n"
	got, err := c4m.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("stable tree output differs from 1.0.17:\n got:\n%s\nwant:\n%s", got, want)
	}
}

// A single-file scan applies the same rule.
func TestRacySingleFile(t *testing.T) {
	dir := t.TempDir()
	ref := filepath.Join(dir, "ref")
	writeStamped(t, ref, "v1-aaaa", racyT)
	useFakeClock(t, racyT, func(round, n int) {
		if round == 1 {
			writeStamped(t, ref, "v2-bbbb", racyT)
		}
	})
	m, err := NewGeneratorWithOptions().GenerateFromPath(ref)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Entries[0].C4ID; got != idOf("v2-bbbb") {
		t.Fatalf("single-file ID %s, want v2 %s", got, idOf("v2-bbbb"))
	}
}
