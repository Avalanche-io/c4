package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

func TestLnHardLink(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "original.txt"), []byte("content"), 0644)
	run(t, bin, dir, "mk", "test.c4m:")
	run(t, bin, dir, "cp", "original.txt", "test.c4m:")

	// Create hard link
	run(t, bin, dir, "ln", "test.c4m:original.txt", "test.c4m:copy.txt")

	m := loadTestManifest(t, filepath.Join(dir, "test.c4m"))

	expectedID := c4.Identify(strings.NewReader("content"))
	var foundOrig, foundCopy bool
	for _, e := range m.Entries {
		if e.Name == "original.txt" {
			foundOrig = true
			if e.C4ID != expectedID {
				t.Errorf("original C4 ID wrong: got %s", e.C4ID)
			}
		}
		if e.Name == "copy.txt" {
			foundCopy = true
			if e.C4ID != expectedID {
				t.Errorf("copy C4 ID should match original: got %s", e.C4ID)
			}
		}
	}
	if !foundOrig {
		t.Error("original.txt not found")
	}
	if !foundCopy {
		t.Error("copy.txt not found after ln")
	}
}

func TestLnSymlink(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	run(t, bin, dir, "mk", "test.c4m:")

	// Create symlink
	run(t, bin, dir, "ln", "-s", "../shared/config.yaml", "test.c4m:config.yaml")

	m := loadTestManifest(t, filepath.Join(dir, "test.c4m"))

	found := false
	for _, e := range m.Entries {
		if e.Name == "config.yaml" {
			found = true
			if !e.IsSymlink() {
				t.Error("expected symlink mode")
			}
			if e.Target != "../shared/config.yaml" {
				t.Errorf("wrong target: got %q, want %q", e.Target, "../shared/config.yaml")
			}
		}
	}
	if !found {
		t.Error("config.yaml symlink not found")
	}
}

func TestLnHardLinkNotFound(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	run(t, bin, dir, "mk", "test.c4m:")
	os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0644)
	run(t, bin, dir, "cp", "x.txt", "test.c4m:")

	cmd := exec.Command(bin, "ln", "test.c4m:nonexistent.txt", "test.c4m:link.txt")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for ln of nonexistent source")
	}
	if !strings.Contains(string(out), "not found") {
		t.Errorf("expected 'not found' error, got %q", out)
	}
}

func TestLnHardLinkDirectory(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	run(t, bin, dir, "mk", "test.c4m:")
	run(t, bin, dir, "mkdir", "test.c4m:mydir/")

	cmd := exec.Command(bin, "ln", "test.c4m:mydir/", "test.c4m:link/")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for hard linking a directory")
	}
	if !strings.Contains(string(out), "cannot hard link directories") {
		t.Errorf("expected directory error, got %q", out)
	}
}

func TestLnSymlinkInSubdir(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	run(t, bin, dir, "mk", "test.c4m:")
	run(t, bin, dir, "mkdir", "-p", "test.c4m:config/")

	// Create symlink inside a subdirectory
	run(t, bin, dir, "ln", "-s", "../../defaults.yaml", "test.c4m:config/settings.yaml")

	m := loadTestManifest(t, filepath.Join(dir, "test.c4m"))

	found := false
	for _, e := range m.Entries {
		if e.Name == "settings.yaml" && e.Depth == 1 {
			found = true
			if !e.IsSymlink() {
				t.Error("expected symlink mode")
			}
			if e.Target != "../../defaults.yaml" {
				t.Errorf("wrong target: got %q", e.Target)
			}
		}
	}
	if !found {
		t.Error("config/settings.yaml symlink not found at depth 1")
	}
}

// --- Flow link tests ---

func TestLnFlowOutbound(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	// Create c4m file with a directory entry
	run(t, bin, dir, "mk", "project.c4m:")
	run(t, bin, dir, "mkdir", "project.c4m:footage/")

	// Create outbound flow link
	run(t, bin, dir, "ln", "->", "nas:", "project.c4m:footage/")

	m := loadTestManifest(t, filepath.Join(dir, "project.c4m"))

	found := false
	for _, e := range m.Entries {
		if e.Name == "footage/" {
			found = true
			if e.FlowDirection != c4m.FlowOutbound {
				t.Errorf("expected FlowOutbound, got %d", e.FlowDirection)
			}
			if e.FlowTarget != "nas:" {
				t.Errorf("expected flow target %q, got %q", "nas:", e.FlowTarget)
			}
		}
	}
	if !found {
		t.Error("footage/ not found in manifest")
	}
}

func TestLnFlowInbound(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	run(t, bin, dir, "mk", "project.c4m:")
	run(t, bin, dir, "mkdir", "project.c4m:incoming/")

	// Create inbound flow link with subpath
	run(t, bin, dir, "ln", "<-", "studio:dailies/", "project.c4m:incoming/")

	m := loadTestManifest(t, filepath.Join(dir, "project.c4m"))

	found := false
	for _, e := range m.Entries {
		if e.Name == "incoming/" {
			found = true
			if e.FlowDirection != c4m.FlowInbound {
				t.Errorf("expected FlowInbound, got %d", e.FlowDirection)
			}
			if e.FlowTarget != "studio:dailies/" {
				t.Errorf("expected flow target %q, got %q", "studio:dailies/", e.FlowTarget)
			}
		}
	}
	if !found {
		t.Error("incoming/ not found in manifest")
	}
}

func TestLnFlowBidirectional(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	run(t, bin, dir, "mk", "project.c4m:")
	run(t, bin, dir, "mkdir", "project.c4m:shared/")

	// Create bidirectional flow link (shell-quoted <>)
	run(t, bin, dir, "ln", "<>", "desktop:", "project.c4m:shared/")

	m := loadTestManifest(t, filepath.Join(dir, "project.c4m"))

	found := false
	for _, e := range m.Entries {
		if e.Name == "shared/" {
			found = true
			if e.FlowDirection != c4m.FlowBidirectional {
				t.Errorf("expected FlowBidirectional, got %d", e.FlowDirection)
			}
			if e.FlowTarget != "desktop:" {
				t.Errorf("expected flow target %q, got %q", "desktop:", e.FlowTarget)
			}
		}
	}
	if !found {
		t.Error("shared/ not found in manifest")
	}
}

func TestLnFlowEntryNotFound(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	run(t, bin, dir, "mk", "project.c4m:")

	cmd := exec.Command(bin, "ln", "->", "nas:", "project.c4m:nonexistent/")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for flow link on nonexistent entry")
	}
	if !strings.Contains(string(out), "not found") {
		t.Errorf("expected 'not found' error, got %q", out)
	}
}

func TestLnFlowInvalidLocation(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	run(t, bin, dir, "mk", "project.c4m:")
	run(t, bin, dir, "mkdir", "project.c4m:data/")

	// Invalid location name (starts with number)
	cmd := exec.Command(bin, "ln", "->", "123bad:", "project.c4m:data/")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for invalid location name")
	}
	if !strings.Contains(string(out), "invalid location") {
		t.Errorf("expected 'invalid location' error, got %q", out)
	}
}

func TestLnFlowNoRemoteColon(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	run(t, bin, dir, "mk", "project.c4m:")
	run(t, bin, dir, "mkdir", "project.c4m:data/")

	// Remote ref missing colon
	cmd := exec.Command(bin, "ln", "->", "nas", "project.c4m:data/")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for remote ref without colon")
	}
	if !strings.Contains(string(out), ":") {
		t.Errorf("expected colon-related error, got %q", out)
	}
}

func TestLnFlowUsage(t *testing.T) {
	bin := buildTestBinary(t)

	// Too few arguments
	cmd := exec.Command(bin, "ln", "->", "nas:")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for missing local target")
	}
	if !strings.Contains(string(out), "Usage") {
		t.Errorf("expected usage message, got %q", out)
	}
}

func TestLnFlowManagedDirError(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	// Set up a managed directory
	run(t, bin, dir, "mk", ":")

	cmd := exec.Command(bin, "ln", "->", "nas:", ":footage/")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for flow link on managed directory")
	}
	if !strings.Contains(string(out), "not yet supported") {
		t.Errorf("expected 'not yet supported' error, got %q", out)
	}
}

func TestLnFlowNotEstablished(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	cmd := exec.Command(bin, "ln", "->", "nas:", "project.c4m:footage/")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for non-established c4m file")
	}
	if !strings.Contains(string(out), "not established") {
		t.Errorf("expected 'not established' error, got %q", out)
	}
}
