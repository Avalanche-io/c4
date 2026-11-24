package main

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Avalanche-io/c4/c4m"
)

// A dirChange is a stated, non-null directory value the normaliser
// replaced with the one its listed children determine.
type dirChange struct {
	path, field, old, new string
}

// normaliseManifest is the tree walk `c4 id FILE.c4m` does: every
// directory whose children are listed is re-derived by rederiveDirectory,
// deepest first, so a null size, timestamp or C4 ID its children
// determine is filled and a stale size or ID is corrected. A stated
// directory timestamp is kept (SPECIFICATION.md "Directory Timestamps").
// A directory listed without children keeps its stated values. A stated
// size whose children cannot support one (a child with a null size) is
// re-derived to null, as the maintainer ruled on 2026-11-03, and reported
// like any other replaced value. A directory with a null-ID child keeps
// its stated C4 ID, as `c4 id FILE.c4m` always has: that case is not
// ruled. It returns each stated value it replaced, in entry order; filled
// nulls are not changes.
func normaliseManifest(m *c4m.Manifest) []dirChange {
	spans := directorySpans(m, func(_ int, sp dirSpan) bool { return sp.end > sp.start+1 })
	if len(spans) == 0 {
		return nil
	}
	paths := make(map[*c4m.Entry]string, len(m.Entries))
	for p, e := range c4m.EntryPaths(m.Entries) {
		paths[e] = p
	}
	type found struct {
		at int
		c  dirChange
	}
	var all []found
	for _, sp := range spans {
		dir := m.Entries[sp.start]
		stated := *dir
		rederiveDirectory(m, sp)
		if hasNullIDChild(m, dir) {
			dir.C4ID = stated.C4ID
		}
		p := paths[dir]
		if stated.Size >= 0 && dir.Size != stated.Size {
			all = append(all, found{sp.start, dirChange{p, "size", sizeText(stated.Size), sizeText(dir.Size)}})
		}
		if !stated.TimestampIsNull() && !dir.Timestamp.Equal(stated.Timestamp) {
			all = append(all, found{sp.start, dirChange{p, "timestamp", stated.Timestamp.UTC().Format(time.RFC3339), dir.Timestamp.UTC().Format(time.RFC3339)}})
		}
		if !stated.C4ID.IsNil() && dir.C4ID != stated.C4ID {
			all = append(all, found{sp.start, dirChange{p, "id", stated.C4ID.String(), idText(dir)}})
		}
	}
	sort.SliceStable(all, func(a, b int) bool { return all[a].at < all[b].at })
	changes := make([]dirChange, len(all))
	for i, f := range all {
		changes[i] = f.c
	}
	return changes
}

func hasNullIDChild(m *c4m.Manifest, dir *c4m.Entry) bool {
	for _, c := range m.Children(dir) {
		if c.C4ID.IsNil() {
			return true
		}
	}
	return false
}

func sizeText(n int64) string {
	if n < 0 {
		return "-"
	}
	return fmt.Sprint(n)
}

func idText(e *c4m.Entry) string {
	if e.C4ID.IsNil() {
		return "-"
	}
	return e.C4ID.String()
}

// reportNormalized narrates, on stderr, every stated non-null directory
// value the normaliser replaced: one line per directory. Stdout stays
// data. A stale value is never kept, and never replaced silently.
func reportNormalized(source string, changes []dirChange) {
	byPath := make(map[string][]string)
	var order []string
	for _, c := range changes {
		if _, seen := byPath[c.path]; !seen {
			order = append(order, c.path)
		}
		byPath[c.path] = append(byPath[c.path], fmt.Sprintf("%s %s -> %s", c.field, c.old, c.new))
	}
	for _, p := range order {
		fmt.Fprintf(os.Stderr, "normalized: %s: %s: %s\n", source, p, strings.Join(byPath[p], ", "))
	}
}

// normalizedText is what `c4 id FILE.c4m` prints: the normalised
// description in canonical form.
func normalizedText(m *c4m.Manifest) string {
	var buf bytes.Buffer
	c4m.NewEncoder(&buf).Encode(m)
	return buf.String()
}

// runIDCheck is `c4 id -l` and `c4 id -d`: gofmt-style checking of .c4m
// files. -l prints the names of files whose bytes differ from their
// normalised text; -d prints a diff per such file. Both exit 1 when any
// file differs and 0 when none does; errors exit 1 after naming the file.
// The files are never written.
func runIDCheck(paths []string, list, diff bool) int {
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: c4 id -l|-d <file.c4m>...")
		return 1
	}
	differs := false
	for _, p := range paths {
		if !strings.HasSuffix(p, ".c4m") {
			fmt.Fprintf(os.Stderr, "%s: -l and -d check .c4m files\n", p)
			return 1
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", p, err)
			return 1
		}
		m, err := loadManifest(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", p, err)
			return 1
		}
		normaliseManifest(m)
		want := normalizedText(m)
		if string(raw) == want {
			continue
		}
		differs = true
		if list {
			fmt.Println(p)
		}
		if diff {
			fmt.Printf("--- %s\n+++ %s (normalised)\n", p, p)
			fmt.Print(lineDiff(string(raw), want))
		}
	}
	if differs {
		return 1
	}
	return 0
}

// lineDiff renders a minimal line diff from a to b: unchanged lines
// prefixed by a space, removed by "-", added by "+". It is a longest
// common subsequence over lines, enough for a reviewer or a hook, with
// no dependency.
func lineDiff(a, b string) string {
	al := splitDiffLines(a)
	bl := splitDiffLines(b)
	n, m := len(al), len(bl)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case al[i] == bl[j]:
				lcs[i][j] = lcs[i+1][j+1] + 1
			case lcs[i+1][j] >= lcs[i][j+1]:
				lcs[i][j] = lcs[i+1][j]
			default:
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var out strings.Builder
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case al[i] == bl[j]:
			out.WriteString(" " + al[i] + "\n")
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out.WriteString("-" + al[i] + "\n")
			i++
		default:
			out.WriteString("+" + bl[j] + "\n")
			j++
		}
	}
	for ; i < n; i++ {
		out.WriteString("-" + al[i] + "\n")
	}
	for ; j < m; j++ {
		out.WriteString("+" + bl[j] + "\n")
	}
	return out.String()
}

func splitDiffLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
