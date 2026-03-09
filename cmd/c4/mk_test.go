package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4/cmd/c4/internal/establish"
)

// --- mk subprocess tests ---

func TestMkC4m(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	cmd := exec.Command(bin, "mk", "test.c4m:")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mk: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "established") {
		t.Errorf("expected 'established' message, got %q", out)
	}

	// Verify c4m file is established in central registry
	if !establish.IsC4mEstablished(filepath.Join(dir, "test.c4m")) {
		t.Error("c4m file not established after mk")
	}
}

func TestMkC4mIdempotent(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	runMk := func() ([]byte, error) {
		cmd := exec.Command(bin, "mk", "test.c4m:")
		cmd.Dir = dir
		return cmd.CombinedOutput()
	}

	// First call establishes
	if out, err := runMk(); err != nil {
		t.Fatalf("mk first: %v\n%s", err, out)
	}

	// Second call reports already established (exits 0)
	out, err := runMk()
	if err != nil {
		t.Fatalf("mk second: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "already established") {
		t.Errorf("expected 'already established', got %q", out)
	}
}

func TestMkLocation(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	// Override HOME so location registry is in temp dir
	cmd := exec.Command(bin, "mk", "studio:", "cloud.example.com:7433")
	cmd.Dir = dir
	cmd.Env = subprocEnv(dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mk location: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "established") {
		t.Errorf("expected 'established' message, got %q", out)
	}
}

func TestMkLocationMissingAddress(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	cmd := exec.Command(bin, "mk", "studio:")
	cmd.Dir = dir
	cmd.Env = subprocEnv(dir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected error for location without address")
	}
	if !strings.Contains(string(out), "address") {
		t.Errorf("expected 'address' in error, got %q", out)
	}
}

func TestMkNoColonSuffix(t *testing.T) {
	bin := buildTestBinary(t)

	cmd := exec.Command(bin, "mk", "test.c4m")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected error for target without colon")
	}
	if !strings.Contains(string(out), "colon") {
		t.Errorf("expected 'colon' in error, got %q", out)
	}
}

func TestMkUsage(t *testing.T) {
	bin := buildTestBinary(t)

	cmd := exec.Command(bin, "mk")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected non-zero exit for no args")
	}
	if !strings.Contains(string(out), "Usage") {
		t.Errorf("expected usage message, got %q", out)
	}
}

// --- rm subprocess tests ---

func TestRmC4m(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	// First establish
	establish.EstablishC4m(filepath.Join(dir, "test.c4m"))

	// Then remove
	cmd := exec.Command(bin, "rm", "test.c4m:")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rm: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "removed") {
		t.Errorf("expected 'removed' message, got %q", out)
	}

	// Verify no longer established
	if establish.IsC4mEstablished(filepath.Join(dir, "test.c4m")) {
		t.Error("c4m file still established after rm")
	}
}

func TestRmC4mNotEstablished(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	cmd := exec.Command(bin, "rm", "test.c4m:")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected error for non-established c4m file")
	}
	if !strings.Contains(string(out), "not established") {
		t.Errorf("expected 'not established' in error, got %q", out)
	}
}

func TestRmLocation(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	// Set HOME to temp dir for this test and establish location
	setTestHome(t, dir)
	establish.EstablishLocation("studio", "cloud:7433")

	cmd := exec.Command(bin, "rm", "studio:")
	cmd.Dir = dir
	cmd.Env = subprocEnv(dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rm location: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "removed") {
		t.Errorf("expected 'removed' message, got %q", out)
	}
}

func TestRmNoColonSuffix(t *testing.T) {
	bin := buildTestBinary(t)

	cmd := exec.Command(bin, "rm", "test.c4m")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected error for target without colon")
	}
	if !strings.Contains(string(out), "local") {
		t.Errorf("expected 'local' in error, got %q", out)
	}
}

func TestRmUsage(t *testing.T) {
	bin := buildTestBinary(t)

	cmd := exec.Command(bin, "rm")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected non-zero exit for no args")
	}
	if !strings.Contains(string(out), "Usage") {
		t.Errorf("expected usage message, got %q", out)
	}
}

// --- mkdir subprocess tests ---

func TestMkdirBasic(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	// Establish c4m file first
	establish.EstablishC4m(filepath.Join(dir, "project.c4m"))

	cmd := exec.Command(bin, "mkdir", "project.c4m:renders/")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mkdir: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "created") {
		t.Errorf("expected 'created' message, got %q", out)
	}

	// Verify directory was added to manifest
	loaded, err := loadManifest(filepath.Join(dir, "project.c4m"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	found := false
	for _, e := range loaded.Entries {
		if e.Name == "renders/" && e.IsDir() {
			found = true
			break
		}
	}
	if !found {
		t.Error("renders/ not found in manifest after mkdir")
	}
}

func TestMkdirNotEstablished(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	cmd := exec.Command(bin, "mkdir", "project.c4m:renders/")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected error for non-established c4m file")
	}
	if !strings.Contains(string(out), "not established") {
		t.Errorf("expected 'not established' error, got %q", out)
	}
}

func TestMkdirNoSubpath(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	establish.EstablishC4m(filepath.Join(dir, "project.c4m"))

	cmd := exec.Command(bin, "mkdir", "project.c4m:")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected error for mkdir without subpath")
	}
	if !strings.Contains(string(out), "must specify") {
		t.Errorf("expected 'must specify' error, got %q", out)
	}
}

func TestMkdirIdempotent(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	establish.EstablishC4m(filepath.Join(dir, "project.c4m"))

	runMkdir := func() ([]byte, error) {
		cmd := exec.Command(bin, "mkdir", "project.c4m:renders/")
		cmd.Dir = dir
		return cmd.CombinedOutput()
	}

	// First call creates
	if out, err := runMkdir(); err != nil {
		t.Fatalf("mkdir first: %v\n%s", err, out)
	}

	// Second call reports already exists (exits 0)
	out, err := runMkdir()
	if err != nil {
		t.Fatalf("mkdir second: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "already exists") {
		t.Errorf("expected 'already exists', got %q", out)
	}
}

func TestMkdirLocalPath(t *testing.T) {
	bin := buildTestBinary(t)

	// Should fail because mkdir requires a c4m file path
	cmd := exec.Command(bin, "mkdir", "localdir/")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected error for local path")
	}
	if !strings.Contains(string(out), "c4m file") {
		t.Errorf("expected 'c4m file' in error, got %q", out)
	}
}

func TestMkdirUsage(t *testing.T) {
	bin := buildTestBinary(t)

	cmd := exec.Command(bin, "mkdir")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected non-zero exit for no args")
	}
	if !strings.Contains(string(out), "Usage") {
		t.Errorf("expected usage message, got %q", out)
	}
}

func TestMkdirNestedFailsWithoutParent(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	establish.EstablishC4m(filepath.Join(dir, "project.c4m"))

	// mkdir renders/shots/ should fail — renders/ doesn't exist
	cmd := exec.Command(bin, "mkdir", "project.c4m:renders/shots/")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected error for nested mkdir without -p")
	}
	if !strings.Contains(string(out), "does not exist") {
		t.Errorf("expected 'does not exist' error, got %q", out)
	}
}

func TestMkdirNestedWithParentFlag(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	establish.EstablishC4m(filepath.Join(dir, "project.c4m"))

	// mkdir -p renders/shots/ should create both directories
	cmd := exec.Command(bin, "mkdir", "-p", "project.c4m:renders/shots/")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mkdir -p: %v\n%s", err, out)
	}

	// Verify manifest has both directories at correct depths
	loaded, err := loadManifest(filepath.Join(dir, "project.c4m"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	var foundRenders, foundShots bool
	for _, e := range loaded.Entries {
		if e.Name == "renders/" && e.Depth == 0 && e.IsDir() {
			foundRenders = true
		}
		if e.Name == "shots/" && e.Depth == 1 && e.IsDir() {
			foundShots = true
		}
	}
	if !foundRenders {
		t.Error("renders/ not found at depth 0")
	}
	if !foundShots {
		t.Error("shots/ not found at depth 1")
	}
}

func TestMkdirNestedWithExistingParent(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	establish.EstablishC4m(filepath.Join(dir, "project.c4m"))

	// First create renders/
	cmd := exec.Command(bin, "mkdir", "project.c4m:renders/")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mkdir renders/: %v\n%s", err, out)
	}

	// Then mkdir renders/shots/ should succeed (parent exists)
	cmd = exec.Command(bin, "mkdir", "project.c4m:renders/shots/")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mkdir renders/shots/: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "created") {
		t.Errorf("expected 'created' message, got %q", out)
	}

	// Verify manifest structure
	loaded, err := loadManifest(filepath.Join(dir, "project.c4m"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	var foundShots bool
	for _, e := range loaded.Entries {
		if e.Name == "shots/" && e.Depth == 1 && e.IsDir() {
			foundShots = true
		}
	}
	if !foundShots {
		t.Error("shots/ not found at depth 1")
	}
}

func TestMkdirParentFlagIdempotent(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	establish.EstablishC4m(filepath.Join(dir, "project.c4m"))

	// First mkdir -p creates directories
	cmd := exec.Command(bin, "mkdir", "-p", "project.c4m:renders/shots/")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mkdir -p first: %v\n%s", err, out)
	}

	// Second mkdir -p reports already exists
	cmd = exec.Command(bin, "mkdir", "-p", "project.c4m:renders/shots/")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mkdir -p second: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "already exists") {
		t.Errorf("expected 'already exists', got %q", out)
	}
}

func TestMkdirDeepNested(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	establish.EstablishC4m(filepath.Join(dir, "project.c4m"))

	// mkdir -p a/b/c/ should create 3 levels
	cmd := exec.Command(bin, "mkdir", "-p", "project.c4m:a/b/c/")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mkdir -p a/b/c/: %v\n%s", err, out)
	}

	loaded, err := loadManifest(filepath.Join(dir, "project.c4m"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	expected := map[string]int{"a/": 0, "b/": 1, "c/": 2}
	for name, depth := range expected {
		found := false
		for _, e := range loaded.Entries {
			if e.Name == name && e.Depth == depth && e.IsDir() {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s not found at depth %d", name, depth)
		}
	}
}

// --- mk + mkdir + rm integration test ---

func TestEstablishmentLifecycle(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	mockURL := startMockC4d(t)

	runInDir := func(args ...string) ([]byte, error) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "C4D_ADDR="+mockURL)
		return cmd.CombinedOutput()
	}

	// 1. mk c4m file
	out, err := runInDir("mk", "project.c4m:")
	if err != nil {
		t.Fatalf("mk: %v\n%s", err, out)
	}

	// 2. mkdir inside c4m file
	out, err = runInDir("mkdir", "project.c4m:renders/")
	if err != nil {
		t.Fatalf("mkdir: %v\n%s", err, out)
	}

	// 3. cp file into c4m file
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("data"), 0644)
	out, err = runInDir("cp", "test.txt", "project.c4m:")
	if err != nil {
		t.Fatalf("cp: %v\n%s", err, out)
	}

	// 4. Verify manifest has both entries
	loaded, err := loadManifest(filepath.Join(dir, "project.c4m"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Entries) < 2 {
		t.Errorf("expected at least 2 entries, got %d", len(loaded.Entries))
	}

	// 5. rm establishment
	out, err = runInDir("rm", "project.c4m:")
	if err != nil {
		t.Fatalf("rm: %v\n%s", err, out)
	}

	// 6. cp should now fail (not established)
	out, err = runInDir("cp", "test.txt", "project.c4m:")
	if err == nil {
		t.Error("expected error after rm — c4m file should not be established")
	}
}

// --- rm --flow tests ---

func TestRmFlowClearsFlowLink(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	// Set up: create c4m with a directory and flow link
	run(t, bin, dir, "mk", "project.c4m:")
	run(t, bin, dir, "mkdir", "project.c4m:footage/")
	run(t, bin, dir, "ln", "->", "project.c4m:footage/", "nas:")

	// Verify flow link exists
	m := loadTestManifest(t, filepath.Join(dir, "project.c4m"))
	found := false
	for _, e := range m.Entries {
		if e.Name == "footage/" && e.IsFlowLinked() {
			found = true
		}
	}
	if !found {
		t.Fatal("flow link not found on footage/ before rm --flow")
	}

	// Remove the flow link
	run(t, bin, dir, "rm", "--flow", "project.c4m:footage/")

	// Verify entry remains but flow link is cleared
	m = loadTestManifest(t, filepath.Join(dir, "project.c4m"))
	for _, e := range m.Entries {
		if e.Name == "footage/" {
			if e.IsFlowLinked() {
				t.Error("flow link should be cleared after rm --flow")
			}
			if !e.IsDir() {
				t.Error("entry should still be a directory")
			}
			return
		}
	}
	t.Error("footage/ entry should still exist after rm --flow")
}

func TestRmFlowEntryNotFound(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	run(t, bin, dir, "mk", "project.c4m:")
	run(t, bin, dir, "mkdir", "project.c4m:existing/")

	cmd := exec.Command(bin, "rm", "--flow", "project.c4m:nonexistent/")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for rm --flow on nonexistent entry")
	}
	if !strings.Contains(string(out), "not found") {
		t.Errorf("expected 'not found' error, got %q", out)
	}
}

func TestRmFlowNoFlowLink(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	run(t, bin, dir, "mk", "project.c4m:")
	run(t, bin, dir, "mkdir", "project.c4m:renders/")

	cmd := exec.Command(bin, "rm", "--flow", "project.c4m:renders/")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for rm --flow on entry without flow link")
	}
	if !strings.Contains(string(out), "no flow link") {
		t.Errorf("expected 'no flow link' error, got %q", out)
	}
}

func TestRmFlowUsage(t *testing.T) {
	bin := buildTestBinary(t)

	cmd := exec.Command(bin, "rm", "--flow")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for rm --flow without target")
	}
	if !strings.Contains(string(out), "Usage") {
		t.Errorf("expected usage message, got %q", out)
	}
}

func TestRmFlowNotEstablished(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	cmd := exec.Command(bin, "rm", "--flow", "project.c4m:footage/")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for rm --flow on non-established c4m")
	}
	if !strings.Contains(string(out), "not established") {
		t.Errorf("expected 'not established' error, got %q", out)
	}
}

// --- Round-trip test: ln -> ls -> rm --flow -> ls ---

func TestFlowRoundTrip(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	run(t, bin, dir, "mk", "project.c4m:")
	run(t, bin, dir, "mkdir", "project.c4m:footage/")

	// 1. Create flow link
	run(t, bin, dir, "ln", "->", "project.c4m:footage/", "nas:raw/")

	// 2. ls shows flow inline
	cmd := exec.Command(bin, "ls", "project.c4m:")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ls: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "-> nas:raw/") {
		t.Errorf("ls should show flow operator, got %q", out)
	}

	// 3. ls -p (pretty) also shows flow
	cmd = exec.Command(bin, "ls", "-p", "project.c4m:")
	cmd.Dir = dir
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ls -p: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "-> nas:raw/") {
		t.Errorf("ls -p should show flow operator, got %q", out)
	}

	// 4. Remove flow link
	run(t, bin, dir, "rm", "--flow", "project.c4m:footage/")

	// 5. ls shows entry without flow
	cmd = exec.Command(bin, "ls", "project.c4m:")
	cmd.Dir = dir
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ls after rm --flow: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "-> nas:raw/") {
		t.Errorf("ls should NOT show flow after rm --flow, got %q", out)
	}
	// Entry should still exist
	if !strings.Contains(string(out), "footage/") {
		t.Errorf("footage/ entry should remain after rm --flow, got %q", out)
	}
}
