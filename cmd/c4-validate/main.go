package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
)

type ValidationIssue struct {
	Severity string // "error", "warning", "info"
	Location string // file path or context
	Message  string
}

type BundleValidator struct {
	bundlePath string
	issues     []ValidationIssue
	verbose    bool
}

func NewBundleValidator(bundlePath string, verbose bool) *BundleValidator {
	return &BundleValidator{
		bundlePath: bundlePath,
		verbose:    verbose,
	}
}

func (v *BundleValidator) AddIssue(severity, location, message string) {
	v.issues = append(v.issues, ValidationIssue{
		Severity: severity,
		Location: location,
		Message:  message,
	})
	if v.verbose || severity == "error" {
		fmt.Printf("[%s] %s: %s\n", severity, location, message)
	}
}

func (v *BundleValidator) ValidateBundle() error {
	// Check bundle directory exists
	info, err := os.Stat(v.bundlePath)
	if err != nil {
		return fmt.Errorf("bundle not found: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("bundle path is not a directory")
	}

	// Check for required files
	headerPath := filepath.Join(v.bundlePath, "header.c4")
	if _, err := os.Stat(headerPath); err != nil {
		v.AddIssue("error", "header.c4", "missing header.c4 file")
	} else {
		if err := v.validateHeader(); err != nil {
			v.AddIssue("error", "header.c4", err.Error())
		}
	}

	// Check c4 directory
	c4Dir := filepath.Join(v.bundlePath, "c4")
	if _, err := os.Stat(c4Dir); err != nil {
		v.AddIssue("error", "c4/", "missing c4 directory")
	} else {
		if err := v.validateC4Directory(); err != nil {
			v.AddIssue("error", "c4/", err.Error())
		}
	}

	return nil
}

func (v *BundleValidator) validateHeader() error {
	headerPath := filepath.Join(v.bundlePath, "header.c4")
	content, err := os.ReadFile(headerPath)
	if err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	idStr := strings.TrimSpace(string(content))
	headerID, err := c4.Parse(idStr)
	if err != nil {
		return fmt.Errorf("invalid C4 ID in header: %w", err)
	}

	// Check if referenced manifest exists
	manifestPath := filepath.Join(v.bundlePath, "c4", headerID.String())
	manifestContent, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("header manifest not found: %s", headerID)
	}

	// Validate header manifest
	if err := v.validateManifest(manifestPath, manifestContent); err != nil {
		return fmt.Errorf("header manifest validation failed: %w", err)
	}

	return nil
}

func (v *BundleValidator) validateC4Directory() error {
	c4Dir := filepath.Join(v.bundlePath, "c4")
	entries, err := os.ReadDir(c4Dir)
	if err != nil {
		return fmt.Errorf("failed to read c4 directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			v.AddIssue("warning", "c4/"+entry.Name(), "unexpected subdirectory in c4/")
			continue
		}

		// Validate C4 ID format
		if _, err := c4.Parse(entry.Name()); err != nil {
			v.AddIssue("error", "c4/"+entry.Name(), "invalid C4 ID filename")
			continue
		}

		// Verify content matches ID
		filePath := filepath.Join(c4Dir, entry.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			v.AddIssue("warning", "c4/"+entry.Name(), "failed to read file")
			continue
		}

		actualID := c4.Identify(strings.NewReader(string(content)))
		if actualID.String() != entry.Name() {
			v.AddIssue("error", "c4/"+entry.Name(), 
				fmt.Sprintf("content hash mismatch (expected %s, got %s)", 
					entry.Name(), actualID))
		}

		// Check if it's a C4M file
		if strings.HasPrefix(string(content), "@c4m ") {
			if err := v.validateManifest(filePath, content); err != nil {
				v.AddIssue("warning", "c4/"+entry.Name(), err.Error())
			}
		}
	}

	return nil
}

func (v *BundleValidator) validateManifest(path string, content []byte) error {
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return fmt.Errorf("empty manifest")
	}

	// Check C4M header
	if !strings.HasPrefix(lines[0], "@c4m ") {
		return fmt.Errorf("missing @c4m header")
	}

	lineNum := 1
	hasBase := false
	lastDepth := -1
	lastWasDir := false
	filesBeforeDirs := make(map[int]bool) // track if we've seen files at each depth
	depthHasEntries := make(map[int]bool) // track if each depth has any entries
	directoriesAtDepth := make(map[int][]string) // track directories at each depth

	for i := 1; i < len(lines); i++ {
		line := lines[i]  // Don't trim - we need leading spaces for depth!
		lineNum++
		
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Check for @base directive
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "@base ") {
			if hasBase {
				v.AddIssue("warning", fmt.Sprintf("%s:%d", path, lineNum), 
					"multiple @base directives")
			}
			hasBase = true
			baseID := strings.TrimPrefix(trimmed, "@base ")
			if _, err := c4.Parse(baseID); err != nil {
				v.AddIssue("error", fmt.Sprintf("%s:%d", path, lineNum), 
					"invalid C4 ID in @base directive")
			}
			continue
		}

		// Skip layer directives for now
		if strings.HasPrefix(trimmed, "@") {
			continue
		}

		// Parse entry line
		entry, err := v.parseEntryLine(line)
		if err != nil {
			v.AddIssue("warning", fmt.Sprintf("%s:%d", path, lineNum), err.Error())
			continue
		}
		
		if v.verbose {
			fmt.Printf("Line %d: depth=%d, isDir=%v, name=%s\n", lineNum, entry.depth, entry.isDir, entry.name)
		}

		// Track what we've seen at each depth
		depthHasEntries[entry.depth] = true
		if entry.isDir {
			directoriesAtDepth[entry.depth] = append(directoriesAtDepth[entry.depth], entry.name)
		}
		
		// Check depth ordering - depth can only increase by 1, but can decrease by any amount
		if entry.depth > lastDepth + 1 {
			v.AddIssue("error", fmt.Sprintf("%s:%d", path, lineNum), 
				fmt.Sprintf("incorrect depth ordering (depth %d after %d, can only increase by 1)", 
					entry.depth, lastDepth))
		}

		// Check files-before-directories rule at same depth
		if entry.depth == lastDepth {
			if lastWasDir && !entry.isDir {
				v.AddIssue("error", fmt.Sprintf("%s:%d", path, lineNum), 
					fmt.Sprintf("file '%s' appears after directory at same depth (depth=%d)", entry.name, entry.depth))
			}
		} else {
			// New depth level, reset tracking
			filesBeforeDirs[entry.depth] = !entry.isDir
		}

		// Check directory name ends with /
		if entry.isDir && !strings.HasSuffix(entry.name, "/") {
			v.AddIssue("error", fmt.Sprintf("%s:%d", path, lineNum), 
				fmt.Sprintf("directory '%s' missing trailing slash", entry.name))
		}

		// Check file name doesn't end with /
		if !entry.isDir && strings.HasSuffix(entry.name, "/") {
			v.AddIssue("error", fmt.Sprintf("%s:%d", path, lineNum), 
				fmt.Sprintf("file '%s' should not have trailing slash", entry.name))
		}

		lastDepth = entry.depth
		lastWasDir = entry.isDir
	}
	
	// Check for structural issues
	// If we have entries at depth > 0 but no directories at depth 0, that's wrong
	// (unless this is a @base continuation)
	if !hasBase {
		hasDeepEntries := false
		for depth := range depthHasEntries {
			if depth > 0 {
				hasDeepEntries = true
				break
			}
		}
		
		if hasDeepEntries && len(directoriesAtDepth[0]) == 0 {
			v.AddIssue("warning", path, 
				"manifest has entries at depth > 0 but no directories at depth 0 (missing directory structure?)")
		}
	}

	return nil
}

type entryInfo struct {
	isDir     bool
	depth     int
	name      string
	size      int64
	timestamp time.Time
	c4id      string
}

func (v *BundleValidator) parseEntryLine(line string) (*entryInfo, error) {
	// Count leading spaces for depth
	spaceCount := 0
	for i := 0; i < len(line); i++ {
		if line[i] == ' ' {
			spaceCount++
		} else {
			break
		}
	}
	depth := spaceCount / 2 // Each indent level is 2 spaces

	trimmed := strings.TrimSpace(line)
	
	// Parse more carefully - first three fields are fixed, rest is name (and optional C4 ID)
	// We need to handle names with spaces, especially for directories like "Alex /"
	
	// Find the mode (first field)
	modeEnd := strings.IndexByte(trimmed, ' ')
	if modeEnd == -1 {
		return nil, fmt.Errorf("invalid entry format: missing fields")
	}
	mode := trimmed[:modeEnd]
	if len(mode) != 10 {
		return nil, fmt.Errorf("invalid mode string: %s", mode)
	}
	isDir := mode[0] == 'd'
	
	// Skip whitespace and find timestamp
	rest := strings.TrimSpace(trimmed[modeEnd:])
	timestampEnd := strings.IndexByte(rest, ' ')
	if timestampEnd == -1 {
		return nil, fmt.Errorf("invalid entry format: missing timestamp")
	}
	timestamp := rest[:timestampEnd]
	if timestamp != "-" {
		if _, err := time.Parse(time.RFC3339, timestamp); err != nil {
			return nil, fmt.Errorf("invalid timestamp: %s", timestamp)
		}
	}
	
	// Skip whitespace and find size
	rest = strings.TrimSpace(rest[timestampEnd:])
	sizeEnd := strings.IndexByte(rest, ' ')
	if sizeEnd == -1 {
		return nil, fmt.Errorf("invalid entry format: missing size")
	}
	sizeStr := rest[:sizeEnd]
	if sizeStr != "-" {
		// Remove commas if present
		sizeStr = strings.ReplaceAll(sizeStr, ",", "")
		// Parse size - not validating for now
	}
	
	// Everything else is the name (and optional C4 ID at the end)
	rest = strings.TrimSpace(rest[sizeEnd:])
	
	// Check if there's a C4 ID at the end
	// C4 IDs start with 'c4' and are 88 characters long
	var name string
	var c4id string
	
	// Split only on the last space to check for C4 ID
	lastSpace := strings.LastIndexByte(rest, ' ')
	if lastSpace != -1 {
		possibleC4ID := rest[lastSpace+1:]
		if _, err := c4.Parse(possibleC4ID); err == nil {
			// Valid C4 ID found
			c4id = possibleC4ID
			name = strings.TrimSpace(rest[:lastSpace])
		} else {
			// No C4 ID, entire rest is the name
			name = rest
		}
	} else {
		// No spaces, entire rest is the name
		name = rest
	}

	return &entryInfo{
		isDir: isDir,
		depth: depth,
		name:  name,
		c4id:  c4id,
	}, nil
}

func (v *BundleValidator) PrintSummary() {
	errorCount := 0
	warningCount := 0
	infoCount := 0

	for _, issue := range v.issues {
		switch issue.Severity {
		case "error":
			errorCount++
		case "warning":
			warningCount++
		case "info":
			infoCount++
		}
	}

	fmt.Printf("\n=== Validation Summary ===\n")
	fmt.Printf("Bundle: %s\n", v.bundlePath)
	fmt.Printf("Errors:   %d\n", errorCount)
	fmt.Printf("Warnings: %d\n", warningCount)
	fmt.Printf("Info:     %d\n", infoCount)

	if errorCount > 0 {
		fmt.Println("\nValidation FAILED")
		os.Exit(1)
	} else if warningCount > 0 {
		fmt.Println("\nValidation passed with warnings")
	} else {
		fmt.Println("\nValidation PASSED")
	}
}

func main() {
	var bundlePath string
	var verbose bool
	var showAll bool

	flag.StringVar(&bundlePath, "bundle", "", "Path to C4M bundle directory")
	flag.BoolVar(&verbose, "verbose", false, "Show all validation messages")
	flag.BoolVar(&verbose, "v", false, "Show all validation messages (shorthand)")
	flag.BoolVar(&showAll, "all", false, "Show all issues including info level")
	flag.Parse()

	if bundlePath == "" {
		// Check for argument
		if flag.NArg() > 0 {
			bundlePath = flag.Arg(0)
		} else {
			fmt.Fprintf(os.Stderr, "Usage: %s [--bundle] <bundle-path> [--verbose]\n", os.Args[0])
			os.Exit(1)
		}
	}

	validator := NewBundleValidator(bundlePath, verbose || showAll)
	
	if err := validator.ValidateBundle(); err != nil {
		fmt.Fprintf(os.Stderr, "Validation error: %v\n", err)
		os.Exit(1)
	}

	validator.PrintSummary()
}