package main

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4/c4m"
)

// TestStreamShapeAndFinalLine pins the v9 output contract: a directory
// listing is a valid chain whose final line is the root ID — equal to
// -q on the same tree — and directory base lines carry null aggregates
// refined by one patch.
func TestStreamShapeAndFinalLine(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	tree := filepath.Join(dir, "proj")
	writeTree(t, tree, map[string]string{
		"a.txt":         "alpha",
		"sub/b.txt":     "bravo",
		"sub/deep/c.md": "charlie",
	})

	out, _, code := runC4(t, bin, "id", tree)
	if code != 0 {
		t.Fatalf("id exit %d", code)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	last := lines[0]
	if len(lines) > 0 {
		last = lines[len(lines)-1]
	}

	qOut, _, _ := runC4(t, bin, "id", "-q", tree)
	if last != strings.TrimSpace(qOut) {
		t.Fatalf("final stream line %q != -q ID %q", last, strings.TrimSpace(qOut))
	}

	// The stream is a valid chain resolving to the same root ID
	// (checkpoint and validator verified by the decoder).
	m, err := c4m.Unmarshal([]byte(out))
	if err != nil {
		t.Fatalf("stream must decode: %v", err)
	}
	if m.ComputeC4ID().String() != last {
		t.Fatal("stream must resolve to its own validator")
	}

	// The base sub/ line carries a null ID (refined later).
	sawNullDir := false
	for _, l := range lines {
		f := strings.Fields(l)
		if len(f) >= 2 && f[len(f)-2] == "sub/" && f[len(f)-1] == "-" {
			sawNullDir = true
			break
		}
	}
	if !sawNullDir {
		t.Fatalf("base directory lines should stream with null IDs:\n%s", out)
	}
}

// TestStreamSnapshotClaimIsFinalLine pins -s streaming: the stream's
// final line equals -s -q's single line (THE claim), and the claim is
// journaled before it prints.
func TestStreamSnapshotClaimIsFinalLine(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store")}
	tree := filepath.Join(dir, "proj")
	writeTree(t, tree, map[string]string{"a.txt": "alpha", "sub/b.txt": "bravo"})

	out, _, code := runC4WithEnv(t, bin, env, "id", "-s", tree)
	if code != 0 {
		t.Fatalf("id -s exit %d", code)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("-s should stream the listing, got %d lines", len(lines))
	}
	claim := lines[len(lines)-1]

	qOut, _, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", tree)
	if code != 0 {
		t.Fatal("id -s -q failed")
	}
	if claim != strings.TrimSpace(qOut) {
		t.Fatalf("stream claim %q != -s -q %q", claim, qOut)
	}

	claims, err := c4m.OpenJournal(filepath.Join(dir, "store")).Claims()
	if err != nil || len(claims) == 0 {
		t.Fatalf("no journal claims: %v", err)
	}
	if claims[0].ID.String() != claim {
		t.Fatal("printed claim must be journaled")
	}
}

// TestMultiPathRefusedWithoutQuiet pins pin 19: without -q, id takes
// one path per invocation.
func TestMultiPathRefusedWithoutQuiet(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	writeTree(t, a, map[string]string{"x.txt": "x"})
	writeTree(t, b, map[string]string{"y.txt": "y"})

	stdout, stderr, code := runC4(t, bin, "id", a, b)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if stdout != "" {
		t.Fatalf("refusal printed to stdout: %q", stdout)
	}
	if !strings.Contains(stderr, "-q") {
		t.Fatalf("refusal should teach -q: %q", stderr)
	}

	// With -q: one line per path, in argument order.
	stdout, _, code = runC4(t, bin, "id", "-q", a, b)
	if code != 0 {
		t.Fatal("id -q multi-path failed")
	}
	if lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n"); len(lines) != 2 {
		t.Fatalf("expected 2 ID lines, got %q", stdout)
	}
}

// TestSigpipeNeverCancelsSnapshot pins pin 20: a dying stdout reader
// (EPIPE mid-stream) never cancels the snapshot — ingest and journal
// complete, the process exits 1, and c4 log holds the claim.
func TestSigpipeNeverCancelsSnapshot(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	tree := filepath.Join(dir, "proj")

	// Enough entries that the stream cannot fit any pipe buffer.
	files := map[string]string{}
	for i := 0; i < 2000; i++ {
		files[filepath.Join("sub", "f"+strings.Repeat("x", 40)+string(rune('a'+i%26))+string(rune('a'+(i/26)%26))+string(rune('a'+(i/676)%26))+".txt")] = strings.Repeat("d", 64) + string(rune(i))
	}
	writeTree(t, tree, files)

	cmd := exec.Command(bin, "id", "-s", tree)
	cmd.Env = append(os.Environ(), "C4_STORE="+storeDir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Read one line, then slam the pipe shut mid-stream.
	r := bufio.NewReader(stdout)
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	stdout.Close()

	err = cmd.Wait()
	if err == nil {
		t.Fatal("expected exit 1 after stdout death")
	}
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got %v", err)
	}

	// The snapshot completed and journaled despite the dead reader.
	claims, err := c4m.OpenJournal(storeDir).Claims()
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 1 {
		t.Fatalf("expected 1 journaled claim after stdout death, got %d", len(claims))
	}
}
