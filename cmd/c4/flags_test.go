package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests verify flag parsing behavior. They exercise both short (-e)
// and long (--ergonomic) flag forms, repeatable flags (--exclude), and
// flag/argument separation to ensure identical behavior after replacing pflag.

func TestIDFlagShortMode(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644)

	// -m s = structure mode (no C4 IDs)
	out, _, code := runC4(t, bin, "id", "-m", "s", dir)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(out, "c4") && !strings.Contains(out, " - ") && !strings.Contains(out, " -\n") {
		t.Fatalf("structure mode should not produce C4 IDs: %s", out)
	}
}

func TestIDFlagLongMode(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644)

	// --mode=s = structure mode
	out, _, code := runC4(t, bin, "id", "--mode", "s", dir)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	// Structure mode: C4 ID should be null
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		last := fields[len(fields)-1]
		if last != "-" {
			t.Fatalf("structure mode should have null C4 IDs, got: %s", last)
		}
	}
}

func TestIDFlagErgonomicShort(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644)

	// -e = ergonomic/pretty output
	out, _, code := runC4(t, bin, "id", "-e", dir)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	// Ergonomic form has aligned columns (extra spaces between fields)
	if !strings.Contains(out, "a.txt") {
		t.Fatalf("expected a.txt in output: %s", out)
	}
}

func TestIDFlagErgonomicLong(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644)

	// --ergonomic = same as -e
	out1, _, _ := runC4(t, bin, "id", "-e", dir)
	out2, _, code := runC4(t, bin, "id", "--ergonomic", dir)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if out1 != out2 {
		t.Fatalf("--ergonomic and -e should produce identical output")
	}
}

func TestIDFlagStoreShort(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("content"), 0644)

	// -s = store content
	out, _, code := runC4WithEnv(t, bin, map[string]string{"C4_STORE": storeDir}, "id", "-s", filepath.Join(dir, "test.txt"))
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "c4") {
		t.Fatalf("expected C4 ID in output: %s", out)
	}

	// Store directory should have been created
	if _, err := os.Stat(storeDir); os.IsNotExist(err) {
		t.Fatal("store directory should exist after -s")
	}
}

func TestIDFlagExclude(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0644)
	os.WriteFile(filepath.Join(dir, "skip.log"), []byte("skip"), 0644)

	// --exclude="*.log" should exclude .log files
	out, _, code := runC4(t, bin, "id", "--exclude", "*.log", dir)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(out, "skip.log") {
		t.Fatalf("--exclude should have excluded skip.log: %s", out)
	}
	if !strings.Contains(out, "keep.txt") {
		t.Fatalf("keep.txt should still be present: %s", out)
	}
}

func TestIDFlagExcludeMultiple(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0644)
	os.WriteFile(filepath.Join(dir, "skip.log"), []byte("skip"), 0644)
	os.WriteFile(filepath.Join(dir, "skip.tmp"), []byte("skip"), 0644)

	// Multiple --exclude flags
	out, _, code := runC4(t, bin, "id", "--exclude", "*.log", "--exclude", "*.tmp", dir)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(out, "skip.log") || strings.Contains(out, "skip.tmp") {
		t.Fatalf("both patterns should be excluded: %s", out)
	}
	if !strings.Contains(out, "keep.txt") {
		t.Fatalf("keep.txt should still be present: %s", out)
	}
}

func TestComposedShortFlags(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()

	dir1 := filepath.Join(dir, "v1")
	dir2 := filepath.Join(dir, "v2")
	os.MkdirAll(dir1, 0755)
	os.MkdirAll(dir2, 0755)
	os.WriteFile(filepath.Join(dir1, "a.txt"), []byte("old"), 0644)
	os.WriteFile(filepath.Join(dir2, "a.txt"), []byte("new"), 0644)

	// -re composes reverse + ergonomic on diff
	c4m1, _, _ := runC4(t, bin, "id", dir1)
	c4m2, _, _ := runC4(t, bin, "id", dir2)
	p1 := filepath.Join(dir, "v1.c4m")
	p2 := filepath.Join(dir, "v2.c4m")
	os.WriteFile(p1, []byte(c4m1), 0644)
	os.WriteFile(p2, []byte(c4m2), 0644)

	// -re should equal -r -e
	out1, _, _ := runC4(t, bin, "diff", "-r", "-e", p1, p2)
	out2, _, code := runC4(t, bin, "diff", "-re", p1, p2)
	if code != 0 {
		t.Fatalf("diff -re exit %d", code)
	}
	if out1 != out2 {
		t.Fatalf("-re and -r -e should produce identical output")
	}

	// -eS composes ergonomic + sequence on id
	out3, _, _ := runC4(t, bin, "id", "-e", "-S", dir1)
	out4, _, code := runC4(t, bin, "id", "-eS", dir1)
	if code != 0 {
		t.Fatalf("id -eS exit %d", code)
	}
	if out3 != out4 {
		t.Fatalf("-eS and -e -S should produce identical output")
	}
}

func TestDiffFlagErgonomicShort(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()

	dir1 := filepath.Join(dir, "v1")
	dir2 := filepath.Join(dir, "v2")
	os.MkdirAll(dir1, 0755)
	os.MkdirAll(dir2, 0755)
	os.WriteFile(filepath.Join(dir1, "a.txt"), []byte("old"), 0644)
	os.WriteFile(filepath.Join(dir2, "a.txt"), []byte("new"), 0644)

	c4m1, _, _ := runC4(t, bin, "id", dir1)
	c4m2, _, _ := runC4(t, bin, "id", dir2)
	p1 := filepath.Join(dir, "v1.c4m")
	p2 := filepath.Join(dir, "v2.c4m")
	os.WriteFile(p1, []byte(c4m1), 0644)
	os.WriteFile(p2, []byte(c4m2), 0644)

	// -e on diff
	_, _, code := runC4(t, bin, "diff", "-e", p1, p2)
	if code != 0 {
		t.Fatalf("diff -e exit %d", code)
	}
}

func TestPatchFlagNumberShort(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)
	os.WriteFile(filepath.Join(projectDir, "a.txt"), []byte("v1"), 0644)

	base, _, _ := runC4(t, bin, "id", projectDir)
	c4mPath := filepath.Join(dir, "project.c4m")
	os.WriteFile(c4mPath, []byte(base), 0644)

	// Add a patch
	os.WriteFile(filepath.Join(projectDir, "b.txt"), []byte("new"), 0644)
	newState, _, _ := runC4(t, bin, "id", projectDir)
	newPath := filepath.Join(dir, "new.c4m")
	os.WriteFile(newPath, []byte(newState), 0644)

	diff, _, _ := runC4(t, bin, "diff", c4mPath, newPath)
	f, _ := os.OpenFile(c4mPath, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString(diff)
	f.Close()

	// -n 1 = resolve only base (no patches applied)
	resolved, _, code := runC4(t, bin, "patch", "-n", "1", c4mPath)
	if code != 0 {
		t.Fatalf("patch -n 1 exit %d", code)
	}
	// Should match original base (no b.txt)
	if strings.Contains(resolved, "b.txt") {
		t.Fatalf("-n 1 should resolve to base only, but contains b.txt: %s", resolved)
	}
}

func TestPatchFlagNumberLong(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)
	os.WriteFile(filepath.Join(projectDir, "a.txt"), []byte("v1"), 0644)

	base, _, _ := runC4(t, bin, "id", projectDir)
	c4mPath := filepath.Join(dir, "project.c4m")
	os.WriteFile(c4mPath, []byte(base), 0644)

	os.WriteFile(filepath.Join(projectDir, "b.txt"), []byte("new"), 0644)
	newState, _, _ := runC4(t, bin, "id", projectDir)
	newPath := filepath.Join(dir, "new.c4m")
	os.WriteFile(newPath, []byte(newState), 0644)

	diff, _, _ := runC4(t, bin, "diff", c4mPath, newPath)
	f, _ := os.OpenFile(c4mPath, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString(diff)
	f.Close()

	// --number=1 = same as -n 1
	out1, _, _ := runC4(t, bin, "patch", "-n", "1", c4mPath)
	out2, _, code := runC4(t, bin, "patch", "--number", "1", c4mPath)
	if code != 0 {
		t.Fatalf("patch --number exit %d", code)
	}
	if out1 != out2 {
		t.Fatalf("--number and -n should produce identical output")
	}
}

func TestIDFlagContinue(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb"), 0644)

	// Structure scan first
	structOut, _, _ := runC4(t, bin, "id", "-m", "s", dir)
	guidePath := filepath.Join(dir, "guide.c4m")
	os.WriteFile(guidePath, []byte(structOut), 0644)

	// Continue with full mode using -c
	fullOut, _, code := runC4(t, bin, "id", "-c", guidePath, dir)
	if code != 0 {
		t.Fatalf("id -c exit %d", code)
	}
	// Should have C4 IDs now
	if !strings.Contains(fullOut, "c4") {
		t.Fatalf("continue scan should produce C4 IDs: %s", fullOut)
	}
}

func TestIDFlagContinueLong(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644)

	structOut, _, _ := runC4(t, bin, "id", "-m", "s", dir)
	guidePath := filepath.Join(dir, "guide.c4m")
	os.WriteFile(guidePath, []byte(structOut), 0644)

	// --continue = same as -c
	out1, _, _ := runC4(t, bin, "id", "-c", guidePath, dir)
	out2, _, code := runC4(t, bin, "id", "--continue", guidePath, dir)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if out1 != out2 {
		t.Fatalf("--continue and -c should produce identical output")
	}
}
