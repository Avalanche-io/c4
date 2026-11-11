package reconcile

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Avalanche-io/c4/c4m"
)

// A folder may take two different names for one: a case-insensitive
// volume folds case (README.md, readme.md), APFS folds Unicode forms (the
// NFC and NFD spellings of é), and some volumes fold ß and ss or σ and ς.
// Writing both paired one file's name with the other's bytes, or merged
// two directories into one, silently.
//
// The collision is measured on the destination before anything is
// written, never predicted from a rule: in a throwaway folder at each
// destination directory (or its nearest existing ancestor), that
// directory's names are made as empty files in byte order, and a name that
// resolves to a file already made for an earlier name (the same file, by
// os.SameFile) is folded onto it. Nothing is removed while measuring, so a
// file system that reuses inodes cannot fake a match. In each set the last
// name in byte order is written whole (name and bytes); the others are
// refused and reported, with their subtrees, and are never written.

// foldCollisions finds the target paths the destination folds onto a
// sibling's name. It returns the refused entries (a directory once, with
// its entry count) and every path to skip. Paths in already (refused by
// name) are neither probed nor counted.
//
// A dry run writes nothing, so it measures nothing: names that differ only
// in case are returned in maybe as a hint, unmeasured, and nothing is
// refused. A real run measures every directory; a directory it could not
// measure is returned in unmeasured, with why, and nothing in it is
// refused on a guess.
func foldCollisions(dirPath string, targetPaths map[string]*c4m.Entry, already map[string]bool, measure bool) (refusals []Refusal, skip map[string]bool, maybe, unmeasured []Collision, winners []string) {
	skip = make(map[string]bool)
	siblings := make(map[string][]string) // parent path -> its children's paths
	for p := range targetPaths {
		if already[p] {
			continue
		}
		siblings[parentOf(p)] = append(siblings[parentOf(p)], p)
	}

	winnerOf := make(map[string]string) // loser path -> winner path
	var losers []string
	for parent, members := range siblings {
		if len(members) < 2 {
			continue
		}
		sort.Strings(members)
		if !measure {
			maybe = append(maybe, caseHints(members)...)
			continue
		}
		sets, err := measureFolds(filepath.Join(dirPath, filepath.FromSlash(parent)), members)
		if err != nil {
			unmeasured = append(unmeasured, Collision{Path: parent, Err: err.Error()})
			continue
		}
		for _, set := range sets {
			winner := set[len(set)-1]
			winners = append(winners, winner)
			for _, l := range set[:len(set)-1] {
				winnerOf[l] = winner
				losers = append(losers, l)
			}
		}
	}

	sort.Strings(losers)
	for _, l := range losers {
		if skip[l] {
			continue // already refused under a refused ancestor
		}
		r := Refusal{Path: l, Reason: "this folder takes it for " + lastSegment(winnerOf[l]) + " (measured)"}
		for p := range targetPaths {
			if already[p] {
				continue
			}
			if p == l || (strings.HasSuffix(l, "/") && strings.HasPrefix(p, l)) {
				skip[p] = true
				r.Entries++
			}
		}
		refusals = append(refusals, r)
	}
	sortCollisions(maybe)
	sortCollisions(unmeasured)
	sort.Strings(winners)
	return refusals, skip, maybe, unmeasured, winners
}

// caseHints returns, for a dry run, the sibling names that differ only in
// case: a hint, not a measurement. Folds the hint cannot predict (Unicode
// forms) are caught by a real run.
func caseHints(members []string) []Collision {
	byFold := make(map[string][]string)
	for _, m := range members {
		k := strings.ToLower(lastSegment(m))
		byFold[k] = append(byFold[k], m)
	}
	var out []Collision
	for _, set := range byFold {
		if len(set) < 2 {
			continue
		}
		sort.Strings(set)
		for _, m := range set[:len(set)-1] {
			out = append(out, Collision{Path: m, With: set[len(set)-1]})
		}
	}
	return out
}

// measureFolds makes members' names (siblings, in byte order) as empty
// files in a throwaway folder at destDir, or at its nearest existing
// ancestor, and returns the sets of two or more names the folder takes for
// one, each in byte order. A name whose lookup already resolves to a file
// made for an earlier name joins that name's set; nothing is removed
// until the measurement is done.
func measureFolds(destDir string, members []string) ([][]string, error) {
	at := destDir
	for {
		info, err := os.Lstat(at)
		if err == nil && info.IsDir() {
			break
		}
		if err == nil {
			// A link or file stands where a directory goes: measure beside
			// it, never through it.
			at = filepath.Dir(at)
			continue
		}
		next := filepath.Dir(at)
		if next == at {
			return nil, os.ErrNotExist
		}
		at = next
	}
	probe, err := os.MkdirTemp(at, ".c4-fold-probe-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(probe)

	type made struct {
		path string
		info os.FileInfo
	}
	var files []made
	setOf := make(map[string]int) // made path -> index into sets
	var sets [][]string
	for _, m := range members {
		name := lastSegment(m)
		p := filepath.Join(probe, name)
		if info, err := os.Lstat(p); err == nil {
			// The folder already lists a file this probe made for an
			// earlier name: find which, by identity.
			for _, f := range files {
				if os.SameFile(f.info, info) {
					i, ok := setOf[f.path]
					if !ok {
						i = len(sets)
						sets = append(sets, []string{f.path})
						setOf[f.path] = i
					}
					sets[i] = append(sets[i], m)
					setOf[m] = i
					break
				}
			}
			continue
		}
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, err
		}
		f.Close()
		info, err := os.Lstat(p)
		if err != nil {
			return nil, err
		}
		files = append(files, made{m, info})
	}
	return sets, nil
}

func parentOf(p string) string {
	b := strings.TrimSuffix(p, "/")
	if i := strings.LastIndex(b, "/"); i >= 0 {
		return b[:i+1]
	}
	return ""
}

func lastSegment(p string) string {
	b := strings.TrimSuffix(p, "/")
	if i := strings.LastIndex(b, "/"); i >= 0 {
		return b[i+1:]
	}
	return b
}

func sortCollisions(c []Collision) {
	sort.Slice(c, func(i, j int) bool { return c[i].Path < c[j].Path })
}

// respell makes the folder list path's entry under exactly path's name.
// When the folder lists the same file under another spelling, it is
// renamed in two steps through a temporary name, since a rename between
// two spellings of one name is not reliably a rename everywhere.
func respell(path string) error {
	dir, want := filepath.Split(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	target, err := os.Lstat(path)
	if err != nil {
		return nil // not written; nothing to respell
	}
	for _, e := range entries {
		if e.Name() == want {
			return nil // already listed exactly
		}
	}
	for _, e := range entries {
		info, err := os.Lstat(filepath.Join(dir, e.Name()))
		if err != nil || !os.SameFile(info, target) {
			continue
		}
		tmp := filepath.Join(dir, ".c4-respell-"+want)
		if err := os.Rename(filepath.Join(dir, e.Name()), tmp); err != nil {
			return err
		}
		return os.Rename(tmp, path)
	}
	return nil
}
