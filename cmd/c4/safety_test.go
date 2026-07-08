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
		if rest, ok := strings.CutPrefix(line, "stored: "); ok {
			return strings.TrimSpace(rest)
		}
	}
	t.Fatalf("no 'stored:' line in stderr: %s", stderr)
	return ""
}

// revertCommand extracts the verbatim revert command from a
// "prior state stored: <id> (revert: c4 patch -r <id> <dir>)" line.
func revertCommand(t *testing.T, stderr string) []string {
	t.Helper()
	for _, line := range strings.Split(stderr, "\n") {
		if !strings.HasPrefix(line, "prior state stored: ") {
			continue
		}
		start := strings.Index(line, "(revert: ")
		end := strings.LastIndex(line, ")")
		if start < 0 || end < start {
			t.Fatalf("malformed prior state line: %s", line)
		}
		fields := strings.Fields(line[start+len("(revert: ") : end])
		if len(fields) < 2 || fields[0] != "c4" {
			t.Fatalf("malformed revert command in: %s", line)
		}
		return fields[1:] // drop the "c4" binary name
	}
	t.Fatalf("no 'prior state stored:' line in stderr: %s", stderr)
	return nil
}

// TestIDStoreSelfCapture verifies that a snapshot captures itself:
// after losing both the tree and the c4m file, the manifest is
// recoverable from the store by the ID reported on stderr, and the
// tree materializes from the store alone.
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

	manifest, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", tree)
	if code != 0 {
		t.Fatalf("id -s exit %d: %s", code, stderr)
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

	// Recover the manifest from the store, byte-identical.
	recovered, stderr, code := runC4WithEnv(t, bin, env, "cat", id)
	if code != 0 {
		t.Fatalf("cat %s exit %d: %s", id, code, stderr)
	}
	if recovered != manifest {
		t.Fatalf("recovered manifest differs from original:\n--- original\n%s--- recovered\n%s", manifest, recovered)
	}

	// Materialize the tree from the store alone.
	recoveredPath := filepath.Join(dir, "recovered.c4m")
	if err := os.WriteFile(recoveredPath, []byte(recovered), 0644); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(dir, "restored")
	// Trailing slash: mark the (not yet existing) dest as a directory.
	_, stderr, code = runC4WithEnv(t, bin, env, "patch", recoveredPath, restored+"/")
	if code != 0 {
		t.Fatalf("patch exit %d: %s", code, stderr)
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

// TestPatchPreStateRevertVerbatim reproduces the accident: patching the
// WRONG directory destroys its prior state — then the revert command
// printed on stderr, run verbatim, restores it byte-identically.
func TestPatchPreStateRevertVerbatim(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	env := map[string]string{"C4_STORE": storeDir}

	precious := filepath.Join(dir, "precious")
	writeTree(t, precious, map[string]string{
		"notes.txt":        "irreplaceable notes",
		"docs/design.md":   "the design",
		"docs/deep/api.md": "the api",
		"shared.txt":       "shared v1", // overwritten, not removed (different size than v2)
	})
	if err := os.MkdirAll(filepath.Join(precious, "emptydir"), 0755); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(dir, "target")
	writeTree(t, target, map[string]string{
		"release.txt": "release payload",
		"shared.txt":  "shared version two",
	})
	targetC4m := filepath.Join(dir, "target.c4m")
	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", target)
	if code != 0 {
		t.Fatalf("id -s exit %d: %s", code, stderr)
	}
	if err := os.WriteFile(targetC4m, []byte(out), 0644); err != nil {
		t.Fatal(err)
	}

	// Record the pre-accident state without touching the store.
	original, _, code := runC4WithEnv(t, bin, env, "id", precious)
	if code != 0 {
		t.Fatal("id precious failed")
	}
	originalC4m := filepath.Join(dir, "original.c4m")
	if err := os.WriteFile(originalC4m, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	// The accident: patch the wrong directory.
	_, stderr, code = runC4WithEnv(t, bin, env, "patch", targetC4m, precious)
	if code != 0 {
		t.Fatalf("patch exit %d: %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(precious, "notes.txt")); !os.IsNotExist(err) {
		t.Fatal("accident did not happen: notes.txt still present")
	}
	if data, _ := os.ReadFile(filepath.Join(precious, "shared.txt")); string(data) != "shared version two" {
		t.Fatalf("shared.txt not overwritten: %q", data)
	}

	// Run the printed revert command verbatim.
	revert := revertCommand(t, stderr)
	_, stderr, code = runC4WithEnv(t, bin, env, revert...)
	if code != 0 {
		t.Fatalf("revert exit %d: %s", code, stderr)
	}

	// Byte-identical restoration.
	out, _, code = runC4WithEnv(t, bin, env, "diff", originalC4m, precious)
	if code != 0 || out != "" {
		t.Fatalf("revert incomplete (exit %d): %s", code, out)
	}
	for path, want := range map[string]string{
		"notes.txt":        "irreplaceable notes",
		"docs/deep/api.md": "the api",
		"shared.txt":       "shared v1",
	} {
		data, err := os.ReadFile(filepath.Join(precious, filepath.FromSlash(path)))
		if err != nil || string(data) != want {
			t.Fatalf("%s not restored: %q, %v", path, data, err)
		}
	}
	if info, err := os.Stat(filepath.Join(precious, "emptydir")); err != nil || !info.IsDir() {
		t.Fatal("emptydir not restored")
	}
}

// TestPatchChangesetRevertNestedTree verifies the changeset-file revert
// path on a nested tree (dir-to-dir form): the changeset's OldID
// resolves to the stored root record, which expands through stored
// directory records into the full prior state.
func TestPatchChangesetRevertNestedTree(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	env := map[string]string{"C4_STORE": storeDir}

	src := filepath.Join(dir, "src")
	writeTree(t, src, map[string]string{"only.txt": "the new state"})
	dest := filepath.Join(dir, "dest")
	writeTree(t, dest, map[string]string{
		"keep.txt":       "kept",
		"sub/lose.txt":   "lost one",
		"sub/deep/l2.md": "lost two",
	})

	original, _, code := runC4WithEnv(t, bin, env, "id", dest)
	if code != 0 {
		t.Fatal("id dest failed")
	}
	originalC4m := filepath.Join(dir, "original.c4m")
	if err := os.WriteFile(originalC4m, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	changeset, stderr, code := runC4WithEnv(t, bin, env, "patch", src, dest)
	if code != 0 {
		t.Fatalf("patch exit %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "stored: ") {
		t.Fatalf("source snapshot not reported: %s", stderr)
	}
	if !strings.Contains(stderr, "prior state stored: ") {
		t.Fatalf("prior state not reported: %s", stderr)
	}
	changesetPath := filepath.Join(dir, "changes.c4m")
	if err := os.WriteFile(changesetPath, []byte(changeset), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code = runC4WithEnv(t, bin, env, "patch", "-r", changesetPath, dest)
	if code != 0 {
		t.Fatalf("patch -r exit %d: %s", code, stderr)
	}
	out, _, code := runC4WithEnv(t, bin, env, "diff", originalC4m, dest)
	if code != 0 || out != "" {
		t.Fatalf("changeset revert incomplete (exit %d): %s", code, out)
	}
	if data, _ := os.ReadFile(filepath.Join(dest, "sub", "deep", "l2.md")); string(data) != "lost two" {
		t.Fatalf("nested content not restored: %q", data)
	}
}

// TestPatchNoStoreAndDryRunSkipCapture verifies the opt-outs: --no-store
// skips the prior-state capture entirely, --dry-run changes nothing.
func TestPatchNoStoreAndDryRunSkipCapture(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	env := map[string]string{"C4_STORE": storeDir}

	dest := filepath.Join(dir, "dest")
	writeTree(t, dest, map[string]string{"doomed.txt": "doomed"})
	target := filepath.Join(dir, "target")
	writeTree(t, target, map[string]string{"new.txt": "new"})
	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", target)
	if code != 0 {
		t.Fatalf("id -s exit %d: %s", code, stderr)
	}
	targetC4m := filepath.Join(dir, "target.c4m")
	if err := os.WriteFile(targetC4m, []byte(out), 0644); err != nil {
		t.Fatal(err)
	}
	before := countObjects(t, storeDir)

	// Dry run: nothing applied, nothing captured.
	_, stderr, code = runC4WithEnv(t, bin, env, "patch", "--dry-run", targetC4m, dest)
	if code != 0 {
		t.Fatalf("dry run exit %d: %s", code, stderr)
	}
	if strings.Contains(stderr, "prior state stored") {
		t.Fatalf("dry run captured prior state: %s", stderr)
	}
	if _, err := os.Stat(filepath.Join(dest, "doomed.txt")); err != nil {
		t.Fatal("dry run modified dest")
	}
	if got := countObjects(t, storeDir); got != before {
		t.Fatalf("dry run wrote %d objects to store", got-before)
	}

	// --no-store: applied, but no capture and no report.
	_, stderr, code = runC4WithEnv(t, bin, env, "patch", "--no-store", targetC4m, dest)
	if code != 0 {
		t.Fatalf("--no-store exit %d: %s", code, stderr)
	}
	if strings.Contains(stderr, "prior state stored") {
		t.Fatalf("--no-store captured prior state: %s", stderr)
	}
	if _, err := os.Stat(filepath.Join(dest, "doomed.txt")); !os.IsNotExist(err) {
		t.Fatal("--no-store did not apply the patch")
	}
	if got := countObjects(t, storeDir); got != before {
		t.Fatalf("--no-store wrote %d objects to store", got-before)
	}
}
