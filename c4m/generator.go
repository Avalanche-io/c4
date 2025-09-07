package c4m

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Avalanche-io/c4"
)

// Generator creates C4M manifests from filesystem paths
type Generator struct {
	computeC4IDs   bool
	followSymlinks bool
	includeHidden  bool
	detectSequences bool
}

// NewGenerator creates a new manifest generator
func NewGenerator() *Generator {
	return &Generator{
		computeC4IDs:    true,
		followSymlinks:  false,
		includeHidden:   false,
		detectSequences: false,
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

// NewGeneratorWithOptions creates a generator with options
func NewGeneratorWithOptions(opts ...GeneratorOption) *Generator {
	g := NewGenerator()
	for _, opt := range opts {
		opt(g)
	}
	return g
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
	
	// Sort entries
	manifest.Sort()
	
	// Detect and group sequences if enabled
	if g.detectSequences {
		g.groupSequences(manifest)
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
			subManifest, err := g.GenerateFromPath(dirPath)
			if err == nil {
				dirEntry.C4ID = subManifest.ComputeC4ID()
			}
		}
		
		manifest.AddEntry(dirEntry)
		// Children of this directory are one level deeper
		childDepth = depth + 1
	}
	
	// Process entries
	for _, entry := range entries {
		name := entry.Name()
		
		// Skip hidden files if not included
		if !g.includeHidden && strings.HasPrefix(name, ".") {
			continue
		}
		
		fullPath := filepath.Join(dirPath, name)
		
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
				
				// Set symlink target
				if bmd, ok := md.(*BasicFileMetadata); ok {
					target, err := os.Readlink(fullPath)
					if err == nil {
						bmd.SetTarget(target)
						
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
		subGen := &Generator{
			computeC4IDs:    g.computeC4IDs,
			followSymlinks:  g.followSymlinks,
			includeHidden:   g.includeHidden,
			detectSequences: g.detectSequences,
		}
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

// groupSequences detects and groups file sequences
func (g *Generator) groupSequences(manifest *Manifest) {
	if len(manifest.Entries) < 2 {
		return
	}
	
	// Group entries by directory depth
	depthGroups := make(map[int][]*Entry)
	for _, entry := range manifest.Entries {
		depthGroups[entry.Depth] = append(depthGroups[entry.Depth], entry)
	}
	
	// Process each depth level
	for depth, entries := range depthGroups {
		grouped := g.groupFileSequences(entries)
		
		// Replace original entries with grouped ones
		// First, remove all entries at this depth
		newEntries := make([]*Entry, 0)
		for _, e := range manifest.Entries {
			if e.Depth != depth {
				newEntries = append(newEntries, e)
			}
		}
		
		// Add the grouped entries
		for _, e := range grouped {
			newEntries = append(newEntries, e)
		}
		
		manifest.Entries = newEntries
	}
	
	// Re-sort after grouping
	manifest.Sort()
}

// groupFileSequences groups a list of entries into sequences
func (g *Generator) groupFileSequences(entries []*Entry) []*Entry {
	if len(entries) < 2 {
		return entries
	}
	
	// Map to track sequences by pattern
	sequenceMap := make(map[string]*sequenceGroup)
	var result []*Entry
	
	for _, entry := range entries {
		// Skip directories (but not symlinks - we can group those)
		if entry.IsDir() {
			result = append(result, entry)
			continue
		}
		
		// Try to extract sequence pattern
		pattern, num := extractSequencePattern(entry.Name)
		if pattern == "" {
			// Not part of a sequence
			result = append(result, entry)
			continue
		}
		
		// For symlinks, also consider the target pattern
		var targetPattern string
		if entry.IsSymlink() && entry.Target != "" {
			targetPattern, _ = extractSequencePattern(entry.Target)
		}
		
		// Create a key that includes both the name pattern and whether it's a symlink
		// This ensures symlinks and regular files are grouped separately
		groupKey := pattern
		if entry.IsSymlink() {
			groupKey = "symlink:" + pattern
		}
		
		// Add to sequence group
		if sg, exists := sequenceMap[groupKey]; exists {
			sg.addEntry(num, entry)
			// Check if symlink targets are uniform
			if entry.IsSymlink() && sg.targetPattern != "" {
				if targetPattern != sg.targetPattern {
					sg.mixedTargets = true
				}
			}
		} else {
			sg := newSequenceGroup(pattern, entry.Depth)
			sg.isSymlink = entry.IsSymlink()
			sg.targetPattern = targetPattern
			sg.addEntry(num, entry)
			sequenceMap[groupKey] = sg
		}
	}
	
	// Convert sequence groups to entries
	for _, sg := range sequenceMap {
		if sg.count() > 1 {
			// Create a sequence entry
			seqEntry := sg.toEntry()
			result = append(result, seqEntry)
		} else {
			// Single file, add as-is
			for _, e := range sg.entries {
				result = append(result, e)
			}
		}
	}
	
	return result
}

type sequenceGroup struct {
	pattern       string
	depth         int
	entries       map[int]*Entry
	minNum        int
	maxNum        int
	padding       int
	isSymlink     bool
	targetPattern string // For symlinks with uniform targets
	mixedTargets  bool   // True if symlink targets don't follow a pattern
}

func newSequenceGroup(pattern string, depth int) *sequenceGroup {
	return &sequenceGroup{
		pattern: pattern,
		depth:   depth,
		entries: make(map[int]*Entry),
		minNum:  999999999,
		maxNum:  -1,
	}
}

func (sg *sequenceGroup) addEntry(num int, entry *Entry) {
	sg.entries[num] = entry
	if num < sg.minNum {
		sg.minNum = num
	}
	if num > sg.maxNum {
		sg.maxNum = num
	}
	// Track padding from the first entry
	if sg.padding == 0 && entry.Name != "" {
		if idx := strings.LastIndex(entry.Name, fmt.Sprintf("%d", num)); idx >= 0 {
			// Count leading zeros
			for i := idx - 1; i >= 0 && entry.Name[i] == '0'; i-- {
				sg.padding++
			}
			sg.padding++ // Add one for the digit itself
		}
	}
}

func (sg *sequenceGroup) count() int {
	return len(sg.entries)
}

func (sg *sequenceGroup) toEntry() *Entry {
	// Extract the base pattern without NUM placeholder
	dotIdx := strings.LastIndex(sg.pattern, ".")
	prefix := sg.pattern[:strings.LastIndex(sg.pattern[:dotIdx], "NUM")]
	suffix := sg.pattern[dotIdx:]
	
	// Create a sequence pattern entry
	seqName := fmt.Sprintf("%s[%0*d-%0*d]%s", 
		prefix,
		sg.padding, sg.minNum,
		sg.padding, sg.maxNum,
		suffix)
	
	// Use the first entry as template
	var firstEntry *Entry
	for _, e := range sg.entries {
		firstEntry = e
		break
	}
	
	entry := &Entry{
		Mode:       firstEntry.Mode,
		Timestamp:  firstEntry.Timestamp,
		Size:       firstEntry.Size * int64(len(sg.entries)), // Total size
		Name:       seqName,
		Depth:      sg.depth,
		IsSequence: true,
		Pattern:    seqName,
	}
	
	// Handle symlink sequences
	if sg.isSymlink {
		if sg.mixedTargets || sg.targetPattern == "" {
			// Mixed or no pattern - use "..."
			entry.Target = "..."
		} else {
			// Uniform pattern - create target range
			targetDotIdx := strings.LastIndex(sg.targetPattern, ".")
			if targetDotIdx > 0 {
				targetPrefix := sg.targetPattern[:strings.LastIndex(sg.targetPattern[:targetDotIdx], "NUM")]
				targetSuffix := sg.targetPattern[targetDotIdx:]
				entry.Target = fmt.Sprintf("%s[%0*d-%0*d]%s",
					targetPrefix,
					sg.padding, sg.minNum,
					sg.padding, sg.maxNum,
					targetSuffix)
			} else {
				entry.Target = "..."
			}
		}
		
		// For symlink sequences, we don't compute a single C4 ID
		// The expansion would show individual C4 IDs
		entry.C4ID = c4.ID{}
	}
	
	return entry
}

// extractSequencePattern extracts the pattern and frame number from a filename
func extractSequencePattern(name string) (string, int) {
	// Look for numeric sequences in the filename
	re := regexp.MustCompile(`^(.+?)(\d+)(\.\w+)$`)
	matches := re.FindStringSubmatch(name)
	if len(matches) != 4 {
		return "", 0
	}
	
	prefix := matches[1]
	numStr := matches[2]
	suffix := matches[3]
	
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return "", 0
	}
	
	// Return pattern with placeholder for number
	pattern := prefix + "NUM" + suffix
	return pattern, num
}

// GenerateFromReader creates a manifest by parsing an existing C4M
func GenerateFromReader(r io.Reader) (*Manifest, error) {
	parser := NewParser(r)
	return parser.ParseAll()
}