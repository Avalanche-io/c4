package reconcile

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Avalanche-io/c4/c4m"
)

// A folder on a case-insensitive volume (default APFS, NTFS) takes two
// names that differ only in case for one. Writing both paired one file's
// name with the other's bytes, or merged two directories into one, and
// through 1.0.25 `c4 patch` did that with exit 0. Two names the
// destination folds together are measured there, not predicted from the
// platform: the last in byte order is written whole (name and bytes), and
// the others are refused and reported, with their subtrees. Unicode-form
// collisions (NFC and NFD) are not detected here: finding the candidates
// needs normalisation, which the standard library lacks.

// foldCollisions finds the target paths the destination folds onto a
// sibling's name. It returns the refused entries (a directory once, with
// its entry count) and every path to skip.
func foldCollisions(dirPath string, targetPaths map[string]*c4m.Entry) (refusals []Refusal, skip map[string]bool) {
	skip = make(map[string]bool)
	groups := make(map[string][]string) // parent + folded name -> sibling paths
	for p := range targetPaths {
		base := strings.TrimSuffix(p, "/")
		parent, name := "", base
		if i := strings.LastIndex(base, "/"); i >= 0 {
			parent, name = base[:i+1], base[i+1:]
		}
		key := parent + "\x00" + strings.ToLower(name)
		groups[key] = append(groups[key], p)
	}
	var losers []string
	for _, members := range groups {
		if len(members) < 2 {
			continue
		}
		sort.Strings(members)
		winner := members[len(members)-1]
		for _, m := range members[:len(members)-1] {
			if foldsTogether(dirPath, winner, m) {
				losers = append(losers, m)
			}
		}
	}
	sort.Strings(losers)
	for _, l := range losers {
		r := Refusal{Path: l, Reason: "this folder takes it for " + strings.TrimSuffix(lastSegment(winnerOf(l, groups)), "/") + " (measured)"}
		for p := range targetPaths {
			if p == l || (strings.HasSuffix(l, "/") && strings.HasPrefix(p, l)) {
				skip[p] = true
				if p != l {
					r.Entries++
				}
			}
		}
		r.Entries++
		refusals = append(refusals, r)
	}
	return refusals, skip
}

// winnerOf returns the byte-order last member of path's collision group.
func winnerOf(path string, groups map[string][]string) string {
	for _, members := range groups {
		for _, m := range members {
			if m == path {
				return members[len(members)-1]
			}
		}
	}
	return path
}

func lastSegment(p string) string {
	b := strings.TrimSuffix(p, "/")
	if i := strings.LastIndex(b, "/"); i >= 0 {
		return b[i+1:]
	}
	return b
}

// foldsTogether measures whether the destination folder for paths a and b
// (siblings) takes their names for one: in a throwaway directory beside
// it, it creates a's name and looks up b's. A probe that cannot run (an
// unwritable volume, say) measures nothing, and nothing is refused on a
// guess.
func foldsTogether(dirPath, a, b string) bool {
	nameA, nameB := lastSegment(a), lastSegment(b)
	parent := filepath.Dir(filepath.Join(dirPath, filepath.FromSlash(strings.TrimSuffix(a, "/"))))
	for {
		if info, err := os.Stat(parent); err == nil && info.IsDir() {
			break
		}
		next := filepath.Dir(parent)
		if next == parent {
			return false
		}
		parent = next
	}
	probe, err := os.MkdirTemp(parent, ".c4-fold-probe-")
	if err != nil {
		return false
	}
	defer os.RemoveAll(probe)
	f, err := os.Create(filepath.Join(probe, nameA))
	if err != nil {
		return false
	}
	f.Close()
	_, err = os.Lstat(filepath.Join(probe, nameB))
	return err == nil
}
