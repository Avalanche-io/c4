package c4m

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/Avalanche-io/c4"
)

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
	parser := NewParser(bytes.NewReader(buf.Bytes()))
	manifest, err := parser.ParseAll()
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

// Cache returns the manifest cache (allows external access for manifest operations)
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
