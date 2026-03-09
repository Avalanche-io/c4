package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/gitignore"
)

// Generator creates C4M manifests from filesystem paths
type Generator struct {
	computeC4IDs     bool
	followSymlinks   bool
	includeHidden    bool
	detectSequences  bool
	respectGitignore bool
	excludePatterns  []string
	gi               *gitignore.Matcher
	scanRoot         string
}

// NewGenerator creates a new manifest generator
func NewGenerator() *Generator {
	return &Generator{
		computeC4IDs:     true,
		followSymlinks:   false,
		includeHidden:    false,
		detectSequences:  false,
		respectGitignore: false,
	}
}

// GeneratorOption configures a Generator
type GeneratorOption func(*Generator)

// WithC4IDs enables/disables C4 ID computation
func WithC4IDs(compute bool) GeneratorOption {
	return func(g *Generator) {
		g.computeC4IDs = compute
	}
}

// WithSymlinks enables/disables following symlinks
func WithSymlinks(follow bool) GeneratorOption {
	return func(g *Generator) {
		g.followSymlinks = follow
	}
}

// WithHidden enables/disables including hidden files
func WithHidden(include bool) GeneratorOption {
	return func(g *Generator) {
		g.includeHidden = include
	}
}

// WithSequenceDetection enables/disables sequence detection
func WithSequenceDetection(detect bool) GeneratorOption {
	return func(g *Generator) {
		g.detectSequences = detect
	}
}

// WithGitignore enables/disables respecting .gitignore files
func WithGitignore(enable bool) GeneratorOption {
	return func(g *Generator) {
		g.respectGitignore = enable
	}
}

// WithExclude adds glob patterns to exclude from scanning
func WithExclude(patterns []string) GeneratorOption {
	return func(g *Generator) {
		g.excludePatterns = append(g.excludePatterns, patterns...)
	}
}

// NewGeneratorWithOptions creates a generator with options
func NewGeneratorWithOptions(opts ...GeneratorOption) *Generator {
	g := NewGenerator()
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// clone creates a copy with the same settings but fresh state.
func (g *Generator) clone() *Generator {
	clone := &Generator{
		computeC4IDs:     g.computeC4IDs,
		followSymlinks:   g.followSymlinks,
		includeHidden:    g.includeHidden,
		detectSequences:  g.detectSequences,
		respectGitignore: g.respectGitignore,
	}
	if len(g.excludePatterns) > 0 {
		clone.excludePatterns = make([]string, len(g.excludePatterns))
		copy(clone.excludePatterns, g.excludePatterns)
	}
	return clone
}

// GenerateFromPath creates a manifest from a filesystem path
func (g *Generator) GenerateFromPath(path string) (*Manifest, error) {
	manifest := NewManifest()
	
	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}
	
	// Check if path exists
	info, err := os.Lstat(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}
	
	// Initialize gitignore matcher for this scan
	g.scanRoot = absPath
	if g.respectGitignore && info.IsDir() {
		g.gi = gitignore.New()
		g.gi.AddFromFile(filepath.Join(absPath, ".gitignore"), 0)
	}

	// Generate entries recursively
	if info.IsDir() {
		err = g.generateDir(manifest, absPath, "", 0)
	} else {
		entry, err := g.generateEntry(absPath, info, 0)
		if err != nil {
			return nil, err
		}
		manifest.AddEntry(entry)
	}
	
	if err != nil {
		return nil, err
	}
	
	// Sort entries hierarchically (files before directories at each level)
	manifest.SortEntries()

	// Compute directory sizes from children (OS-reported dir sizes are platform-dependent)
	PropagateMetadata(manifest.Entries)

	// Detect and collapse file sequences if enabled
	if g.detectSequences {
		collapsed := c4m.DetectSequences(manifest)
		manifest.Entries = collapsed.Entries
		manifest.DataBlocks = collapsed.DataBlocks
	}
	
	return manifest, nil
}

// generateDir recursively generates entries for a directory
func (g *Generator) generateDir(manifest *Manifest, dirPath, dirName string, depth int) error {
	// Read directory
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}
	
	// Determine the depth for children
	childDepth := depth
	
	// Add directory entry itself if not root
	if dirName != "" {
		dirInfo, err := os.Lstat(dirPath)
		if err != nil {
			return err
		}
		dirEntry, err := g.generateEntry(dirPath, dirInfo, depth)
		if err != nil {
			return err
		}
		dirEntry.Name = dirName + "/"
		
		// For directories, compute C4 ID from their recursive manifest
		if g.computeC4IDs && dirInfo.IsDir() {
			subGen := g.clone()
			subManifest, err := subGen.GenerateFromPath(dirPath)
			if err == nil {
				dirEntry.C4ID = subManifest.ComputeC4ID()
			}
		}
		
		manifest.AddEntry(dirEntry)
		// Children of this directory are one level deeper
		childDepth = depth + 1
	}
	
	// Load gitignore rules for this directory (if not root, which was loaded in GenerateFromPath)
	if g.gi != nil && dirName != "" {
		depth := depthFromRoot(g.scanRoot, dirPath)
		g.gi.AddFromFile(filepath.Join(dirPath, ".gitignore"), depth)
	}

	// Process entries
	for _, entry := range entries {
		name := entry.Name()

		// Always skip .git directory
		if name == ".git" {
			continue
		}

		// Skip hidden files if not included
		if !g.includeHidden && strings.HasPrefix(name, ".") {
			continue
		}

		fullPath := filepath.Join(dirPath, name)

		// Check gitignore
		if g.gi != nil {
			relPath := relFromRoot(g.scanRoot, fullPath)
			if g.gi.Match(relPath, entry.IsDir()) {
				continue
			}
		}

		// Check exclude patterns
		if len(g.excludePatterns) > 0 {
			relPath := relFromRoot(g.scanRoot, fullPath)
			if g.matchExclude(relPath, name, entry.IsDir()) {
				continue
			}
		}

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("failed to get info for %s: %w", fullPath, err)
		}
		
		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			if g.followSymlinks {
				// Follow the symlink
				targetInfo, err := os.Stat(fullPath)
				if err == nil {
					info = targetInfo
				}
			} else {
				// Create symlink metadata
				md := g.generateMetadata(fullPath, info, childDepth)
				
				// Set symlink target (always forward slashes for c4m portability)
				if bmd, ok := md.(*BasicFileMetadata); ok {
					target, err := os.Readlink(fullPath)
					if err == nil {
						bmd.SetTarget(filepath.ToSlash(target))
						
						// Compute C4 ID of symlink target if enabled
						if g.computeC4IDs {
							id := g.computeSymlinkTargetC4ID(fullPath, target)
							bmd.SetID(id)
						}
					}
				}
				
				// Convert to entry and add
				fileEntry := MetadataToEntry(md)
				fileEntry.Name = name
				manifest.AddEntry(fileEntry)
				continue
			}
		}
		
		// Recurse into directories
		if info.IsDir() {
			err = g.generateDir(manifest, fullPath, name, childDepth)
			if err != nil {
				return err
			}
		} else {
			// Add file entry
			fileEntry, err := g.generateEntry(fullPath, info, childDepth)
			if err != nil {
				return err
			}
			fileEntry.Name = name
			manifest.AddEntry(fileEntry)
		}
	}
	
	return nil
}

// generateEntry creates an entry from file info
func (g *Generator) generateEntry(path string, info os.FileInfo, depth int) (*Entry, error) {
	// Create metadata first
	md := g.generateMetadata(path, info, depth)
	
	// Convert to Entry
	entry := MetadataToEntry(md)
	// MetadataToEntry adds trailing slash for directories, but we handle that elsewhere
	if entry.IsDir() && strings.HasSuffix(entry.Name, "/") {
		entry.Name = entry.Name[:len(entry.Name)-1]
	}
	
	return entry, nil
}

// generateMetadata creates metadata from file info
func (g *Generator) generateMetadata(path string, info os.FileInfo, depth int) FileMetadata {
	md := NewFileMetadata(path, info, depth)
	
	// Compute C4 ID if enabled and it's a regular file
	if g.computeC4IDs && info.Mode().IsRegular() {
		id, err := g.computeFileC4ID(path)
		if err == nil {
			md.SetID(id)
		}
	}
	
	return md
}

// computeFileC4ID computes the C4 ID for a file
func (g *Generator) computeFileC4ID(path string) (c4.ID, error) {
	file, err := os.Open(path)
	if err != nil {
		return c4.ID{}, err
	}
	defer file.Close()
	
	return c4.Identify(file), nil
}

// computeSymlinkTargetC4ID computes the C4 ID for a symlink's target
func (g *Generator) computeSymlinkTargetC4ID(symlinkPath, target string) c4.ID {
	// Resolve target path (handle relative paths)
	targetPath := target
	if !filepath.IsAbs(target) {
		targetPath = filepath.Join(filepath.Dir(symlinkPath), target)
	}
	
	// Get target info
	targetInfo, err := os.Lstat(targetPath)
	if err != nil {
		// Broken symlink or out-of-scope target
		return c4.ID{}
	}
	
	// If target is also a symlink, return empty ID (prevent infinite recursion)
	if targetInfo.Mode()&os.ModeSymlink != 0 {
		return c4.ID{}
	}
	
	// If target is a directory, compute its manifest C4 ID
	if targetInfo.IsDir() {
		subGen := g.clone()
		manifest, err := subGen.GenerateFromPath(targetPath)
		if err != nil {
			return c4.ID{}
		}
		return manifest.ComputeC4ID()
	}
	
	// For regular files, compute content C4 ID
	if targetInfo.Mode().IsRegular() {
		id, err := g.computeFileC4ID(targetPath)
		if err != nil {
			return c4.ID{}
		}
		return id
	}
	
	// Other types (devices, pipes, etc.) get empty ID
	return c4.ID{}
}

// matchExclude checks if a path matches any exclude pattern.
// Patterns are matched against both the basename and the relative path from scan root.
func (g *Generator) matchExclude(relPath, name string, isDir bool) bool {
	for _, pattern := range g.excludePatterns {
		// Match against basename
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
		// Match against relative path
		if matched, _ := filepath.Match(pattern, relPath); matched {
			return true
		}
	}
	return false
}

func relFromRoot(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(rel)
}

func depthFromRoot(root, path string) int {
	rel := relFromRoot(root, path)
	if rel == "." {
		return 0
	}
	return strings.Count(rel, "/") + 1
}


