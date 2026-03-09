package c4m

import (
	"bufio"
	"fmt"
	"io"
	"os"
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
	Chunks          int64 // Number of referenced chunks
	MaxDepth        int   // Maximum directory depth
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
	stats        ValidationStats
	isErgonomic  bool // Whether the file uses ergonomic format
	formatDetected bool // Whether we've detected the format yet
	currentPath  string // Current full path being processed
	lastPathAtDepth map[int]string // Track last path at each depth for sorting validation
}

// NewValidator creates a new validator
func NewValidator(strict bool) *Validator {
	return &Validator{
		Strict:    strict,
		MaxErrors: 100000, // Increased to handle large files
		seenPaths: make(map[string]int),
		depthStack: []string{},
		lastPathAtDepth: make(map[int]string),
		seenDirAtDepth: make(map[int]bool),
	}
}

// ValidateManifest validates a C4M manifest from a reader.
// The format is entry-only: no header, no directives.
func (v *Validator) ValidateManifest(r io.Reader) error {
	v.errors = nil
	v.warnings = nil
	v.lineNum = 0
	v.seenPaths = make(map[string]int)
	v.lastDepth = -1
	v.depthStack = []string{}
	v.seenDirAtDepth = make(map[int]bool)
	v.stats = ValidationStats{}
	v.formatDetected = false
	v.isErgonomic = false
	v.lastPathAtDepth = make(map[int]string)

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB max line

	// Process entries
	for scanner.Scan() {
		v.lineNum++
		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		// Reject directive lines
		if strings.HasPrefix(strings.TrimSpace(line), "@") {
			v.addError(v.lineNum, 0, "directive", fmt.Sprintf("directives not supported: %s", line), false)
			continue
		}

		// Detect format on first entry if not yet detected
		if !v.formatDetected && !strings.HasPrefix(line, "#") {
			v.detectFormat(line)
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
	var timestamp, size string
	var nameStart int
	
	// Handle different timestamp formats
	if v.formatDetected && v.isErgonomic {
		// Ergonomic format: mode Month Day Time Year TZ size name [c4id]
		// Example: drwxr-xr-x Feb 19 03:17:53 2025 CST 108,420 types/ c41x...
		if len(fields) < 8 {
			v.addError(v.lineNum, 0, "fields", fmt.Sprintf("insufficient fields for ergonomic format: need at least 8, got %d", len(fields)), false)
			return
		}
		// Timestamp spans fields 1-5 (Month Day Time Year TZ)
		timestamp = strings.Join(fields[1:6], " ")
		size = fields[6]
		nameStart = 7
	} else {
		// Canonical format: mode timestamp size name [c4id]
		timestamp = fields[1]
		size = fields[2]
		nameStart = 3
	}
	
	// Validate mode
	v.validateMode(mode)
	
	// Validate timestamp
	v.validateTimestamp(timestamp)
	
	// Validate size
	v.validateSize(size)
	
	// Validate raw name field before parsing — catch path separators that
	// parseNameAndRest would otherwise silently interpret.
	rawName := strings.Join(fields[nameStart:], " ")
	if !strings.HasPrefix(mode, "d") && !strings.HasPrefix(mode, "l") {
		// For files (not directories/symlinks), the name must not contain "/"
		if strings.Contains(rawName, "/") {
			v.addError(v.lineNum, 0, "name", fmt.Sprintf("file name contains path separator '/': '%s' — use depth/indentation for hierarchy", rawName), false)
		}
	}
	if strings.Contains(rawName, "\\") {
		v.addError(v.lineNum, 0, "name", fmt.Sprintf("name contains backslash: '%s' — backslash is not allowed in c4m names", rawName), false)
	}

	// Extract name (handle quoted names)
	name, symTarget, flowOp, flowTgt, c4id := v.parseNameAndRest(fields[nameStart:])
	
	// Update statistics
	v.updateStats(depthLevel, mode, timestamp, size, name, c4id)
	
	// Check files-before-directories rule at same depth
	isDir := strings.HasSuffix(name, "/")
	if !isDir && v.seenDirAtDepth[depthLevel] {
		v.addError(v.lineNum, 0, "ordering", "files must come before directories at the same depth", false)
	}
	if isDir {
		v.seenDirAtDepth[depthLevel] = true
	}
	
	// Build the current path
	v.currentPath = v.buildPath(name, depthLevel)
	
	// Check that non-directory entries at wrong depth relative to open directories
	// A file should be at depth = len(depthStack) (inside the deepest directory)
	// or at depth = 0 when no directories are open
	if !isDir {
		expectedDepth := originalStackSize
		if depthLevel != expectedDepth {
			if originalStackSize > 0 {
				expectedPath := v.buildExpectedPath(name, expectedDepth)
				v.addError(v.lineNum, 0, "indentation", fmt.Sprintf("file '%s' at depth %d but should be at depth %d (expected path: '%s')", 
					v.currentPath, depthLevel, expectedDepth, expectedPath), false)
			}
		}
	}
	
	// Validate name
	v.validateName(name, depthLevel)

	// Check sorting in strict mode
	if v.Strict {
		if lastPath, exists := v.lastPathAtDepth[depthLevel]; exists {
			// Natural sort comparison - simple string comparison for now
			// In a real implementation, would use natural sort algorithm
			if v.currentPath < lastPath {
				v.addError(v.lineNum, 0, "sorting", fmt.Sprintf("entries not sorted: '%s' should come before '%s'", v.currentPath, lastPath), false)
			}
		}
		v.lastPathAtDepth[depthLevel] = v.currentPath
	}

	// Check for duplicates
	if prevLine, exists := v.seenPaths[v.currentPath]; exists {
		v.addError(v.lineNum, 0, "duplicate", fmt.Sprintf("duplicate path '%s' (first seen at line %d)", v.currentPath, prevLine), false)
	} else {
		v.seenPaths[v.currentPath] = v.lineNum
	}
	
	// Validate symlink target if present
	if symTarget != "" && !strings.HasPrefix(mode, "l") {
		v.addError(v.lineNum, 0, "symlink", "symlink target specified for non-symlink", false)
	}

	// Validate flow link if present
	if flowOp != "" {
		if symTarget != "" {
			v.addError(v.lineNum, 0, "flow", "flow link mutually exclusive with symlink target", false)
		}
		v.validateFlowTarget(flowTgt)
		if (flowOp == "<-" || flowOp == "<>") && strings.HasPrefix(mode, "l") {
			v.addError(v.lineNum, 0, "flow", fmt.Sprintf("%s flow link cannot have symlink mode", flowOp), false)
		}
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
	
	// If we've detected ergonomic format, don't validate timestamp format
	if v.isErgonomic {
		// Could add validation for ergonomic format here if needed
		return
	}
	
	// For canonical format, check ISO 8601 with Z suffix
	if !strings.HasSuffix(ts, "Z") {
		v.addError(v.lineNum, 0, "timestamp", "timestamp must end with 'Z' for UTC in canonical format", false)
		return
	}
	
	// Try to parse canonical format
	_, err := time.Parse(TimestampFormat, ts)
	if err != nil {
		v.addError(v.lineNum, 0, "timestamp", fmt.Sprintf("invalid ISO 8601 timestamp: %v", err), false)
	}
}

func (v *Validator) validateSize(size string) {
	if size == "-" {
		return // Null size is valid
	}
	
	// Remove comma separators if present (common in ergonomic form)
	cleanSize := strings.ReplaceAll(size, ",", "")
	
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

	// A c4m entry name is a bare filename, never a path. Strip the
	// trailing "/" directory marker and validate the base name.
	base := strings.TrimSuffix(name, "/")

	if base == "" {
		v.addError(v.lineNum, 0, "name", "name cannot be '/' — the c4m file itself is the root", false)
		return
	}

	if base == "." || base == ".." {
		v.addError(v.lineNum, 0, "name", fmt.Sprintf("'%s' is a path component, not a valid entry name", name), false)
		return
	}

	if strings.Contains(base, "/") {
		v.addError(v.lineNum, 0, "name", fmt.Sprintf("name contains path separator '/': '%s' — use depth/indentation for hierarchy", name), false)
	}

	if strings.Contains(base, "\\") {
		v.addError(v.lineNum, 0, "name", fmt.Sprintf("name contains backslash: '%s' — backslash is not allowed in c4m names", name), false)
	}

	if strings.Contains(base, "\x00") {
		v.addError(v.lineNum, 0, "name", "null bytes not allowed in names", false)
	}
}

func (v *Validator) validateC4ID(id string) {
	// Skip validation for null C4 ID
	if id == "-" {
		return
	}

	if !strings.HasPrefix(id, "c4") {
		v.addError(v.lineNum, 0, "c4id", fmt.Sprintf("C4 ID must start with 'c4', got: %s", id), false)
		return
	}

	// C4 IDs should be exactly 90 characters
	if len(id) != 90 {
		if v.Strict {
			v.addError(v.lineNum, 0, "c4id", fmt.Sprintf("C4 ID must be 90 characters, got %d", len(id)), false)
		} else {
			v.addWarning(v.lineNum, 0, "c4id", fmt.Sprintf("unusual C4 ID length: %d (expected 90)", len(id)))
		}
	}

	// Check for valid base58 characters (C4 uses a specific base58 alphabet)
	// Base58 alphabet excludes: 0, O, I, l to avoid confusion
	validChars := regexp.MustCompile(`^c4[1-9A-HJ-NP-Za-km-z]+$`)
	if !validChars.MatchString(id) {
		v.addError(v.lineNum, 0, "c4id", fmt.Sprintf("invalid C4 ID format: contains invalid characters in: %s", id), false)
	}
}

func (v *Validator) parseNameAndRest(fields []string) (name, symTarget, flowOp, flowTgt, c4id string) {
	if len(fields) == 0 {
		return "", "", "", "", ""
	}

	// Join all fields first to handle directory names with spaces
	allFields := strings.Join(fields, " ")

	// Check if it's a directory (ends with /)
	if slashIdx := strings.LastIndex(allFields, "/"); slashIdx != -1 {
		// Directory: everything up to and including the slash is the name
		name = allFields[:slashIdx+1]
		rest := allFields[slashIdx+1:]

		// Check if there's a quote after the slash (form: "dirname/")
		if strings.HasPrefix(rest, `"`) && strings.HasPrefix(name, `"`) {
			name = strings.TrimPrefix(name, `"`)
			rest = strings.TrimPrefix(rest, `"`)
			v.addWarning(v.lineNum, 0, "name", "quoted directory names are non-canonical")
		} else if strings.HasPrefix(name, `"`) {
			nameWithoutSlash := name[:len(name)-1]
			if strings.HasSuffix(nameWithoutSlash, `"`) {
				name = strings.TrimPrefix(nameWithoutSlash, `"`)
				name = strings.TrimSuffix(name, `"`)
				name = name + "/"
				v.addWarning(v.lineNum, 0, "name", "quoted directory names are non-canonical")
			}
		}
		// Parse remaining fields after the directory name
		if rest != "" {
			restFields := strings.Fields(rest)
			for i, field := range restFields {
				if (field == "->" || field == "<-" || field == "<>") && i+1 < len(restFields) {
					nextField := restFields[i+1]
					if field == "->" && (strings.HasPrefix(nextField, "c4") || nextField == "-") {
						break // hard link
					}
					if field == "->" && !strings.Contains(nextField, ":") {
						break // symlink
					}
					if strings.Contains(nextField, ":") {
						flowOp = field
						flowTgt = nextField
						restFields = append(restFields[:i], restFields[i+2:]...)
					}
					break
				}
			}
			for _, field := range restFields {
				if strings.HasPrefix(field, "c4") {
					c4id = field
					break
				}
			}
		}
		return name, "", flowOp, flowTgt, c4id
	}

	// Not a directory - handle quoted names for files
	if strings.HasPrefix(fields[0], `"`) {
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

	// Look for link operators: ->, <-, <>
	for i, field := range fields {
		if (field == "->" || field == "<-" || field == "<>") && i+1 < len(fields) {
			nextField := fields[i+1]
			if field == "->" {
				if strings.HasPrefix(nextField, "c4") || nextField == "-" {
					break // hard link
				}
				if strings.Contains(nextField, ":") {
					flowOp = field
					flowTgt = nextField
					fields = append(fields[:i], fields[i+2:]...)
					break
				}
				// symlink target
				symTarget = nextField
				fields = append(fields[:i], fields[i+2:]...)
				break
			}
			// <- or <> are always flow
			flowOp = field
			flowTgt = nextField
			fields = append(fields[:i], fields[i+2:]...)
			break
		}
	}

	// Look for C4 ID
	if len(fields) > 0 {
		c4id = fields[len(fields)-1]
	}

	return name, symTarget, flowOp, flowTgt, c4id
}


// flowLocationPattern matches a valid flow location label.
var flowLocationPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

func (v *Validator) validateFlowTarget(target string) {
	if target == "" {
		v.addError(v.lineNum, 0, "flow", "empty flow target", false)
		return
	}
	colonIdx := strings.Index(target, ":")
	if colonIdx < 0 {
		v.addError(v.lineNum, 0, "flow", fmt.Sprintf("flow target missing ':' separator: %q", target), false)
		return
	}
	label := target[:colonIdx]
	if !flowLocationPattern.MatchString(label) {
		v.addError(v.lineNum, 0, "flow", fmt.Sprintf("invalid flow location label: %q", label), false)
		return
	}
	path := target[colonIdx+1:]
	if strings.HasPrefix(path, "/") {
		v.addError(v.lineNum, 0, "flow", fmt.Sprintf("flow target path must not start with '/': %q", target), false)
	}
	if strings.Contains(path, "..") {
		v.addError(v.lineNum, 0, "flow", fmt.Sprintf("flow target path must not contain '..': %q", target), false)
	}
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

// buildPath constructs the full path for an entry at the given depth
func (v *Validator) buildPath(name string, depth int) string {
	if depth == 0 {
		return name
	}
	
	// Build path from stack up to the specified depth
	path := ""
	for i := 0; i < depth && i < len(v.depthStack); i++ {
		path += v.depthStack[i]
	}
	path += name
	return path
}

// buildExpectedPath constructs what the path should be if at the correct depth
func (v *Validator) buildExpectedPath(name string, expectedDepth int) string {
	if expectedDepth == 0 {
		return name
	}
	
	// Build path from stack up to the expected depth
	path := ""
	for i := 0; i < expectedDepth && i < len(v.depthStack); i++ {
		path += v.depthStack[i]
	}
	path += name
	return path
}

// GetCurrentPath returns the full path of the entry being processed
func (v *Validator) GetCurrentPath() string {
	return v.currentPath
}

// detectFormat determines if the manifest uses canonical or ergonomic format
func (v *Validator) detectFormat(line string) {
	v.formatDetected = true
	
	// Look for timestamp pattern in the line
	trimmed := strings.TrimSpace(line)
	fields := strings.Fields(trimmed)
	
	if len(fields) < 3 {
		return
	}
	
	// Skip mode field, look at timestamp field (usually second field)
	for i := 1; i < len(fields) && i < 4; i++ {
		field := fields[i]
		
		// Check for canonical format (ISO 8601 with Z)
		if strings.Contains(field, "T") && strings.HasSuffix(field, "Z") {
			v.isErgonomic = false
			return
		}

		// Check for ergonomic format (month name)
		months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
		for _, month := range months {
			if field == month {
				v.isErgonomic = true
				return
			}
		}
	}
}

// updateStats updates validation statistics based on an entry
func (v *Validator) updateStats(depth int, mode, timestamp, sizeStr, name, c4id string) {
	v.stats.TotalEntries++

	// Track depth
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
	} else if t, err := time.Parse(TimestampFormat, timestamp); err == nil {
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

// ValidateFile validates a C4M manifest file
func ValidateFile(path string, strict bool) error {
	validator := NewValidator(strict)

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return validator.ValidateManifest(file)
}