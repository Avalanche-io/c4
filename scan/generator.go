package scan

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// ScanMode controls how much information is gathered during a scan.
type ScanMode int

const (
	ModeStructure ScanMode = iota // names and hierarchy only
	ModeMetadata                  // structure + permissions, timestamps, sizes
	ModeFull                      // structure + metadata + C4 IDs
)

// ParseScanMode parses a mode string: "s"/"1" → structure, "m"/"2" → metadata, "f"/"3" → full.
func ParseScanMode(s string) (ScanMode, error) {
	switch strings.ToLower(s) {
	case "s", "1", "structure":
		return ModeStructure, nil
	case "m", "2", "metadata":
		return ModeMetadata, nil
	case "f", "3", "full", "":
		return ModeFull, nil
	default:
		return ModeFull, fmt.Errorf("unknown scan mode %q (use s/m/f or 1/2/3)", s)
	}
}

// Generator creates C4M manifests from filesystem paths
type Generator struct {
	mode            ScanMode
	followSymlinks  bool
	includeHidden   bool
	detectSequences bool
	excludePatterns []string
	excludeFile     string // explicit exclude file path
	excludeFileName string // filename to look for in scanned dirs (from env)
	guide           map[string]bool // paths from guide c4m (nil = no guide)
	scanRoot        string
}

// NewGenerator creates a new manifest generator
func NewGenerator() *Generator {
	return &Generator{
		mode:            ModeFull,
		followSymlinks:  false,
		includeHidden:   false,
		detectSequences: false,
		excludeFileName: os.Getenv("C4_EXCLUDE_FILE"),
	}
}

// GeneratorOption configures a Generator
type GeneratorOption func(*Generator)

// WithC4IDs enables/disables C4 ID computation (shorthand for WithMode).
func WithC4IDs(compute bool) GeneratorOption {
	return func(g *Generator) {
		if compute {
			g.mode = ModeFull
		} else {
			g.mode = ModeMetadata
		}
	}
}

// WithMode sets the scan mode (structure, metadata, or full).
func WithMode(mode ScanMode) GeneratorOption {
	return func(g *Generator) {
		g.mode = mode
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

// WithExclude adds glob patterns to exclude from scanning.
func WithExclude(patterns []string) GeneratorOption {
	return func(g *Generator) {
		g.excludePatterns = append(g.excludePatterns, patterns...)
	}
}

// WithExcludeFile sets an explicit exclude file to load patterns from.
func WithExcludeFile(path string) GeneratorOption {
	return func(g *Generator) {
		g.excludeFile = path
	}
}

// WithGuide sets an existing manifest as a guide. Only entries present
// in the guide will be included in the scan. This enables the
// scan-filter-continue workflow.
func WithGuide(m *Manifest) GeneratorOption {
	return func(g *Generator) {
		g.guide = buildGuideSet(m)
	}
}

// buildGuideSet extracts all paths from a manifest into a lookup set.
func buildGuideSet(m *Manifest) map[string]bool {
	set := make(map[string]bool)
	var dirStack []string
	for _, entry := range m.Entries {
		if entry.Depth < len(dirStack) {
			dirStack = dirStack[:entry.Depth]
		}
		var fullPath string
		if entry.Depth > 0 && entry.Depth <= len(dirStack) {
			prefix := ""
			for i := 0; i < entry.Depth; i++ {
				prefix += dirStack[i]
			}
			fullPath = prefix + entry.Name
		} else {
			fullPath = entry.Name
		}
		set[fullPath] = true
		if entry.IsDir() {
			for len(dirStack) <= entry.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[entry.Depth] = entry.Name
		}
	}
	return set
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
		mode:            g.mode,
		followSymlinks:  g.followSymlinks,
		includeHidden:   g.includeHidden,
		detectSequences: g.detectSequences,
		excludeFile:     g.excludeFile,
		excludeFileName: g.excludeFileName,
		guide:           g.guide,
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
	
	g.scanRoot = absPath

	// Load exclude patterns from file if specified.
	if g.excludeFile != "" {
		g.loadExcludeFile(g.excludeFile)
	}
	// Load exclude patterns from env-named file in scanned directory.
	if g.excludeFileName != "" && info.IsDir() {
		g.loadExcludeFile(filepath.Join(absPath, g.excludeFileName))
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
		if g.mode == ModeFull && dirInfo.IsDir() {
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
	
	// Load exclude patterns from env-named file in subdirectories.
	if g.excludeFileName != "" && dirName != "" {
		g.loadExcludeFile(filepath.Join(dirPath, g.excludeFileName))
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

		// Check exclude patterns
		if len(g.excludePatterns) > 0 {
			relPath := relFromRoot(g.scanRoot, fullPath)
			if g.matchExclude(relPath, name, entry.IsDir()) {
				continue
			}
		}

		// Check guide — skip entries not in the guide manifest.
		if g.guide != nil {
			relPath := relFromRoot(g.scanRoot, fullPath)
			guideName := relPath
			if entry.IsDir() {
				guideName += "/"
			}
			if !g.guide[guideName] {
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
						if g.mode == ModeFull {
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
	if g.mode == ModeStructure {
		// Structure only: just name and directory status.
		return NewStructureMetadata(path, info, depth)
	}

	md := NewFileMetadata(path, info, depth)

	// Compute C4 ID if in full mode and it's a regular file.
	if g.mode == ModeFull && info.Mode().IsRegular() {
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

// loadExcludeFile reads glob patterns from a file (one per line, # comments, blank lines skipped).
func (g *Generator) loadExcludeFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // file not found is not an error
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		g.excludePatterns = append(g.excludePatterns, line)
	}
}

// Dir scans a directory and returns a c4m manifest. This is the primary
// entry point for the scan package.
//
//	m, err := scan.Dir("/path/to/dir")
//	m, err := scan.Dir("/path", scan.WithMode(scan.ModeMetadata))
func Dir(path string, opts ...GeneratorOption) (*c4m.Manifest, error) {
	return NewGeneratorWithOptions(opts...).GenerateFromPath(path)
}

