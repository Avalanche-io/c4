package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

func allTools() []toolDef {
	return []toolDef{
		{
			Name:        "c4_id",
			Description: "Compute the C4 ID of a file. Returns the content-addressable identifier (SMPTE ST 2114).",
			InputSchema: obj(
				prop("path", "string", "Path to the file"),
				required("path"),
			),
		},
		{
			Name:        "c4_scan",
			Description: "Scan a directory and return its c4m file. Computes C4 IDs for all files and builds the hierarchical structure. Scans all files by default (always skips .git/).",
			InputSchema: obj(
				prop("path", "string", "Directory path to scan"),
				optProp("no_ids", "boolean", "Skip C4 ID computation for speed"),
				optProp("gitignore", "boolean", "Respect .gitignore files when scanning"),
				optProp("exclude", "array", "Glob patterns to exclude (e.g. [\"*.tmp\", \"build/\"])"),
				optProp("pretty", "boolean", "Pretty-print with aligned columns"),
				required("path"),
			),
		},
		{
			Name:        "c4_ls",
			Description: "List the contents of a c4m file. Supports colon syntax for subtree listing (e.g. project.c4m:renders/).",
			InputSchema: obj(
				prop("path", "string", "Path to .c4m file, optionally with colon subpath (e.g. project.c4m:renders/)"),
				optProp("pretty", "boolean", "Pretty-print with aligned columns"),
				required("path"),
			),
		},
		{
			Name:        "c4_diff",
			Description: "Compare two c4m files or directories. Shows added, removed, and modified entries.",
			InputSchema: obj(
				prop("source", "string", "Source path (.c4m file or directory)"),
				prop("target", "string", "Target path (.c4m file or directory)"),
				required("source", "target"),
			),
		},
		{
			Name:        "c4_search",
			Description: "Find files by glob pattern within a c4m file. Returns matching entries.",
			InputSchema: obj(
				prop("manifest", "string", "Path to .c4m file"),
				prop("pattern", "string", "Glob pattern to match (e.g. *.exr, renders/*.png)"),
				required("manifest", "pattern"),
			),
		},
		{
			Name:        "c4_mk",
			Description: "Establish a c4m file for writing. Creates the .c4m.established marker file that enables write operations.",
			InputSchema: obj(
				prop("target", "string", "C4m file path ending with colon (e.g. project.c4m:)"),
				required("target"),
			),
		},
		{
			Name:        "c4_mkdir",
			Description: "Create a directory inside a c4m file. The c4m file must be established first with c4_mk.",
			InputSchema: obj(
				prop("path", "string", "C4m file path with directory (e.g. project.c4m:renders/)"),
				required("path"),
			),
		},
		{
			Name:        "c4_cp",
			Description: "Copy files between local filesystem and c4m files. Capture: local dir -> c4m file. Materialize: c4m file -> local dir.",
			InputSchema: obj(
				prop("source", "string", "Source (local path or file.c4m:subpath)"),
				prop("dest", "string", "Destination (local path or file.c4m:subpath)"),
				required("source", "dest"),
			),
		},
		{
			Name:        "c4_validate",
			Description: "Validate a c4m file for spec compliance. Reports errors, warnings, and statistics.",
			InputSchema: obj(
				prop("path", "string", "Path to .c4m file"),
				required("path"),
			),
		},
	}
}

func callTool(name string, args map[string]any) toolResult {
	switch name {
	case "c4_id":
		return toolID(args)
	case "c4_scan":
		return toolScan(args)
	case "c4_ls":
		return toolLs(args)
	case "c4_diff":
		return toolDiff(args)
	case "c4_search":
		return toolSearch(args)
	case "c4_mk":
		return toolMk(args)
	case "c4_mkdir":
		return toolMkdir(args)
	case "c4_cp":
		return toolCp(args)
	case "c4_validate":
		return toolValidate(args)
	default:
		return toolErr("unknown tool: " + name)
	}
}

// --- Tool implementations ---

func toolID(args map[string]any) toolResult {
	path := str(args, "path")
	if path == "" {
		return toolErr("path is required")
	}
	f, err := os.Open(path)
	if err != nil {
		return toolErr(err.Error())
	}
	defer f.Close()
	return toolOK(c4.Identify(f).String())
}

func toolScan(args map[string]any) toolResult {
	path := str(args, "path")
	if path == "" {
		return toolErr("path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return toolErr(err.Error())
	}
	if !info.IsDir() {
		return toolErr("path must be a directory")
	}

	manifest, err := scanDir(path, !boolean(args, "no_ids"), boolean(args, "gitignore"), strSlice(args, "exclude"))
	if err != nil {
		return toolErr(err.Error())
	}
	return toolOK(encodeManifest(manifest, boolean(args, "pretty")))
}

func toolLs(args map[string]any) toolResult {
	rawPath := str(args, "path")
	if rawPath == "" {
		return toolErr("path is required")
	}

	c4mPath, subPath := parseColonPath(rawPath)
	f, err := os.Open(c4mPath)
	if err != nil {
		return toolErr(err.Error())
	}
	defer f.Close()

	manifest, err := c4m.NewDecoder(f).Decode()
	if err != nil {
		return toolErr(fmt.Sprintf("decode failed: %v", err))
	}
	if subPath != "" {
		manifest = filterBySubPath(manifest, subPath)
	}
	return toolOK(encodeManifest(manifest, boolean(args, "pretty")))
}

func toolDiff(args map[string]any) toolResult {
	srcPath := str(args, "source")
	tgtPath := str(args, "target")
	if srcPath == "" || tgtPath == "" {
		return toolErr("source and target are required")
	}

	src, err := toSource(srcPath)
	if err != nil {
		return toolErr(err.Error())
	}
	tgt, err := toSource(tgtPath)
	if err != nil {
		return toolErr(err.Error())
	}

	result, err := c4m.Diff(src, tgt)
	if err != nil {
		return toolErr(fmt.Sprintf("diff failed: %v", err))
	}
	if result.IsEmpty() {
		return toolOK("No differences found.")
	}

	var lines []string
	if len(result.Added.Entries) > 0 {
		lines = append(lines, "Added:")
		for _, e := range result.Added.Entries {
			lines = append(lines, "  + "+e.Name)
		}
	}
	if len(result.Removed.Entries) > 0 {
		lines = append(lines, "Removed:")
		for _, e := range result.Removed.Entries {
			lines = append(lines, "  - "+e.Name)
		}
	}
	if len(result.Modified.Entries) > 0 {
		lines = append(lines, "Modified:")
		for _, e := range result.Modified.Entries {
			lines = append(lines, "  M "+e.Name)
		}
	}
	return toolOK(strings.Join(lines, "\n"))
}

func toolSearch(args map[string]any) toolResult {
	manifestPath := str(args, "manifest")
	pattern := str(args, "pattern")
	if manifestPath == "" || pattern == "" {
		return toolErr("manifest and pattern are required")
	}

	f, err := os.Open(manifestPath)
	if err != nil {
		return toolErr(err.Error())
	}
	defer f.Close()

	manifest, err := c4m.NewDecoder(f).Decode()
	if err != nil {
		return toolErr(fmt.Sprintf("decode failed: %v", err))
	}

	var dirStack []string
	var lines []string
	for _, entry := range manifest.Entries {
		if entry.Depth < len(dirStack) {
			dirStack = dirStack[:entry.Depth]
		}
		fullPath := strings.Join(dirStack, "") + entry.Name
		if entry.IsDir() {
			for len(dirStack) <= entry.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[entry.Depth] = entry.Name
		}

		matched, _ := matchGlob(pattern, fullPath)
		if !matched {
			continue
		}
		line := fullPath
		if !entry.C4ID.IsNil() {
			line += " " + entry.C4ID.String()
		}
		if entry.Size >= 0 {
			line += fmt.Sprintf(" (%d bytes)", entry.Size)
		}
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return toolOK("No matches found.")
	}
	return toolOK(fmt.Sprintf("%d matches:\n%s", len(lines), strings.Join(lines, "\n")))
}

func toolMk(args map[string]any) toolResult {
	target := str(args, "target")
	if target == "" {
		return toolErr("target is required")
	}
	if !strings.HasSuffix(target, ":") {
		return toolErr("target must end with colon (e.g. project.c4m:)")
	}
	name := strings.TrimSuffix(target, ":")
	if !strings.HasSuffix(name, ".c4m") {
		return toolErr("only c4m file establishment supported (name must end with .c4m)")
	}
	marker := name + ".established"
	if _, err := os.Stat(marker); err == nil {
		return toolOK(target + " already established")
	}
	f, err := os.Create(marker)
	if err != nil {
		return toolErr(err.Error())
	}
	f.Close()
	return toolOK("established " + target)
}

func toolMkdir(args map[string]any) toolResult {
	rawPath := str(args, "path")
	if rawPath == "" {
		return toolErr("path is required")
	}
	c4mPath, subPath := parseColonPath(rawPath)
	if c4mPath == rawPath || subPath == "" {
		return toolErr("path must be file.c4m:directory/ (e.g. project.c4m:renders/)")
	}
	if !strings.HasSuffix(subPath, "/") {
		subPath += "/"
	}
	if _, err := os.Stat(c4mPath + ".established"); err != nil {
		return toolErr(c4mPath + " is not established — run c4_mk first")
	}

	manifest, err := loadOrCreate(c4mPath)
	if err != nil {
		return toolErr(err.Error())
	}
	ensureParents(manifest, subPath)
	manifest.SortEntries()
	if err := saveManifest(c4mPath, manifest); err != nil {
		return toolErr(err.Error())
	}
	return toolOK(fmt.Sprintf("created %s:%s", c4mPath, subPath))
}

func toolCp(args map[string]any) toolResult {
	srcStr := str(args, "source")
	dstStr := str(args, "dest")
	if srcStr == "" || dstStr == "" {
		return toolErr("source and dest are required")
	}
	srcC4m, srcSub := parseColonPath(srcStr)
	dstC4m, dstSub := parseColonPath(dstStr)
	srcIsCapsule := srcC4m != srcStr && strings.HasSuffix(srcC4m, ".c4m")
	dstIsCapsule := dstC4m != dstStr && strings.HasSuffix(dstC4m, ".c4m")

	switch {
	case !srcIsCapsule && dstIsCapsule:
		return cpLocalToCapsule(srcStr, dstC4m, dstSub)
	case srcIsCapsule && !dstIsCapsule:
		return cpCapsuleToLocal(srcC4m, srcSub, dstStr)
	case !srcIsCapsule && !dstIsCapsule:
		return toolErr("use OS cp for local-to-local copies")
	default:
		return toolErr("c4m-to-c4m copy not yet supported")
	}
}

func toolValidate(args map[string]any) toolResult {
	path := str(args, "path")
	if path == "" {
		return toolErr("path is required")
	}
	f, err := os.Open(path)
	if err != nil {
		return toolErr(err.Error())
	}
	defer f.Close()

	validator := c4m.NewValidator(true)
	validationErr := validator.ValidateManifest(f)

	stats := validator.GetStats()
	var lines []string
	lines = append(lines, fmt.Sprintf("Entries: %d (files: %d, dirs: %d)", stats.TotalEntries, stats.Files, stats.Directories))
	lines = append(lines, fmt.Sprintf("Size: %d bytes", stats.TotalSize))
	lines = append(lines, fmt.Sprintf("Max depth: %d", stats.MaxDepth))

	if warnings := validator.GetWarnings(); len(warnings) > 0 {
		lines = append(lines, fmt.Sprintf("\nWarnings (%d):", len(warnings)))
		for _, w := range warnings {
			lines = append(lines, "  "+w.Error())
		}
	}
	if validationErr != nil {
		errors := validator.GetErrors()
		lines = append(lines, fmt.Sprintf("\nErrors (%d):", len(errors)))
		for _, e := range errors {
			lines = append(lines, "  "+e.Error())
		}
		lines = append(lines, "\nValidation FAILED")
		return toolOK(strings.Join(lines, "\n"))
	}
	lines = append(lines, "\nValidation PASSED")
	return toolOK(strings.Join(lines, "\n"))
}

// --- Directory scanner ---

func scanDir(root string, computeIDs, respectGitignore bool, excludePatterns []string) (*c4m.Manifest, error) {
	manifest := c4m.NewManifest()
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	var gi *gitignoreState
	if respectGitignore {
		gi = newGitignoreState(absRoot)
	}

	if err := walkDir(manifest, absRoot, "", 0, computeIDs, gi, excludePatterns); err != nil {
		return nil, err
	}
	manifest.SortEntries()
	propagateMetadata(manifest.Entries)
	return manifest, nil
}

// gitignoreState tracks .gitignore rules for the MCP scanner.
type gitignoreState struct {
	layers []giLayer
}

type giLayer struct {
	basePath string
	rules    []giRule
}

type giRule struct {
	pattern  string
	negated  bool
	dirOnly  bool
	anchored bool
}

func newGitignoreState(rootDir string) *gitignoreState {
	gs := &gitignoreState{}
	gs.loadDir(rootDir)
	return gs
}

func (gs *gitignoreState) loadDir(dir string) {
	path := filepath.Join(dir, ".gitignore")
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	var rules []giRule
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), " \t")
		if line == "" || line[0] == '#' {
			continue
		}
		var rule giRule
		if line[0] == '!' {
			rule.negated = true
			line = line[1:]
		}
		if len(line) > 0 && line[0] == '\\' {
			line = line[1:]
		}
		if strings.HasSuffix(line, "/") {
			rule.dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		if strings.HasPrefix(line, "/") {
			rule.anchored = true
			line = line[1:]
		}
		if strings.Contains(line, "/") {
			rule.anchored = true
		}
		rule.pattern = line
		if rule.pattern != "" {
			rules = append(rules, rule)
		}
	}
	if len(rules) > 0 {
		gs.layers = append(gs.layers, giLayer{basePath: dir, rules: rules})
	}
}

func (gs *gitignoreState) popTo(dir string) {
	n := 0
	for _, layer := range gs.layers {
		if dir == layer.basePath || strings.HasPrefix(dir, layer.basePath+string(filepath.Separator)) {
			gs.layers[n] = layer
			n++
		}
	}
	gs.layers = gs.layers[:n]
}

func (gs *gitignoreState) match(fullPath, name string, isDir bool) bool {
	ignored := false
	for _, layer := range gs.layers {
		rel, err := filepath.Rel(layer.basePath, fullPath)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		for _, rule := range layer.rules {
			if rule.dirOnly && !isDir {
				continue
			}
			if giMatchPattern(rule.pattern, rel, name, rule.anchored) {
				ignored = !rule.negated
			}
		}
	}
	return ignored
}

func giMatchPattern(pattern, rel, name string, anchored bool) bool {
	if strings.Contains(pattern, "**") {
		return giMatchDoublestar(pattern, rel)
	}
	if anchored {
		return giMatchGlob(pattern, rel)
	}
	if giMatchGlob(pattern, name) {
		return true
	}
	parts := strings.Split(rel, "/")
	for i := range parts {
		if giMatchGlob(pattern, strings.Join(parts[i:], "/")) {
			return true
		}
	}
	return false
}

func giMatchDoublestar(pattern, path string) bool {
	parts := strings.SplitN(pattern, "**", 2)
	prefix := strings.TrimSuffix(parts[0], "/")
	suffix := ""
	if len(parts) > 1 {
		suffix = strings.TrimPrefix(parts[1], "/")
	}
	pathParts := strings.Split(path, "/")
	if prefix == "" && suffix == "" {
		return true
	}
	if prefix == "" {
		for i := range pathParts {
			if giMatchGlob(suffix, strings.Join(pathParts[i:], "/")) {
				return true
			}
		}
		return false
	}
	if suffix == "" {
		for i := 1; i <= len(pathParts); i++ {
			if giMatchGlob(prefix, strings.Join(pathParts[:i], "/")) {
				return true
			}
		}
		return false
	}
	for i := 1; i <= len(pathParts); i++ {
		if giMatchGlob(prefix, strings.Join(pathParts[:i], "/")) {
			for j := i; j <= len(pathParts); j++ {
				if giMatchGlob(suffix, strings.Join(pathParts[j:], "/")) {
					return true
				}
			}
		}
	}
	return false
}

func giMatchGlob(pattern, str string) bool {
	for len(pattern) > 0 {
		switch pattern[0] {
		case '*':
			for len(pattern) > 0 && pattern[0] == '*' {
				pattern = pattern[1:]
			}
			if len(pattern) == 0 {
				return true
			}
			for i := 0; i <= len(str); i++ {
				if giMatchGlob(pattern, str[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(str) == 0 {
				return false
			}
			pattern = pattern[1:]
			str = str[1:]
		case '[':
			if len(str) == 0 {
				return false
			}
			end := strings.IndexByte(pattern, ']')
			if end < 0 {
				if str[0] != pattern[0] {
					return false
				}
				pattern = pattern[1:]
				str = str[1:]
				continue
			}
			class := pattern[1:end]
			negate := len(class) > 0 && class[0] == '!'
			if negate {
				class = class[1:]
			}
			matched := false
			for i := 0; i < len(class); i++ {
				if i+2 < len(class) && class[i+1] == '-' {
					if str[0] >= class[i] && str[0] <= class[i+2] {
						matched = true
					}
					i += 2
				} else if class[i] == str[0] {
					matched = true
				}
			}
			if negate {
				matched = !matched
			}
			if !matched {
				return false
			}
			pattern = pattern[end+1:]
			str = str[1:]
		case '\\':
			pattern = pattern[1:]
			if len(pattern) == 0 || len(str) == 0 || str[0] != pattern[0] {
				return false
			}
			pattern = pattern[1:]
			str = str[1:]
		default:
			if len(str) == 0 || str[0] != pattern[0] {
				return false
			}
			pattern = pattern[1:]
			str = str[1:]
		}
	}
	return len(str) == 0
}

func walkDir(manifest *c4m.Manifest, dirPath, dirName string, depth int, computeIDs bool, gi *gitignoreState, excludePatterns []string) error {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return err
	}

	childDepth := depth
	if dirName != "" {
		info, err := os.Stat(dirPath)
		if err != nil {
			return err
		}
		dirEntry := &c4m.Entry{
			Name:      dirName + "/",
			Depth:     depth,
			Mode:      info.Mode(),
			Size:      -1,
			Timestamp: info.ModTime().UTC(),
		}
		if computeIDs {
			if sub, err := scanDir(dirPath, true, gi != nil, excludePatterns); err == nil {
				dirEntry.C4ID = sub.ComputeC4ID()
			}
		}
		manifest.AddEntry(dirEntry)
		childDepth = depth + 1
	}

	// Load gitignore for this directory
	if gi != nil {
		gi.popTo(dirPath)
		gi.loadDir(dirPath)
	}

	for _, de := range entries {
		name := de.Name()

		// Always skip .git directory
		if name == ".git" {
			continue
		}

		if strings.HasPrefix(name, ".") {
			continue
		}
		fullPath := filepath.Join(dirPath, name)

		// Check gitignore
		if gi != nil && gi.match(fullPath, name, de.IsDir()) {
			continue
		}

		// Check exclude patterns
		if matchesExclude(name, excludePatterns) {
			continue
		}

		info, err := de.Info()
		if err != nil {
			continue
		}
		if info.IsDir() {
			if err := walkDir(manifest, fullPath, name, childDepth, computeIDs, gi, excludePatterns); err != nil {
				return err
			}
			continue
		}
		entry := &c4m.Entry{
			Name:      name,
			Depth:     childDepth,
			Mode:      info.Mode(),
			Size:      info.Size(),
			Timestamp: info.ModTime().UTC(),
		}
		if computeIDs && info.Mode().IsRegular() {
			if f, err := os.Open(fullPath); err == nil {
				entry.C4ID = c4.Identify(f)
				f.Close()
			}
		}
		manifest.AddEntry(entry)
	}
	return nil
}

func propagateMetadata(entries []*c4m.Entry) {
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if !e.IsDir() {
			continue
		}
		if e.Size < 0 {
			var size int64
			for _, child := range directChildren(entries, i) {
				if child.Size > 0 {
					size += child.Size
				}
			}
			e.Size = size
		}
		if e.Timestamp.IsZero() || e.Timestamp.Unix() == 0 {
			var newest time.Time
			for _, child := range directChildren(entries, i) {
				if child.Timestamp.After(newest) {
					newest = child.Timestamp
				}
			}
			if !newest.IsZero() {
				e.Timestamp = newest
			}
		}
	}
}

func directChildren(entries []*c4m.Entry, dirIdx int) []*c4m.Entry {
	dir := entries[dirIdx]
	var children []*c4m.Entry
	for j := dirIdx + 1; j < len(entries); j++ {
		if entries[j].Depth <= dir.Depth {
			break
		}
		if entries[j].Depth == dir.Depth+1 {
			children = append(children, entries[j])
		}
	}
	return children
}

// --- Copy operations ---

func cpLocalToCapsule(srcPath, c4mPath, subPath string) toolResult {
	if _, err := os.Stat(c4mPath + ".established"); err != nil {
		return toolErr(c4mPath + " is not established — run c4_mk first")
	}
	info, err := os.Stat(srcPath)
	if err != nil {
		return toolErr(err.Error())
	}
	manifest, err := loadOrCreate(c4mPath)
	if err != nil {
		return toolErr(err.Error())
	}

	var added int
	if info.IsDir() {
		scanned, err := scanDir(srcPath, true, true, nil)
		if err != nil {
			return toolErr(err.Error())
		}
		prefixDepth := 0
		if subPath != "" {
			prefixDepth = strings.Count(strings.TrimSuffix(subPath, "/"), "/") + 1
			ensureParents(manifest, subPath)
		}
		for _, entry := range scanned.Entries {
			e := *entry
			e.Depth += prefixDepth
			manifest.AddEntry(&e)
			added++
		}
	} else {
		f, err := os.Open(srcPath)
		if err != nil {
			return toolErr(err.Error())
		}
		id := c4.Identify(f)
		f.Close()
		depth := 0
		if subPath != "" {
			ensureParents(manifest, subPath)
			depth = strings.Count(strings.TrimSuffix(subPath, "/"), "/") + 1
		}
		manifest.AddEntry(&c4m.Entry{
			Name:      filepath.Base(srcPath),
			Depth:     depth,
			Mode:      info.Mode(),
			Size:      info.Size(),
			Timestamp: info.ModTime().UTC(),
			C4ID:      id,
		})
		added = 1
	}

	manifest.SortEntries()
	propagateMetadata(manifest.Entries)
	if err := saveManifest(c4mPath, manifest); err != nil {
		return toolErr(err.Error())
	}
	return toolOK(fmt.Sprintf("captured %d entries into %s", added, c4mPath))
}

func cpCapsuleToLocal(c4mPath, subPath, destPath string) toolResult {
	f, err := os.Open(c4mPath)
	if err != nil {
		return toolErr(err.Error())
	}
	defer f.Close()
	manifest, err := c4m.NewDecoder(f).Decode()
	if err != nil {
		return toolErr(err.Error())
	}

	type resolved struct {
		path  string
		entry *c4m.Entry
	}
	var items []resolved
	var dirStack []string
	for _, entry := range manifest.Entries {
		if entry.Depth < len(dirStack) {
			dirStack = dirStack[:entry.Depth]
		}
		fullPath := strings.Join(dirStack, "") + entry.Name
		items = append(items, resolved{path: fullPath, entry: entry})
		if entry.IsDir() {
			for len(dirStack) <= entry.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[entry.Depth] = entry.Name
		}
	}

	if subPath != "" {
		var filtered []resolved
		for _, item := range items {
			if strings.HasPrefix(item.path, subPath) {
				item.path = strings.TrimPrefix(item.path, subPath)
				if item.path != "" {
					filtered = append(filtered, item)
				}
			}
		}
		items = filtered
	}
	if len(items) == 0 {
		return toolErr("no entries match " + c4mPath + ":" + subPath)
	}

	if err := os.MkdirAll(destPath, 0755); err != nil {
		return toolErr(err.Error())
	}
	created := 0
	for _, item := range items {
		full := filepath.Join(destPath, item.path)
		if item.entry.IsDir() {
			os.MkdirAll(full, item.entry.Mode.Perm()|0755)
		} else {
			os.MkdirAll(filepath.Dir(full), 0755)
			writeStubFile(full, item.entry)
		}
		created++
	}
	return toolOK(fmt.Sprintf("materialized %d entries to %s", created, destPath))
}

func writeStubFile(path string, entry *c4m.Entry) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	if !entry.C4ID.IsNil() {
		fmt.Fprintf(f, "# c4 stub: content available via c4d\n# C4ID: %s\n# Size: %d\n", entry.C4ID, entry.Size)
	}
}

// --- Helpers ---

func parseColonPath(s string) (string, string) {
	// Find .c4m: in the string — handles both relative and absolute paths
	if idx := strings.Index(s, ".c4m:"); idx >= 0 {
		c4mPath := s[:idx+4]
		subPath := s[idx+5:]
		return c4mPath, strings.TrimPrefix(subPath, "/")
	}
	return s, ""
}

func matchGlob(pattern, name string) (bool, error) {
	if matched, err := filepath.Match(pattern, name); matched || err != nil {
		return matched, err
	}
	if !strings.Contains(pattern, "/") {
		base := name
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			base = name[idx+1:]
		}
		return filepath.Match(pattern, base)
	}
	return false, nil
}

func filterBySubPath(manifest *c4m.Manifest, subPath string) *c4m.Manifest {
	var dirStack []string
	result := c4m.NewManifest()
	for _, entry := range manifest.Entries {
		if entry.Depth < len(dirStack) {
			dirStack = dirStack[:entry.Depth]
		}
		fullPath := strings.Join(dirStack, "") + entry.Name
		if strings.HasPrefix(fullPath, subPath) {
			result.AddEntry(entry)
		}
		if entry.IsDir() {
			for len(dirStack) <= entry.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[entry.Depth] = entry.Name
		}
	}
	return result
}

func toSource(path string) (c4m.Source, error) {
	if strings.HasSuffix(path, ".c4m") {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		manifest, err := c4m.NewDecoder(f).Decode()
		if err != nil {
			return nil, err
		}
		return c4m.ManifestSource{Manifest: manifest}, nil
	}
	manifest, err := scanDir(path, true, true, nil)
	if err != nil {
		return nil, err
	}
	return c4m.ManifestSource{Manifest: manifest}, nil
}

func loadOrCreate(path string) (*c4m.Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c4m.NewManifest(), nil
		}
		return nil, err
	}
	defer f.Close()
	return c4m.NewDecoder(f).Decode()
}

func saveManifest(path string, m *c4m.Manifest) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return c4m.NewEncoder(f).Encode(m)
}

func encodeManifest(m *c4m.Manifest, pretty bool) string {
	var buf bytes.Buffer
	enc := c4m.NewEncoder(&buf)
	if pretty {
		enc.SetPretty(true)
	}
	enc.Encode(m)
	return buf.String()
}

func ensureParents(manifest *c4m.Manifest, path string) {
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	for i, part := range parts {
		name := part + "/"
		found := false
		for _, e := range manifest.Entries {
			if e.Name == name && e.Depth == i {
				found = true
				break
			}
		}
		if !found {
			manifest.AddEntry(&c4m.Entry{
				Name:  name,
				Depth: i,
				Mode:  os.ModeDir | 0755,
				Size:  -1,
			})
		}
	}
}

// --- JSON Schema helpers ---

func obj(fields ...map[string]any) map[string]any {
	result := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
	for _, f := range fields {
		for k, v := range f {
			if k == "required" {
				result["required"] = v
			} else {
				result["properties"].(map[string]any)[k] = v
			}
		}
	}
	return result
}

func prop(name, typ, desc string) map[string]any {
	return map[string]any{name: map[string]any{"type": typ, "description": desc}}
}

func optProp(name, typ, desc string) map[string]any {
	return prop(name, typ, desc)
}

func required(names ...string) map[string]any {
	return map[string]any{"required": names}
}

func str(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}

func boolean(args map[string]any, key string) bool {
	v, _ := args[key].(bool)
	return v
}

func strSlice(args map[string]any, key string) []string {
	v, ok := args[key].([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(v))
	for _, item := range v {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func matchesExclude(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
	}
	return false
}

func toolOK(text string) toolResult {
	return toolResult{Content: []textContent{{Type: "text", Text: text}}}
}

func toolErr(msg string) toolResult {
	return toolResult{Content: []textContent{{Type: "text", Text: "Error: " + msg}}, IsError: true}
}
