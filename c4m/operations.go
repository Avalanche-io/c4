package c4m

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

// Source represents anything that can be converted to a manifest
type Source interface {
	ToManifest() (*Manifest, error)
}

// ManifestSource wraps an existing manifest to implement the Source interface.
type ManifestSource struct {
	Manifest *Manifest
}

// ToManifest returns the wrapped manifest.
func (ms ManifestSource) ToManifest() (*Manifest, error) {
	return ms.Manifest, nil
}

// PatchResult contains the output of a patch-format diff.
type PatchResult struct {
	Patch *Manifest // Entries constituting the patch delta
	OldID c4.ID     // C4 ID of the old state (prior page boundary)
	NewID c4.ID     // C4 ID of the new state (closing page boundary)
}

// IsEmpty returns true if there are no differences.
func (pr *PatchResult) IsEmpty() bool {
	return len(pr.Patch.Entries) == 0
}

// PatchDiff compares old and new manifests and produces a patch-format diff.
// The result contains only changed entries with proper nesting. Subtrees with
// matching C4 IDs are skipped entirely — for a million-file tree where one
// file changed, this touches only entries along the path to that file.
//
// Patch entry semantics (applied against the old state):
//   - Addition: entry exists only in new → emitted as-is
//   - Removal: entry exists only in old → re-emitted (exact duplicate = removal)
//   - Modification: same name, different C4 ID → new entry emitted (clobber)
//   - Directory with changes: new dir entry emitted (updated C4 ID), children recursed
func PatchDiff(old, new *Manifest) *PatchResult {
	oldIdx := old.ensureIndex()
	newIdx := new.ensureIndex()

	var entries []*Entry
	diffTree(old, new, oldIdx.root, newIdx.root, 0, &entries)

	patch := NewManifest()
	patch.Entries = entries

	return &PatchResult{
		Patch: patch,
		OldID: old.ComputeC4ID(),
		NewID: new.ComputeC4ID(),
	}
}

// diffTree recursively compares children at a given depth, emitting patch entries.
func diffTree(old, new *Manifest, oldChildren, newChildren []*Entry, depth int, result *[]*Entry) {
	oldByName := make(map[string]*Entry, len(oldChildren))
	for _, e := range oldChildren {
		oldByName[e.Name] = e
	}
	newByName := make(map[string]*Entry, len(newChildren))
	for _, e := range newChildren {
		newByName[e.Name] = e
	}

	names := diffUnionNames(oldByName, newByName)

	for _, name := range names {
		oldEntry := oldByName[name]
		newEntry := newByName[name]

		if newEntry != nil && oldEntry == nil {
			// Addition — emit new entry and full subtree
			*result = append(*result, entryAtDepth(newEntry, depth))
			if newEntry.IsDir() {
				emitSubtree(new, newEntry, depth+1, result)
			}
			continue
		}

		if oldEntry != nil && newEntry == nil {
			// Removal — re-emit old entry (exact duplicate signals removal)
			*result = append(*result, entryAtDepth(oldEntry, depth))
			continue
		}

		// Both exist — check if completely identical (content + metadata)
		if entriesIdentical(oldEntry, newEntry) {
			if !oldEntry.IsDir() {
				// Identical file — skip
				continue
			}
			// Identical directory entry — but children may differ, so recurse
			oldChildren := old.Children(oldEntry)
			newChildren := new.Children(newEntry)
			var childEntries []*Entry
			diffTree(old, new, oldChildren, newChildren, depth+1, &childEntries)
			if len(childEntries) == 0 {
				// No child differences — skip entirely
				continue
			}
			// Children differ — emit dir entry and child diffs
			*result = append(*result, entryAtDepth(newEntry, depth))
			*result = append(*result, childEntries...)
			continue
		}

		if oldEntry.IsDir() && newEntry.IsDir() {
			// Both directories, different content — emit new dir entry and recurse
			*result = append(*result, entryAtDepth(newEntry, depth))
			diffTree(old, new, old.Children(oldEntry), new.Children(newEntry), depth+1, result)
		} else {
			// File modified or type changed — emit new entry (clobber)
			*result = append(*result, entryAtDepth(newEntry, depth))
		}
	}
}

// entryAtDepth returns a copy of src with the given depth.
func entryAtDepth(src *Entry, depth int) *Entry {
	e := *src
	e.Depth = depth
	return &e
}

// emitSubtree recursively emits all children of a directory.
func emitSubtree(m *Manifest, dir *Entry, depth int, result *[]*Entry) {
	for _, child := range m.Children(dir) {
		*result = append(*result, entryAtDepth(child, depth))
		if child.IsDir() {
			emitSubtree(m, child, depth+1, result)
		}
	}
}

// diffUnionNames returns the sorted union of entry names from both maps.
// Sort order: files before directories, then lexicographic within each group.
func diffUnionNames(a, b map[string]*Entry) []string {
	seen := make(map[string]bool, len(a)+len(b))
	for name := range a {
		seen[name] = true
	}
	for name := range b {
		seen[name] = true
	}

	var files, dirs []string
	for name := range seen {
		// Check if it's a directory in either manifest
		isDir := false
		if e, ok := a[name]; ok && e.IsDir() {
			isDir = true
		}
		if e, ok := b[name]; ok && e.IsDir() {
			isDir = true
		}
		if isDir {
			dirs = append(dirs, name)
		} else {
			files = append(files, name)
		}
	}

	sort.Strings(files)
	sort.Strings(dirs)
	return append(files, dirs...)
}

// ApplyPatch applies patch entries to a base manifest, producing the result.
// Patch entry semantics (matching by path):
//   - Exact duplicate of a base entry → removal (entry and children deleted)
//   - Same path, different content → clobber (replace entry, recurse for dirs)
//   - New path → addition
//
// ApplyPatch panics with an error wrapping ErrInvalidEntry, naming the
// entry, if either manifest holds an entry whose depth skips past its
// parent directory; Decoder.Decode reports the same condition as an error.
func ApplyPatch(base, patch *Manifest) *Manifest {
	baseTree := buildPatchTree(base)
	patchTree := buildPatchTree(patch)
	applyPatchTree(baseTree, patchTree)

	var entries []*Entry
	flattenPatchTree(baseTree, 0, &entries)

	result := NewManifest()
	result.Entries = entries
	result.SortEntries()

	// Merge RangeData from both base and patch.
	if len(base.RangeData) > 0 || len(patch.RangeData) > 0 {
		result.RangeData = make(map[c4.ID]string)
		for k, v := range base.RangeData {
			result.RangeData[k] = v
		}
		for k, v := range patch.RangeData {
			result.RangeData[k] = v
		}
	}

	return result
}

// ApplyPatchFrom applies to target the patch that was diffed from base.
//
// A patch's entries mean "removal" or "this entry" only against the state
// the patch's boundary names: an entry identical to base's with no
// children is a removal there, anything else is what the path now holds.
// Applying a patch directly to a tree other than its base reads those
// meanings against the wrong state: an "add" that meets an identical
// entry already present in the target reads as a removal, so two units
// writing the same bytes to the same path leave the path empty, and
// replaying a fold removes what it added (found in the operator's fold,
// 2026-11-09). ApplyPatchFrom reads each entry against base first, then
// does to target what the entry means: removes the path, or writes the
// entry (overwriting whatever target holds there, including a directory
// or file of the same name but the other kind; a caller wanting a
// conflict policy compares first, Merge keeps its own rules). A directory
// line with children under it is context for those children, not a
// write: it never creates a directory on its own, and directory sizes
// and IDs in the result are derived from the children listed, not copied
// (see applyPatchFromTree). With target equal to base it is ApplyPatch
// up to those derived values, and it is idempotent. SPECIFICATION.md
// "Patch Entry Semantics": a patch is meaningful only over its own base.
func ApplyPatchFrom(target, base, patch *Manifest) (*Manifest, error) {
	targetTree := buildPatchTree(target)
	baseTree := buildPatchTree(base)
	patchTree := buildPatchTree(patch)
	descended := make(map[*patchNode]bool)
	ownBase := target.ComputeC4ID() == base.ComputeC4ID()
	if _, err := applyPatchFromTree(targetTree, baseTree, patchTree, descended, ownBase, ""); err != nil {
		return nil, err
	}

	// Every directory the walk went inside has children this call may have
	// changed, so its size and ID are stale by construction: re-derive them
	// from what remains. A directory left with no children is the empty
	// directory (size 0, the empty-content ID), never a directory "known
	// only by its ID", which is what a stale line with no children would
	// read as and what Normalize cannot tell apart (found by the operator's
	// cross-check, 2026-11-09). Directories the walk never entered keep
	// their lines. Entries are copied before they change: the inputs are
	// never mutated.
	for node := range descended {
		cp := *node.entry
		if len(node.children) == 0 {
			cp.Size = 0
			cp.C4ID = c4.Identify(bytes.NewReader(nil))
		} else {
			cp.Size = -1
			cp.C4ID = c4.ID{}
		}
		node.entry = &cp
	}

	var entries []*Entry
	flattenPatchTree(targetTree, 0, &entries)

	result := NewManifest()
	result.Entries = entries
	result.SortEntries()
	// Directory sizes and IDs are derived from the children the result
	// lists, never copied from the patch: a directory that now holds
	// other units' children would otherwise carry this unit's aggregate.
	Normalize(result)

	if len(target.RangeData) > 0 || len(patch.RangeData) > 0 {
		result.RangeData = make(map[c4.ID]string)
		for k, v := range target.RangeData {
			result.RangeData[k] = v
		}
		for k, v := range patch.RangeData {
			result.RangeData[k] = v
		}
	}
	return result, nil
}

// applyPatchFromTree walks patch beside base to learn each entry's
// meaning, and edits target accordingly. base may be nil below a
// directory the base never held. It reports whether it wrote anything
// into target (a removal that found nothing is not a write).
//
// A directory line with children under it means nothing by itself: it
// is the deepening line of the 2026-07-31 erratum, context for the
// children, which are read against the base's children. So a directory
// the target lacks is created only when a write lands under it, never by
// a line that only carries removals (a unit that deleted a file never
// made that directory), and a directory the target already holds keeps
// its own line unless that line is still the base's, in which case the
// patch's line is the same observation continued. A directory line with
// no children is read like a file. Derived directory values (size, ID)
// are re-derived by the caller; nothing copies them from the patch onto
// a directory whose children now differ.
//
// A name is one thing. Directory names carry their trailing slash, so
// file x and directory x/ are different keys here and could otherwise
// coexist in the result (75 of 601 applications with kind swaps did,
// in the storage project's review, 2026-11-09). A write of x removes
// x/ and everything under it; a write that lands under x/, or writes
// x/ itself, removes a file or symlink x: the later write wins by kind
// as it does by bytes. A removal removes only the entry it names, of
// the kind the base held, so where the target has since replaced file
// x with directory x/ (or the reverse), removing x leaves the new
// thing. A directory written by ID alone, with no children listed, over
// a directory the target holds is refused (ErrDirByIDOverHeld) unless the
// target is the patch's own base, where it replaces the directory as
// ApplyPatch does: replacing would drop the target's subtree, and keeping
// its children would discard the written ID (decided 2026-11-13 by the
// release lead under the maintainer's standing instruction to decide;
// reversible). The empty listing is the exception: its ID names zero
// children, so an empty directory line means the directory exists and
// removes nothing under it.
func applyPatchFromTree(target, base, patch *patchNode, descended map[*patchNode]bool, ownBase bool, prefix string) (wrote bool, err error) {
	for name, pNode := range patch.children {
		var bNode *patchNode
		if base != nil {
			bNode = base.children[name]
		}
		if pNode.entry.IsDir() && len(pNode.children) > 0 {
			tNode, exists := target.children[name]
			if !exists {
				fresh := &patchNode{entry: pNode.entry, children: make(map[string]*patchNode)}
				w, err := applyPatchFromTree(fresh, bNode, pNode, descended, ownBase, prefix+name)
				if err != nil {
					return false, err
				}
				if w {
					target.children[name] = fresh
					delete(target.children, otherKind(name))
					descended[fresh] = true
					wrote = true
				}
				continue
			}
			if bNode != nil && entriesIdentical(tNode.entry, bNode.entry) {
				tNode.entry = pNode.entry
			}
			descended[tNode] = true
			w, err := applyPatchFromTree(tNode, bNode, pNode, descended, ownBase, prefix+name)
			if err != nil {
				return false, err
			}
			if w {
				delete(target.children, otherKind(name))
				wrote = true
			}
			continue
		}
		// Removal: identical to the base's entry, with no children listed.
		if bNode != nil && entriesIdentical(bNode.entry, pNode.entry) {
			delete(target.children, name)
			continue
		}
		// An empty directory: a childless line carrying the empty
		// listing's ID, which is what capture writes for a unit that made
		// a directory and put nothing in it. The empty listing is the one
		// listing known without lookup, so this is a directory line with
		// its (zero) children listed: the directory exists, and anything
		// the target already holds under it is merged beside nothing, not
		// removed. A unit that ran mkdir never observed another unit's
		// files there, so it cannot remove them (the storage project's
		// fold case, 2026-11-10). Over no directory it creates the empty
		// one; over the target's directory it keeps the target's line
		// unless that line is still the base's.
		if pNode.entry.IsDir() && pNode.entry.C4ID == emptyListingID {
			delete(target.children, otherKind(name))
			tNode, exists := target.children[name]
			if !exists {
				target.children[name] = &patchNode{entry: pNode.entry, children: make(map[string]*patchNode)}
				wrote = true
				continue
			}
			if bNode != nil && entriesIdentical(tNode.entry, bNode.entry) {
				tNode.entry = pNode.entry
			}
			descended[tNode] = true
			wrote = true
			continue
		}
		// Written: the path now holds this entry, and nothing else of
		// its name. For a directory this is a listing given by ID alone:
		// over the patch's own base it replaces the directory, children
		// and all; carried to another tree that holds a directory there,
		// it is refused.
		if pNode.entry.IsDir() && !ownBase {
			if tNode, held := target.children[name]; held && tNode.entry.IsDir() {
				return false, fmt.Errorf("%w: %s", ErrDirByIDOverHeld, prefix+name)
			}
		}
		delete(target.children, otherKind(name))
		target.children[name] = &patchNode{entry: pNode.entry, children: make(map[string]*patchNode)}
		wrote = true
	}
	return wrote, nil
}

// otherKind returns the key of the entry a write at name displaces:
// the directory x/ for a file or symlink x, and x for x/.
func otherKind(name string) string {
	if strings.HasSuffix(name, "/") {
		return strings.TrimSuffix(name, "/")
	}
	return name + "/"
}

// patchNode is a tree node for patch application.
type patchNode struct {
	entry    *Entry
	children map[string]*patchNode
}

// buildPatchTree builds a tree from a manifest's flat entry list.
func buildPatchTree(m *Manifest) *patchNode {
	root := &patchNode{children: make(map[string]*patchNode)}
	stack := make([]*patchNode, 1)
	stack[0] = root

	for _, e := range m.Entries {
		// An entry whose depth skips past its parent directory (a corrupt,
		// truncated or hand-edited c4m) used to index past the stack and
		// panic with a bare "index out of range" from every verb. It is a
		// named error wrapping ErrInvalidEntry: Decoder.Decode returns it,
		// and the CLI exits 1 with it (release 1.0.x: 59ce7cc, 6cfba2c).
		if e.Depth < 0 {
			panic(fmt.Errorf("%w: entry %q has negative depth %d", ErrInvalidEntry, e.Name, e.Depth))
		}
		if e.Depth >= len(stack) || stack[e.Depth] == nil {
			panic(fmt.Errorf("%w: entry %q at depth %d has no parent directory at depth %d",
				ErrInvalidEntry, e.Name, e.Depth, e.Depth-1))
		}
		if e.Depth+1 < len(stack) {
			stack = stack[:e.Depth+1]
		}
		parent := stack[e.Depth]

		node := &patchNode{entry: e, children: make(map[string]*patchNode)}
		parent.children[e.Name] = node

		if e.IsDir() {
			for len(stack) <= e.Depth+1 {
				stack = append(stack, nil)
			}
			stack[e.Depth+1] = node
		}
	}
	return root
}

// applyPatchTree recursively applies patch changes to a base tree.
func applyPatchTree(base, patch *patchNode) {
	for name, pNode := range patch.children {
		bNode, exists := base.children[name]

		if !exists {
			// Addition — graft entire subtree
			base.children[name] = pNode
			continue
		}

		// Check if exact match (removal). A restatement that declares
		// children under it is deepening that directory, not removing it:
		// breadth-first discovery learns a directory's children before it
		// learns anything about the directory itself, so its parent line is
		// necessarily unchanged. Falling through clobbers with an identical
		// entry (a no-op) and recurses to merge the new children.
		if len(pNode.children) == 0 && entriesIdentical(bNode.entry, pNode.entry) {
			delete(base.children, name)
			continue
		}

		// Clobber — replace entry
		bNode.entry = pNode.entry

		// For directories, recurse to apply child-level changes
		if bNode.entry.IsDir() && pNode.entry.IsDir() {
			applyPatchTree(bNode, pNode)
		}
	}
}

// entriesIdentical checks if two entries are exactly the same across all
// metadata fields, at the format's resolution: timestamps compare to the
// second, since that is all a c4m line carries. Used by patch semantics:
// an exact duplicate signals removal. Comparing nanoseconds here would
// let PatchDiff emit a "change" for two scans of the same file that a
// text round trip then turns into a removal.
func entriesIdentical(a, b *Entry) bool {
	return a.Name == b.Name &&
		a.Mode == b.Mode &&
		a.Timestamp.Truncate(time.Second).Equal(b.Timestamp.Truncate(time.Second)) &&
		a.Size == b.Size &&
		a.C4ID == b.C4ID &&
		a.Target == b.Target &&
		a.HardLink == b.HardLink &&
		a.FlowDirection == b.FlowDirection &&
		a.FlowTarget == b.FlowTarget
}

// flattenPatchTree converts a tree back to a flat entry list.
func flattenPatchTree(node *patchNode, depth int, result *[]*Entry) {
	for _, child := range node.children {
		e := *child.entry
		e.Depth = depth
		*result = append(*result, &e)
		if child.entry.IsDir() {
			flattenPatchTree(child, depth+1, result)
		}
	}
}

// Diff compares two sources and returns a categorized diff result.
// Entries are matched by their full path within each manifest, so
// same-named files in different directories never collide.
// For patch-format output, prefer PatchDiff which produces properly
// nested entries suitable for direct serialization.
func Diff(a, b Source) (*DiffResult, error) {
	manifestA, err := a.ToManifest()
	if err != nil {
		return nil, fmt.Errorf("failed to get manifest A: %w", err)
	}

	manifestB, err := b.ToManifest()
	if err != nil {
		return nil, fmt.Errorf("failed to get manifest B: %w", err)
	}

	result := &DiffResult{
		Added:    NewManifest(),
		Removed:  NewManifest(),
		Modified: NewManifest(),
		Same:     NewManifest(),
	}

	// Build maps for efficient lookup, keyed by full path (never by bare
	// name — same-named entries in different directories must not collide).
	aMap := make(map[string]*Entry)
	for _, entry := range manifestA.Entries {
		aMap[manifestA.EntryPath(entry)] = entry
	}

	bMap := make(map[string]*Entry)
	for _, entry := range manifestB.Entries {
		bMap[manifestB.EntryPath(entry)] = entry
	}

	// Check entries in A
	for path, entryA := range aMap {
		if entryB, exists := bMap[path]; exists {
			if entriesEqual(entryA, entryB) {
				result.Same.AddEntry(entryA)
			} else {
				result.Modified.AddEntry(entryB)
			}
		} else {
			result.Removed.AddEntry(entryA)
		}
	}

	// Check entries only in B (added)
	for path, entryB := range bMap {
		if _, exists := aMap[path]; !exists {
			result.Added.AddEntry(entryB)
		}
	}

	result.Added.SortEntries()
	result.Removed.SortEntries()
	result.Modified.SortEntries()
	result.Same.SortEntries()

	return result, nil
}

// DiffResult contains the results of a diff operation
type DiffResult struct {
	Added    *Manifest
	Removed  *Manifest
	Modified *Manifest
	Same     *Manifest
}

// IsEmpty returns true if there are no differences
func (dr *DiffResult) IsEmpty() bool {
	return len(dr.Added.Entries) == 0 &&
		len(dr.Removed.Entries) == 0 &&
		len(dr.Modified.Entries) == 0
}

// entriesEqual compares two entries for equality
func entriesEqual(a, b *Entry) bool {
	if a.Name != b.Name {
		return false
	}

	// Compare C4 IDs if both present
	if !a.C4ID.IsNil() && !b.C4ID.IsNil() {
		return a.C4ID == b.C4ID && a.Mode == b.Mode
	}

	// Otherwise compare all attributes, at the format's resolution (a
	// c4m line carries whole seconds).
	return a.Mode == b.Mode &&
		a.Size == b.Size &&
		a.Timestamp.Truncate(time.Second).Equal(b.Timestamp.Truncate(time.Second)) &&
		a.Target == b.Target
}

// PathList returns just the paths from a manifest
func (m *Manifest) PathList() []string {
	paths := make([]string, 0, len(m.Entries))
	for _, entry := range m.Entries {
		paths = append(paths, entry.Name)
	}
	sort.Strings(paths)
	return paths
}

// FilterByPath returns a new manifest with only entries matching the path pattern
func (m *Manifest) FilterByPath(pattern string) *Manifest {
	result := NewManifest()
	for _, entry := range m.Entries {
		if matched, _ := path.Match(pattern, entry.Name); matched {
			result.AddEntry(entry)
		}
	}
	return result
}

// FilterByPrefix returns entries under a given path prefix
func (m *Manifest) FilterByPrefix(prefix string) *Manifest {
	result := NewManifest()
	for _, entry := range m.Entries {
		if strings.HasPrefix(entry.Name, prefix) {
			result.AddEntry(entry)
		}
	}
	return result
}

// ----------------------------------------------------------------------------
// Caching
// ----------------------------------------------------------------------------

// ManifestCache provides cached access to manifests
type ManifestCache struct {
	src   store.Source
	cache map[string]*Manifest
	mu    sync.RWMutex
}

// NewManifestCache creates a new manifest cache
func NewManifestCache(src store.Source) *ManifestCache {
	return &ManifestCache{
		src:   src,
		cache: make(map[string]*Manifest),
	}
}

// Get retrieves a manifest from cache or storage
func (mc *ManifestCache) Get(id c4.ID) (*Manifest, error) {
	idStr := id.String()

	// Check cache first
	mc.mu.RLock()
	if manifest, ok := mc.cache[idStr]; ok {
		mc.mu.RUnlock()
		return manifest, nil
	}
	mc.mu.RUnlock()

	// Load from store
	reader, err := mc.src.Open(id)
	if err != nil {
		return nil, fmt.Errorf("loading manifest: %w", err)
	}
	defer reader.Close()

	// Read all content
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	// Parse manifest
	manifest, err := Unmarshal(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	// Cache it
	mc.mu.Lock()
	mc.cache[idStr] = manifest
	mc.mu.Unlock()

	return manifest, nil
}

// Clear clears the cache (useful for testing)
func (mc *ManifestCache) Clear() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.cache = make(map[string]*Manifest)
}

// ----------------------------------------------------------------------------
// Path Resolution
// ----------------------------------------------------------------------------

// Resolver resolves paths through manifest hierarchy
type Resolver struct {
	cache *ManifestCache
}

// NewResolver creates a new path resolver
func NewResolver(src store.Source) *Resolver {
	return &Resolver{
		cache: NewManifestCache(src),
	}
}

// Cache returns the manifest cache for direct access
func (r *Resolver) Cache() *ManifestCache {
	return r.cache
}

// ResolveResult contains the result of path resolution
type ResolveResult struct {
	ID       c4.ID     // C4 ID of the resolved item
	IsDir    bool      // True if this is a directory
	Manifest *Manifest // If IsDir, the manifest for this directory
}

// Resolve resolves a path through a manifest hierarchy
func (r *Resolver) Resolve(rootManifestID c4.ID, path string) (*ResolveResult, error) {
	// Check for valid root manifest ID
	if rootManifestID.IsNil() {
		return nil, fmt.Errorf("nil root manifest ID")
	}

	// Start at root manifest
	manifest, err := r.cache.Get(rootManifestID)
	if err != nil {
		return nil, fmt.Errorf("loading root manifest: %w", err)
	}

	// Clean and split path
	path = strings.Trim(path, "/")
	if path == "" {
		// Root path - return root manifest
		return &ResolveResult{
			ID:       rootManifestID,
			IsDir:    true,
			Manifest: manifest,
		}, nil
	}

	parts := strings.Split(path, "/")

	// Traverse path components
	// For flat manifests, we need to track the current depth
	currentDepth := 0

	for i, part := range parts {
		// Find entry in current manifest at the expected depth.
		// Use GetEntryByName because the Resolver traverses one component
		// at a time; for hierarchical manifests each sub-manifest starts at
		// depth 0, so bare names are sufficient.
		entry := manifest.GetEntryByName(part)
		if entry == nil {
			// Try with trailing slash (for directories)
			entry = manifest.GetEntryByName(part + "/")
		}

		if entry == nil {
			// Debug: list available entries at current depth
			var available []string
			for _, e := range manifest.Entries {
				if e.Depth == currentDepth {
					available = append(available, fmt.Sprintf("'%s'", e.Name))
				}
			}
			if len(available) == 0 {
				// If no entries at this depth, show all entries
				for _, e := range manifest.Entries {
					available = append(available, fmt.Sprintf("'%s' (depth %d)", e.Name, e.Depth))
				}
			}
			return nil, fmt.Errorf("path not found: %s (at component: %s, depth: %d) - available entries: [%s]",
				path, part, currentDepth, strings.Join(available, ", "))
		}

		isLastPart := i == len(parts)-1

		// Check if this is a directory
		if strings.HasSuffix(entry.Name, "/") {
			// Directory entry
			if !isLastPart {
				// Check if this is a hierarchical manifest (has C4 ID) or flat (null C4 ID)
				if !entry.C4ID.IsNil() {
					// Hierarchical: load sub-manifest and reset depth
					manifest, err = r.cache.Get(entry.C4ID)
					if err != nil {
						return nil, fmt.Errorf("loading manifest for %s: %w", entry.Name, err)
					}
					currentDepth = 0 // Reset depth for new manifest
				} else {
					// Flat manifest: continue searching in current manifest at next depth
					currentDepth = entry.Depth + 1
				}
			} else {
				// Last component and it's a directory
				if !entry.C4ID.IsNil() {
					// Hierarchical: load sub-manifest
					manifest, err = r.cache.Get(entry.C4ID)
					if err != nil {
						return nil, fmt.Errorf("loading manifest for %s: %w", entry.Name, err)
					}
					return &ResolveResult{
						ID:       entry.C4ID,
						IsDir:    true,
						Manifest: manifest,
					}, nil
				} else {
					// Flat manifest: return current manifest filtered to this depth+1
					// The manifest contains the children at depth+1
					return &ResolveResult{
						ID:       c4.ID{}, // No C4 ID for directory in flat manifest
						IsDir:    true,
						Manifest: manifest, // Current manifest contains children
					}, nil
				}
			}
		} else {
			// File entry
			if !isLastPart {
				return nil, fmt.Errorf("cannot traverse through file: %s", entry.Name)
			}
			return &ResolveResult{
				ID:    entry.C4ID,
				IsDir: false,
			}, nil
		}
	}

	// Should not reach here
	return nil, fmt.Errorf("path resolution failed: %s", path)
}
