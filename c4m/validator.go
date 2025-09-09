package c4m

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ValidationError represents a validation error with context
type ValidationError struct {
	Line    int
	Column  int
	Field   string
	Message string
	Fatal   bool // If true, validation cannot continue
}

func (e ValidationError) Error() string {
	if e.Line > 0 {
		if e.Column > 0 {
			return fmt.Sprintf("line %d, col %d: %s", e.Line, e.Column, e.Message)
		}
		return fmt.Sprintf("line %d: %s", e.Line, e.Message)
	}
	return e.Message
}

// ValidationStats tracks statistics about the validated content
type ValidationStats struct {
	TotalEntries    int64
	Files           int64
	Directories     int64
	Symlinks        int64
	SpecialFiles    int64 // block/char devices, pipes, sockets
	TotalSize       int64
	OldestTime      time.Time
	NewestTime      time.Time
	NullTimes       int64 // Entries with null timestamps
	NullSizes       int64 // Entries with null sizes
	Layers          int64 // Number of @layer directives
	Chunks          int64 // Number of referenced chunks
	MaxDepth        int   // Maximum directory depth
	ChunkedManifests int64 // Number of .c4m chunks found
	CollapsedDirs   []string // Names of collapsed directories
}

// Validator validates C4M manifests and bundles
type Validator struct {
	Strict       bool // Enforce all rules strictly
	MaxErrors    int  // Stop after this many errors (0 = unlimited)
	errors       []ValidationError
	warnings     []ValidationError
	lineNum      int
	seenPaths    map[string]int // Track duplicate paths
	lastDepth    int
	depthStack   []string // Track parent directories
	seenDirAtDepth map[int]bool // Track if we've seen a directory at each depth
	inLayer      bool // Whether we're in a @layer section
	stats        ValidationStats
}

// NewValidator creates a new validator
func NewValidator(strict bool) *Validator {
	return &Validator{
		Strict:    strict,
		MaxErrors: 100,
		seenPaths: make(map[string]int),
		depthStack: []string{},
	}
}

// ValidateManifest validates a C4M manifest from a reader
func (v *Validator) ValidateManifest(r io.Reader) error {
	v.errors = nil
	v.warnings = nil
	v.lineNum = 0
	v.seenPaths = make(map[string]int)
	v.lastDepth = -1
	v.depthStack = []string{}
	v.seenDirAtDepth = make(map[int]bool)
	v.stats = ValidationStats{}
	
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB max line
	
	// Check first line for version
	if !scanner.Scan() {
		v.addError(0, 0, "header", "missing @c4m version header", true)
		return v.getResult()
	}
	
	v.lineNum = 1
	firstLine := scanner.Text()
	if !v.validateHeader(firstLine) {
		return v.getResult()
	}
	
	// Process entries
	for scanner.Scan() {
		v.lineNum++
		line := scanner.Text()
		
		// Skip empty lines
		if line == "" {
			continue
		}
		
		// Handle directives
		if strings.HasPrefix(line, "@") {
			v.handleDirective(line)
			continue
		}
		
		v.validateEntry(line)
		
		if v.MaxErrors > 0 && len(v.errors) >= v.MaxErrors {
			v.addError(v.lineNum, 0, "", fmt.Sprintf("stopping after %d errors", v.MaxErrors), true)
			break
		}
	}
	
	if err := scanner.Err(); err != nil {
		v.addError(v.lineNum, 0, "", fmt.Sprintf("scan error: %v", err), true)
	}
	
	return v.getResult()
}

// ValidateBundle validates a C4M bundle directory
func (v *Validator) ValidateBundle(bundlePath string) error {
	// Reset statistics for bundle validation
	v.errors = nil
	v.warnings = nil
	v.stats = ValidationStats{}
	
	// Check if bundle directory exists
	info, err := os.Stat(bundlePath)
	if err != nil {
		return fmt.Errorf("cannot access bundle: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("bundle path is not a directory")
	}
	
	// Check for header.c4 file
	headerPath := filepath.Join(bundlePath, "header.c4")
	headerData, err := os.ReadFile(headerPath)
	if err != nil {
		v.addError(0, 0, "bundle", "missing or unreadable header.c4", true)
		return v.getResult()
	}
	
	// Parse header to get manifest C4 ID
	headerID := strings.TrimSpace(string(headerData))
	if !strings.HasPrefix(headerID, "c4") {
		v.addError(0, 0, "header", "invalid C4 ID in header.c4", true)
		return v.getResult()
	}
	
	// Read the header manifest from c4 directory
	manifestPath := filepath.Join(bundlePath, "c4", headerID)
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		v.addError(0, 0, "bundle", fmt.Sprintf("cannot read header manifest %s: %v", headerID, err), true)
		return v.getResult()
	}
	
	// Parse header manifest to find all chunks
	chunkIDs := v.extractChunkIDs(string(manifestData))
	
	// Create an aggregated stats tracker
	aggregatedStats := ValidationStats{}
	
	// Validate header manifest
	headerReader := strings.NewReader(string(manifestData))
	if err := v.ValidateManifest(headerReader); err != nil {
		v.addError(0, 0, "header", "invalid header manifest format", false)
	}
	// Add header stats to aggregated
	aggregatedStats = v.addStats(aggregatedStats, v.stats)
	
	// Validate each chunk
	aggregatedStats.ChunkedManifests = int64(len(chunkIDs))
	for i, chunkID := range chunkIDs {
		chunkPath := filepath.Join(bundlePath, "c4", chunkID)
		chunkData, err := os.ReadFile(chunkPath)
		if err != nil {
			v.addError(0, 0, "chunk", fmt.Sprintf("cannot read chunk %d (%s): %v", i+1, chunkID, err), false)
			continue
		}
		
		// Create a new validator for each chunk to avoid state contamination
		chunkValidator := NewValidator(v.Strict)
		chunkValidator.MaxErrors = 0 // Don't limit errors in chunks
		
		// Validate chunk manifest
		chunkReader := strings.NewReader(string(chunkData))
		if err := chunkValidator.ValidateManifest(chunkReader); err != nil {
			// Don't report individual chunk validation errors as they can be numerous
			// Just note that the chunk had issues
			v.addWarning(0, 0, "chunk", fmt.Sprintf("chunk %d has %d validation issues", i+1, len(chunkValidator.GetErrors())))
		}
		
		// Add chunk stats to aggregated
		aggregatedStats = v.addStats(aggregatedStats, chunkValidator.stats)
	}
	
	// Set final aggregated stats
	v.stats = aggregatedStats
	
	return v.getResult()
}

// extractChunkIDs finds all C4 IDs referenced in .c4m files within a manifest
func (v *Validator) extractChunkIDs(manifestContent string) []string {
	var chunkIDs []string
	lines := strings.Split(manifestContent, "\n")
	
	for _, line := range lines {
		// Skip empty lines and directives
		if line == "" || strings.HasPrefix(line, "@") {
			continue
		}
		
		// Look for .c4m files and extract directory names
		if strings.Contains(line, ".c4m ") {
			// Extract the directory name if this is in a progress/ subdirectory
			if strings.Contains(line, "progress/") {
				// Find the parent directory name
				trimmed := strings.TrimSpace(line)
				if idx := strings.LastIndex(trimmed, "progress/"); idx > 0 {
					// Look backwards to find the parent directory
					for i := idx-1; i >= 0; i-- {
						if trimmed[i] == '/' || trimmed[i] == ' ' {
							parentName := trimmed[i+1:idx]
							if parentName != "" && !contains(v.stats.CollapsedDirs, parentName) {
								v.stats.CollapsedDirs = append(v.stats.CollapsedDirs, parentName)
							}
							break
						}
					}
				}
			}
			
			// Extract the C4 ID at the end of the line
			fields := strings.Fields(line)
			for _, field := range fields {
				if strings.HasPrefix(field, "c4") && len(field) > 10 {
					chunkIDs = append(chunkIDs, field)
					break
				}
			}
		}
	}
	
	return chunkIDs
}

// contains checks if a string slice contains a value
func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// addStats combines two ValidationStats structures
func (v *Validator) addStats(a, b ValidationStats) ValidationStats {
	result := ValidationStats{
		TotalEntries:     a.TotalEntries + b.TotalEntries,
		Files:            a.Files + b.Files,
		Directories:      a.Directories + b.Directories,
		Symlinks:         a.Symlinks + b.Symlinks,
		SpecialFiles:     a.SpecialFiles + b.SpecialFiles,
		TotalSize:        a.TotalSize + b.TotalSize,
		NullTimes:        a.NullTimes + b.NullTimes,
		NullSizes:        a.NullSizes + b.NullSizes,
		Layers:           a.Layers + b.Layers,
		Chunks:           a.Chunks + b.Chunks,
		MaxDepth:         a.MaxDepth,
		ChunkedManifests: a.ChunkedManifests + b.ChunkedManifests,
		CollapsedDirs:    append(a.CollapsedDirs, b.CollapsedDirs...),
	}
	
	if b.MaxDepth > result.MaxDepth {
		result.MaxDepth = b.MaxDepth
	}
	
	// Handle time comparisons
	if a.OldestTime.IsZero() || (!b.OldestTime.IsZero() && b.OldestTime.Before(a.OldestTime)) {
		result.OldestTime = b.OldestTime
	} else {
		result.OldestTime = a.OldestTime
	}
	
	if a.NewestTime.IsZero() || (!b.NewestTime.IsZero() && b.NewestTime.After(a.NewestTime)) {
		result.NewestTime = b.NewestTime
	} else {
		result.NewestTime = a.NewestTime
	}
	
	return result
}

func (v *Validator) validateHeader(line string) bool {
	if !strings.HasPrefix(line, "@c4m ") {
		v.addError(1, 1, "header", "first line must start with '@c4m '", true)
		return false
	}
	
	parts := strings.Fields(line)
	if len(parts) != 2 {
		v.addError(1, 1, "header", "invalid header format", true)
		return false
	}
	
	version := parts[1]
	if version != "1.0" {
		v.addWarning(1, 6, "version", fmt.Sprintf("unknown version: %s", version))
	}
	
	return true
}

func (v *Validator) validateEntry(line string) {
	// Check UTF-8 validity
	if !utf8.ValidString(line) {
		v.addError(v.lineNum, 0, "encoding", "invalid UTF-8", false)
		return
	}
	
	// Check for control characters
	for i, r := range line {
		if r < 0x20 && r != '\t' {
			v.addError(v.lineNum, i+1, "character", fmt.Sprintf("forbidden control character: 0x%02X", r), false)
			return
		}
	}
	
	// Calculate depth (must be multiples of 2 spaces)
	depth := 0
	for i := 0; i < len(line); i++ {
		if line[i] == ' ' {
			depth++
		} else {
			break
		}
	}
	
	// Depth must be even (2 spaces per level)
	if depth%2 != 0 {
		v.addError(v.lineNum, depth, "indentation", "indentation must be multiples of 2 spaces", false)
		return
	}
	depthLevel := depth / 2
	
	// Validate depth progression (can only increase by 1)
	if depthLevel > v.lastDepth+1 {
		v.addError(v.lineNum, 1, "depth", fmt.Sprintf("invalid depth jump from %d to %d", v.lastDepth, depthLevel), false)
	}
	
	// Store original stack size for validation
	originalStackSize := len(v.depthStack)
	
	// Check dedentation rules (can dedent by any amount)
	if depthLevel < v.lastDepth {
		// After dedenting, reset directory tracking for this level
		v.seenDirAtDepth[depthLevel] = false
		// Don't adjust stack yet - we need it for validation
	}
	
	// Parse fields
	trimmed := strings.TrimLeft(line, " ")
	fields := strings.Fields(trimmed)
	
	if len(fields) < 4 {
		v.addError(v.lineNum, 0, "fields", fmt.Sprintf("insufficient fields: need at least 4, got %d", len(fields)), false)
		return
	}
	
	mode := fields[0]
	timestamp := fields[1]
	size := fields[2]
	nameStart := 3
	
	// Validate mode
	v.validateMode(mode)
	
	// Validate timestamp
	v.validateTimestamp(timestamp)
	
	// Validate size
	v.validateSize(size)
	
	// Extract name (handle quoted names)
	name, symTarget, c4id := v.parseNameAndRest(fields[nameStart:])
	
	// Update statistics
	v.updateStats(mode, timestamp, size, name, c4id)
	
	// Check files-before-directories rule at same depth
	isDir := strings.HasSuffix(name, "/")
	if !isDir && v.seenDirAtDepth[depthLevel] {
		v.addError(v.lineNum, 0, "ordering", "files must come before directories at the same depth", false)
	}
	if isDir {
		v.seenDirAtDepth[depthLevel] = true
	}
	
	// Check that non-directory entries at wrong depth relative to open directories
	// A file should be at depth = len(depthStack) (inside the deepest directory)
	// or at depth = 0 when no directories are open
	if !isDir {
		expectedDepth := originalStackSize
		if depthLevel != expectedDepth {
			if originalStackSize > 0 {
				v.addError(v.lineNum, 0, "indentation", fmt.Sprintf("file at depth %d but should be at depth %d (inside directory '%s')", 
					depthLevel, expectedDepth, v.depthStack[originalStackSize-1]), false)
			}
		}
	}
	
	// Validate name
	v.validateName(name, depthLevel)
	
	// Check for duplicates (unless in a layer which can override)
	if !v.inLayer {
		if prevLine, exists := v.seenPaths[name]; exists {
			v.addError(v.lineNum, 0, "duplicate", fmt.Sprintf("duplicate path (first seen at line %d)", prevLine), false)
		} else {
			v.seenPaths[name] = v.lineNum
		}
	} else {
		// In layers, duplicates override previous entries
		v.seenPaths[name] = v.lineNum
	}
	
	// Validate symlink target if present
	if symTarget != "" && !strings.HasPrefix(mode, "l") {
		v.addError(v.lineNum, 0, "symlink", "symlink target specified for non-symlink", false)
	}
	
	// Validate C4 ID if present
	if c4id != "" {
		v.validateC4ID(c4id)
	}
	
	// Update depth tracking
	v.lastDepth = depthLevel
	
	// Track directory structure  
	// First adjust stack for dedentation
	if depthLevel < len(v.depthStack) {
		v.depthStack = v.depthStack[:depthLevel]
	}
	
	// Then add new directory if this is one
	if strings.HasSuffix(name, "/") {
		v.depthStack = append(v.depthStack, name)
	}
}

func (v *Validator) validateMode(mode string) {
	if mode == "-" || mode == "----------" {
		return // Null mode is valid
	}
	
	if len(mode) != 10 {
		v.addError(v.lineNum, 0, "mode", fmt.Sprintf("mode must be 10 characters, got %d", len(mode)), false)
		return
	}
	
	// Check first character (file type)
	validTypes := "-dlbcps"
	if !strings.ContainsRune(validTypes, rune(mode[0])) {
		v.addError(v.lineNum, 0, "mode", fmt.Sprintf("invalid file type: %c", mode[0]), false)
	}
	
	// Check permission characters
	for i := 1; i < 10; i++ {
		c := mode[i]
		validChars := "-rwxstST"
		if !strings.ContainsRune(validChars, rune(c)) {
			v.addError(v.lineNum, i+1, "mode", fmt.Sprintf("invalid permission character: %c", c), false)
		}
	}
}

func (v *Validator) validateTimestamp(ts string) {
	if ts == "-" || ts == "0" {
		return // Null timestamp is valid
	}
	
	// Check ISO 8601 format with Z suffix
	if !strings.HasSuffix(ts, "Z") {
		v.addError(v.lineNum, 0, "timestamp", "timestamp must end with 'Z' for UTC", false)
		return
	}
	
	// Try to parse
	_, err := time.Parse("2006-01-02T15:04:05Z", ts)
	if err != nil {
		v.addError(v.lineNum, 0, "timestamp", fmt.Sprintf("invalid ISO 8601 timestamp: %v", err), false)
	}
}

func (v *Validator) validateSize(size string) {
	if size == "-" {
		return // Null size is valid
	}
	
	// Check for comma separators (only allowed in ergonomic form)
	cleanSize := strings.ReplaceAll(size, ",", "")
	if cleanSize != size && v.Strict {
		v.addWarning(v.lineNum, 0, "size", "comma separators in size (ergonomic form)")
	}
	
	// Parse as integer
	val, err := strconv.ParseInt(cleanSize, 10, 64)
	if err != nil {
		v.addError(v.lineNum, 0, "size", fmt.Sprintf("invalid size: %v", err), false)
		return
	}
	
	if val < -1 {
		v.addError(v.lineNum, 0, "size", "size cannot be less than -1", false)
	}
}

func (v *Validator) validateName(name string, depth int) {
	if name == "" {
		v.addError(v.lineNum, 0, "name", "name cannot be empty", false)
		return
	}
	
	// Check for path traversal
	if strings.Contains(name, "../") || strings.Contains(name, "./") {
		v.addError(v.lineNum, 0, "name", "path traversal not allowed", false)
	}
	
	// Check for null bytes
	if strings.Contains(name, "\x00") {
		v.addError(v.lineNum, 0, "name", "null bytes not allowed in names", false)
	}
	
	// Check directory naming
	isDir := strings.HasSuffix(name, "/")
	if isDir && len(name) == 1 {
		v.addError(v.lineNum, 0, "name", "directory name cannot be just '/'", false)
	}
}

func (v *Validator) validateC4ID(id string) {
	if !strings.HasPrefix(id, "c4") {
		v.addError(v.lineNum, 0, "c4id", "C4 ID must start with 'c4'", false)
		return
	}
	
	// Basic length check (C4 IDs are typically 90 characters)
	if len(id) < 20 || len(id) > 100 {
		v.addWarning(v.lineNum, 0, "c4id", fmt.Sprintf("unusual C4 ID length: %d", len(id)))
	}
	
	// Check for valid base58 characters (simplified check)
	validChars := regexp.MustCompile(`^c4[1-9A-HJ-NP-Za-km-z]+$`)
	if !validChars.MatchString(id) {
		v.addError(v.lineNum, 0, "c4id", "invalid C4 ID format", false)
	}
}

func (v *Validator) parseNameAndRest(fields []string) (name, symTarget, c4id string) {
	if len(fields) == 0 {
		return "", "", ""
	}
	
	// Join all fields first to handle directory names with spaces
	allFields := strings.Join(fields, " ")
	
	// Check if it's a directory (ends with /)
	if slashIdx := strings.LastIndex(allFields, "/"); slashIdx != -1 {
		// Directory: everything up to and including the slash is the name
		name = allFields[:slashIdx+1]
		rest := allFields[slashIdx+1:]
		
		// Parse remaining fields after the directory name
		if rest != "" {
			restFields := strings.Fields(rest)
			for _, field := range restFields {
				if strings.HasPrefix(field, "c4") {
					c4id = field
					break
				}
			}
		}
		return name, "", c4id
	}
	
	// Not a directory - handle quoted names for files
	if strings.HasPrefix(fields[0], `"`) {
		// Find end quote
		nameFields := []string{fields[0]}
		endIdx := 0
		for i, field := range fields {
			if i > 0 {
				nameFields = append(nameFields, field)
			}
			if strings.HasSuffix(field, `"`) && !strings.HasSuffix(field, `\"`) {
				endIdx = i
				break
			}
		}
		name = strings.Join(nameFields, " ")
		name = strings.Trim(name, `"`)
		fields = fields[endIdx+1:]
	} else {
		name = fields[0]
		fields = fields[1:]
	}
	
	// Look for symlink target
	for i, field := range fields {
		if field == "->" && i+1 < len(fields) {
			symTarget = fields[i+1]
			fields = append(fields[:i], fields[i+2:]...)
			break
		}
	}
	
	// Look for C4 ID
	for _, field := range fields {
		if strings.HasPrefix(field, "c4") {
			c4id = field
			break
		}
	}
	
	return name, symTarget, c4id
}


func (v *Validator) addError(line, col int, field, msg string, fatal bool) {
	v.errors = append(v.errors, ValidationError{
		Line:    line,
		Column:  col,
		Field:   field,
		Message: msg,
		Fatal:   fatal,
	})
}

func (v *Validator) addWarning(line, col int, field, msg string) {
	v.warnings = append(v.warnings, ValidationError{
		Line:    line,
		Column:  col,
		Field:   field,
		Message: msg,
		Fatal:   false,
	})
}

func (v *Validator) getResult() error {
	if len(v.errors) == 0 {
		return nil
	}
	
	// Build error message
	var msgs []string
	for _, e := range v.errors {
		msgs = append(msgs, e.Error())
	}
	
	summary := fmt.Sprintf("validation failed with %d errors", len(v.errors))
	if len(v.warnings) > 0 {
		summary += fmt.Sprintf(" and %d warnings", len(v.warnings))
	}
	
	return fmt.Errorf("%s:\n%s", summary, strings.Join(msgs, "\n"))
}

// GetErrors returns all validation errors
func (v *Validator) GetErrors() []ValidationError {
	return v.errors
}

// GetWarnings returns all validation warnings
func (v *Validator) GetWarnings() []ValidationError {
	return v.warnings
}

// GetStats returns validation statistics
func (v *Validator) GetStats() ValidationStats {
	return v.stats
}

// handleDirective processes @ directives
func (v *Validator) handleDirective(line string) {
	if strings.HasPrefix(line, "@layer") {
		v.inLayer = true
		v.stats.Layers++
	} else if strings.HasPrefix(line, "@end") {
		v.inLayer = false
	}
	// Other directives are allowed but not validated in detail
}

// updateStats updates validation statistics based on an entry
func (v *Validator) updateStats(mode, timestamp, sizeStr, name, c4id string) {
	v.stats.TotalEntries++
	
	// Track depth
	depth := v.lastDepth
	if depth > v.stats.MaxDepth {
		v.stats.MaxDepth = depth
	}
	
	// Track entry type
	if mode != "-" && mode != "----------" && len(mode) > 0 {
		switch mode[0] {
		case '-':
			v.stats.Files++
		case 'd':
			v.stats.Directories++
		case 'l':
			v.stats.Symlinks++
		case 'b', 'c', 'p', 's':
			v.stats.SpecialFiles++
		}
	} else if strings.HasSuffix(name, "/") {
		v.stats.Directories++
	} else {
		v.stats.Files++
	}
	
	// Track size
	if sizeStr == "-" {
		v.stats.NullSizes++
	} else {
		cleanSize := strings.ReplaceAll(sizeStr, ",", "")
		if size, err := strconv.ParseInt(cleanSize, 10, 64); err == nil && size >= 0 {
			v.stats.TotalSize += size
		}
	}
	
	// Track timestamp
	if timestamp == "-" || timestamp == "0" {
		v.stats.NullTimes++
	} else if t, err := time.Parse("2006-01-02T15:04:05Z", timestamp); err == nil {
		if v.stats.OldestTime.IsZero() || t.Before(v.stats.OldestTime) {
			v.stats.OldestTime = t
		}
		if v.stats.NewestTime.IsZero() || t.After(v.stats.NewestTime) {
			v.stats.NewestTime = t
		}
	}
	
	// Track chunks
	if c4id != "" && strings.HasPrefix(c4id, "c4") {
		v.stats.Chunks++
	}
}

// ValidateFile validates a C4M or bundle file
func ValidateFile(path string, strict bool) error {
	validator := NewValidator(strict)
	
	// Check if it's a bundle
	if strings.HasSuffix(path, ".c4m_bundle") || strings.HasSuffix(path, "_bundle") {
		return validator.ValidateBundle(path)
	}
	
	// Otherwise treat as manifest
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	
	return validator.ValidateManifest(file)
}