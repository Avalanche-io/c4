package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/establish"
	"github.com/Avalanche-io/c4/cmd/c4/internal/scan"
)

// startMockC4d starts a test HTTP server that accepts PUT (store) and GET (retrieve)
// requests, mimicking c4d's content-addressed storage behavior.
func startMockC4d(t *testing.T) string {
	var mu sync.Mutex
	store := make(map[string][]byte)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			data, _ := io.ReadAll(r.Body)
			id := c4.Identify(bytes.NewReader(data))
			mu.Lock()
			store[id.String()] = data
			mu.Unlock()
		case http.MethodGet:
			key := strings.TrimPrefix(r.URL.Path, "/")
			mu.Lock()
			data, ok := store[key]
			mu.Unlock()
			if ok {
				w.Write(data)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(ts.Close)
	return ts.URL
}

var testBinaryPath string

func TestMain(m *testing.M) {
	// Build the binary once for all subprocess tests
	tmpDir, err := os.MkdirTemp("", "c4-test-*")
	if err != nil {
		panic(err)
	}
	bin := filepath.Join(tmpDir, "c4")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		panic("build c4: " + err.Error() + "\n" + string(out))
	}
	testBinaryPath = bin

	// Set HOME for centralized capsule/location registry (~/.c4/)
	testHome := filepath.Join(tmpDir, "home")
	os.MkdirAll(testHome, 0755)
	os.Setenv("HOME", testHome)

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func buildTestBinary(t *testing.T) string {
	t.Helper()
	return testBinaryPath
}

// --- Helper function tests (no subprocess needed) ---

func TestEnsureParentDirs(t *testing.T) {
	m := c4m.NewManifest()
	ensureParentDirs(m, "renders/shots/")

	if len(m.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m.Entries))
	}

	// First should be renders/ at depth 0
	if m.Entries[0].Name != "renders/" || m.Entries[0].Depth != 0 {
		t.Errorf("entry[0] = {%q, depth=%d}, want {renders/, depth=0}",
			m.Entries[0].Name, m.Entries[0].Depth)
	}

	// Second should be shots/ at depth 1
	if m.Entries[1].Name != "shots/" || m.Entries[1].Depth != 1 {
		t.Errorf("entry[1] = {%q, depth=%d}, want {shots/, depth=1}",
			m.Entries[1].Name, m.Entries[1].Depth)
	}
}

func TestEnsureParentDirsIdempotent(t *testing.T) {
	m := c4m.NewManifest()
	ensureParentDirs(m, "renders/")
	ensureParentDirs(m, "renders/") // second call should be a no-op

	if len(m.Entries) != 1 {
		t.Fatalf("expected 1 entry after idempotent call, got %d", len(m.Entries))
	}
}

func TestEnsureParentDirsDeep(t *testing.T) {
	m := c4m.NewManifest()
	ensureParentDirs(m, "a/b/c/")

	if len(m.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(m.Entries))
	}

	names := []string{"a/", "b/", "c/"}
	for i, want := range names {
		if m.Entries[i].Name != want {
			t.Errorf("entry[%d].Name = %q, want %q", i, m.Entries[i].Name, want)
		}
		if m.Entries[i].Depth != i {
			t.Errorf("entry[%d].Depth = %d, want %d", i, m.Entries[i].Depth, i)
		}
	}
}

func TestInsertUnderParentNoSubpath(t *testing.T) {
	m := c4m.NewManifest()
	entry := &c4m.Entry{Name: "file.txt", Depth: 0, Size: 100}
	insertUnderParent(m, entry, "")

	if len(m.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m.Entries))
	}
	if m.Entries[0].Name != "file.txt" {
		t.Errorf("entry name = %q, want file.txt", m.Entries[0].Name)
	}
}

func TestInsertUnderParentWithSubpath(t *testing.T) {
	m := c4m.NewManifest()

	// Add parent directory
	m.AddEntry(&c4m.Entry{
		Name:  "renders/",
		Depth: 0,
		Mode:  os.ModeDir | 0755,
		Size:  -1,
	})

	// Insert file under renders/
	entry := &c4m.Entry{Name: "frame.exr", Depth: 1, Size: 1024}
	insertUnderParent(m, entry, "renders/")

	if len(m.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m.Entries))
	}
	if m.Entries[1].Name != "frame.exr" {
		t.Errorf("entry[1].Name = %q, want frame.exr", m.Entries[1].Name)
	}
	if m.Entries[1].Depth != 1 {
		t.Errorf("entry[1].Depth = %d, want 1", m.Entries[1].Depth)
	}
}

func TestInsertUnderParentMultipleChildren(t *testing.T) {
	m := c4m.NewManifest()

	// Add two directories at depth 0
	m.AddEntry(&c4m.Entry{Name: "assets/", Depth: 0, Mode: os.ModeDir | 0755, Size: -1})
	m.AddEntry(&c4m.Entry{Name: "renders/", Depth: 0, Mode: os.ModeDir | 0755, Size: -1})

	// Insert a child under renders/ (should go after renders/, not at the end after assets/)
	entry := &c4m.Entry{Name: "frame.exr", Depth: 1, Size: 1024}
	insertUnderParent(m, entry, "renders/")

	if len(m.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(m.Entries))
	}

	// Entry should be right after renders/, not after assets/
	if m.Entries[1].Name != "renders/" {
		t.Errorf("entry[1].Name = %q, want renders/", m.Entries[1].Name)
	}
	if m.Entries[2].Name != "frame.exr" {
		t.Errorf("entry[2].Name = %q, want frame.exr", m.Entries[2].Name)
	}
}

func TestLoadOrCreateManifestNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.c4m")

	m, err := loadOrCreateManifest(path)
	if err != nil {
		t.Fatalf("loadOrCreateManifest: %v", err)
	}
	if m == nil {
		t.Fatal("loadOrCreateManifest returned nil")
	}
	if len(m.Entries) != 0 {
		t.Errorf("new manifest has %d entries, want 0", len(m.Entries))
	}
}

func TestLoadOrCreateManifestExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.c4m")

	// Write a manifest
	m := c4m.NewManifest()
	m.AddEntry(&c4m.Entry{
		Name: "test.txt",
		Mode: 0644,
		Size: 42,
	})
	if err := writeManifest(path, m); err != nil {
		t.Fatalf("writeManifest: %v", err)
	}

	// Load it back
	loaded, err := loadOrCreateManifest(path)
	if err != nil {
		t.Fatalf("loadOrCreateManifest: %v", err)
	}
	if len(loaded.Entries) != 1 {
		t.Fatalf("loaded manifest has %d entries, want 1", len(loaded.Entries))
	}
	if loaded.Entries[0].Name != "test.txt" {
		t.Errorf("entry name = %q, want test.txt", loaded.Entries[0].Name)
	}
}

func TestWriteManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roundtrip.c4m")

	m := c4m.NewManifest()
	m.AddEntry(&c4m.Entry{Name: "a.txt", Mode: 0644, Size: 10})
	m.AddEntry(&c4m.Entry{Name: "b.txt", Mode: 0644, Size: 20})

	if err := writeManifest(path, m); err != nil {
		t.Fatalf("writeManifest: %v", err)
	}

	loaded, err := loadManifest(path)
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}

	if len(loaded.Entries) != 2 {
		t.Fatalf("loaded %d entries, want 2", len(loaded.Entries))
	}
}

// --- c4d HTTP integration tests ---

func TestPutToC4d(t *testing.T) {
	var receivedBody []byte
	var receivedMethod string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		body := new(bytes.Buffer)
		body.ReadFrom(r.Body)
		receivedBody = body.Bytes()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	t.Setenv("C4D_ADDR", ts.URL)

	// Create a temp file to push
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello c4d"), 0644)

	if err := putToC4d(path); err != nil {
		t.Fatalf("putToC4d: %v", err)
	}

	if receivedMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", receivedMethod)
	}
	if string(receivedBody) != "hello c4d" {
		t.Errorf("body = %q, want %q", receivedBody, "hello c4d")
	}
}

func TestPutToC4dError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	t.Setenv("C4D_ADDR", ts.URL)

	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("data"), 0644)

	err := putToC4d(path)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestWriteFileContentStreams(t *testing.T) {
	content := "streamed file content"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(content))
	}))
	defer ts.Close()

	t.Setenv("C4D_ADDR", ts.URL)

	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	id := identifyString(content)
	entry := &c4m.Entry{
		Name: "out.txt",
		Mode: 0644,
		Size: int64(len(content)),
		C4ID: id,
	}

	if err := writeFileContent(path, entry); err != nil {
		t.Fatalf("writeFileContent: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(got) != content {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestWriteFileContentFailsOnNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	t.Setenv("C4D_ADDR", ts.URL)

	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	id := identifyString("test content")
	entry := &c4m.Entry{
		Name: "out.txt",
		Mode: 0644,
		Size: 12,
		C4ID: id,
	}

	err := writeFileContent(path, entry)
	if err == nil {
		t.Error("expected error for 404 response, got nil")
	}
}

func TestC4dAddr(t *testing.T) {
	// Default
	t.Setenv("C4D_ADDR", "")
	os.Unsetenv("C4D_ADDR")
	if got := c4dAddr(); got != "http://localhost:17433" {
		t.Errorf("default c4dAddr = %q, want http://localhost:17433", got)
	}

	// Custom
	t.Setenv("C4D_ADDR", "http://myhost:9999")
	if got := c4dAddr(); got != "http://myhost:9999" {
		t.Errorf("custom c4dAddr = %q, want http://myhost:9999", got)
	}
}

// --- Integration tests using subprocess ---

func TestCpLocalToCapsuleIntegration(t *testing.T) {
	dir := t.TempDir()

	// Create source files
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("file a"), 0644)
	os.WriteFile(filepath.Join(srcDir, "b.txt"), []byte("file b"), 0644)

	capsulePath := filepath.Join(dir, "test.c4m")

	// Establish the capsule
	if err := establish.EstablishCapsule(capsulePath); err != nil {
		t.Fatalf("establish: %v", err)
	}

	// Scan source directory to build manifest
	gen := scan.NewGeneratorWithOptions(scan.WithC4IDs(true))
	scanned, err := gen.GenerateFromPath(srcDir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	// Create the manifest from scanned entries
	manifest := c4m.NewManifest()
	for _, entry := range scanned.Entries {
		manifest.AddEntry(entry)
	}
	manifest.SortEntries()
	scan.PropagateMetadata(manifest.Entries)

	if err := writeManifest(capsulePath, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	// Verify the manifest was created and has entries
	loaded, err := loadManifest(capsulePath)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	if len(loaded.Entries) != 2 {
		t.Errorf("manifest has %d entries, want 2", len(loaded.Entries))
	}

	// Check entry names
	names := make(map[string]bool)
	for _, e := range loaded.Entries {
		names[e.Name] = true
	}
	if !names["a.txt"] || !names["b.txt"] {
		t.Errorf("entries = %v, want a.txt and b.txt", names)
	}
}

func TestCpLocalToCapsuleWithSubpath(t *testing.T) {
	dir := t.TempDir()

	// Create source file
	srcFile := filepath.Join(dir, "frame.exr")
	os.WriteFile(srcFile, []byte("frame data"), 0644)

	capsulePath := filepath.Join(dir, "project.c4m")

	// Establish the capsule
	establish.EstablishCapsule(capsulePath)

	// Create manifest with renders/ directory
	manifest := c4m.NewManifest()
	ensureParentDirs(manifest, "renders/")
	if err := writeManifest(capsulePath, manifest); err != nil {
		t.Fatalf("write initial manifest: %v", err)
	}

	// Simulate adding a file under renders/
	manifest, _ = loadManifest(capsulePath)
	entry := &c4m.Entry{
		Name:  "frame.exr",
		Depth: 1,
		Mode:  0644,
		Size:  10,
	}
	insertUnderParent(manifest, entry, "renders/")
	manifest.SortEntries()

	if err := writeManifest(capsulePath, manifest); err != nil {
		t.Fatalf("write updated manifest: %v", err)
	}

	// Verify
	loaded, _ := loadManifest(capsulePath)
	if len(loaded.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(loaded.Entries))
	}

	// Find the file entry
	found := false
	for _, e := range loaded.Entries {
		if e.Name == "frame.exr" && e.Depth == 1 {
			found = true
			break
		}
	}
	if !found {
		t.Error("frame.exr at depth 1 not found in manifest")
	}
}

func TestCpCapsuleToLocalIntegration(t *testing.T) {
	dir := t.TempDir()

	// Start a mock c4d that returns file content
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("restored content"))
	}))
	defer ts.Close()
	t.Setenv("C4D_ADDR", ts.URL)

	// Create a capsule with a file entry
	capsulePath := filepath.Join(dir, "project.c4m")
	manifest := c4m.NewManifest()

	fileID := identifyString("restored content")
	manifest.AddEntry(&c4m.Entry{
		Name: "output.txt",
		Mode: 0644,
		Size: 16,
		C4ID: fileID,
	})
	manifest.SortEntries()
	writeManifest(capsulePath, manifest)

	// Materialize to output directory
	outDir := filepath.Join(dir, "output")
	os.MkdirAll(outDir, 0755)

	// Load and materialize manually (since cpCapsuleToLocal calls os.Exit)
	loaded, err := loadManifest(capsulePath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Write file content
	for _, entry := range loaded.Entries {
		if entry.IsDir() {
			continue
		}
		fullPath := filepath.Join(outDir, entry.Name)
		if err := writeFileContent(fullPath, entry); err != nil {
			t.Fatalf("writeFileContent: %v", err)
		}
	}

	// Verify the file was created with content from c4d
	data, err := os.ReadFile(filepath.Join(outDir, "output.txt"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "restored content" {
		t.Errorf("output = %q, want %q", data, "restored content")
	}
}

func TestWriteFileContentErrorsOnFetchFailure(t *testing.T) {
	dir := t.TempDir()

	// Mock c4d that returns 404 for all content
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	t.Setenv("C4D_ADDR", ts.URL)

	fileID := identifyString("some content")
	entry := &c4m.Entry{
		Name: "test.txt",
		Mode: 0644,
		Size: 12,
		C4ID: fileID,
	}

	outPath := filepath.Join(dir, "test.txt")
	err := writeFileContent(outPath, entry)
	if err == nil {
		t.Fatal("expected error when c4d returns 404, got nil")
	}
	if !strings.Contains(err.Error(), "c4d fetch") {
		t.Errorf("error should mention c4d fetch, got: %v", err)
	}
}

func TestCpCapsuleToLocalNilID(t *testing.T) {
	dir := t.TempDir()

	// Entry with nil C4 ID should create an empty file
	entry := &c4m.Entry{
		Name: "empty.txt",
		Mode: 0644,
		Size: 0,
	}

	outPath := filepath.Join(dir, "empty.txt")
	if err := writeFileContent(outPath, entry); err != nil {
		t.Fatalf("writeFileContent: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("nil ID file should be empty, got %d bytes", len(data))
	}
}

func TestCpCapsuleToLocalWithDirectories(t *testing.T) {
	dir := t.TempDir()

	// Mock c4d
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("file data"))
	}))
	defer ts.Close()
	t.Setenv("C4D_ADDR", ts.URL)

	// Create a capsule with a nested directory structure
	capsulePath := filepath.Join(dir, "project.c4m")
	manifest := c4m.NewManifest()
	manifest.AddEntry(&c4m.Entry{
		Name:  "renders/",
		Depth: 0,
		Mode:  os.ModeDir | 0755,
		Size:  -1,
	})
	manifest.AddEntry(&c4m.Entry{
		Name:  "frame.exr",
		Depth: 1,
		Mode:  0644,
		Size:  9,
		C4ID:  identifyString("file data"),
	})
	manifest.SortEntries()
	scan.PropagateMetadata(manifest.Entries)
	writeManifest(capsulePath, manifest)

	// Materialize
	outDir := filepath.Join(dir, "output")

	// Build resolved paths (replicate the logic from cpCapsuleToLocal)
	loaded, _ := loadManifest(capsulePath)
	type pathEntry struct {
		fullPath string
		entry    *c4m.Entry
	}
	var resolved []pathEntry
	var dirStack []string

	for _, entry := range loaded.Entries {
		if entry.Depth < len(dirStack) {
			dirStack = dirStack[:entry.Depth]
		}
		var fullPath string
		if len(dirStack) > 0 {
			fullPath = strings.Join(dirStack, "") + entry.Name
		} else {
			fullPath = entry.Name
		}
		resolved = append(resolved, pathEntry{fullPath: fullPath, entry: entry})
		if entry.IsDir() {
			for len(dirStack) <= entry.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[entry.Depth] = entry.Name
		}
	}

	os.MkdirAll(outDir, 0755)
	for _, pe := range resolved {
		fullPath := filepath.Join(outDir, pe.fullPath)
		if pe.entry.IsDir() {
			os.MkdirAll(fullPath, 0755)
		} else {
			os.MkdirAll(filepath.Dir(fullPath), 0755)
			writeFileContent(fullPath, pe.entry)
		}
	}

	// Verify renders/ directory exists
	if _, err := os.Stat(filepath.Join(outDir, "renders")); err != nil {
		t.Error("renders/ directory not created")
	}

	// Verify file exists
	data, err := os.ReadFile(filepath.Join(outDir, "renders", "frame.exr"))
	if err != nil {
		t.Fatalf("read frame.exr: %v", err)
	}
	if string(data) != "file data" {
		t.Errorf("frame.exr = %q, want %q", data, "file data")
	}
}

func TestCpIncrementalCapture(t *testing.T) {
	dir := t.TempDir()

	capsulePath := filepath.Join(dir, "project.c4m")
	establish.EstablishCapsule(capsulePath)

	// First capture: add a.txt
	m := c4m.NewManifest()
	m.AddEntry(&c4m.Entry{Name: "a.txt", Mode: 0644, Size: 5})
	m.SortEntries()
	writeManifest(capsulePath, m)

	// Second capture: add b.txt via loadOrCreate + AddEntry
	m, _ = loadOrCreateManifest(capsulePath)
	m.AddEntry(&c4m.Entry{Name: "b.txt", Mode: 0644, Size: 10})
	m.SortEntries()
	writeManifest(capsulePath, m)

	// Verify both entries exist
	loaded, _ := loadManifest(capsulePath)
	if len(loaded.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(loaded.Entries))
	}

	names := make(map[string]bool)
	for _, e := range loaded.Entries {
		names[e.Name] = true
	}
	if !names["a.txt"] || !names["b.txt"] {
		t.Errorf("entries = %v, want a.txt and b.txt", names)
	}
}

func TestCpIncrementalCaptureSubpath(t *testing.T) {
	dir := t.TempDir()

	capsulePath := filepath.Join(dir, "project.c4m")
	establish.EstablishCapsule(capsulePath)

	// Create manifest with renders/ directory and a file
	m := c4m.NewManifest()
	ensureParentDirs(m, "renders/")
	m.AddEntry(&c4m.Entry{Name: "frame_001.exr", Depth: 1, Mode: 0644, Size: 100})
	m.SortEntries()
	writeManifest(capsulePath, m)

	// Add another file under renders/
	m, _ = loadOrCreateManifest(capsulePath)
	entry := &c4m.Entry{Name: "frame_002.exr", Depth: 1, Mode: 0644, Size: 100}
	insertUnderParent(m, entry, "renders/")
	m.SortEntries()
	writeManifest(capsulePath, m)

	// Verify
	loaded, _ := loadManifest(capsulePath)
	fileCount := 0
	for _, e := range loaded.Entries {
		if !e.IsDir() {
			fileCount++
		}
	}
	if fileCount != 2 {
		t.Errorf("expected 2 files, got %d", fileCount)
	}
}

// --- CLI subprocess tests ---

func TestCpSubprocessUsage(t *testing.T) {
	bin := buildTestBinary(t)

	// No args should print usage
	cmd := exec.Command(bin, "cp")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected non-zero exit for no args")
	}
	if !strings.Contains(string(out), "Usage: c4 cp") {
		t.Errorf("expected usage message, got %q", out)
	}
}

func TestCpSubprocessLocalToLocal(t *testing.T) {
	bin := buildTestBinary(t)

	// local-to-local should error
	cmd := exec.Command(bin, "cp", "file1", "file2")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected non-zero exit for local-to-local")
	}
	if !strings.Contains(string(out), "use OS cp") {
		t.Errorf("expected 'use OS cp' message, got %q", out)
	}
}

func TestCpSubprocessNotEstablished(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "src"), 0755)
	os.WriteFile(filepath.Join(dir, "src", "test.txt"), []byte("data"), 0644)

	// cp into non-established capsule should error
	cmd := exec.Command(bin, "cp", "src", "test.c4m:")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected non-zero exit for non-established capsule")
	}
	if !strings.Contains(string(out), "not established") {
		t.Errorf("expected 'not established' message, got %q", out)
	}
}

func TestCpSubprocessCaptureAndMaterialize(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	mockURL := startMockC4d(t)

	// Create source files
	os.MkdirAll(filepath.Join(dir, "src"), 0755)
	os.WriteFile(filepath.Join(dir, "src", "hello.txt"), []byte("hello world"), 0644)

	runInDir := func(args ...string) ([]byte, error) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "C4D_ADDR="+mockURL)
		return cmd.CombinedOutput()
	}

	// Establish
	if out, err := runInDir("mk", "test.c4m:"); err != nil {
		t.Fatalf("mk: %v\n%s", err, out)
	}

	// Capture
	out, err := runInDir("cp", "src", "test.c4m:")
	if err != nil {
		t.Fatalf("cp capture: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "captured") {
		t.Errorf("expected 'captured' message, got %q", out)
	}

	// Verify c4m file exists and has content
	loaded, err := loadManifest(filepath.Join(dir, "test.c4m"))
	if err != nil {
		t.Fatalf("load c4m: %v", err)
	}
	if len(loaded.Entries) == 0 {
		t.Error("c4m has no entries after capture")
	}

	// Materialize — should produce real content via mock c4d
	out, err = runInDir("cp", "test.c4m:", "out")
	if err != nil {
		t.Fatalf("cp materialize: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "materialized") {
		t.Errorf("expected 'materialized' message, got %q", out)
	}

	// Verify actual file content (not a stub)
	data, err := os.ReadFile(filepath.Join(dir, "out", "hello.txt"))
	if err != nil {
		t.Fatalf("hello.txt not created: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("hello.txt content = %q, want %q", data, "hello world")
	}
}

func TestCpSubprocessRecursiveFlag(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	mockURL := startMockC4d(t)

	// Create nested source
	os.MkdirAll(filepath.Join(dir, "src", "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "src", "top.txt"), []byte("top"), 0644)
	os.WriteFile(filepath.Join(dir, "src", "sub", "nested.txt"), []byte("nested"), 0644)

	runInDir := func(args ...string) ([]byte, error) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "C4D_ADDR="+mockURL)
		return cmd.CombinedOutput()
	}

	// mk + cp -r
	runInDir("mk", "test.c4m:")
	out, err := runInDir("cp", "-r", "src", "test.c4m:")
	if err != nil {
		t.Fatalf("cp -r: %v\n%s", err, out)
	}

	// Verify capsule has entries
	loaded, err := loadManifest(filepath.Join(dir, "test.c4m"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Should have: sub/, nested.txt (depth 1), top.txt (depth 0)
	if len(loaded.Entries) < 3 {
		t.Errorf("expected at least 3 entries, got %d", len(loaded.Entries))
		for _, e := range loaded.Entries {
			t.Logf("  %s (depth=%d)", e.Name, e.Depth)
		}
	}
}

func TestCpSubprocessCaptureSubpath(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()
	mockURL := startMockC4d(t)

	os.WriteFile(filepath.Join(dir, "frame.exr"), []byte("frame data"), 0644)

	runInDir := func(args ...string) ([]byte, error) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "C4D_ADDR="+mockURL)
		return cmd.CombinedOutput()
	}

	// mk + mkdir + cp into subpath
	runInDir("mk", "project.c4m:")
	runInDir("mkdir", "project.c4m:renders/")

	out, err := runInDir("cp", "frame.exr", "project.c4m:renders/")
	if err != nil {
		t.Fatalf("cp into subpath: %v\n%s", err, out)
	}

	// Verify file is under renders/
	loaded, _ := loadManifest(filepath.Join(dir, "project.c4m"))
	found := false
	for _, e := range loaded.Entries {
		if e.Name == "frame.exr" && e.Depth > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("frame.exr not found at depth > 0 in manifest")
		for _, e := range loaded.Entries {
			t.Logf("  depth=%d %s", e.Depth, e.Name)
		}
	}
}

func TestCpSubprocessMaterializeSubpath(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	// Start mock c4d that returns content
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Write([]byte("materialized content"))
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	// Create capsule with nested structure
	capsulePath := filepath.Join(dir, "project.c4m")
	m := c4m.NewManifest()
	fileID := identifyString("materialized content")
	m.AddEntry(&c4m.Entry{Name: "renders/", Depth: 0, Mode: os.ModeDir | 0755, Size: -1})
	m.AddEntry(&c4m.Entry{Name: "frame.exr", Depth: 1, Mode: 0644, Size: 21, C4ID: fileID})
	m.AddEntry(&c4m.Entry{Name: "assets/", Depth: 0, Mode: os.ModeDir | 0755, Size: -1})
	m.AddEntry(&c4m.Entry{Name: "texture.png", Depth: 1, Mode: 0644, Size: 15, C4ID: identifyString("texture data")})
	m.SortEntries()
	scan.PropagateMetadata(m.Entries)
	writeManifest(capsulePath, m)

	// Materialize only renders/ subpath
	cmd := exec.Command(bin, "cp", "project.c4m:renders/", "out")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "C4D_ADDR="+ts.URL)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cp subpath materialize: %v\n%s", err, out)
	}

	// frame.exr should exist in output (not renders/frame.exr — prefix is stripped)
	data, err := os.ReadFile(filepath.Join(dir, "out", "frame.exr"))
	if err != nil {
		t.Fatalf("read frame.exr: %v", err)
	}
	if string(data) != "materialized content" {
		t.Errorf("frame.exr = %q, want %q", data, "materialized content")
	}

	// texture.png should NOT exist (it's in assets/, not renders/)
	if _, err := os.Stat(filepath.Join(dir, "out", "texture.png")); err == nil {
		t.Error("texture.png should not be in output (wrong subpath)")
	}
}

func TestCpSubprocessThreeArgs(t *testing.T) {
	bin := buildTestBinary(t)

	// Three args should fail with usage
	cmd := exec.Command(bin, "cp", "a", "b", "c")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected non-zero exit for three args")
	}
	if !strings.Contains(string(out), "Usage") {
		t.Errorf("expected usage message, got %q", out)
	}
}

func TestCpC4dFullRoundTrip(t *testing.T) {
	// Store content indexed by C4 ID
	store := make(map[string][]byte)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			body := new(bytes.Buffer)
			body.ReadFrom(r.Body)
			id := identifyString(body.String())
			store[id.String()] = body.Bytes()
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			key := strings.TrimPrefix(r.URL.Path, "/")
			if data, ok := store[key]; ok {
				w.Write(data)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))
	defer ts.Close()

	bin := buildTestBinary(t)
	dir := t.TempDir()

	// Create source
	os.MkdirAll(filepath.Join(dir, "src"), 0755)
	content := "hello from c4d roundtrip"
	os.WriteFile(filepath.Join(dir, "src", "data.txt"), []byte(content), 0644)

	env := append(os.Environ(), "C4D_ADDR="+ts.URL)

	runInDir := func(args ...string) ([]byte, error) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		cmd.Env = env
		return cmd.CombinedOutput()
	}

	// mk
	if out, err := runInDir("mk", "roundtrip.c4m:"); err != nil {
		t.Fatalf("mk: %v\n%s", err, out)
	}

	// capture
	if out, err := runInDir("cp", "src", "roundtrip.c4m:"); err != nil {
		t.Fatalf("capture: %v\n%s", err, out)
	}

	// Verify c4d received the content
	if len(store) == 0 {
		t.Fatal("c4d store is empty after capture")
	}

	// materialize to new location
	if out, err := runInDir("cp", "roundtrip.c4m:", "restored"); err != nil {
		t.Fatalf("materialize: %v\n%s", err, out)
	}

	// Verify content matches
	data, err := os.ReadFile(filepath.Join(dir, "restored", "data.txt"))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(data) != content {
		t.Errorf("restored content = %q, want %q", data, content)
	}
}

// --- helpers ---

func identifyString(s string) c4.ID {
	return c4.Identify(strings.NewReader(s))
}
