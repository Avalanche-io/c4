package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// TestLogLongChainLinear is the regression test for the super-quadratic
// `c4 log`: the shipped implementation resolved the whole chain from
// scratch for every section (500 sections took ~60s; 1000 never
// finished). The incremental implementation applies one patch per
// section over the previous layer.
//
// It measures work, not seconds: log builds every layer through
// logResolve, and the test counts the sections each call applies. Each
// layer applies at most two (the previous layer, then its patch), so a
// chain of N sections applies at most 2N; resolving from scratch applies
// i sections at layer i, N(N+1)/2 in all, which exceeds 2N for every N
// above 3 (20100 against 400 here). The count does not depend on the
// machine's load or clock, so the bound is exact rather than generous.
// Every layer must also apply at least one section through logResolve, so
// a log that stopped resolving through it fails rather than passing on a
// count of zero. Through 2026-11-16 this asserted c4 log finished within
// 60 seconds, which failed at load 30 after 251s.
func TestLogLongChainLinear(t *testing.T) {
	const sections = 200

	// Build a chain: a base with one entry, then N patch sections each
	// adding one file. Format per c4m chain tests: base entries, base
	// ID line, patch entries, patch ID line, ..., ending on the last
	// patch's entries (a chain never ends with its own ID).
	var chain bytes.Buffer
	base := c4m.NewManifest()
	base.AddEntry(&c4m.Entry{
		Name: "file0000.txt",
		Size: 5,
		C4ID: c4.Identify(strings.NewReader("content-0")),
	})
	var baseBuf bytes.Buffer
	c4m.NewEncoder(&baseBuf).Encode(base)
	chain.Write(baseBuf.Bytes())
	chain.WriteString(base.ComputeC4ID().String() + "\n")

	resolved := base
	for i := 1; i < sections; i++ {
		if i > 1 {
			// Each patch is preceded by the accumulated state's bare ID
			// line — the chain-format section boundary.
			chain.WriteString(resolved.ComputeC4ID().String() + "\n")
		}
		patch := c4m.NewManifest()
		patch.AddEntry(&c4m.Entry{
			Name: fmt.Sprintf("file%04d.txt", i),
			Size: 5,
			C4ID: c4.Identify(strings.NewReader(fmt.Sprintf("content-%d", i))),
		})
		c4m.NewEncoder(&chain).Encode(patch)
		resolved = c4m.ApplyPatch(resolved, patch)
	}

	dir := t.TempDir()
	chainPath := filepath.Join(dir, "history.c4m")
	if err := os.WriteFile(chainPath, chain.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("C4_STORE", filepath.Join(dir, "store"))

	applied := 0
	orig := logResolve
	logResolve = func(secs []*c4m.PatchSection, limit int) *c4m.Manifest {
		n := len(secs)
		if limit > 0 && limit < n {
			n = limit
		}
		applied += n
		return orig(secs, limit)
	}
	defer func() { logResolve = orig }()

	stdout := captureStdout(t, func() { runLog([]string{chainPath}) })

	lines := strings.Count(strings.TrimSpace(stdout), "\n") + 1
	if lines != sections {
		t.Fatalf("expected %d log lines, got %d", sections, lines)
	}
	if applied < sections {
		t.Fatalf("c4 log applied %d sections through logResolve for %d layers; every layer resolves through it", applied, sections)
	}
	if applied > 2*sections {
		t.Fatalf("c4 log applied %d sections for %d layers, more than 2 per layer — complexity regression", applied, sections)
	}
	t.Logf("c4 log: %d layers, %d sections applied", sections, applied)
}

// captureStdout runs f with os.Stdout sent to a file and returns what it
// wrote.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	saved := os.Stdout
	os.Stdout = out
	defer func() { os.Stdout = saved }()
	f()
	os.Stdout = saved
	b, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
