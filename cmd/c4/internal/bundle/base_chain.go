package bundle

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Avalanche-io/c4"
)

// BaseChainResolver resolves @base chains in C4M manifests
type BaseChainResolver struct {
	// Cache of loaded manifests to avoid reloading
	cache map[c4.ID]*Manifest

	// Bundle path for loading chunks
	bundlePath string

	// Maximum chain depth to prevent infinite loops
	maxDepth int
}

// NewBaseChainResolver creates a new resolver
func NewBaseChainResolver(bundlePath string) *BaseChainResolver {
	return &BaseChainResolver{
		cache:      make(map[c4.ID]*Manifest),
		bundlePath: bundlePath,
		maxDepth:   1000, // Reasonable limit for chain depth
	}
}

// ResolveChain follows @base references to build the complete manifest
func (r *BaseChainResolver) ResolveChain(manifest *Manifest) (*Manifest, error) {
	// Build the complete manifest by following @base chain
	result := NewManifest()
	result.Version = manifest.Version

	// Track visited IDs to detect cycles
	visited := make(map[c4.ID]bool)

	// We need to process the chain from base to derived
	// So collect all manifests first
	var chain []*Manifest
	depth := 0
	current := manifest

	// Collect the chain from derived to base
	for current != nil && depth < r.maxDepth {
		chain = append(chain, current)

		// Check for @base reference
		var emptyID c4.ID
		if current.Base == emptyID {
			break
		}

		// Check for cycles
		if visited[current.Base] {
			return nil, fmt.Errorf("cycle detected in @base chain at %s", current.Base)
		}
		visited[current.Base] = true

		// Load the base manifest
		baseManifest, err := r.loadManifest(current.Base)
		if err != nil {
			return nil, fmt.Errorf("failed to load base manifest %s: %w", current.Base, err)
		}

		current = baseManifest
		depth++
	}

	// Now process from base to derived (reverse order)
	for i := len(chain) - 1; i >= 0; i-- {
		for _, entry := range chain[i].Entries {
			// Check if this entry already exists (for updates/overrides)
			existingIndex := r.findEntry(result, entry)
			if existingIndex >= 0 {
				// Update existing entry
				result.Entries[existingIndex] = entry
			} else {
				// Add new entry
				result.AddEntry(entry)
			}
		}
	}

	if depth >= r.maxDepth {
		return nil, fmt.Errorf("@base chain exceeded maximum depth of %d", r.maxDepth)
	}

	// Sort the final result
	result.SortEntries()

	return result, nil
}

// findEntry finds an entry in the manifest by matching path and depth
func (r *BaseChainResolver) findEntry(manifest *Manifest, entry *Entry) int {
	for i, e := range manifest.Entries {
		if e.Name == entry.Name && e.Depth == entry.Depth {
			return i
		}
	}
	return -1
}

// loadManifest loads a manifest by its C4 ID
func (r *BaseChainResolver) loadManifest(id c4.ID) (*Manifest, error) {
	// Check cache first
	if manifest, ok := r.cache[id]; ok {
		return manifest, nil
	}

	// Try to load from bundle
	if r.bundlePath != "" {
		chunkPath := filepath.Join(r.bundlePath, "c4", id.String())
		if _, err := os.Stat(chunkPath); err == nil {
			manifest, err := r.loadManifestFromFile(chunkPath)
			if err == nil {
				r.cache[id] = manifest
				return manifest, nil
			}
		}
	}

	// Try to load from a .c4m file with the ID name
	idPath := id.String() + ".c4m"
	if _, err := os.Stat(idPath); err == nil {
		manifest, err := r.loadManifestFromFile(idPath)
		if err == nil {
			r.cache[id] = manifest
			return manifest, nil
		}
	}

	return nil, fmt.Errorf("manifest %s not found", id)
}

// loadManifestFromFile loads and parses a manifest from a file
func (r *BaseChainResolver) loadManifestFromFile(path string) (*Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := NewDecoder(file)
	return decoder.Decode()
}

// ResolveFromReader resolves @base chain from a reader
func ResolveBaseChain(reader io.Reader, bundlePath string) (*Manifest, error) {
	// Parse the initial manifest
	decoder := NewDecoder(reader)
	manifest, err := decoder.Decode()
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// If no @base, return as-is
	var emptyID c4.ID
	if manifest.Base == emptyID {
		return manifest, nil
	}

	// Resolve the chain
	resolver := NewBaseChainResolver(bundlePath)
	return resolver.ResolveChain(manifest)
}

// MaterializeBundle reads a bundle and produces the complete manifest
func MaterializeBundle(bundlePath string) (*Manifest, error) {
	// Open the bundle
	bundle, err := OpenBundle(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open bundle: %w", err)
	}

	// Find the most recent complete scan
	var latestScan *BundleScan
	for _, scan := range bundle.Scans {
		if scan.CompletedAt != nil && scan.SnapshotID != nil {
			latestScan = scan
		}
	}

	if latestScan == nil {
		return nil, fmt.Errorf("no complete scan found in bundle")
	}

	// Load the snapshot
	snapshotPath := filepath.Join(bundlePath, "c4", latestScan.SnapshotID.String())
	snapshotFile, err := os.Open(snapshotPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open snapshot: %w", err)
	}
	defer snapshotFile.Close()

	// Resolve the complete chain
	return ResolveBaseChain(snapshotFile, bundlePath)
}

// WriteChunkedManifest writes a manifest as chunks with @base references
func WriteChunkedManifest(manifest *Manifest, bundlePath string, config *BundleConfig) error {
	if config == nil {
		config = DefaultBundleConfig()
	}

	// Ensure c4 directory exists
	c4Dir := filepath.Join(bundlePath, "c4")
	if err := os.MkdirAll(c4Dir, 0755); err != nil {
		return fmt.Errorf("failed to create c4 directory: %w", err)
	}

	var previousID c4.ID
	chunkNum := 0

	// Process entries in chunks
	for i := 0; i < len(manifest.Entries); {
		chunk := NewManifest()
		chunk.Version = manifest.Version

		// Set @base reference if not first chunk
		var emptyID c4.ID
		if previousID != emptyID {
			chunk.Base = previousID
		}

		// Add entries up to chunk limit
		entriesInChunk := 0
		for i < len(manifest.Entries) && entriesInChunk < config.MaxEntriesPerChunk {
			chunk.AddEntry(manifest.Entries[i])
			i++
			entriesInChunk++
		}

		// Write chunk to string
		var buf strings.Builder
		if err := NewEncoder(&buf).Encode(chunk); err != nil {
			return fmt.Errorf("failed to write chunk %d: %w", chunkNum, err)
		}

		// Compute C4 ID of chunk
		chunkContent := buf.String()
		chunkID := c4.Identify(strings.NewReader(chunkContent))

		// Write chunk to file
		chunkPath := filepath.Join(c4Dir, chunkID.String())
		if err := os.WriteFile(chunkPath, []byte(chunkContent), 0644); err != nil {
			return fmt.Errorf("failed to write chunk file %d: %w", chunkNum, err)
		}

		previousID = chunkID
		chunkNum++
	}

	return nil
}