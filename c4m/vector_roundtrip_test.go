package c4m

import (
	"bufio"
	"bytes"
	"crypto/sha512"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// roundTripRow is one line of a vector README's "## Round trip" table.
type roundTripRow struct {
	file string // a name in the directory, or a glob such as */*.c4m
	want string // itself | `NAME` | ID in `NAME` | refused | unpaired: why
}

// readRoundTrip returns the rows of the "## Round trip" table in a vector
// README. Each cell keeps its backticks; the first column's are removed.
func readRoundTrip(t *testing.T, readme string) []roundTripRow {
	t.Helper()
	f, err := os.Open(readme)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var rows []roundTripRow
	in, header := false, 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "## ") {
			in = line == "## Round trip"
			continue
		}
		if !in || !strings.HasPrefix(line, "|") {
			continue
		}
		if header < 2 { // the header row and its separator
			header++
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) != 2 {
			t.Fatalf("%s: a Round trip row needs two cells: %q", readme, line)
		}
		rows = append(rows, roundTripRow{
			file: strings.Trim(strings.TrimSpace(cells[0]), "`"),
			want: strings.TrimSpace(cells[1]),
		})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return rows
}

// quoted returns the name inside the first pair of backticks in s.
func quoted(s string) string {
	i := strings.IndexByte(s, '`')
	j := strings.LastIndexByte(s, '`')
	if i < 0 || j <= i {
		return ""
	}
	return s[i+1 : j]
}

// TestVectorRoundTrip decodes every .c4m in every shared vector directory
// and encodes it canonically, checking the bytes against what the
// directory's README states. A port that keeps or drops bytes on a round
// trip changes a c4m file's byte ID, which is a C4 ID: that is data loss.
// The README's table is the shared statement every port runs (decided
// 2026-11-13 by the release lead; reversible).
func TestVectorRoundTrip(t *testing.T) {
	readmes, err := filepath.Glob(filepath.Join("testdata", "*", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	sum := func(b []byte) [64]byte { return sha512.Sum512(b) }
	for _, readme := range readmes {
		dir := filepath.Dir(readme)
		var all []string
		filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(p, ".c4m") {
				all = append(all, p)
			}
			return nil
		})
		rows := readRoundTrip(t, readme)
		if len(all) == 0 {
			if len(rows) != 0 {
				t.Errorf("%s: a Round trip table, but no .c4m files", dir)
			}
			continue
		}
		if len(rows) == 0 {
			t.Errorf("%s: %d .c4m files and no Round trip table in its README", dir, len(all))
			continue
		}

		covered := map[string]int{}
		for _, row := range rows {
			matches, err := filepath.Glob(filepath.Join(dir, row.file))
			if err != nil || len(matches) == 0 {
				t.Errorf("%s: the Round trip table names %q, which matches no file", dir, row.file)
				continue
			}
			sort.Strings(matches)
			for _, path := range matches {
				covered[path]++
				checkRoundTrip(t, dir, path, row.want, sum)
			}
		}
		for _, p := range all {
			if covered[p] != 1 {
				t.Errorf("%s: named %d times in its README's Round trip table, want once", p, covered[p])
			}
		}
	}
}

func checkRoundTrip(t *testing.T, dir, path, want string, sum func([]byte) [64]byte) {
	t.Helper()
	in, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, decodeErr := Unmarshal(in)
	if want == "refused" {
		if decodeErr == nil {
			t.Errorf("%s: decoded, but its README says it is refused", path)
		}
		return
	}
	if decodeErr != nil {
		t.Errorf("%s: %v", path, decodeErr)
		return
	}
	var out bytes.Buffer
	if err := NewEncoder(&out).Encode(m); err != nil {
		t.Errorf("%s: encode: %v", path, err)
		return
	}
	switch {
	case want == "itself":
		if sum(out.Bytes()) != sum(in) {
			t.Errorf("%s does not re-encode to itself:\n--- file\n%s--- re-encoded\n%s", path, in, out.Bytes())
		}
	case strings.HasPrefix(want, "unpaired:"):
		if strings.TrimSpace(strings.TrimPrefix(want, "unpaired:")) == "" {
			t.Errorf("%s: unpaired without a reason", path)
		}
	case strings.HasPrefix(want, "ID in "):
		b, err := os.ReadFile(filepath.Join(dir, quoted(want)))
		if err != nil {
			t.Errorf("%s: %v", path, err)
			return
		}
		if id := m.ComputeC4ID().String(); id != strings.TrimSpace(string(b)) {
			t.Errorf("%s resolves to %s, want %s (%s)", path, id, strings.TrimSpace(string(b)), quoted(want))
		}
	case quoted(want) != "" && quoted(want) == strings.Trim(want, "`"):
		partner, err := os.ReadFile(filepath.Join(dir, quoted(want)))
		if err != nil {
			t.Errorf("%s: %v", path, err)
			return
		}
		if sum(out.Bytes()) != sum(partner) {
			t.Errorf("%s does not re-encode to %s:\n--- %s\n%s--- re-encoded\n%s", path, quoted(want), quoted(want), partner, out.Bytes())
		}
	default:
		t.Errorf("%s: unknown Round trip entry %q", path, want)
	}
}
