package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4/c4m"
)

// TestRangeListParityCLI runs a tree holding a subdirectory and a tree
// holding a folded range through each CLI verb side by side: each must keep
// its content-by-ID the same way, the subdirectory's entries or listing and
// the range's ID list (ruled 2026-11-13 by the maintainer: "Everything that
// you would expect of the creation and retention of that subdirectory c4m
// file you would also expect of the frame id list."). The list's format is
// unchanged: the spec's bare concatenation of member C4 IDs.
func TestRangeListParityCLI(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	frames := map[string]string{"1": "frame 1", "2": "frame 2", "3": "frame 3"}
	dirFiles := map[string]string{"readme.txt": "readme"}
	rngFiles := map[string]string{"readme.txt": "readme"}
	for n, body := range frames {
		dirFiles["f/"+map[string]string{"1": "a", "2": "b", "3": "c"}[n]+".txt"] = body
		rngFiles["f.00"+n+".exr"] = body
	}
	trees := map[string]string{"subdirectory": filepath.Join(dir, "tdir"), "range": filepath.Join(dir, "trng")}
	writeTree(t, trees["subdirectory"], dirFiles)
	writeTree(t, trees["range"], rngFiles)
	otherTree := filepath.Join(dir, "tother")
	writeTree(t, otherTree, map[string]string{"other.txt": "other"})
	names := map[string]string{"subdirectory": "f/", "range": "f.[001-003].exr"}

	envFor := func(name string) map[string]string {
		home := filepath.Join(dir, "home-"+name)
		st := filepath.Join(dir, "store-"+name)
		for _, d := range []string{home, st} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		return map[string]string{"HOME": home, "USERPROFILE": home, "C4_STORE": st}
	}
	run := func(env map[string]string, args ...string) string {
		t.Helper()
		out, stderr, code := runC4WithEnv(t, bin, env, args...)
		if code != 0 {
			t.Fatalf("c4 %v: exit %d: %s", args, code, stderr)
		}
		return out
	}
	write := func(name, text string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	decode := func(what, text string) *c4m.Manifest {
		t.Helper()
		m, err := c4m.Unmarshal([]byte(text))
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		return m
	}
	// kept reports whether out holds name with in's C4 ID and its content:
	// the subdirectory's entries listed under it, or the range's ID list.
	kept := func(in, out *c4m.Manifest, name string) bool {
		inP, outP := c4m.EntryPaths(in.Entries), c4m.EntryPaths(out.Entries)
		want, got := inP[name], outP[name]
		if want == nil || got == nil || want.C4ID != got.C4ID {
			return false
		}
		if want.IsDir() {
			for p, e := range inP {
				if strings.HasPrefix(p, name) && p != name && (outP[p] == nil || outP[p].C4ID != e.C4ID) {
					return false
				}
			}
			return true
		}
		return in.RangeData[want.C4ID] != "" && out.RangeData[want.C4ID] == in.RangeData[want.C4ID]
	}
	// storedByID reports whether the store holds what name links by ID: the
	// subdirectory's listing, or the range's ID list.
	storedByID := func(env map[string]string, in *c4m.Manifest, name string) bool {
		e := c4m.EntryPaths(in.Entries)[name]
		if e == nil {
			return false
		}
		_, _, code := runC4WithEnv(t, bin, env, "cat", e.C4ID.String())
		return code == 0
	}

	var table strings.Builder
	table.WriteString("| Verb | subdirectory f/ | range f.[001-003].exr |\n")
	results := map[string]map[string]bool{}
	var order []string
	record := func(verb, kind string, ok bool) {
		if results[verb] == nil {
			results[verb] = map[string]bool{}
			order = append(order, verb)
		}
		results[verb][kind] = ok
	}

	env := envFor("plain")
	otherText := run(env, "id", "-S", otherTree)
	other := write("other.c4m", otherText)
	for _, kind := range []string{"subdirectory", "range"} {
		name := names[kind]
		scan := run(env, "id", "-S", trees[kind])
		in := decode("scan", scan)
		record("c4 id -S DIR", kind, kept(in, in, name) && strings.Contains(scan, name))
		tPath := write(kind+".c4m", scan)

		record("c4 id FILE.c4m", kind, kept(in, decode("id", run(env, "id", tPath)), name))
		record("c4 merge FILE other", kind, kept(in, decode("merge", run(env, "merge", tPath, other)), name))
		record("c4 merge other FILE", kind, kept(in, decode("merge", run(env, "merge", other, tPath)), name))
		diff := run(env, "diff", other, tPath)
		record("c4 diff other FILE (the patch)", kind, kept(in, decode("diff", diff), name))
		chain := write(kind+"-chain.c4m", otherText+diff)
		record("c4 patch (other + diff)", kind, kept(in, decode("patch", run(env, "patch", chain)), name))
		record("c4 patch other diff (two files)", kind, kept(in, decode("patch", run(env, "patch", other, write(kind+"-diff.c4m", diff))), name))
		record("c4 patch FILE", kind, kept(in, decode("patch", run(env, "patch", tPath)), name))
		before, after := filepath.Join(dir, kind+"-before.c4m"), filepath.Join(dir, kind+"-after.c4m")
		run(env, "split", chain, "1", before, after)
		afterText, err := os.ReadFile(after)
		if err != nil {
			t.Fatal(err)
		}
		record("c4 split (other + diff) 1: the after part", kind, kept(in, decode("split", string(afterText)), name))

		senv := envFor("file-" + kind)
		run(senv, "id", "-s", tPath)
		record("c4 id -s FILE.c4m: in the store", kind, storedByID(senv, in, name))
		senv = envFor("scan-" + kind)
		snap := run(senv, "id", "-S", "-s", trees[kind])
		record("c4 id -S -s DIR: inline", kind, kept(in, decode("snap", snap), name))
		record("c4 id -S -s DIR: in the store", kind, storedByID(senv, in, name))
	}
	for _, verb := range order {
		r := results[verb]
		word := map[bool]string{true: "kept", false: "lost"}
		fmt.Fprintf(&table, "| %s | %s | %s |\n", verb, word[r["subdirectory"]], word[r["range"]])
		if !r["subdirectory"] || !r["range"] {
			t.Errorf("%s: subdirectory %s, range %s; both must be kept", verb, word[r["subdirectory"]], word[r["range"]])
		}
	}
	t.Log("\n" + table.String())
}
