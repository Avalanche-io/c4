package c4m

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"sync"

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
func ApplyPatch(base, patch *Manifest) *Manifest {
	baseTree := buildPatchTree(base)
	patchTree := buildPatchTree(patch)
	applyPatchTree(baseTree, patchTree)

	var entries []*Entry
	flattenPatchTree(baseTree, 0, &entries)

	result := NewManifest()
	result.Entries = entries
	result.SortEntries()
	return result
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

		// Check if exact match (removal)
		if entriesIdentical(bNode.entry, pNode.entry) {
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
// metadata fields. Used by patch semantics: an exact duplicate signals removal.
func entriesIdentical(a, b *Entry) bool {
	return a.Name == b.Name &&
		a.Mode == b.Mode &&
		a.Timestamp.Equal(b.Timestamp) &&
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

	// Build maps for efficient lookup
	aMap := make(map[string]*Entry)
	for _, entry := range manifestA.Entries {
		aMap[entry.Name] = entry
	}

	bMap := make(map[string]*Entry)
	for _, entry := range manifestB.Entries {
		bMap[entry.Name] = entry
	}

	// Check entries in A
	for name, entryA := range aMap {
		if entryB, exists := bMap[name]; exists {
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
	for name, entryB := range bMap {
		if _, exists := aMap[name]; !exists {
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

	// Otherwise compare all attributes
	return a.Mode == b.Mode &&
		a.Size == b.Size &&
		a.Timestamp.Equal(b.Timestamp) &&
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
		// Find entry in current manifest at the expected depth
		entry := manifest.GetEntry(part)
		if entry == nil {
			// Try with trailing slash (for directories)
			entry = manifest.GetEntry(part + "/")
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