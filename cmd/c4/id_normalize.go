package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/Avalanche-io/c4/c4m"
)

// reportNormalized narrates, on stderr, every stated non-null directory
// value the normaliser replaced: one line per directory. Stdout stays
// data. A stale value is never kept, and never replaced silently.
func reportNormalized(source string, changes []c4m.Change) {
	if len(changes) == 0 {
		return
	}
	byPath := make(map[string][]string)
	var order []string
	for _, c := range changes {
		if _, seen := byPath[c.Path]; !seen {
			order = append(order, c.Path)
		}
		byPath[c.Path] = append(byPath[c.Path], fmt.Sprintf("%s %s -> %s", c.Field, c.Old, c.New))
	}
	for _, p := range order {
		fmt.Fprintf(os.Stderr, "normalized: %s: %s: %s\n", source, p, strings.Join(byPath[p], ", "))
	}
}

// normalizedText is what `c4 id FILE.c4m` prints: the resolved,
// normalised description in canonical form. It never ends with the
// description's own ID: a file cannot contain its own ID.
func normalizedText(m *c4m.Manifest) string {
	var buf bytes.Buffer
	c4m.NewEncoder(&buf).Encode(m)
	return buf.String()
}

// runIDCheck is `c4 id -l` and `c4 id -d`: gofmt-style checking of .c4m
// files. -l prints the names of files whose bytes differ from their
// normalised text; -d prints a diff per such file. Both exit 1 when any
// file differs and 0 when none does, the pre-commit contract; errors
// exit 1 after naming the file. The files are never written.
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
		// -l and -d read as the normaliser does: a file ending with a
		// bare ID (the shape `c4 diff >>` wrote through 1.0.24) is
		// listed as not normal, since its bytes differ from the
		// normalised text, rather than refused.
		m, _, err := loadManifestMigrating(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", p, err)
			return 1
		}
		c4m.Normalize(m)
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
	al := splitLines(a)
	bl := splitLines(b)
	n, m := len(al), len(bl)
	// lcs[i][j]: length of the LCS of al[i:] and bl[j:]
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

func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
