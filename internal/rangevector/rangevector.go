// Package rangevector builds the trees of the shared folded-range
// round-trip vector (c4m/testdata/range-roundtrip-vector/trees.txt) on
// disk, so the generator and every Go test that runs the vector make
// exactly the same trees. trees.txt is plain data, not c4m: every c4m in
// the vector is written by the library from a scan of these trees.
package rangevector

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Path is one path of a vector tree.
type Path struct {
	Rel     string // slash-separated, relative to the case's root
	Kind    byte   // 'd' directory, 'f' file, 'l' symbolic link
	Mode    os.FileMode
	MTime   time.Time // the Unix epoch for a null-dated path
	Content string    // file bytes, or the link target
}

// Read parses trees.txt into its cases, keyed by case name.
func Read(file string) (map[string][]Path, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cases := make(map[string][]Path)
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) == 5 && fields[2] == "d" {
			fields = append(fields, "") // a directory has no content field
		}
		if len(fields) != 6 || len(fields[2]) != 1 {
			return nil, fmt.Errorf("%s:%d: want 6 tab-separated fields", file, n)
		}
		mode, err := strconv.ParseUint(fields[3], 8, 32)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: mode: %v", file, n, err)
		}
		mt := time.Unix(0, 0).UTC()
		if fields[4] != "0" {
			if mt, err = time.Parse(time.RFC3339, fields[4]); err != nil {
				return nil, fmt.Errorf("%s:%d: mtime: %v", file, n, err)
			}
		}
		content := fields[5]
		if fields[2][0] == 'f' {
			content = strings.ReplaceAll(content, `\n`, "\n")
		}
		cases[fields[0]] = append(cases[fields[0]], Path{
			Rel: fields[1], Kind: fields[2][0], Mode: os.FileMode(mode), MTime: mt, Content: content,
		})
	}
	return cases, sc.Err()
}

// Names returns the case names in sorted order.
func Names(cases map[string][]Path) []string {
	var names []string
	for k := range cases {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// Build makes one case's tree under root (which must not exist yet). The
// root folder and every directory get their times after their contents
// are written, deepest first.
func Build(root string, paths []Path) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	var dirs []Path
	for _, p := range paths {
		full := filepath.Join(root, filepath.FromSlash(p.Rel))
		switch p.Kind {
		case 'd':
			if err := os.MkdirAll(full, p.Mode); err != nil {
				return err
			}
			dirs = append(dirs, p)
		case 'f':
			if err := os.WriteFile(full, []byte(p.Content), 0o644); err != nil {
				return err
			}
			if err := os.Chmod(full, p.Mode); err != nil {
				return err
			}
			if err := os.Chtimes(full, p.MTime, p.MTime); err != nil {
				return err
			}
		case 'l':
			if err := os.Symlink(p.Content, full); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s: unknown kind %q", p.Rel, p.Kind)
		}
	}
	sort.SliceStable(dirs, func(i, j int) bool {
		return strings.Count(dirs[i].Rel, "/") > strings.Count(dirs[j].Rel, "/")
	})
	for _, d := range dirs {
		full := filepath.Join(root, filepath.FromSlash(d.Rel))
		if err := os.Chtimes(full, d.MTime, d.MTime); err != nil {
			return err
		}
	}
	stamp := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	return os.Chtimes(root, stamp, stamp)
}
