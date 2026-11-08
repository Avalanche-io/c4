package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4/c4m"
)

// A c4m file never ends with its own ID (ruled 2026-11-08). Through
// 1.0.24 `c4 diff` ended its output with the new state's bare ID, so
// every file built with `c4 diff >>` has that shape; the normaliser
// (`c4 id FILE.c4m`) is the migration, and every other reader refuses
// the file with an error that names it.
const legacyDiffChain = "../../c4m/testdata/legacy/diff-chain-1.0.24.c4m"

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[len(lines)-1]
}

func TestDiffOutputEndsOnAnEntry(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}
	writeTree(t, filepath.Join(dir, "a"), map[string]string{"x.txt": "one", "y.txt": "two"})
	writeTree(t, filepath.Join(dir, "b"), map[string]string{"x.txt": "one", "y.txt": "two!"})

	out, stderr, code := runC4WithEnv(t, bin, env, "diff", filepath.Join(dir, "a"), filepath.Join(dir, "b"))
	if code != 0 {
		t.Fatalf("diff: exit %d: %s", code, stderr)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 || !isBareID(lines[0]) || isBareID(lines[1]) {
		t.Fatalf("diff must print the boundary then the entries and nothing after:\n%s", out)
	}

	// A chain built with `c4 id DIR > f; c4 diff f DIR2 >> f` is a valid
	// file: the strict reader accepts it, and the normaliser drops
	// nothing. (The description side sets the level, so the boundary
	// matches the scan's ID; a dir/dir diff compares at content level.)
	scan, _, code := runC4WithEnv(t, bin, env, "id", filepath.Join(dir, "a"))
	if code != 0 {
		t.Fatal("c4 id a")
	}
	chain := filepath.Join(dir, "chain.c4m")
	if err := os.WriteFile(chain, []byte(scan), 0o644); err != nil {
		t.Fatal(err)
	}
	out, stderr, code = runC4WithEnv(t, bin, env, "diff", chain, filepath.Join(dir, "b"))
	if code != 0 {
		t.Fatalf("diff chain b: exit %d: %s", code, stderr)
	}
	if err := os.WriteFile(chain, []byte(scan+out), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := c4m.Unmarshal([]byte(scan + out))
	if err != nil {
		t.Fatalf("a diff chain must decode strictly: %v", err)
	}
	wantID, _, code := runC4WithEnv(t, bin, env, "id", "-q", filepath.Join(dir, "b"))
	if code != 0 {
		t.Fatal("c4 id -q b")
	}
	if got := m.ComputeC4ID().String(); got != strings.TrimSpace(wantID) {
		t.Fatalf("chain resolved to %s, want b's ID %s", got, strings.TrimSpace(wantID))
	}
	_, stderr, code = runC4WithEnv(t, bin, env, "id", chain)
	if code != 0 || strings.Contains(stderr, "trailing root ID") {
		t.Fatalf("normaliser on a valid chain: exit %d, stderr %q", code, stderr)
	}
}

func TestLegacyTrailingIDRefusedWithHint(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}

	// Every strict reader refuses the 1.0.24 shape and says how to fix it.
	_, stderr, code := runC4WithEnv(t, bin, env, "patch", legacyDiffChain)
	if code == 0 {
		t.Fatal("c4 patch must refuse a file that ends with a bare ID")
	}
	if !strings.Contains(stderr, "never ends with its own ID") || !strings.Contains(stderr, "c4 id FILE.c4m") {
		t.Fatalf("refusal must name the rule and the fix, got: %s", stderr)
	}
}

func TestNormalizerMigratesLegacyTrailingID(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}

	legacy, err := os.ReadFile(legacyDiffChain)
	if err != nil {
		t.Fatal(err)
	}
	trailing := strings.TrimSpace(lastLine(string(legacy)))
	if !isBareID(trailing) {
		t.Fatal("fixture must end with a bare ID")
	}

	// -l lists it as not normal rather than refusing it.
	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-l", legacyDiffChain)
	if code != 1 || !strings.Contains(out, "diff-chain-1.0.24.c4m") {
		t.Fatalf("c4 id -l must list the legacy file (exit 1): exit %d, out %q, err %q", code, out, stderr)
	}

	// The normaliser rewrites it, says so, and the result is the valid
	// form that resolves to the state the dropped line named.
	out, stderr, code = runC4WithEnv(t, bin, env, "id", legacyDiffChain)
	if code != 0 {
		t.Fatalf("c4 id on the legacy file: exit %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "trailing root ID removed") {
		t.Fatalf("normaliser must report the dropped line, stderr: %q", stderr)
	}
	if isBareID(lastLine(out)) {
		t.Fatalf("normalised output must end on an entry:\n%s", out)
	}
	m, err := c4m.Unmarshal([]byte(out))
	if err != nil {
		t.Fatalf("normalised output must decode strictly: %v", err)
	}
	if got := m.ComputeC4ID().String(); got != trailing {
		t.Fatalf("normalised file resolves to %s, the dropped line named %s", got, trailing)
	}

	// Written to a file, the migrated form is accepted everywhere and is
	// already normal.
	fixed := filepath.Join(dir, "fixed.c4m")
	if err := os.WriteFile(fixed, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runC4WithEnv(t, bin, env, "patch", fixed); code != 0 {
		t.Fatalf("c4 patch on the migrated file: exit %d: %s", code, stderr)
	}
	if out, _, code := runC4WithEnv(t, bin, env, "id", "-l", fixed); code != 0 || out != "" {
		t.Fatalf("migrated file must be normal: exit %d, out %q", code, out)
	}
}
