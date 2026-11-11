package c4m

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// The shared damaged-line vector (testdata/damaged-line-vector; README
// there). One line written by the library, then damaged in one field at a
// time, as a torn write, a bad copy or a hand edit damages it. A conforming
// decoder reads ok.c4m and refuses every other file: a damaged field is an
// error naming the line, never a value (a null, a clear bit, a size with the
// rest of the token moved into the name). C4M_UPDATE_VECTORS=1 rewrites the
// files.
type damagedCase struct {
	name   string
	damage func(line string) string
	why    string
}

func damagedLineBase() string {
	m := NewManifest()
	m.AddEntry(&Entry{
		Mode:      0o644,
		Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Size:      6,
		Name:      "real.txt",
		C4ID:      c4.Identify(strings.NewReader("hello\n")),
	})
	text, err := Marshal(m)
	if err != nil {
		panic(err)
	}
	return strings.TrimSuffix(string(text), "\n")
}

func damagedCases() []damagedCase {
	field := func(i int, f func(string) string) func(string) string {
		return func(line string) string {
			parts := strings.Split(line, " ")
			parts[i] = f(parts[i])
			return strings.Join(parts, " ")
		}
	}
	return []damagedCase{
		{"id-89", field(4, func(s string) string { return s[:89] }), "C4 ID cut by one character (a torn write)"},
		{"id-91", field(4, func(s string) string { return s + "1" }), "C4 ID one character too long"},
		{"id-bad-char", field(4, func(s string) string { return s[:89] + "0" }), "C4 ID with a character outside base58 ('0')"},
		{"size-junk", field(2, func(s string) string { return s + "x" }), "size followed by junk: read as size 6 and name \"x real.txt\" before"},
		{"size-null-junk", field(2, func(string) string { return "-x" }), "null size followed by junk"},
		{"mode-bad-char", field(0, func(s string) string { return s[:9] + "z" }), "mode with a character no encoder writes: read as a clear bit before"},
		{"mode-wrong-letter", field(0, func(s string) string { return s[:2] + "r" + s[3:] }), "an 'r' in the owner-write position"},
		{"mode-sticky-letter-misplaced", field(0, func(s string) string { return s[:3] + "t" + s[4:] }), "'t' in the owner-execute position, where only x, s, S or - belong"},
		{"time-impossible", field(1, func(string) string { return "2026-13-45T03:04:05Z" }), "month 13, day 45"},
	}
}

func TestDamagedLineVector(t *testing.T) {
	dir := filepath.Join("testdata", "damaged-line-vector")
	base := damagedLineBase()
	files := map[string][]byte{"ok.c4m": []byte(base + "\n")}
	for _, c := range damagedCases() {
		d := c.damage(base)
		if d == base {
			t.Fatalf("%s: damage changed nothing", c.name)
		}
		files[c.name+".c4m"] = []byte(d + "\n")
	}
	if os.Getenv("C4M_UPDATE_VECTORS") != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, data := range files {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from what the library and the damage write:\n--- file\n%s--- generated\n%s", name, got, want)
		}
	}

	if _, err := Unmarshal(files["ok.c4m"]); err != nil {
		t.Fatalf("ok.c4m must decode: %v", err)
	}
	for _, c := range damagedCases() {
		m, err := Unmarshal(files[c.name+".c4m"])
		if err == nil {
			e := m.Entries[0]
			t.Errorf("%s (%s) decoded instead of being refused: mode %v size %d name %q id %v",
				c.name, c.why, e.Mode, e.Size, e.Name, e.C4ID)
			continue
		}
		if !strings.Contains(err.Error(), "line 1") {
			t.Errorf("%s: the error must name the line, got %v", c.name, err)
		}
	}
}
