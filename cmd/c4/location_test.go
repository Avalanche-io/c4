package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/establish"
)

// startMockC4dLocation starts a mock c4d that serves c4m manifests at /~{location}/mnt/
func startMockC4dLocation(t *testing.T, manifest *c4m.Manifest) string {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check path starts with /~
		if !strings.HasPrefix(r.URL.Path, "/~") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		enc := c4m.NewEncoder(w)
		enc.Encode(manifest)
	}))
	t.Cleanup(ts.Close)
	return ts.URL
}

func TestLocationLs(t *testing.T) {
	// Create a manifest to serve
	m := c4m.NewManifest()
	m.AddEntry(&c4m.Entry{
		Name:      "renders/",
		Mode:      os.ModeDir | 0755,
		Timestamp: c4m.NullTimestamp(),
		Size:      -1,
	})
	m.AddEntry(&c4m.Entry{
		Name:      "data.txt",
		Mode:      0644,
		Timestamp: c4m.NullTimestamp(),
		Size:      42,
	})

	mockURL := startMockC4dLocation(t, m)

	bin := buildTestBinary(t)
	dir := t.TempDir()

	// Establish the location
	setTestHome(t, dir)
	establish.EstablishLocation("testloc", "localhost:9999")

	cmd := exec.Command(bin, "ls", "testloc:")
	cmd.Dir = dir
	cmd.Env = subprocEnv(dir, "C4D_ADDR="+mockURL)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ls testloc: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "renders/") {
		t.Errorf("expected renders/ in output, got %q", out)
	}
	if !strings.Contains(string(out), "data.txt") {
		t.Errorf("expected data.txt in output, got %q", out)
	}
}

func TestLocationLsWithSubpath(t *testing.T) {
	// Create a manifest with nested entries
	m := c4m.NewManifest()
	m.AddEntry(&c4m.Entry{
		Name:      "footage/",
		Mode:      os.ModeDir | 0755,
		Timestamp: c4m.NullTimestamp(),
		Size:      -1,
	})
	m.AddEntry(&c4m.Entry{
		Name:  "shot01.exr",
		Depth: 1,
		Mode:  0644,
		Timestamp: c4m.NullTimestamp(),
		Size:  1024,
	})

	mockURL := startMockC4dLocation(t, m)

	bin := buildTestBinary(t)
	dir := t.TempDir()

	setTestHome(t, dir)
	establish.EstablishLocation("nas", "localhost:9999")

	cmd := exec.Command(bin, "ls", "nas:footage/")
	cmd.Dir = dir
	cmd.Env = subprocEnv(dir, "C4D_ADDR="+mockURL)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ls nas:footage/: %v\n%s", err, out)
	}
	// The mock serves the full manifest regardless of path,
	// so just verify no error occurred and we got c4m output
	if len(out) == 0 {
		t.Error("expected non-empty output")
	}
}

func TestLocationLsPretty(t *testing.T) {
	m := c4m.NewManifest()
	m.AddEntry(&c4m.Entry{
		Name:      "data.txt",
		Mode:      0644,
		Timestamp: c4m.NullTimestamp(),
		Size:      42,
	})

	mockURL := startMockC4dLocation(t, m)

	bin := buildTestBinary(t)
	dir := t.TempDir()

	setTestHome(t, dir)
	establish.EstablishLocation("remote", "localhost:9999")

	cmd := exec.Command(bin, "ls", "-p", "remote:")
	cmd.Dir = dir
	cmd.Env = subprocEnv(dir, "C4D_ADDR="+mockURL)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ls -p remote: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "data.txt") {
		t.Errorf("expected data.txt in pretty output, got %q", out)
	}
}

func TestLocationLsC4dNotRunning(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	setTestHome(t, dir)
	establish.EstablishLocation("offline", "localhost:9999")

	// Use a port that nothing listens on
	cmd := exec.Command(bin, "ls", "offline:")
	cmd.Dir = dir
	cmd.Env = subprocEnv(dir, "C4D_ADDR=http://localhost:19999")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error when c4d is not running")
	}
	if !strings.Contains(string(out), "c4d not reachable") {
		t.Errorf("expected 'c4d not reachable' error, got %q", out)
	}
}

func TestLocationDiff(t *testing.T) {
	// Test that getManifest handles Location type for diff
	m := c4m.NewManifest()
	m.AddEntry(&c4m.Entry{
		Name:      "file.txt",
		Mode:      0644,
		Timestamp: c4m.NullTimestamp(),
		Size:      100,
	})

	mockURL := startMockC4dLocation(t, m)

	bin := buildTestBinary(t)
	dir := t.TempDir()

	setTestHome(t, dir)
	establish.EstablishLocation("loc1", "localhost:9999")

	// Diff location against itself (should produce empty diff)
	cmd := exec.Command(bin, "diff", "loc1:", "loc1:")
	cmd.Dir = dir
	cmd.Env = subprocEnv(dir, "C4D_ADDR="+mockURL)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("diff loc1: loc1: %v\n%s", err, out)
	}
	// Empty diff should produce no output
	if len(strings.TrimSpace(string(out))) > 0 {
		fmt.Printf("diff output: %q\n", string(out))
	}
}

func TestLocationLsId(t *testing.T) {
	m := c4m.NewManifest()
	m.AddEntry(&c4m.Entry{
		Name:      "file.txt",
		Mode:      0644,
		Timestamp: c4m.NullTimestamp(),
		Size:      50,
	})

	mockURL := startMockC4dLocation(t, m)

	bin := buildTestBinary(t)
	dir := t.TempDir()

	setTestHome(t, dir)
	establish.EstablishLocation("idloc", "localhost:9999")

	cmd := exec.Command(bin, "ls", "-i", "idloc:")
	cmd.Dir = dir
	cmd.Env = subprocEnv(dir, "C4D_ADDR="+mockURL)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ls -i idloc: %v\n%s", err, out)
	}
	trimmed := strings.TrimSpace(string(out))
	if !strings.HasPrefix(trimmed, "c4") {
		t.Errorf("expected C4 ID output, got %q", trimmed)
	}
}
