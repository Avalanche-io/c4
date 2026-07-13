package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// storedID extracts the manifest ID from a "stored: <id>" stderr line.
func storedID(t *testing.T, stderr string) string {
	t.Helper()
	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(line, "stored: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "stored: "))
		}
	}
	t.Fatalf("no 'stored:' line in stderr: %s", stderr)
	return ""
}

// TestIDStoreSelfCapture verifies that a snapshot captures itself:
// after losing both the tree and the c4m file, the manifest is
// recoverable from the store by the ID reported on stderr, and the
// tree materializes from the store alone via restore.
func TestIDStoreSelfCapture(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	env := map[string]string{"C4_STORE": storeDir}

	tree := filepath.Join(dir, "tree")
	writeTree(t, tree, map[string]string{
		"a.txt":         "alpha",
		"sub/b.txt":     "bravo",
		"sub/deep/c.md": "charlie",
	})

	// The full-form description (a chain stream). -s streams the same
	// listing and closes with the claim line, echoed on stderr too.
	manifest, _, code := runC4WithEnv(t, bin, env, "id", "-m", "f", tree)
	if code != 0 {
		t.Fatal("id -m f failed")
	}
	stdout, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", tree)
	if code != 0 {
		t.Fatalf("id -s exit %d: %s", code, stderr)
	}
	outLines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if outLines[len(outLines)-1] != storedID(t, stderr) {
		t.Fatalf("-s final line %q != stored: line %q", outLines[len(outLines)-1], storedID(t, stderr))
	}
	id := storedID(t, stderr)

	// The root record is stored under the root directory's ID.
	m, err := c4m.Unmarshal([]byte(manifest))
	if err != nil {
		t.Fatal(err)
	}
	rootID := m.ComputeC4ID()
	s, err := store.NewTreeStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Has(rootID) {
		t.Fatalf("root record %s not in store", rootID)
	}

	// Lose the tree. The c4m was never written to disk at all.
	if err := os.RemoveAll(tree); err != nil {
		t.Fatal(err)
	}

	// THE snapshot ID is the root ID: stored:, -q, and ComputeC4ID agree.
	if id != rootID.String() {
		t.Fatalf("stored: ID %s != root ID %s", id, rootID)
	}

	// cat <id> returns the root record (one-level); cat -r expands the
	// full tree from the store alone, reproducing the original
	// description. The id capture is a chain stream — resolve it to
	// flat text (c4 patch is the text algebra) before comparing.
	chainPath := filepath.Join(dir, "captured.c4m")
	if err := os.WriteFile(chainPath, []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	resolved, _, code := runC4WithEnv(t, bin, env, "patch", chainPath)
	if code != 0 {
		t.Fatal("patch resolve failed")
	}
	recovered, stderr, code := runC4WithEnv(t, bin, env, "cat", "-r", id)
	if code != 0 {
		t.Fatalf("cat -r %s exit %d: %s", id, code, stderr)
	}
	if recovered != resolved {
		t.Fatalf("recovered manifest differs from original:\n--- original (resolved)\n%s--- recovered\n%s", resolved, recovered)
	}

	// Materialize the tree from the store alone.
	restored := filepath.Join(dir, "restored")
	_, stderr, code = runC4WithEnv(t, bin, env, "restore", "--force", id, restored)
	if code != 0 {
		t.Fatalf("restore exit %d: %s", code, stderr)
	}
	recoveredPath := filepath.Join(dir, "recovered.c4m")
	if err := os.WriteFile(recoveredPath, []byte(recovered), 0644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runC4WithEnv(t, bin, env, "diff", recoveredPath, restored)
	if code != 0 || out != "" {
		t.Fatalf("restored tree differs (exit %d): %s", code, out)
	}
	data, err := os.ReadFile(filepath.Join(restored, "sub", "deep", "c.md"))
	if err != nil || string(data) != "charlie" {
		t.Fatalf("restored content wrong: %q, %v", data, err)
	}
}

// TestPatchRefusesDirectories pins the demotion: patch is text algebra —
// c4m chains in, c4m text out — and a directory argument is refused with
// a pointer at restore, exit 1, nothing touched.
func TestPatchRefusesDirectories(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()

	tree := filepath.Join(dir, "tree")
	writeTree(t, tree, map[string]string{"doomed.txt": "untouched"})

	out, _, code := runC4(t, bin, "id", tree)
	if code != 0 {
		t.Fatal("id failed")
	}
	c4mPath := filepath.Join(dir, "target.c4m")
	if err := os.WriteFile(c4mPath, []byte(out), 0644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"patch", c4mPath, tree},
		{"patch", tree},
		{"patch", tree, c4mPath},
	} {
		stdout, stderr, code := runC4(t, bin, args...)
		if code != 1 {
			t.Fatalf("%v: expected exit 1, got %d", args, code)
		}
		if stdout != "" {
			t.Fatalf("%v: refusal printed to stdout: %q", args, stdout)
		}
		if !strings.Contains(stderr, "patch composes descriptions") ||
			!strings.Contains(stderr, "c4 restore") {
			t.Fatalf("%v: refusal must point at restore: %q", args, stderr)
		}
	}
	if data, _ := os.ReadFile(filepath.Join(tree, "doomed.txt")); string(data) != "untouched" {
		t.Fatal("patch touched the directory")
	}
}
