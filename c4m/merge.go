package c4m

import (
	"os"
	"sort"
	"strings"
)

// Conflict describes a single entry where both local and remote made
// incompatible changes relative to the base.
type Conflict struct {
	Path        string // Full path (e.g., "footage/shot01.mov")
	LocalEntry  *Entry
	RemoteEntry *Entry
}

// Merge performs a three-way merge of c4m manifests.
//
// base is the common ancestor (the state at last successful sync).
// local and remote are the current states of each side.
//
// Returns a merged manifest containing all auto-merged changes. When
// both sides modify the same entry differently, the newer version (by
// timestamp) keeps the original name and the other is preserved as
// "{name}.conflict". Both versions are in the merged manifest so
// neither side loses data. The returned conflicts list identifies
// which paths had genuine conflicts.
//
// If base is nil, local is used as the base (first sync).
func Merge(base, local, remote *Manifest) (*Manifest, []Conflict, error) {
	if base == nil {
		base = NewManifest()
	}

	baseMap := entryPaths(base.Entries)
	localMap := entryPaths(local.Entries)
	remoteMap := entryPaths(remote.Entries)

	// Collect all unique paths.
	allSet := make(map[string]struct{})
	for p := range baseMap {
		allSet[p] = struct{}{}
	}
	for p := range localMap {
		allSet[p] = struct{}{}
	}
	for p := range remoteMap {
		allSet[p] = struct{}{}
	}
	paths := make([]string, 0, len(allSet))
	for p := range allSet {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	// merged holds full-path → entry for the result.
	merged := make(map[string]*Entry, len(paths))
	var conflicts []Conflict

	for _, p := range paths {
		b := baseMap[p]
		l := localMap[p]
		r := remoteMap[p]

		switch {
		// --- Only one side has it ---
		case b == nil && l != nil && r == nil:
			merged[p] = cloneEntry(l)
		case b == nil && l == nil && r != nil:
			merged[p] = cloneEntry(r)

		// --- Both added ---
		case b == nil && l != nil && r != nil:
			if mergeEqual(l, r) {
				merged[p] = cloneEntry(l)
			} else {
				addConflict(merged, &conflicts, p, l, r)
			}

		// --- All three exist ---
		case b != nil && l != nil && r != nil:
			lChanged := !mergeEqual(b, l)
			rChanged := !mergeEqual(b, r)
			switch {
			case !lChanged && !rChanged:
				merged[p] = cloneEntry(b)
			case lChanged && !rChanged:
				merged[p] = cloneEntry(l)
			case !lChanged && rChanged:
				merged[p] = cloneEntry(r)
			default:
				if mergeEqual(l, r) {
					merged[p] = cloneEntry(l) // converged
				} else {
					addConflict(merged, &conflicts, p, l, r)
				}
			}

		// --- Remote deleted ---
		case b != nil && l != nil && r == nil:
			if mergeEqual(b, l) {
				// Unchanged locally, remote deleted → delete
			} else {
				// Local modified after remote deleted → conflict
				addConflict(merged, &conflicts, p, l, nil)
			}

		// --- Local deleted ---
		case b != nil && l == nil && r != nil:
			if mergeEqual(b, r) {
				// Unchanged remotely, local deleted → delete
			} else {
				// Remote modified after local deleted → conflict
				addConflict(merged, &conflicts, p, nil, r)
			}

		// --- Both deleted ---
		case b != nil && l == nil && r == nil:
			// Agreement: both deleted.
		}
	}

	// Ensure parent directories exist for all entries in the merged set.
	ensureDirs(merged)

	result := rebuildManifest(merged)
	return result, conflicts, nil
}

// addConflict adds both versions to the merged map and records the conflict.
// The version with the newer timestamp keeps the original name; the other
// gets a ".conflict" suffix. If one side is nil (delete-vs-modify), only
// the surviving version is included.
func addConflict(merged map[string]*Entry, conflicts *[]Conflict, path string, local, remote *Entry) {
	*conflicts = append(*conflicts, Conflict{
		Path:        path,
		LocalEntry:  local,
		RemoteEntry: remote,
	})

	if local == nil {
		merged[path] = cloneEntry(remote)
		return
	}
	if remote == nil {
		merged[path] = cloneEntry(local)
		return
	}

	// LWW: newer timestamp keeps the original name.
	winner, loser := local, remote
	if remote.Timestamp.After(local.Timestamp) {
		winner, loser = remote, local
	}

	merged[path] = cloneEntry(winner)

	// Preserve the losing version with a .conflict suffix.
	conflictPath := conflictName(path)
	merged[conflictPath] = cloneEntry(loser)
}

// conflictName appends ".conflict" to a path, preserving directory trailing slash.
func conflictName(path string) string {
	if strings.HasSuffix(path, "/") {
		return strings.TrimSuffix(path, "/") + ".conflict/"
	}
	return path + ".conflict"
}

// mergeEqual returns true if two entries represent the same content.
// For files, this means the same C4 ID. For directories, existence is
// sufficient. For symlinks, same target. For flow links, same direction
// and target.
func mergeEqual(a, b *Entry) bool {
	if a.IsDir() != b.IsDir() {
		return false
	}
	if a.IsDir() {
		// Directories: equal if both exist with same flow state.
		return a.FlowDirection == b.FlowDirection && a.FlowTarget == b.FlowTarget
	}
	if a.IsSymlink() || b.IsSymlink() {
		return a.IsSymlink() == b.IsSymlink() && a.Target == b.Target
	}
	return a.C4ID == b.C4ID
}

// cloneEntry creates a shallow copy of an entry.
func cloneEntry(e *Entry) *Entry {
	clone := *e
	return &clone
}

// entryPaths builds a map from full path to entry by walking the tree.
func entryPaths(entries []*Entry) map[string]*Entry {
	result := make(map[string]*Entry, len(entries))
	stack := make([]string, 0, 8)

	for _, e := range entries {
		for len(stack) > e.Depth {
			stack = stack[:len(stack)-1]
		}

		var sb strings.Builder
		for _, s := range stack {
			sb.WriteString(s)
		}
		sb.WriteString(e.Name)
		fullPath := sb.String()

		result[fullPath] = e

		if e.IsDir() {
			stack = append(stack, e.Name)
		}
	}

	return result
}

// ensureDirs creates directory entries for any parent paths that are
// implied by entries in the map but not explicitly present.
func ensureDirs(m map[string]*Entry) {
	var dirs []string
	for p := range m {
		parts := strings.Split(strings.TrimSuffix(p, "/"), "/")
		for i := 1; i < len(parts); i++ {
			dirPath := strings.Join(parts[:i], "/") + "/"
			if _, exists := m[dirPath]; !exists {
				dirs = append(dirs, dirPath)
			}
		}
	}
	for _, d := range dirs {
		if _, exists := m[d]; exists {
			continue
		}
		name := pathEntryName(d)
		m[d] = &Entry{
			Mode:  0755 | os.ModeDir,
			Size:  -1,
			Name:  name,
			Depth: pathToDepth(d),
		}
	}
}

// rebuildManifest constructs a Manifest from a full-path → entry map.
// Entries are sorted by path to produce correct tree ordering.
func rebuildManifest(m map[string]*Entry) *Manifest {
	paths := make([]string, 0, len(m))
	for p := range m {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	manifest := NewManifest()
	for _, p := range paths {
		e := m[p]
		e.Name = pathEntryName(p)
		e.Depth = pathToDepth(p)
		manifest.AddEntry(e)
	}
	return manifest
}

// pathToDepth returns the depth of an entry given its full path.
func pathToDepth(fullPath string) int {
	clean := fullPath
	if strings.HasSuffix(clean, "/") {
		clean = clean[:len(clean)-1]
	}
	return strings.Count(clean, "/")
}

// pathEntryName returns the Name field (last component) for a full path.
func pathEntryName(fullPath string) string {
	isDir := strings.HasSuffix(fullPath, "/")
	clean := strings.TrimSuffix(fullPath, "/")
	idx := strings.LastIndex(clean, "/")
	name := clean
	if idx >= 0 {
		name = clean[idx+1:]
	}
	if isDir {
		name += "/"
	}
	return name
}
