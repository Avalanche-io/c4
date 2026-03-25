package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// buildC4 builds the c4 binary and returns its path.
func buildC4(t *testing.T) string {
	t.Helper()
	name := "c4"
	if runtime.GOOS == "windows" {
		name = "c4.exe"
	}
	bin := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

func runC4(t *testing.T, bin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
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

func runC4WithStdin(t *testing.T, bin string, stdin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
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

func TestVersion(t *testing.T) {
	bin := buildC4(t)
	out, _, code := runC4(t, bin, "version")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.HasPrefix(out, "c4 1.0.") {
		t.Fatalf("unexpected version output: %s", out)
	}
}

func TestStdinID(t *testing.T) {
	bin := buildC4(t)
	out, _, code := runC4WithStdin(t, bin, "hello\n")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	out = strings.TrimSpace(out)
	if !strings.HasPrefix(out, "c4") || len(out) != 90 {
		t.Fatalf("expected C4 ID, got: %s", out)
	}
}

func TestIDSingleFile(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("test content"), 0644)

	out, _, code := runC4(t, bin, "id", path)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	// Should be a c4m entry line.
	if !strings.Contains(out, "test.txt") {
		t.Fatalf("expected test.txt in output: %s", out)
	}
	if !strings.Contains(out, "c4") {
		t.Fatalf("expected C4 ID in output: %s", out)
	}
}

func TestIDDirectory(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb"), 0644)

	out, _, code := runC4(t, bin, "id", dir)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "a.txt") || !strings.Contains(out, "b.txt") {
		t.Fatalf("expected both files in output: %s", out)
	}
}

func TestIDC4mNormalize(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644)

	// Generate c4m.
	out1, _, _ := runC4(t, bin, "id", dir)

	// Save to file and normalize.
	c4mPath := filepath.Join(dir, "test.c4m")
	os.WriteFile(c4mPath, []byte(out1), 0644)

	out2, _, code := runC4(t, bin, "id", c4mPath)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	// Normalized output should match original.
	if out1 != out2 {
		t.Fatalf("normalize mismatch:\n  original: %s\n  normalized: %s", out1, out2)
	}
}

func TestDiffProducesPatch(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()

	// Create two states.
	dir1 := filepath.Join(dir, "v1")
	dir2 := filepath.Join(dir, "v2")
	os.MkdirAll(dir1, 0755)
	os.MkdirAll(dir2, 0755)

	os.WriteFile(filepath.Join(dir1, "a.txt"), []byte("aaa"), 0644)
	os.WriteFile(filepath.Join(dir2, "a.txt"), []byte("aaa modified"), 0644)
	os.WriteFile(filepath.Join(dir2, "b.txt"), []byte("bbb"), 0644)

	c4m1, _, _ := runC4(t, bin, "id", dir1)
	c4m2, _, _ := runC4(t, bin, "id", dir2)

	c4m1Path := filepath.Join(dir, "v1.c4m")
	c4m2Path := filepath.Join(dir, "v2.c4m")
	os.WriteFile(c4m1Path, []byte(c4m1), 0644)
	os.WriteFile(c4m2Path, []byte(c4m2), 0644)

	diff, _, code := runC4(t, bin, "diff", c4m1Path, c4m2Path)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}

	// Diff should contain base ID, entries, and new ID.
	lines := strings.Split(strings.TrimSpace(diff), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines in diff, got %d: %s", len(lines), diff)
	}
	// First line should be a C4 ID.
	if !strings.HasPrefix(lines[0], "c4") || len(lines[0]) != 90 {
		t.Fatalf("first line should be C4 ID: %s", lines[0])
	}
	// Last line should be a C4 ID.
	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, "c4") || len(last) != 90 {
		t.Fatalf("last line should be C4 ID: %s", last)
	}
}

func TestDiffEmpty(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("same"), 0644)

	out, _, _ := runC4(t, bin, "id", dir)
	c4mPath := filepath.Join(dir, "test.c4m")
	os.WriteFile(c4mPath, []byte(out), 0644)

	diff, _, code := runC4(t, bin, "diff", c4mPath, c4mPath)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.TrimSpace(diff) != "" {
		t.Fatalf("expected empty diff, got: %s", diff)
	}
}

func TestPatchResolveChain(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()

	// Create base state.
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)
	os.WriteFile(filepath.Join(projectDir, "a.txt"), []byte("version 1"), 0644)

	base, _, _ := runC4(t, bin, "id", projectDir)
	c4mPath := filepath.Join(dir, "project.c4m")
	os.WriteFile(c4mPath, []byte(base), 0644)

	// Modify and append patch.
	os.WriteFile(filepath.Join(projectDir, "a.txt"), []byte("version 2"), 0644)
	newState, _, _ := runC4(t, bin, "id", projectDir)
	newC4mPath := filepath.Join(dir, "new.c4m")
	os.WriteFile(newC4mPath, []byte(newState), 0644)

	diff, _, _ := runC4(t, bin, "diff", c4mPath, newC4mPath)

	// Append patch to the original.
	f, _ := os.OpenFile(c4mPath, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString(diff)
	f.Close()

	// Resolve the chain.
	resolved, _, code := runC4(t, bin, "patch", c4mPath)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(resolved, "version 2") {
		// The resolved manifest should have the modified a.txt entry.
		// We can't check content, but we can check the c4m entry changed.
		if resolved == base {
			t.Fatal("resolved manifest should differ from base")
		}
	}
}

func TestLogShowsPatches(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()

	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)
	os.WriteFile(filepath.Join(projectDir, "a.txt"), []byte("v1"), 0644)

	base, _, _ := runC4(t, bin, "id", projectDir)
	c4mPath := filepath.Join(dir, "project.c4m")
	os.WriteFile(c4mPath, []byte(base), 0644)

	// Add a patch.
	os.WriteFile(filepath.Join(projectDir, "b.txt"), []byte("new"), 0644)
	newState, _, _ := runC4(t, bin, "id", projectDir)
	newPath := filepath.Join(dir, "new.c4m")
	os.WriteFile(newPath, []byte(newState), 0644)

	diff, _, _ := runC4(t, bin, "diff", c4mPath, newPath)
	f, _ := os.OpenFile(c4mPath, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString(diff)
	f.Close()

	log, _, code := runC4(t, bin, "log", c4mPath)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}

	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 log entries, got %d: %s", len(lines), log)
	}
	if !strings.Contains(lines[0], "(base)") {
		t.Fatalf("first line should be base: %s", lines[0])
	}
	if !strings.Contains(lines[1], "+1") {
		t.Fatalf("second line should show +1: %s", lines[1])
	}
}

func TestSplitAndRejoin(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()

	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)
	os.WriteFile(filepath.Join(projectDir, "a.txt"), []byte("v1"), 0644)

	base, _, _ := runC4(t, bin, "id", projectDir)
	c4mPath := filepath.Join(dir, "project.c4m")
	os.WriteFile(c4mPath, []byte(base), 0644)

	// Append two patches. Each diff is against the resolved current state.
	for i, content := range []string{"v2", "v3"} {
		os.WriteFile(filepath.Join(projectDir, "a.txt"), []byte(content), 0644)
		if i == 1 {
			os.WriteFile(filepath.Join(projectDir, "b.txt"), []byte("extra"), 0644)
		}

		// Resolve current state to compare against.
		currentState, _, _ := runC4(t, bin, "patch", c4mPath)
		currentPath := filepath.Join(dir, "current.c4m")
		os.WriteFile(currentPath, []byte(currentState), 0644)

		newState, _, _ := runC4(t, bin, "id", projectDir)
		newPath := filepath.Join(dir, "new.c4m")
		os.WriteFile(newPath, []byte(newState), 0644)

		diff, _, _ := runC4(t, bin, "diff", currentPath, newPath)
		if strings.TrimSpace(diff) == "" {
			continue // no changes
		}
		f, _ := os.OpenFile(c4mPath, os.O_APPEND|os.O_WRONLY, 0644)
		f.WriteString(diff)
		f.Close()
	}

	// Log should show 3 entries.
	log, _, _ := runC4(t, bin, "log", c4mPath)
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 log entries, got %d", len(lines))
	}

	// Split at patch 2.
	beforePath := filepath.Join(dir, "before.c4m")
	afterPath := filepath.Join(dir, "after.c4m")
	_, _, code := runC4(t, bin, "split", c4mPath, "2", beforePath, afterPath)
	if code != 0 {
		t.Fatalf("split exit %d", code)
	}

	// Resolve original.
	fullResolved, _, _ := runC4(t, bin, "patch", c4mPath)

	// Resolve split + rejoin (concatenate into single file, then resolve).
	beforeData, _ := os.ReadFile(beforePath)
	afterData, _ := os.ReadFile(afterPath)
	combinedPath := filepath.Join(dir, "combined.c4m")
	os.WriteFile(combinedPath, append(beforeData, afterData...), 0644)
	splitResolved, _, _ := runC4(t, bin, "patch", combinedPath)

	if fullResolved != splitResolved {
		t.Fatalf("split+rejoin mismatch:\n  full: %s\n  split: %s", fullResolved, splitResolved)
	}
}

func TestCatFromStore(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")

	content := "store test content"
	filePath := filepath.Join(dir, "test.txt")
	os.WriteFile(filePath, []byte(content), 0644)

	// Store the file.
	out, _, code := runC4WithEnv(t, bin, map[string]string{"C4_STORE": storeDir}, "id", "-s", filePath)
	if code != 0 {
		t.Fatalf("id -s exit %d", code)
	}

	// Extract the C4 ID from the c4m output.
	fields := strings.Fields(out)
	var c4id string
	for _, f := range fields {
		if strings.HasPrefix(f, "c4") && len(f) == 90 {
			c4id = f
			break
		}
	}
	if c4id == "" {
		t.Fatalf("no C4 ID found in output: %s", out)
	}

	// Cat it back.
	catOut, _, code := runC4WithEnv(t, bin, map[string]string{"C4_STORE": storeDir}, "cat", c4id)
	if code != 0 {
		t.Fatalf("cat exit %d", code)
	}
	if catOut != content {
		t.Fatalf("content mismatch: got %q, want %q", catOut, content)
	}
}

func runC4WithEnv(t *testing.T, bin string, env map[string]string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
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

func TestMergeTwoDirectories(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()

	dir1 := filepath.Join(dir, "a")
	dir2 := filepath.Join(dir, "b")
	os.MkdirAll(dir1, 0755)
	os.MkdirAll(dir2, 0755)
	os.WriteFile(filepath.Join(dir1, "from_a.txt"), []byte("aaa"), 0644)
	os.WriteFile(filepath.Join(dir2, "from_b.txt"), []byte("bbb"), 0644)

	out, _, code := runC4(t, bin, "merge", dir1, dir2)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "from_a.txt") {
		t.Fatalf("expected from_a.txt in merged output: %s", out)
	}
	if !strings.Contains(out, "from_b.txt") {
		t.Fatalf("expected from_b.txt in merged output: %s", out)
	}
}

func TestMergeMixed(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()

	realDir := filepath.Join(dir, "real")
	os.MkdirAll(realDir, 0755)
	os.WriteFile(filepath.Join(realDir, "real.txt"), []byte("real"), 0644)

	c4mDir := filepath.Join(dir, "c4mdir")
	os.MkdirAll(c4mDir, 0755)
	os.WriteFile(filepath.Join(c4mDir, "c4m.txt"), []byte("c4m"), 0644)
	c4mOut, _, _ := runC4(t, bin, "id", c4mDir)
	c4mPath := filepath.Join(dir, "test.c4m")
	os.WriteFile(c4mPath, []byte(c4mOut), 0644)

	out, _, code := runC4(t, bin, "merge", c4mPath, realDir)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "c4m.txt") || !strings.Contains(out, "real.txt") {
		t.Fatalf("expected both files in merged output: %s", out)
	}
}

func TestDiffDirectories(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()

	dir1 := filepath.Join(dir, "v1")
	dir2 := filepath.Join(dir, "v2")
	os.MkdirAll(dir1, 0755)
	os.MkdirAll(dir2, 0755)
	os.WriteFile(filepath.Join(dir1, "a.txt"), []byte("original"), 0644)
	os.WriteFile(filepath.Join(dir2, "a.txt"), []byte("modified"), 0644)
	os.WriteFile(filepath.Join(dir2, "b.txt"), []byte("new"), 0644)

	diff, _, code := runC4(t, bin, "diff", dir1, dir2)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(diff, "a.txt") {
		t.Fatalf("diff should contain modified a.txt: %s", diff)
	}
	if !strings.Contains(diff, "b.txt") {
		t.Fatalf("diff should contain added b.txt: %s", diff)
	}
}

func TestDiffReverse(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()

	dir1 := filepath.Join(dir, "v1")
	dir2 := filepath.Join(dir, "v2")
	os.MkdirAll(dir1, 0755)
	os.MkdirAll(dir2, 0755)
	os.WriteFile(filepath.Join(dir1, "a.txt"), []byte("old"), 0644)
	os.WriteFile(filepath.Join(dir2, "a.txt"), []byte("new"), 0644)

	// Forward diff: v1 → v2
	forward, _, _ := runC4(t, bin, "diff", dir1, dir2)
	// Reverse diff: v2 → v1 (using -r flag)
	reverse, _, code := runC4(t, bin, "diff", "-r", dir1, dir2)
	if code != 0 {
		t.Fatalf("diff -r exit %d", code)
	}
	// Explicit swap should match -r
	swapped, _, _ := runC4(t, bin, "diff", dir2, dir1)
	if reverse != swapped {
		t.Fatalf("diff -r should equal swapped args\n  -r:      %s\n  swapped: %s", reverse, swapped)
	}
	// Forward and reverse should differ
	if forward == reverse {
		t.Fatal("forward and reverse diffs should not be identical")
	}
}

func TestDiffGuidedScanReusesIDs(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()

	// Create a directory with files.
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)
	os.WriteFile(filepath.Join(projectDir, "unchanged.txt"), []byte("same"), 0644)
	os.WriteFile(filepath.Join(projectDir, "modified.txt"), []byte("original"), 0644)

	// Backdate timestamps so re-snapshot is clearly in the past.
	past := time.Now().Add(-2 * time.Second)
	os.Chtimes(filepath.Join(projectDir, "unchanged.txt"), past, past)
	os.Chtimes(filepath.Join(projectDir, "modified.txt"), past, past)

	// Snapshot to c4m.
	c4mOut, _, _ := runC4(t, bin, "id", projectDir)
	c4mPath := filepath.Join(dir, "snapshot.c4m")
	os.WriteFile(c4mPath, []byte(c4mOut), 0644)

	// Diff c4m against unchanged directory — should produce empty diff.
	diff, _, code := runC4(t, bin, "diff", c4mPath, projectDir)
	if code != 0 {
		t.Fatalf("diff exit %d", code)
	}
	if strings.TrimSpace(diff) != "" {
		t.Fatalf("diff of unchanged directory should be empty, got:\n%s", diff)
	}

	// Now modify one file.
	os.WriteFile(filepath.Join(projectDir, "modified.txt"), []byte("changed!"), 0644)

	// Diff should show modified.txt changed.
	diff2, _, code := runC4(t, bin, "diff", c4mPath, projectDir)
	if code != 0 {
		t.Fatalf("diff exit %d", code)
	}
	if !strings.Contains(diff2, "modified.txt") {
		t.Fatalf("diff should show modified.txt: %s", diff2)
	}
}

func TestDiffMixed(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()

	dir1 := filepath.Join(dir, "v1")
	os.MkdirAll(dir1, 0755)
	os.WriteFile(filepath.Join(dir1, "a.txt"), []byte("original"), 0644)
	// Backdate v1 so timestamps differ from v2.
	past := time.Now().Add(-2 * time.Second)
	os.Chtimes(filepath.Join(dir1, "a.txt"), past, past)
	c4mOut, _, _ := runC4(t, bin, "id", dir1)
	c4mPath := filepath.Join(dir, "v1.c4m")
	os.WriteFile(c4mPath, []byte(c4mOut), 0644)

	dir2 := filepath.Join(dir, "v2")
	os.MkdirAll(dir2, 0755)
	os.WriteFile(filepath.Join(dir2, "a.txt"), []byte("modified"), 0644)

	diff, _, code := runC4(t, bin, "diff", c4mPath, dir2)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(diff, "a.txt") {
		t.Fatalf("diff should show changed a.txt: %s", diff)
	}
}

// TestC4mCanonicalID verifies that a pretty-printed c4m file and its
// canonical equivalent produce the same C4 ID when identified.
func TestC4mCanonicalID(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()

	// Create a directory and scan it.
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)
	os.WriteFile(filepath.Join(projectDir, "hello.txt"), []byte("hello world"), 0644)
	os.WriteFile(filepath.Join(projectDir, "data.bin"), []byte("binary data"), 0644)

	// Get canonical c4m output.
	canonical, _, _ := runC4(t, bin, "id", projectDir)

	// Get pretty-printed c4m output.
	pretty, _, _ := runC4(t, bin, "id", "-e", projectDir)

	// Canonical and pretty forms should differ in formatting.
	if canonical == pretty {
		t.Fatal("canonical and pretty forms should differ")
	}

	// Save both to files.
	canonicalPath := filepath.Join(dir, "canonical.c4m")
	prettyPath := filepath.Join(dir, "pretty.c4m")
	os.WriteFile(canonicalPath, []byte(canonical), 0644)
	os.WriteFile(prettyPath, []byte(pretty), 0644)

	// Identify both files — they should produce the same C4 ID.
	canonicalID, _, code := runC4(t, bin, "id", canonicalPath)
	if code != 0 {
		t.Fatalf("id canonical.c4m exit %d", code)
	}
	prettyID, _, code := runC4(t, bin, "id", prettyPath)
	if code != 0 {
		t.Fatalf("id pretty.c4m exit %d", code)
	}

	// Both should output the same c4m (since both are canonical when
	// re-identified). The output IS the canonical form.
	if canonicalID != prettyID {
		t.Fatalf("canonical and pretty c4m IDs should match:\n  canonical: %s\n  pretty:   %s", canonicalID, prettyID)
	}
}

// TestC4mCanonicalStore verifies that storing a pretty c4m file via the bare
// form results in the canonical form being stored, retrievable by its
// canonical C4 ID.
func TestC4mCanonicalStore(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	env := map[string]string{"C4_STORE": storeDir}

	// Create a directory and get its c4m.
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)
	os.WriteFile(filepath.Join(projectDir, "a.txt"), []byte("aaa"), 0644)

	// Get canonical form.
	canonical, _, _ := runC4(t, bin, "id", projectDir)
	// Get pretty form.
	pretty, _, _ := runC4(t, bin, "id", "-e", projectDir)

	// Canonical and pretty should differ.
	if canonical == pretty {
		t.Fatal("canonical and pretty should differ in formatting")
	}

	// Compute the canonical c4m's own C4 ID by piping it through c4 -x
	// (which identifies the canonical bytes via c4m detection).
	canonicalID, _, code := runC4WithStdin(t, bin, canonical, "-x")
	if code != 0 {
		t.Fatalf("stdin id exit %d", code)
	}
	canonicalID = strings.TrimSpace(canonicalID)

	// Save pretty form and use bare `c4 <path>` to identify+store.
	prettyPath := filepath.Join(dir, "pretty.c4m")
	os.WriteFile(prettyPath, []byte(pretty), 0644)

	// Bare form: `c4 pretty.c4m` → identify + store canonical form.
	_, _, code = runC4WithEnv(t, bin, env, prettyPath)
	if code != 0 {
		t.Fatalf("bare c4 exit %d", code)
	}

	// Cat the stored content using the canonical c4m's C4 ID.
	catOut, catErr, code := runC4WithEnv(t, bin, env, "cat", canonicalID)
	if code != 0 {
		t.Fatalf("cat exit %d, stderr: %s", code, catErr)
	}

	// The stored content should match canonical form.
	if catOut != canonical {
		t.Fatalf("stored content should be canonical form:\n  stored:    %q\n  canonical: %q", catOut, canonical)
	}
}

// TestCatC4mFile verifies that c4 cat can display a c4m file from disk.
func TestCatC4mFile(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()

	// Create a directory and scan it.
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)
	os.WriteFile(filepath.Join(projectDir, "test.txt"), []byte("test"), 0644)

	canonical, _, _ := runC4(t, bin, "id", projectDir)
	c4mPath := filepath.Join(dir, "project.c4m")
	os.WriteFile(c4mPath, []byte(canonical), 0644)

	// c4 cat <file.c4m> should output canonical form.
	catOut, _, code := runC4(t, bin, "cat", c4mPath)
	if code != 0 {
		t.Fatalf("cat exit %d", code)
	}
	if catOut != canonical {
		t.Fatalf("cat output should match canonical:\n  cat: %q\n  expected: %q", catOut, canonical)
	}
}

// TestCatErgonomicFlag verifies that c4 cat -e pretty-prints c4m content.
func TestCatErgonomicFlag(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()

	// Create a directory with files.
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)
	os.WriteFile(filepath.Join(projectDir, "hello.txt"), []byte("hello"), 0644)

	canonical, _, _ := runC4(t, bin, "id", projectDir)
	pretty, _, _ := runC4(t, bin, "id", "-e", projectDir)

	c4mPath := filepath.Join(dir, "project.c4m")
	os.WriteFile(c4mPath, []byte(canonical), 0644)

	// c4 cat -e <file.c4m> should output pretty form.
	catOut, _, code := runC4(t, bin, "cat", "-e", c4mPath)
	if code != 0 {
		t.Fatalf("cat -e exit %d", code)
	}
	if catOut != pretty {
		t.Fatalf("cat -e output should match pretty:\n  cat: %q\n  expected: %q", catOut, pretty)
	}
}

// TestCatRecursiveExpand verifies that c4 cat -r expands directory entries.
func TestCatRecursiveExpand(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")

	// Create a directory with subdirectories.
	projectDir := filepath.Join(dir, "project")
	subDir := filepath.Join(projectDir, "sub")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(projectDir, "root.txt"), []byte("root"), 0644)
	os.WriteFile(filepath.Join(subDir, "child.txt"), []byte("child"), 0644)

	env := map[string]string{"C4_STORE": storeDir}

	// Scan and store the full tree (stores file content and subdirectory c4m).
	fullOut, _, code := runC4WithEnv(t, bin, env, "id", "-s", projectDir)
	if code != 0 {
		t.Fatalf("id -s exit %d", code)
	}

	if !strings.Contains(fullOut, "sub/") {
		t.Fatalf("expected sub/ in output: %s", fullOut)
	}

	// Save the full manifest.
	c4mPath := filepath.Join(dir, "project.c4m")
	os.WriteFile(c4mPath, []byte(fullOut), 0644)

	// c4 cat -r <file.c4m> should expand sub/ and show child.txt.
	catOut, _, code := runC4WithEnv(t, bin, env, "cat", "-r", c4mPath)
	if code != 0 {
		t.Fatalf("cat -r exit %d", code)
	}
	if !strings.Contains(catOut, "child.txt") {
		t.Fatalf("cat -r should expand sub/ to show child.txt: %s", catOut)
	}
	if !strings.Contains(catOut, "root.txt") {
		t.Fatalf("cat -r should include root.txt: %s", catOut)
	}

	// Also test cat -r from store by piping canonical c4m to get its ID.
	canonicalID, _, _ := runC4WithStdin(t, bin, fullOut, "-x")
	canonicalID = strings.TrimSpace(canonicalID)

	// The manifest should already be stored (storeManifestContent stores it).
	// But it was stored under storeManifestAsContent's Put ID.
	// For cat -r from store to work, we need the manifest stored.
	// Use c4 id -s on the c4m file to ensure it's stored.
	_, _, code = runC4WithEnv(t, bin, env, "id", "-s", c4mPath)
	if code != 0 {
		t.Fatalf("store c4m exit %d", code)
	}

	catOut2, catErr2, code := runC4WithEnv(t, bin, env, "cat", "-r", canonicalID)
	if code != 0 {
		t.Fatalf("cat -r <id> exit %d, stderr: %s", code, catErr2)
	}
	if !strings.Contains(catOut2, "child.txt") {
		t.Fatalf("cat -r <id> should expand sub/ to show child.txt: %s", catOut2)
	}
}

// TestCatRecursiveErgonomic verifies that c4 cat -r -e combines both flags.
func TestCatRecursiveErgonomic(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")

	// Create a directory with subdirectories.
	projectDir := filepath.Join(dir, "project")
	subDir := filepath.Join(projectDir, "sub")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(projectDir, "root.txt"), []byte("root"), 0644)
	os.WriteFile(filepath.Join(subDir, "child.txt"), []byte("child"), 0644)

	env := map[string]string{"C4_STORE": storeDir}

	// Scan and store.
	fullOut, _, _ := runC4WithEnv(t, bin, env, "id", "-s", projectDir)
	// Store subdirectory c4m.
	runC4WithEnv(t, bin, env, "id", "-s", subDir)

	c4mPath := filepath.Join(dir, "project.c4m")
	os.WriteFile(c4mPath, []byte(fullOut), 0644)

	// c4 cat -r -e should produce pretty-printed expanded output.
	catOut, _, code := runC4WithEnv(t, bin, env, "cat", "-r", "-e", c4mPath)
	if code != 0 {
		t.Fatalf("cat -r -e exit %d", code)
	}
	if !strings.Contains(catOut, "child.txt") {
		t.Fatalf("cat -r -e should expand and show child.txt: %s", catOut)
	}
	// Pretty-print produces local timestamps (not UTC Z-suffixed).
	// Just verify the output is non-empty and contains expected entries.
	if !strings.Contains(catOut, "root.txt") {
		t.Fatalf("cat -r -e should include root.txt: %s", catOut)
	}
}
