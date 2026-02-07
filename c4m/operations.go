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

// Diff compares two sources and returns a manifest of differences
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
			// Entry exists in both
			if entriesEqual(entryA, entryB) {
				result.Same.AddEntry(entryA)
			} else {
				result.Modified.AddEntry(entryB)
			}
		} else {
			// Entry only in A (removed from B's perspective)
			result.Removed.AddEntry(entryA)
		}
	}
	
	// Check entries only in B (added)
	for name, entryB := range bMap {
		if _, exists := aMap[name]; !exists {
			result.Added.AddEntry(entryB)
		}
	}
	
	// Sort all results
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

// Union combines multiple sources into a single manifest
func Union(sources ...Source) (*Manifest, error) {
	result := NewManifest()
	seen := make(map[string]*Entry)
	
	for _, source := range sources {
		manifest, err := source.ToManifest()
		if err != nil {
			return nil, fmt.Errorf("failed to get manifest: %w", err)
		}
		
		for _, entry := range manifest.Entries {
			// Use latest version of duplicate entries
			seen[entry.Name] = entry
		}
	}
	
	// Add all unique entries
	for _, entry := range seen {
		result.AddEntry(entry)
	}
	
	result.SortEntries()
	return result, nil
}

// Intersect returns only entries common to all sources
func Intersect(sources ...Source) (*Manifest, error) {
	if len(sources) == 0 {
		return NewManifest(), nil
	}
	
	// Start with first manifest
	first, err := sources[0].ToManifest()
	if err != nil {
		return nil, fmt.Errorf("failed to get first manifest: %w", err)
	}
	
	// Build map of entries that appear in all manifests
	common := make(map[string]*Entry)
	for _, entry := range first.Entries {
		common[entry.Name] = entry
	}
	
	// Check against remaining manifests
	for i := 1; i < len(sources); i++ {
		manifest, err := sources[i].ToManifest()
		if err != nil {
			return nil, fmt.Errorf("failed to get manifest %d: %w", i, err)
		}
		
		// Build map for this manifest
		currentMap := make(map[string]*Entry)
		for _, entry := range manifest.Entries {
			currentMap[entry.Name] = entry
		}
		
		// Remove entries not in current manifest
		for name := range common {
			if _, exists := currentMap[name]; !exists {
				delete(common, name)
			}
		}
	}
	
	// Build result
	result := NewManifest()
	for _, entry := range common {
		result.AddEntry(entry)
	}
	
	result.SortEntries()
	return result, nil
}

// Subtract removes entries in 'remove' from 'from'
func Subtract(from, remove Source) (*Manifest, error) {
	fromManifest, err := from.ToManifest()
	if err != nil {
		return nil, fmt.Errorf("failed to get 'from' manifest: %w", err)
	}
	
	removeManifest, err := remove.ToManifest()
	if err != nil {
		return nil, fmt.Errorf("failed to get 'remove' manifest: %w", err)
	}
	
	// Build map of entries to remove
	removeMap := make(map[string]bool)
	for _, entry := range removeManifest.Entries {
		removeMap[entry.Name] = true
	}
	
	// Build result with entries not in remove set
	result := NewManifest()
	for _, entry := range fromManifest.Entries {
		if !removeMap[entry.Name] {
			result.AddEntry(entry)
		}
	}
	
	result.SortEntries()
	return result, nil
}

// entriesEqual compares two entries for equality
func entriesEqual(a, b *Entry) bool {
	// Entries must have the same name to be equal
	if a.Name != b.Name {
		return false
	}
	
	// Compare C4 IDs if both present
	if !a.C4ID.IsNil() && !b.C4ID.IsNil() {
		return a.C4ID.String() == b.C4ID.String() &&
		       a.Mode == b.Mode
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
// Storage and Caching
// ----------------------------------------------------------------------------

// Storage defines the interface for loading manifests by C4 ID
type Storage interface {
	Get(id c4.ID) (io.ReadCloser, error)
}

// ManifestCache provides cached access to manifests
type ManifestCache struct {
	storage Storage
	cache   map[string]*Manifest
	mu      sync.RWMutex
}

// NewManifestCache creates a new manifest cache
func NewManifestCache(storage Storage) *ManifestCache {
	return &ManifestCache{
		storage: storage,
		cache:   make(map[string]*Manifest),
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

	// Load from storage
	reader, err := mc.storage.Get(id)
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
func NewResolver(storage Storage) *Resolver {
	return &Resolver{
		cache: NewManifestCache(storage),
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