package c4m

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// sortFileInfos sorts FileInfo entries using natural sort
func sortFileInfos(infos []os.FileInfo) {
	sort.Slice(infos, func(i, j int) bool {
		// Files before directories
		iIsDir := infos[i].IsDir()
		jIsDir := infos[j].IsDir()
		if iIsDir != jIsDir {
			return !iIsDir // files first
		}
		return NaturalLess(infos[i].Name(), infos[j].Name())
	})
}

// StreamingWriter outputs C4M manifests progressively with adaptive column support
type StreamingWriter struct {
	writer         io.Writer
	adapter        *ColumnAdapter
	prettyPrint    bool
	indentWidth    int
	maxSize        int64
	currentDepth   int
	lastC4Column   int
	entriesWritten int64
	
	mu sync.Mutex
}

// NewStreamingWriter creates a new streaming manifest writer
func NewStreamingWriter(w io.Writer, prettyPrint bool, initialDelay time.Duration) *StreamingWriter {
	var adapter *ColumnAdapter
	if prettyPrint {
		adapter = NewColumnAdapter(initialDelay)
		adapter.Start()
	}
	
	return &StreamingWriter{
		writer:       w,
		adapter:      adapter,
		prettyPrint:  prettyPrint,
		indentWidth:  2,
		lastC4Column: 80,
	}
}

// WriteHeader writes the manifest header
func (sw *StreamingWriter) WriteHeader(version string) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	
	_, err := fmt.Fprintf(sw.writer, "@c4m %s\n", version)
	return err
}

// WriteEntry writes a single entry with adaptive column support
func (sw *StreamingWriter) WriteEntry(entry *Entry) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	
	// Track max size for pretty printing
	if entry.Size > sw.maxSize {
		sw.maxSize = entry.Size
	}
	
	// Submit entry for column calculation if using adaptive columns
	if sw.adapter != nil {
		sw.adapter.ScanEntry(entry)
	}
	
	// Format the entry
	var line string
	if sw.prettyPrint {
		line = sw.formatEntryStreaming(entry)
	} else {
		line = entry.Format(sw.indentWidth, false)
	}
	
	// Write the line
	_, err := fmt.Fprintf(sw.writer, "%s\n", line)
	sw.entriesWritten++
	
	return err
}

// formatEntryStreaming formats an entry for streaming output with adaptive columns
func (sw *StreamingWriter) formatEntryStreaming(entry *Entry) string {
	// Check if we're entering a new directory level
	if entry.Depth != sw.currentDepth {
		sw.currentDepth = entry.Depth
		// When entering a new directory, we can update the column if needed
		if sw.adapter != nil {
			newColumn := sw.adapter.GetColumn()
			if newColumn > sw.lastC4Column {
				sw.lastC4Column = newColumn
			}
		}
	}
	
	// Build the line components
	indent := strings.Repeat(" ", entry.Depth*sw.indentWidth)
	
	// Format mode
	var modeStr string
	if entry.Mode == 0 && !entry.IsDir() && !entry.IsSymlink() {
		modeStr = "----------"
	} else {
		modeStr = formatMode(entry.Mode)
	}
	
	// Format timestamp
	var timeStr string
	if entry.Timestamp.Unix() == 0 {
		timeStr = "-                        " // Padded null timestamp
	} else {
		timeStr = formatTimestampPretty(entry.Timestamp)
	}
	
	// Format size with padding
	var sizeStr string
	if entry.Size < 0 {
		// Null size - pad appropriately
		maxSizeStr := formatSizeWithCommas(sw.maxSize)
		padding := len(maxSizeStr) - 1
		sizeStr = strings.Repeat(" ", padding) + "-"
	} else {
		sizeStr = formatSizePretty(entry.Size, sw.maxSize)
	}
	
	// Format name
	nameStr := formatName(entry.Name)
	
	// Build base line
	parts := []string{indent + modeStr, timeStr, sizeStr, nameStr}
	
	// Add symlink target
	if entry.Target != "" {
		parts = append(parts, "->", entry.Target)
	}
	
	baseLine := strings.Join(parts, " ")
	
	// Calculate actual line length for potential column update
	lineLength := len(baseLine)
	
	// Check if this line would exceed current column
	if sw.adapter != nil && lineLength+sw.adapter.minSpacing > sw.lastC4Column {
		// Force column update for this and future lines
		sw.adapter.ForceUpdateColumn(lineLength)
		sw.lastC4Column = sw.adapter.GetColumn()
	}
	
	// Add C4 ID with alignment if present
	if !entry.C4ID.IsNil() {
		padding := sw.lastC4Column - lineLength
		if padding < 10 {
			padding = 10
		}
		return baseLine + strings.Repeat(" ", padding) + entry.C4ID.String()
	}
	
	return baseLine
}

// Close finalizes the streaming writer
func (sw *StreamingWriter) Close() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	
	if sw.adapter != nil {
		sw.adapter.Stop()
	}
	
	// Flush if writer supports it
	if flusher, ok := sw.writer.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}
	
	return nil
}

// StreamingGenerator generates manifests with streaming output
type StreamingGenerator struct {
	*Generator
	writer      *StreamingWriter
	manifest    *Manifest
	outputDelay time.Duration
}

// NewStreamingGenerator creates a generator that outputs progressively
func NewStreamingGenerator(w io.Writer, prettyPrint bool, outputDelay time.Duration) *StreamingGenerator {
	return &StreamingGenerator{
		Generator:   NewGenerator(),
		writer:      NewStreamingWriter(w, prettyPrint, outputDelay),
		outputDelay: outputDelay,
	}
}

// GenerateFromPathStreaming generates and streams a manifest
func (sg *StreamingGenerator) GenerateFromPathStreaming(path string) error {
	// Initialize manifest
	sg.manifest = NewManifest()
	
	// Write header
	if err := sg.writer.WriteHeader(sg.manifest.Version); err != nil {
		return err
	}
	
	// Start recursive generation with streaming
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	
	// Generate root entry
	if info.IsDir() {
		return sg.generateDirStreaming(path, "", 0)
	} else {
		// Single file
		entry, err := sg.generateEntry(path, info, 0)
		if err != nil {
			return err
		}
		return sg.writer.WriteEntry(entry)
	}
}

// generateDirStreaming generates directory entries with streaming output
func (sg *StreamingGenerator) generateDirStreaming(dirPath, dirName string, depth int) error {
	// Read directory contents
	dir, err := os.Open(dirPath)
	if err != nil {
		return err
	}
	defer dir.Close()
	
	entries, err := dir.Readdir(-1)
	if err != nil {
		return err
	}
	
	// Filter hidden files if needed
	if !sg.includeHidden {
		filtered := entries[:0]
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), ".") {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}
	
	// Sort entries using natural sort
	sortFileInfos(entries)
	
	// Calculate the depth for children
	childDepth := depth
	if dirName != "" {
		// Add directory entry itself
		dirInfo, err := os.Lstat(dirPath)
		if err != nil {
			return err
		}
		
		dirEntry, err := sg.generateEntry(dirPath, dirInfo, depth)
		if err != nil {
			return err
		}
		dirEntry.Name = dirName + "/"
		
		// For directories, compute C4 ID from their contents
		if sg.computeC4IDs {
			// This requires generating the full subtree first
			// For streaming, we might need to defer this or compute progressively
			subManifest := NewManifest()
			tempGen := &Generator{
				computeC4IDs:    sg.computeC4IDs,
				followSymlinks:  sg.followSymlinks,
				includeHidden:   sg.includeHidden,
				detectSequences: sg.detectSequences,
			}
			if err := tempGen.generateDir(subManifest, dirPath, "", 0); err == nil {
				dirEntry.C4ID = subManifest.ComputeC4ID()
			}
		}
		
		// Stream the directory entry
		if err := sg.writer.WriteEntry(dirEntry); err != nil {
			return err
		}
		
		childDepth = depth + 1
	}
	
	// Process entries
	for _, info := range entries {
		name := info.Name()
		fullPath := dirPath + "/" + name
		
		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			linkEntry, err := sg.generateEntry(fullPath, info, childDepth)
			if err != nil {
				return err
			}
			linkEntry.Name = name
			
			// Get symlink target
			if target, err := os.Readlink(fullPath); err == nil {
				linkEntry.Target = target
				if sg.computeC4IDs {
					linkEntry.C4ID = sg.computeSymlinkTargetC4ID(fullPath, target)
				}
			}
			
			// Stream the entry
			if err := sg.writer.WriteEntry(linkEntry); err != nil {
				return err
			}
			continue
		}
		
		// Process directories and files
		if info.IsDir() {
			err = sg.generateDirStreaming(fullPath, name, childDepth)
			if err != nil {
				return err
			}
		} else {
			// Add file entry
			fileEntry, err := sg.generateEntry(fullPath, info, childDepth)
			if err != nil {
				return err
			}
			fileEntry.Name = name
			
			// Stream the entry
			if err := sg.writer.WriteEntry(fileEntry); err != nil {
				return err
			}
		}
	}
	
	return nil
}

// Close finalizes the streaming generator
func (sg *StreamingGenerator) Close() error {
	return sg.writer.Close()
}