package c4m

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// Source represents anything that can be converted to a manifest
type Source interface {
	ToManifest() (*Manifest, error)
}

// FileSource represents a filesystem path
type FileSource struct {
	Path string
	Generator *Generator
}

func (fs FileSource) ToManifest() (*Manifest, error) {
	if fs.Generator == nil {
		fs.Generator = NewGenerator()
	}
	return fs.Generator.GenerateFromPath(fs.Path)
}

// ManifestSource wraps an existing manifest
type ManifestSource struct {
	Manifest *Manifest
}

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
	result.Added.Sort()
	result.Removed.Sort()
	result.Modified.Sort()
	result.Same.Sort()
	
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
	
	result.Sort()
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
	
	result.Sort()
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
	
	result.Sort()
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