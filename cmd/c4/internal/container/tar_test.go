package container

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4/c4m"
)

func TestReadManifest_Tar(t *testing.T) {
	// Create a test tar archive
	archive := createTestTar(t)

	m, err := ReadManifest(archive, "tar")
	if err != nil {
		t.Fatal(err)
	}

	if len(m.Entries) == 0 {
		t.Fatal("expected entries in manifest")
	}

	// Check we got the expected files
	names := entryNames(m)
	for _, want := range []string{"hello.txt", "src/", "main.go"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected entry %q, got %v", want, names)
		}
	}

	// Check that regular files have C4 IDs
	for _, e := range m.Entries {
		if !e.IsDir() && e.C4ID.IsNil() {
			t.Errorf("file %s has nil C4 ID", e.Name)
		}
	}
}

func TestReadManifest_TarGz(t *testing.T) {
	archive := createTestTarGz(t)

	m, err := ReadManifest(archive, "gzip")
	if err != nil {
		t.Fatal(err)
	}

	if len(m.Entries) == 0 {
		t.Fatal("expected entries in manifest")
	}

	// Verify same content as uncompressed
	names := entryNames(m)
	for _, want := range []string{"hello.txt", "src/", "main.go"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected entry %q, got %v", want, names)
		}
	}
}

func TestReadManifest_Nesting(t *testing.T) {
	// Create tar with deeply nested structure
	archive := filepath.Join(t.TempDir(), "nested.tar")
	f, _ := os.Create(archive)
	tw := tar.NewWriter(f)

	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)

	// Add files without explicit directory entries (common in tars)
	addFile(tw, "a/b/c/deep.txt", "deep content", ts)
	addFile(tw, "a/top.txt", "top content", ts)
	addFile(tw, "root.txt", "root content", ts)

	tw.Close()
	f.Close()

	m, err := ReadManifest(archive, "tar")
	if err != nil {
		t.Fatal(err)
	}

	// Should have implicit directories created
	names := entryNames(m)
	for _, want := range []string{"root.txt", "a/", "top.txt", "b/", "c/", "deep.txt"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected entry %q, got %v", want, names)
		}
	}

	// Check depths
	for _, e := range m.Entries {
		switch e.Name {
		case "root.txt":
			if e.Depth != 0 {
				t.Errorf("root.txt depth = %d, want 0", e.Depth)
			}
		case "a/":
			if e.Depth != 0 {
				t.Errorf("a/ depth = %d, want 0", e.Depth)
			}
		case "top.txt":
			if e.Depth != 1 {
				t.Errorf("top.txt depth = %d, want 1", e.Depth)
			}
		case "b/":
			if e.Depth != 1 {
				t.Errorf("b/ depth = %d, want 1", e.Depth)
			}
		case "c/":
			if e.Depth != 2 {
				t.Errorf("c/ depth = %d, want 2", e.Depth)
			}
		case "deep.txt":
			if e.Depth != 3 {
				t.Errorf("deep.txt depth = %d, want 3", e.Depth)
			}
		}
	}
}

func TestCatFile(t *testing.T) {
	archive := createTestTar(t)

	rc, err := CatFile(archive, "tar", "hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()

	data, _ := io.ReadAll(rc)
	if string(data) != "hello world" {
		t.Errorf("got %q, want %q", data, "hello world")
	}
}

func TestCatFile_Nested(t *testing.T) {
	archive := createTestTar(t)

	rc, err := CatFile(archive, "tar", "src/main.go")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()

	data, _ := io.ReadAll(rc)
	if string(data) != "package main" {
		t.Errorf("got %q, want %q", data, "package main")
	}
}

func TestCatFile_NotFound(t *testing.T) {
	archive := createTestTar(t)

	_, err := CatFile(archive, "tar", "nonexistent.txt")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestCatFile_Directory(t *testing.T) {
	archive := createTestTar(t)

	_, err := CatFile(archive, "tar", "src")
	if err == nil {
		t.Fatal("expected error for directory")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("expected 'directory' error, got: %v", err)
	}
}

func TestWriteTar(t *testing.T) {
	// First create a tar from a manifest
	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("alpha"), 0644)
	os.Mkdir(filepath.Join(srcDir, "sub"), 0755)
	os.WriteFile(filepath.Join(srcDir, "sub", "b.txt"), []byte("beta"), 0644)

	// Read source as manifest
	archive1 := createTarFromDir(t, srcDir)
	m, err := ReadManifest(archive1, "tar")
	if err != nil {
		t.Fatal(err)
	}

	// Write a new tar from the manifest, reading content from the first tar
	archive2 := filepath.Join(t.TempDir(), "output.tar")
	err = WriteTar(archive2, "tar", m, func(fullPath string, entry *c4m.Entry) (io.ReadCloser, error) {
		return CatFile(archive1, "tar", fullPath)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Read the new tar and compare
	m2, err := ReadManifest(archive2, "tar")
	if err != nil {
		t.Fatal(err)
	}

	if len(m.Entries) != len(m2.Entries) {
		t.Errorf("entry count mismatch: %d vs %d", len(m.Entries), len(m2.Entries))
	}

	// Verify content survives the round-trip
	rc, err := CatFile(archive2, "tar", "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != "alpha" {
		t.Errorf("round-trip content: got %q, want %q", data, "alpha")
	}
}

func TestWriteTar_Gzip(t *testing.T) {
	archive1 := createTestTar(t)
	m, err := ReadManifest(archive1, "tar")
	if err != nil {
		t.Fatal(err)
	}

	archive2 := filepath.Join(t.TempDir(), "output.tar.gz")
	err = WriteTar(archive2, "gzip", m, func(fullPath string, entry *c4m.Entry) (io.ReadCloser, error) {
		return CatFile(archive1, "tar", fullPath)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Read back as gzip
	m2, err := ReadManifest(archive2, "gzip")
	if err != nil {
		t.Fatal(err)
	}

	if len(m.Entries) != len(m2.Entries) {
		t.Errorf("entry count mismatch: %d vs %d", len(m.Entries), len(m2.Entries))
	}
}

// Helpers

func createTestTar(t *testing.T) string {
	t.Helper()
	archive := filepath.Join(t.TempDir(), "test.tar")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)

	addFile(tw, "hello.txt", "hello world", ts)
	addDir(tw, "src/", ts)
	addFile(tw, "src/main.go", "package main", ts)

	tw.Close()
	f.Close()
	return archive
}

func createTestTarGz(t *testing.T) string {
	t.Helper()
	archive := filepath.Join(t.TempDir(), "test.tar.gz")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)

	addFile(tw, "hello.txt", "hello world", ts)
	addDir(tw, "src/", ts)
	addFile(tw, "src/main.go", "package main", ts)

	tw.Close()
	gw.Close()
	f.Close()
	return archive
}

func createTarFromDir(t *testing.T, dir string) string {
	t.Helper()
	archive := filepath.Join(t.TempDir(), "dir.tar")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)

	filepath.Walk(dir, func(fpath string, info os.FileInfo, err error) error {
		if err != nil || fpath == dir {
			return err
		}
		rel, _ := filepath.Rel(dir, fpath)
		if info.IsDir() {
			addDir(tw, rel+"/", info.ModTime())
		} else {
			data, _ := os.ReadFile(fpath)
			addFile(tw, rel, string(data), info.ModTime())
		}
		return nil
	})

	tw.Close()
	f.Close()
	return archive
}

func addFile(tw *tar.Writer, name, content string, ts time.Time) {
	tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     name,
		Size:     int64(len(content)),
		Mode:     0644,
		ModTime:  ts,
	})
	tw.Write([]byte(content))
}

func addDir(tw *tar.Writer, name string, ts time.Time) {
	tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     name,
		Mode:     0755,
		ModTime:  ts,
	})
}

func entryNames(m *c4m.Manifest) []string {
	var names []string
	for _, e := range m.Entries {
		names = append(names, e.Name)
	}
	return names
}

// TestRoundTrip_CIDs verifies C4 IDs survive a read→write→read cycle.
func TestRoundTrip_CIDs(t *testing.T) {
	archive1 := createTestTar(t)
	m1, _ := ReadManifest(archive1, "tar")

	archive2 := filepath.Join(t.TempDir(), "round.tar")
	WriteTar(archive2, "tar", m1, func(fullPath string, entry *c4m.Entry) (io.ReadCloser, error) {
		return CatFile(archive1, "tar", fullPath)
	})

	m2, _ := ReadManifest(archive2, "tar")

	// Compare C4 IDs for each file
	ids1 := make(map[string]string)
	ids2 := make(map[string]string)
	var collect func(m *c4m.Manifest, ids map[string]string, prefix string)
	collect = func(m *c4m.Manifest, ids map[string]string, prefix string) {
		var stack []string
		for _, e := range m.Entries {
			if e.Depth < len(stack) {
				stack = stack[:e.Depth]
			}
			p := strings.Join(stack, "") + e.Name
			if !e.C4ID.IsNil() {
				ids[p] = e.C4ID.String()
			}
			if e.IsDir() {
				for len(stack) <= e.Depth {
					stack = append(stack, "")
				}
				stack[e.Depth] = e.Name
			}
		}
	}
	collect(m1, ids1, "")
	collect(m2, ids2, "")

	for path, id1 := range ids1 {
		if id2, ok := ids2[path]; !ok {
			t.Errorf("missing in round-trip: %s", path)
		} else if id1 != id2 {
			t.Errorf("C4 ID mismatch for %s:\n  before: %s\n  after:  %s", path, id1, id2)
		}
	}
}

// TestReadManifest_Stdin verifies reading from a bytes buffer works
// (simulating piped input to ReadManifest is not possible since it takes a path,
// but we can verify the tar reader handles streaming correctly).
func TestReadManifest_EmptyTar(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "empty.tar")
	f, _ := os.Create(archive)
	tw := tar.NewWriter(f)
	tw.Close()
	f.Close()

	m, err := ReadManifest(archive, "tar")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 0 {
		t.Errorf("expected empty manifest, got %d entries", len(m.Entries))
	}
}

