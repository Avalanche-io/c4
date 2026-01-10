package c4m

import (
	"fmt"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

// Getter provides parsed manifests by their C4 ID.
// This is the interface used by Merge to resolve @base references.
// ManifestCache already implements this interface.
type Getter interface {
	Get(id c4.ID) (*Manifest, error)
}

// FromStore wraps a store.Source to create a Getter.
// Manifests are parsed on each Get call (no caching).
// For caching, use NewManifestCache(store) instead.
func FromStore(src store.Source) Getter {
	return &storeAdapter{src: src}
}

// storeAdapter wraps a store.Source, parsing manifests on demand
type storeAdapter struct {
	src store.Source
}

func (a *storeAdapter) Get(id c4.ID) (*Manifest, error) {
	rc, err := a.src.Open(id)
	if err != nil {
		return nil, fmt.Errorf("open manifest %s: %w", id, err)
	}
	defer rc.Close()

	m, err := NewDecoder(rc).Decode()
	if err != nil {
		return nil, fmt.Errorf("decode manifest %s: %w", id, err)
	}

	return m, nil
}

// MapGetter is a Getter backed by pre-parsed manifests.
// Use this when you already have manifests loaded in memory.
type MapGetter map[c4.ID]*Manifest

// Get returns a manifest by ID from the map.
func (g MapGetter) Get(id c4.ID) (*Manifest, error) {
	if m, ok := g[id]; ok {
		return m, nil
	}
	return nil, fmt.Errorf("manifest not found: %s", id)
}

// Merge flattens a manifest by resolving its @base chain and applying layers.
// Returns a standalone manifest with no @base reference.
//
// What gets merged:
//   - All entries from the @base chain (recursively resolved)
//   - All @remove layers applied (entries deleted)
//   - The first add layer's entries (the default/canonical view)
//   - Embedded @data blocks (if content is still referenced)
//
// What does NOT get merged:
//   - Additional add layers (app-specific views)
//   - Layer metadata (by/note/time)
func (m *Manifest) Merge(getter Getter) (*Manifest, error) {
	return merge(m, getter, make(map[c4.ID]bool))
}

// merge is the recursive implementation with cycle detection
func merge(m *Manifest, getter Getter, visited map[c4.ID]bool) (*Manifest, error) {
	// Compute this manifest's ID for cycle detection
	myID := m.ComputeC4ID()
	if visited[myID] {
		return nil, fmt.Errorf("cycle detected in @base chain: %s", myID)
	}
	visited[myID] = true

	result := NewManifest()

	// Collect paths marked for removal in this manifest
	removed := m.Removals()
	removedSet := make(map[string]bool, len(removed))
	for _, path := range removed {
		removedSet[path] = true
	}

	// If there's a base, recursively merge it first
	if !m.Base.IsNil() {
		base, err := getter.Get(m.Base)
		if err != nil {
			return nil, fmt.Errorf("resolve @base %s: %w", m.Base, err)
		}

		mergedBase, err := merge(base, getter, visited)
		if err != nil {
			return nil, err
		}

		// Add base entries (unless removed by this manifest)
		for _, entry := range mergedBase.Entries {
			if !removedSet[entry.Name] {
				// Make a copy to avoid modifying the original
				entryCopy := *entry
				result.AddEntry(&entryCopy)
			}
		}

		// Carry forward data blocks from base (if still referenced)
		for _, block := range mergedBase.DataBlocks {
			result.AddDataBlock(block)
		}
	}

	// Add this manifest's entries (skip remove layer entries)
	for _, entry := range m.Entries {
		if entry.removeLayer {
			continue // Skip entries that are removal markers
		}
		// Make a copy
		entryCopy := *entry
		result.AddEntry(&entryCopy)
	}

	// Add this manifest's data blocks
	for _, block := range m.DataBlocks {
		result.AddDataBlock(block)
	}

	// Sort to ensure canonical order
	result.SortEntries()

	return result, nil
}

// Removals returns all paths marked for removal in @remove layers.
func (m *Manifest) Removals() []string {
	var paths []string
	for _, entry := range m.Entries {
		if entry.removeLayer {
			paths = append(paths, entry.Name)
		}
	}
	return paths
}

// InRemoveLayer returns true if this entry belongs to a @remove layer.
func (e *Entry) InRemoveLayer() bool {
	return e.removeLayer
}
