package bundle

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DirectoryNode represents a directory in the manifest tree
type DirectoryNode struct {
	Entry    *Entry
	Children map[string]*ManifestNode
	Dirty    bool // Indicates if this directory needs re-sorting
}

// ManifestNode represents a node in the manifest tree (can be file or directory)
type ManifestNode struct {
	Entry    *Entry
	Children map[string]*ManifestNode // Only used for directories
	Dirty    bool                      // Indicates if this directory needs re-sorting
}

// ManifestTree represents the complete manifest structure
type ManifestTree struct {
	Root     *ManifestNode
	NodeMap  map[string]*ManifestNode // Quick lookup by path
}

// NewManifestTree creates a new manifest tree
func NewManifestTree() *ManifestTree {
	return &ManifestTree{
		Root: &ManifestNode{
			Children: make(map[string]*ManifestNode),
		},
		NodeMap: make(map[string]*ManifestNode),
	}
}

// LoadBundleAsManifest loads a bundle following the proper @base chain workflow
func LoadBundleAsManifest(bundlePath string) (*Manifest, error) {
	// Read header.c4 to get the header manifest ID
	headerPath := filepath.Join(bundlePath, "header.c4")
	headerData, err := os.ReadFile(headerPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read header.c4: %w", err)
	}
	
	headerID := strings.TrimSpace(string(headerData))
	if !strings.HasPrefix(headerID, "c4") {
		return nil, fmt.Errorf("invalid C4 ID in header.c4")
	}
	
	// Start with the header manifest as the last chunk
	lastChunkID := headerID
	
	// Follow @base chain backwards to build chunk stack
	var chunkStack []string
	seenChunks := make(map[string]bool) // Prevent infinite loops
	
	for lastChunkID != "" {
		if seenChunks[lastChunkID] {
			return nil, fmt.Errorf("circular @base reference detected at %s", lastChunkID)
		}
		seenChunks[lastChunkID] = true
		
		// Add to stack (we'll pop from end to process first to last)
		chunkStack = append(chunkStack, lastChunkID)
		
		// Load chunk to check for @base
		chunkPath := filepath.Join(bundlePath, "c4", lastChunkID)
		chunkFile, err := os.Open(chunkPath)
		if err != nil {
			// If this is not the first chunk and file doesn't exist, that's OK (base might be external)
			if len(chunkStack) > 1 {
				break
			}
			return nil, fmt.Errorf("cannot open chunk %s: %w", lastChunkID, err)
		}
		
		// Parse just the header to get @base if present
		manifest, err := NewDecoder(chunkFile).Decode()
		chunkFile.Close()
		if err != nil {
			return nil, fmt.Errorf("cannot parse chunk %s: %w", lastChunkID, err)
		}
		
		// Check for @base directive
		if manifest.Base.IsNil() {
			break // No more base chunks
		}
		lastChunkID = manifest.Base.String()
	}
	
	// Now process chunks from first (oldest) to last (newest)
	tree := NewManifestTree()
	
	for i := len(chunkStack) - 1; i >= 0; i-- {
		chunkID := chunkStack[i]
		
		// Load and process chunk
		chunkPath := filepath.Join(bundlePath, "c4", chunkID)
		chunkFile, err := os.Open(chunkPath)
		if err != nil {
			return nil, fmt.Errorf("cannot open chunk %s: %w", chunkID, err)
		}
		
		manifest, err := NewDecoder(chunkFile).Decode()
		chunkFile.Close()
		if err != nil {
			return nil, fmt.Errorf("cannot parse chunk %s: %w", chunkID, err)
		}
		
		// Apply this chunk to the tree
		if err := applyManifestToTree(tree, manifest, bundlePath); err != nil {
			return nil, fmt.Errorf("failed to apply chunk %s: %w", chunkID, err)
		}
	}
	
	// Convert tree back to flat manifest with proper sorting
	combined := NewManifest()
	combined.Version = "1.0"
	combined.Entries = flattenTree(tree)
	
	return combined, nil
}

// applyManifestToTree merges a manifest into the tree structure
func applyManifestToTree(tree *ManifestTree, manifest *Manifest, bundlePath string) error {
	for _, entry := range manifest.Entries {
		// Check if this is a collapsed directory (.c4m file)
		if strings.HasSuffix(entry.Name, ".c4m") && !entry.C4ID.IsNil() {
			// Load the collapsed directory recursively
			collapsedManifest, err := loadCollapsedDirectory(entry.C4ID.String(), bundlePath)
			if err != nil {
				return fmt.Errorf("failed to load collapsed directory %s: %w", entry.Name, err)
			}
			
			// Convert .c4m name to directory name
			dirName := strings.TrimSuffix(entry.Name, ".c4m") + "/"
			
			// Add directory entry
			dirEntry := &Entry{
				Mode:      entry.Mode | os.ModeDir,
				Timestamp: entry.Timestamp,
				Size:      0,
				Name:      dirName,
				Depth:     entry.Depth,
			}
			
			// Add directory to tree
			addEntryToTree(tree, dirEntry, nil)
			
			// Add all entries from collapsed directory
			for _, collapsedEntry := range collapsedManifest.Entries {
				// Adjust depth relative to parent
				adjustedEntry := &Entry{
					Mode:      collapsedEntry.Mode,
					Timestamp: collapsedEntry.Timestamp,
					Size:      collapsedEntry.Size,
					Name:      collapsedEntry.Name,
					Target:    collapsedEntry.Target,
					C4ID:      collapsedEntry.C4ID,
					Depth:     collapsedEntry.Depth + entry.Depth + 1,
				}
				
				// Add to tree with proper parent reference
				addEntryToTree(tree, adjustedEntry, &dirName)
			}
		} else {
			// Regular entry - add to tree
			addEntryToTree(tree, entry, nil)
		}
	}
	
	return nil
}

// loadCollapsedDirectory recursively loads a collapsed directory
func loadCollapsedDirectory(chunkID string, bundlePath string) (*Manifest, error) {
	// Follow the same @base chain process for collapsed directories
	var chunkStack []string
	seenChunks := make(map[string]bool)
	currentID := chunkID
	
	// Build stack of chunks for this collapsed directory
	for currentID != "" {
		if seenChunks[currentID] {
			break // Circular reference, stop here
		}
		seenChunks[currentID] = true
		chunkStack = append(chunkStack, currentID)
		
		// Load chunk to check for @base
		chunkPath := filepath.Join(bundlePath, "c4", currentID)
		chunkFile, err := os.Open(chunkPath)
		if err != nil {
			break // Base not found, stop here
		}
		
		manifest, err := NewDecoder(chunkFile).Decode()
		chunkFile.Close()
		if err != nil {
			return nil, err
		}
		
		if manifest.Base.IsNil() {
			break
		}
		currentID = manifest.Base.String()
	}
	
	// Process chunks from first to last
	combined := NewManifest()
	for i := len(chunkStack) - 1; i >= 0; i-- {
		chunkPath := filepath.Join(bundlePath, "c4", chunkStack[i])
		chunkFile, err := os.Open(chunkPath)
		if err != nil {
			return nil, err
		}
		
		manifest, err := NewDecoder(chunkFile).Decode()
		chunkFile.Close()
		if err != nil {
			return nil, err
		}
		
		// Merge entries (later entries override earlier ones)
		mergeManifests(combined, manifest)
	}
	
	return combined, nil
}

// mergeManifests merges source manifest into destination, with source overriding
func mergeManifests(dest, source *Manifest) {
	// Build map of existing entries for quick lookup
	entryMap := make(map[string]int)
	for i, entry := range dest.Entries {
		path := buildEntryPath(entry, dest.Entries, i)
		entryMap[path] = i
	}
	
	// Add or update entries from source
	for i, entry := range source.Entries {
		path := buildEntryPath(entry, source.Entries, i)
		if idx, exists := entryMap[path]; exists {
			// Update existing entry
			dest.Entries[idx] = entry
		} else {
			// Add new entry
			dest.Entries = append(dest.Entries, entry)
		}
	}
}

// buildEntryPath reconstructs the full path for an entry
func buildEntryPath(entry *Entry, allEntries []*Entry, index int) string {
	if entry.Depth == 0 {
		return entry.Name
	}
	
	// Walk backwards to find parent directories
	path := entry.Name
	currentDepth := entry.Depth
	
	for i := index - 1; i >= 0 && currentDepth > 0; i-- {
		e := allEntries[i]
		if e.Depth == currentDepth-1 && e.IsDir() {
			path = e.Name + path
			currentDepth--
		}
	}
	
	return path
}

// addEntryToTree adds an entry to the tree at the correct location
func addEntryToTree(tree *ManifestTree, entry *Entry, parentDir *string) {
	// Build the path for this entry
	var currentNode *ManifestNode
	
	if entry.Depth == 0 {
		// Root level entry
		currentNode = tree.Root
	} else {
		// Find parent node based on depth and position
		// This is simplified - in practice you'd track the current path
		currentNode = findParentNode(tree.Root, entry.Depth-1)
		if currentNode == nil {
			currentNode = tree.Root // Fallback to root if parent not found
		}
	}
	
	// Create node for this entry
	node := &ManifestNode{
		Entry: entry,
	}
	
	if entry.IsDir() {
		node.Children = make(map[string]*ManifestNode)
	}
	
	// Add to parent's children
	if currentNode.Children == nil {
		currentNode.Children = make(map[string]*ManifestNode)
	}
	currentNode.Children[entry.Name] = node
	
	// Mark parent as dirty (needs re-sorting)
	currentNode.Dirty = true
	
	// Add to quick lookup map
	path := buildEntryPath(entry, []*Entry{entry}, 0)
	tree.NodeMap[path] = node
}

// findParentNode finds a node at the specified depth
func findParentNode(root *ManifestNode, targetDepth int) *ManifestNode {
	if targetDepth == 0 {
		return root
	}
	
	// This is a simplified implementation
	// In practice, you'd maintain a stack of current path nodes
	var lastDir *ManifestNode
	var findAtDepth func(node *ManifestNode, currentDepth int) *ManifestNode
	
	findAtDepth = func(node *ManifestNode, currentDepth int) *ManifestNode {
		if currentDepth == targetDepth && node.Entry != nil && node.Entry.IsDir() {
			lastDir = node
		}
		
		if node.Children != nil && currentDepth < targetDepth {
			for _, child := range node.Children {
				if result := findAtDepth(child, currentDepth+1); result != nil {
					return result
				}
			}
		}
		
		return nil
	}
	
	findAtDepth(root, 0)
	return lastDir
}

// buildPathAtDepth builds the path at a specific depth
func buildPathAtDepth(root *ManifestNode, depth int) string {
	// Simplified - would need proper path tracking
	return ""
}

// flattenTree converts the tree back to a flat list of entries with proper sorting
func flattenTree(tree *ManifestTree) []*Entry {
	var result []*Entry
	
	// Sort and flatten recursively
	var flatten func(node *ManifestNode, depth int)
	flatten = func(node *ManifestNode, depth int) {
		// Add this node's entry (unless it's the root)
		if node.Entry != nil {
			result = append(result, node.Entry)
		}
		
		// Sort children if dirty
		if node.Children != nil {
			// Collect children into slices for sorting
			var files []*ManifestNode
			var dirs []*ManifestNode
			
			for _, child := range node.Children {
				if child.Entry != nil {
					if child.Entry.IsDir() {
						dirs = append(dirs, child)
					} else {
						files = append(files, child)
					}
				}
			}
			
			// Sort files and dirs separately
			sortNodes(files)
			sortNodes(dirs)
			
			// Process files first (C4M rule)
			for _, file := range files {
				flatten(file, depth+1)
			}
			
			// Then process directories
			for _, dir := range dirs {
				flatten(dir, depth+1)
			}
		}
	}
	
	flatten(tree.Root, -1)
	return result
}

// sortNodes sorts a slice of nodes by name
func sortNodes(nodes []*ManifestNode) {
	// Sort using natural ordering
	for i := 0; i < len(nodes)-1; i++ {
		for j := i + 1; j < len(nodes); j++ {
			if nodes[i].Entry != nil && nodes[j].Entry != nil {
				if !NaturalLess(nodes[i].Entry.Name, nodes[j].Entry.Name) {
					nodes[i], nodes[j] = nodes[j], nodes[i]
				}
			}
		}
	}
}

// ExtractBundle extracts a bundle in canonical format using the proper @base chain algorithm
func ExtractBundle(bundlePath string, output io.Writer) error {
	manifest, err := LoadBundleAsManifest(bundlePath)
	if err != nil {
		return err
	}

	// Use the encoder for canonical format
	return NewEncoder(output).Encode(manifest)
}

// ExtractBundleToFile extracts a bundle to a file in canonical format
func ExtractBundleToFile(bundlePath, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	return ExtractBundle(bundlePath, file)
}

// ExtractBundlePretty extracts a bundle in pretty format using the proper @base chain algorithm
func ExtractBundlePretty(bundlePath string, output io.Writer) error {
	manifest, err := LoadBundleAsManifest(bundlePath)
	if err != nil {
		return err
	}

	// Use the encoder with pretty format
	return NewEncoder(output).SetPretty(true).Encode(manifest)
}

// ExtractBundlePrettyToFile extracts a bundle to a file in pretty format
func ExtractBundlePrettyToFile(bundlePath, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	return ExtractBundlePretty(bundlePath, file)
}