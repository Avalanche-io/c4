package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildC4 builds the c4 binary for testing.
func buildC4(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "c4")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = filepath.Join(os.Getenv("PWD"))
	if out, err := cmd.CombinedOutput(); err != nil {
		// Try with the test's working directory
		cmd = exec.Command("go", "build", "-o", bin, "./cmd/c4")
		cmd.Dir = filepath.Join(os.Getenv("PWD"), "..", "..")
		if out2, err2 := cmd.CombinedOutput(); err2 != nil {
			t.Fatalf("build c4: %v\n%s\n%s", err, out, out2)
		}
	}
	return bin
}

// setupManagedTestDir creates a temp dir with test files and returns the path.
func setupManagedTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world\n"), 0644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0644)
	sub := filepath.Join(dir, "src")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(sub, "main.go"), []byte("package main\n"), 0644)
	return dir
}

// runC4 runs the c4 binary with the given args in the given directory.
func runC4(t *testing.T, bin, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestManagedMkAndLs(t *testing.T) {
	bin := buildC4(t)
	dir := setupManagedTestDir(t)

	// c4 mk : — establish
	out, err := runC4(t, bin, dir, "mk", ":")
	if err != nil {
		t.Fatalf("c4 mk : failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "established") {
		t.Errorf("expected 'established' in output, got: %s", out)
	}

	// c4 ls : — should show managed state
	out, err = runC4(t, bin, dir, "ls", ":")
	if err != nil {
		t.Fatalf("c4 ls : failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "hello.txt") {
		t.Errorf("c4 ls : should contain hello.txt, got: %s", out)
	}
	if !strings.Contains(out, "README.md") {
		t.Errorf("c4 ls : should contain README.md, got: %s", out)
	}
	if !strings.Contains(out, "src/") {
		t.Errorf("c4 ls : should contain src/, got: %s", out)
	}
}

func TestManagedMkDoubleInit(t *testing.T) {
	bin := buildC4(t)
	dir := setupManagedTestDir(t)

	runC4(t, bin, dir, "mk", ":")

	// Second mk : should report already established
	out, err := runC4(t, bin, dir, "mk", ":")
	if err != nil {
		// Exit code may be non-zero, that's ok
	}
	if !strings.Contains(out, "already established") {
		t.Errorf("expected 'already established', got: %s", out)
	}
}

func TestManagedLsHistory(t *testing.T) {
	bin := buildC4(t)
	dir := setupManagedTestDir(t)

	runC4(t, bin, dir, "mk", ":")

	// c4 ls :~ — should show snapshot history
	out, err := runC4(t, bin, dir, "ls", ":~")
	if err != nil {
		t.Fatalf("c4 ls :~ failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "0/") {
		t.Errorf("expected '0/' in history, got: %s", out)
	}
}

func TestManagedLsSnapshotByNumber(t *testing.T) {
	bin := buildC4(t)
	dir := setupManagedTestDir(t)

	runC4(t, bin, dir, "mk", ":")

	// c4 ls :~0 — should return the initial snapshot
	out, err := runC4(t, bin, dir, "ls", ":~0")
	if err != nil {
		t.Fatalf("c4 ls :~0 failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "hello.txt") {
		t.Errorf("snapshot 0 should contain hello.txt, got: %s", out)
	}
}

func TestManagedUndoRedo(t *testing.T) {
	bin := buildC4(t)
	dir := setupManagedTestDir(t)

	runC4(t, bin, dir, "mk", ":")

	// Add a file and re-sync
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0644)
	out, err := runC4(t, bin, dir, "patch", ".", ":")
	if err != nil {
		t.Fatalf("c4 patch . : failed: %v\n%s", err, out)
	}

	// Verify new.txt is in managed state
	out, _ = runC4(t, bin, dir, "ls", ":")
	if !strings.Contains(out, "new.txt") {
		t.Errorf("after patch, ls : should contain new.txt, got: %s", out)
	}

	// Undo
	out, err = runC4(t, bin, dir, "undo", ":")
	if err != nil {
		t.Fatalf("c4 undo : failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "undone") {
		t.Errorf("expected 'undone' in output, got: %s", out)
	}

	// After undo, new.txt should not be in managed state
	out, _ = runC4(t, bin, dir, "ls", ":")
	if strings.Contains(out, "new.txt") {
		t.Errorf("after undo, ls : should NOT contain new.txt, got: %s", out)
	}

	// Redo
	out, err = runC4(t, bin, dir, "redo", ":")
	if err != nil {
		t.Fatalf("c4 redo : failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "redone") {
		t.Errorf("expected 'redone' in output, got: %s", out)
	}

	// After redo, new.txt should be back
	out, _ = runC4(t, bin, dir, "ls", ":")
	if !strings.Contains(out, "new.txt") {
		t.Errorf("after redo, ls : should contain new.txt, got: %s", out)
	}
}

func TestManagedUnrm(t *testing.T) {
	bin := buildC4(t)
	dir := setupManagedTestDir(t)

	runC4(t, bin, dir, "mk", ":")

	// Add file and snapshot
	os.WriteFile(filepath.Join(dir, "draft.txt"), []byte("draft\n"), 0644)
	runC4(t, bin, dir, "patch", ".", ":")

	// Remove the file from disk and re-sync
	os.Remove(filepath.Join(dir, "draft.txt"))
	runC4(t, bin, dir, "patch", ".", ":")

	// c4 unrm : should list draft.txt as recoverable
	out, err := runC4(t, bin, dir, "unrm", ":")
	if err != nil {
		t.Fatalf("c4 unrm : failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "draft.txt") {
		t.Errorf("unrm should list draft.txt as recoverable, got: %s", out)
	}
}

func TestManagedRmTeardown(t *testing.T) {
	bin := buildC4(t)
	dir := setupManagedTestDir(t)

	runC4(t, bin, dir, "mk", ":")

	// c4 rm : — tear down tracking
	out, err := runC4(t, bin, dir, "rm", ":")
	if err != nil {
		t.Fatalf("c4 rm : failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "removed tracking") {
		t.Errorf("expected 'removed tracking', got: %s", out)
	}

	// c4 ls : should now fail
	out, err = runC4(t, bin, dir, "ls", ":")
	if err == nil {
		t.Error("c4 ls : should fail after rm :")
	}

	// Files should still exist
	if _, err := os.Stat(filepath.Join(dir, "hello.txt")); err != nil {
		t.Error("hello.txt should still exist after rm :")
	}
}

func TestManagedMkWithExclude(t *testing.T) {
	bin := buildC4(t)
	dir := setupManagedTestDir(t)

	// Create a log file
	os.WriteFile(filepath.Join(dir, "app.log"), []byte("log\n"), 0644)

	// c4 mk : --exclude *.log
	out, err := runC4(t, bin, dir, "mk", ":", "--exclude", "*.log")
	if err != nil {
		t.Fatalf("c4 mk : --exclude failed: %v\n%s", err, out)
	}

	// ls : should not contain app.log
	out, _ = runC4(t, bin, dir, "ls", ":")
	if strings.Contains(out, "app.log") {
		t.Errorf("app.log should be excluded, got: %s", out)
	}

	// ls :~.ignore should show the pattern
	out, _ = runC4(t, bin, dir, "ls", ":~.ignore")
	if !strings.Contains(out, "*.log") {
		t.Errorf("expected *.log in ignore list, got: %s", out)
	}
}

func TestManagedLsIdFlag(t *testing.T) {
	bin := buildC4(t)
	dir := setupManagedTestDir(t)

	runC4(t, bin, dir, "mk", ":")

	// c4 ls -i : should output just a C4 ID
	out, err := runC4(t, bin, dir, "ls", "-i", ":")
	if err != nil {
		t.Fatalf("c4 ls -i : failed: %v\n%s", err, out)
	}
	id := strings.TrimSpace(out)
	if !strings.HasPrefix(id, "c4") {
		t.Errorf("expected C4 ID starting with c4, got: %s", id)
	}
	if len(id) != 90 {
		t.Errorf("expected 90-char C4 ID, got %d chars: %s", len(id), id)
	}
}

func TestManagedLnTag(t *testing.T) {
	bin := buildC4(t)
	dir := setupManagedTestDir(t)

	// Initialize managed directory
	runC4(t, bin, dir, "mk", ":")

	// Create a second snapshot by modifying a file and patching
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new content\n"), 0644)
	runC4(t, bin, dir, "patch", ".", ":")

	// Tag snapshot 1 (the initial state) as "baseline"
	out, err := runC4(t, bin, dir, "ln", ":~1", ":~baseline")
	if err != nil {
		t.Fatalf("c4 ln :~1 :~baseline failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "tagged") {
		t.Errorf("expected 'tagged' in output, got: %s", out)
	}

	// Verify the tag is browsable via ls
	out, err = runC4(t, bin, dir, "ls", ":~baseline")
	if err != nil {
		t.Fatalf("c4 ls :~baseline failed: %v\n%s", err, out)
	}

	// The tagged snapshot should match snapshot 1's ID
	tagID, err := runC4(t, bin, dir, "ls", "-i", ":~baseline")
	if err != nil {
		t.Fatalf("c4 ls -i :~baseline failed: %v\n%s", err, tagID)
	}
	snapID, err := runC4(t, bin, dir, "ls", "-i", ":~1")
	if err != nil {
		t.Fatalf("c4 ls -i :~1 failed: %v\n%s", err, snapID)
	}
	if strings.TrimSpace(tagID) != strings.TrimSpace(snapID) {
		t.Errorf("tag ID %s != snapshot 1 ID %s", strings.TrimSpace(tagID), strings.TrimSpace(snapID))
	}
}
