package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

func runPaths(args []string) {
	fs := newFlags("paths")
	fs.parse(args)

	if len(fs.args) > 1 {
		fmt.Fprintf(os.Stderr, "Usage: c4 paths [<file.c4m> | -]\n")
		os.Exit(1)
	}

	var input *os.File
	if len(fs.args) == 0 {
		// No argument — read from stdin (if piped).
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			fmt.Fprintf(os.Stderr, "Usage: c4 paths [<file.c4m> | -]\n")
			os.Exit(1)
		}
		input = os.Stdin
	} else if fs.args[0] == "-" {
		input = os.Stdin
	} else {
		f, err := os.Open(fs.args[0])
		if err != nil {
			fatalf("Error: %v", err)
		}
		defer f.Close()
		input = f
	}

	// Read all input lines to detect format.
	var lines []string
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		fatalf("Error reading input: %v", err)
	}

	if isC4MPatchInput(lines) {
		c4mPatchToPaths(lines)
	} else if isC4MInput(lines) {
		c4mToPaths(lines)
	} else {
		pathsToC4M(lines)
	}
}

// isC4MInput returns true if the lines look like c4m format.
// A c4m entry line starts with either:
//   - A 10-character Unix mode string (e.g., "-rw-r--r--", "drwxr-xr-x")
//   - Whitespace followed by a mode string (indented child entry)
//   - A single "-" followed by space (null mode shorthand)
func isC4MInput(lines []string) bool {
	for _, line := range lines {
		if line == "" {
			continue
		}
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			continue
		}
		return looksLikeC4MLine(trimmed)
	}
	return false
}

// isC4MPatchInput returns true if the lines are a c4m patch stream, as
// written by c4 diff: a leading bare C4 ID (the base reference), then
// entry lines, optionally followed by further bare IDs (checkpoints, the
// closing validator). The first line that is not a bare ID must be an
// entry, so a plain list of ID-named files stays a path list.
func isC4MPatchInput(lines []string) bool {
	sawID := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if isBareC4IDLine(trimmed) {
			sawID = true
			continue
		}
		return sawID && looksLikeC4MLine(trimmed)
	}
	return false
}

// isBareC4IDLine reports whether a trimmed line is exactly one C4 ID.
func isBareC4IDLine(line string) bool {
	if len(line) != 90 || !strings.HasPrefix(line, "c4") {
		return false
	}
	_, err := c4.Parse(line)
	return err == nil
}

// looksLikeC4MLine checks if a trimmed line starts with a valid c4m mode field.
func looksLikeC4MLine(line string) bool {
	if len(line) < 2 {
		return false
	}
	// Null mode shorthand: "- " at start
	if line[0] == '-' && line[1] == ' ' {
		return true
	}
	// Full 10-char mode string: type char + 9 permission chars
	if len(line) < 11 {
		return false
	}
	ch := line[0]
	if ch != '-' && ch != 'd' && ch != 'l' && ch != 'p' &&
		ch != 's' && ch != 'b' && ch != 'c' {
		return false
	}
	// Remaining 9 chars must be from the permission set.
	for i := 1; i < 10; i++ {
		c := line[i]
		if c != 'r' && c != 'w' && c != 'x' && c != '-' &&
			c != 's' && c != 'S' && c != 't' && c != 'T' {
			return false
		}
	}
	// Must be followed by a space (field separator).
	return line[10] == ' '
}

// c4mToPaths parses c4m input and prints one path per line.
func c4mToPaths(lines []string) {
	text := strings.Join(lines, "\n")
	m, err := c4m.NewDecoder(strings.NewReader(text)).Decode()
	if err != nil {
		fatalf("Error parsing c4m: %v", err)
	}

	printPaths(c4m.EntryPaths(m.Entries))
}

// c4mPatchToPaths parses a c4m patch stream and prints the path of every
// entry the patch names (added, modified, or removed), one per line.
func c4mPatchToPaths(lines []string) {
	text := strings.Join(lines, "\n")
	sections, err := c4m.DecodePatchChain(strings.NewReader(text))
	if err != nil {
		fatalf("Error parsing c4m: %v", err)
	}

	paths := make(map[string]*c4m.Entry)
	for _, sec := range sections {
		for p, e := range c4m.EntryPaths(sec.Entries) {
			paths[p] = e
		}
	}
	printPaths(paths)
}

// printPaths prints the keys of paths in sorted order, one per line.
func printPaths(paths map[string]*c4m.Entry) {
	// Sort paths for stable output.
	sorted := make([]string, 0, len(paths))
	for p := range paths {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)

	for i, p := range sorted {
		if i > 0 {
			fmt.Println()
		}
		fmt.Print(p)
	}
	if len(sorted) > 0 {
		fmt.Println()
	}
}

// pathsToC4M takes path lines and builds a c4m with null metadata.
func pathsToC4M(lines []string) {
	// Collect unique paths and ensure parent directories exist.
	pathSet := make(map[string]bool)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pathSet[line] = true
	}

	// Ensure parent directories exist for each path.
	allPaths := make(map[string]bool)
	for p := range pathSet {
		allPaths[p] = true
		// Add parent directories.
		parts := strings.Split(p, "/")
		for i := 1; i < len(parts); i++ {
			dir := strings.Join(parts[:i], "/") + "/"
			allPaths[dir] = true
		}
	}

	// Build sorted path list.
	sorted := make([]string, 0, len(allPaths))
	for p := range allPaths {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)

	// Build manifest from paths.
	m := c4m.NewManifest()
	for _, p := range sorted {
		name := pathEntryName(p)
		depth := pathToDepth(p)

		entry := &c4m.Entry{
			Name:      name,
			Depth:     depth,
			Size:      -1, // null
			Timestamp: c4m.NullTimestamp(),
			// Mode stays 0 (null) for all entries. Directory-ness is
			// indicated by the trailing "/" in the name, not the mode.
		}
		m.AddEntry(entry)
	}

	m.SortEntries()
	enc := c4m.NewEncoder(os.Stdout)
	enc.Encode(m)
}

// pathToDepth returns the nesting depth for a full path.
func pathToDepth(fullPath string) int {
	clean := strings.TrimSuffix(fullPath, "/")
	return strings.Count(clean, "/")
}

// pathEntryName returns the bare name (last component) for a full path.
func pathEntryName(fullPath string) string {
	isDir := strings.HasSuffix(fullPath, "/")
	clean := strings.TrimSuffix(fullPath, "/")
	idx := strings.LastIndex(clean, "/")
	name := clean
	if idx >= 0 {
		name = clean[idx+1:]
	}
	if isDir {
		name += "/"
	}
	return name
}
