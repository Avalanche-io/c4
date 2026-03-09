package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/establish"
	"github.com/Avalanche-io/c4/cmd/c4/internal/managed"
	"github.com/Avalanche-io/c4/cmd/c4/internal/pathspec"
	"github.com/Avalanche-io/c4/cmd/c4/internal/scan"
)

// runLn implements "c4 ln" — create links in a c4m file or managed directory.
//
//	c4 ln project.c4m:master.exr project.c4m:backup.exr     # hard link
//	c4 ln -s ../shared/config.yaml project.c4m:config.yaml   # symlink
//	c4 ln :~2 :~release-v1                                   # tag snapshot
//	c4 ln -> project.c4m:footage/ nas:                        # outbound flow
//	c4 ln <- :incoming/ studio:dailies/                      # inbound flow
//	c4 ln '<>' :projects/ desktop:                           # bidirectional flow
func runLn(args []string) {
	// Check for flow direction as first argument: ->, <-, <>
	if len(args) >= 1 {
		if dir := parseFlowDirection(args[0]); dir != c4m.FlowNone {
			lnFlow(dir, args[1:])
			return
		}
	}

	// Check for -s flag (symlink)
	var symlink bool
	var filtered []string
	for _, a := range args {
		if a == "-s" {
			symlink = true
		} else {
			filtered = append(filtered, a)
		}
	}

	// Check for managed directory tag creation: c4 ln :~N :~name
	if !symlink && len(filtered) == 2 {
		isLoc := establish.IsLocationEstablished
		src, serr := pathspec.Parse(filtered[0], isLoc)
		dst, derr := pathspec.Parse(filtered[1], isLoc)
		if serr == nil && derr == nil && src.Type == pathspec.Managed && dst.Type == pathspec.Managed {
			if strings.HasPrefix(src.SubPath, "~") && strings.HasPrefix(dst.SubPath, "~") {
				lnTag(src.SubPath[1:], dst.SubPath[1:])
				return
			}
		}
	}

	if symlink {
		lnSymlink(filtered)
	} else {
		lnHard(filtered)
	}
}

// parseFlowDirection checks if a string is a flow direction operator.
func parseFlowDirection(s string) c4m.FlowDirection {
	switch s {
	case "->":
		return c4m.FlowOutbound
	case "<-":
		return c4m.FlowInbound
	case "<>":
		return c4m.FlowBidirectional
	default:
		return c4m.FlowNone
	}
}

// lnFlow creates a flow link on an entry in a c4m file or managed directory.
//
// Syntax: c4 ln DIRECTION LOCAL_TARGET REMOTE_REF
//
//	c4 ln -> project.c4m:footage/ nas:
//	c4 ln <- :incoming/ studio:dailies/
//	c4 ln '<>' :projects/ desktop:
func lnFlow(dir c4m.FlowDirection, args []string) {
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 ln <direction> <local-target> <location:path>\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  c4 ln -> project.c4m:footage/ nas:\n")
		fmt.Fprintf(os.Stderr, "  c4 ln <- :incoming/ studio:dailies/\n")
		fmt.Fprintf(os.Stderr, "  c4 ln '<>' :projects/ desktop:\n")
		os.Exit(1)
	}

	localSpec := args[0]  // c4m path or managed dir path
	remoteRef := args[1]  // location:path (e.g., "nas:", "nas:raw/")

	// Validate remote reference contains ":"
	colonIdx := strings.IndexByte(remoteRef, ':')
	if colonIdx < 0 {
		fmt.Fprintf(os.Stderr, "Error: remote reference must contain ':' (e.g., nas: or studio:path/)\n")
		os.Exit(1)
	}
	locName := remoteRef[:colonIdx]
	if locName == "" {
		fmt.Fprintf(os.Stderr, "Error: location name must not be empty\n")
		os.Exit(1)
	}

	// Validate location name format: [a-zA-Z][a-zA-Z0-9_-]*
	if !isValidLocationName(locName) {
		fmt.Fprintf(os.Stderr, "Error: invalid location name %q (must start with letter, contain only letters, digits, - and _)\n", locName)
		os.Exit(1)
	}

	// The flow target is the full remote reference string
	flowTarget := remoteRef

	// Parse local target
	isLoc := establish.IsLocationEstablished
	spec, err := pathspec.Parse(localSpec, isLoc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	switch spec.Type {
	case pathspec.C4m:
		lnFlowC4m(dir, flowTarget, spec)
	case pathspec.Managed:
		lnFlowManaged(dir, flowTarget, spec)
	default:
		fmt.Fprintf(os.Stderr, "Error: flow links require a c4m path or managed directory target\n")
		os.Exit(1)
	}
}

// lnFlowC4m sets a flow link on an entry in a c4m file.
func lnFlowC4m(dir c4m.FlowDirection, flowTarget string, spec pathspec.PathSpec) {
	if spec.SubPath == "" {
		fmt.Fprintf(os.Stderr, "Error: must specify a path within the c4m (e.g., project.c4m:footage/)\n")
		os.Exit(1)
	}

	if !establish.IsC4mEstablished(spec.Source) {
		fmt.Fprintf(os.Stderr, "Error: %s: is not established for writing\n", spec.Source)
		fmt.Fprintf(os.Stderr, "Run: c4 mk %s:\n", spec.Source)
		os.Exit(1)
	}

	unlock, err := lockC4mFile(spec.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error locking %s: %v\n", spec.Source, err)
		os.Exit(1)
	}
	defer unlock()

	manifest, err := loadOrCreateManifest(spec.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	entry := findEntry(manifest, spec.SubPath)
	if entry == nil {
		fmt.Fprintf(os.Stderr, "Error: %s not found in %s\n", spec.SubPath, spec.Source)
		os.Exit(1)
	}

	entry.FlowDirection = dir
	entry.FlowTarget = flowTarget

	manifest.SortEntries()
	scan.PropagateMetadata(manifest.Entries)

	if err := writeManifest(spec.Source, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	opStr := entry.FlowOperator()
	fmt.Printf("flow %s %s on %s:%s\n", opStr, flowTarget, spec.Source, spec.SubPath)
}

// lnFlowManaged sets a flow link on an entry in a managed directory.
func lnFlowManaged(dir c4m.FlowDirection, flowTarget string, spec pathspec.PathSpec) {
	// Managed directory flow links require c4d integration (flow declarations
	// are metadata that don't exist on the filesystem). For now, flow links
	// are supported on c4m file entries.
	fmt.Fprintf(os.Stderr, "Error: flow links on managed directories require c4d (not yet supported)\n")
	fmt.Fprintf(os.Stderr, "Use a c4m file instead: c4 ln %s project.c4m:path/ %s\n",
		(&c4m.Entry{FlowDirection: dir}).FlowOperator(), flowTarget)
	os.Exit(1)
}

// isValidLocationName checks if a string is a valid location name.
// Must start with a letter and contain only letters, digits, hyphens, and underscores.
func isValidLocationName(name string) bool {
	if len(name) == 0 {
		return false
	}
	ch := name[0]
	if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')) {
		return false
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// lnTag creates a named tag for a managed directory snapshot.
// srcRef is the snapshot reference (a number like "2"), dstRef is the tag name.
func lnTag(srcRef, dstRef string) {
	if srcRef == "" || dstRef == "" {
		fmt.Fprintf(os.Stderr, "Usage: c4 ln :~<snapshot> :~<tag-name>\n")
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  c4 ln :~2 :~release-v1\n")
		os.Exit(1)
	}

	n, err := strconv.Atoi(srcRef)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: source must be a snapshot number, got %q\n", srcRef)
		os.Exit(1)
	}

	d, err := managed.Open(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	history, err := d.History()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if n < 0 || n >= len(history) {
		fmt.Fprintf(os.Stderr, "Error: snapshot ~%d does not exist (history has %d entries)\n", n, len(history))
		os.Exit(1)
	}

	c4id := history[n].ID
	if err := d.SetTag(dstRef, c4id); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("tagged :~%d → :~%s\n", n, dstRef)
}

// lnSymlink creates a symbolic link entry in a c4m file.
func lnSymlink(args []string) {
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 ln -s <target-path> <link-location>\n")
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  c4 ln -s ../shared/config.yaml project.c4m:config.yaml\n")
		os.Exit(1)
	}

	target := args[0] // symlink target path (literal string, not a pathspec)

	isLoc := establish.IsLocationEstablished
	link, err := pathspec.Parse(args[1], isLoc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if link.Type != pathspec.C4m {
		fmt.Fprintf(os.Stderr, "Error: link location must be a c4m path\n")
		os.Exit(1)
	}
	if link.SubPath == "" {
		fmt.Fprintf(os.Stderr, "Error: must specify a path within the c4m\n")
		os.Exit(1)
	}

	if !establish.IsC4mEstablished(link.Source) {
		fmt.Fprintf(os.Stderr, "Error: %s: is not established for writing\n", link.Source)
		fmt.Fprintf(os.Stderr, "Run: c4 mk %s:\n", link.Source)
		os.Exit(1)
	}

	unlock, err := lockC4mFile(link.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error locking %s: %v\n", link.Source, err)
		os.Exit(1)
	}
	defer unlock()

	manifest, err := loadOrCreateManifest(link.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Parse link path for depth and name
	parts := strings.Split(link.SubPath, "/")
	// Remove trailing empty string from trailing slash
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	depth := len(parts) - 1
	name := parts[len(parts)-1]

	// Ensure parent directories exist
	if depth > 0 {
		parentPath := strings.Join(parts[:len(parts)-1], "/") + "/"
		ensureParentDirs(manifest, parentPath)
	}

	entry := &c4m.Entry{
		Name:      name,
		Depth:     depth,
		Mode:      os.ModeSymlink | 0777,
		Timestamp: c4m.NullTimestamp(),
		Target:    target,
		Size:      -1,
	}

	manifest.AddEntry(entry)
	manifest.SortEntries()
	scan.PropagateMetadata(manifest.Entries)

	if err := writeManifest(link.Source, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("created symlink %s:%s → %s\n", link.Source, link.SubPath, target)
}

// lnHard creates a hard link — a new entry sharing the same C4 ID as an existing entry.
func lnHard(args []string) {
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 ln <source> <link-name>\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  c4 ln project.c4m:master.exr project.c4m:backup.exr\n")
		os.Exit(1)
	}

	isLoc := establish.IsLocationEstablished
	src, err := pathspec.Parse(args[0], isLoc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	dst, err := pathspec.Parse(args[1], isLoc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if src.Type != pathspec.C4m || dst.Type != pathspec.C4m {
		fmt.Fprintf(os.Stderr, "Error: ln currently supports c4m paths only\n")
		os.Exit(1)
	}
	if src.Source != dst.Source {
		fmt.Fprintf(os.Stderr, "Error: ln across different c4m files not yet supported\n")
		os.Exit(1)
	}
	if src.SubPath == "" || dst.SubPath == "" {
		fmt.Fprintf(os.Stderr, "Error: must specify paths within the c4m\n")
		os.Exit(1)
	}

	if !establish.IsC4mEstablished(src.Source) {
		fmt.Fprintf(os.Stderr, "Error: %s: is not established for writing\n", src.Source)
		fmt.Fprintf(os.Stderr, "Run: c4 mk %s:\n", src.Source)
		os.Exit(1)
	}

	unlock, err := lockC4mFile(src.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error locking %s: %v\n", src.Source, err)
		os.Exit(1)
	}
	defer unlock()

	manifest, err := loadManifest(src.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Find source entry
	srcEntry := findEntry(manifest, src.SubPath)
	if srcEntry == nil {
		fmt.Fprintf(os.Stderr, "Error: %s not found in %s\n", src.SubPath, src.Source)
		os.Exit(1)
	}
	if srcEntry.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: cannot hard link directories\n")
		os.Exit(1)
	}

	// Parse destination path
	parts := strings.Split(dst.SubPath, "/")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	depth := len(parts) - 1
	name := parts[len(parts)-1]

	// Ensure parent directories exist
	if depth > 0 {
		parentPath := strings.Join(parts[:len(parts)-1], "/") + "/"
		ensureParentDirs(manifest, parentPath)
	}

	// Mark source as hard-linked if not already
	if srcEntry.HardLink == 0 {
		srcEntry.HardLink = -1 // ungrouped hard link
	}

	// Create new entry with same content identity and hard link marker
	newEntry := &c4m.Entry{
		Name:      name,
		Depth:     depth,
		Mode:      srcEntry.Mode,
		Timestamp: srcEntry.Timestamp,
		Size:      srcEntry.Size,
		C4ID:      srcEntry.C4ID,
		HardLink:  srcEntry.HardLink,
	}

	manifest.AddEntry(newEntry)
	manifest.SortEntries()
	scan.PropagateMetadata(manifest.Entries)

	if err := writeManifest(src.Source, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("linked %s:%s → %s:%s\n", src.Source, src.SubPath, dst.Source, dst.SubPath)
}

// findEntry locates an entry by its full path within a manifest.
func findEntry(manifest *c4m.Manifest, subPath string) *c4m.Entry {
	var dirStack []string
	for _, entry := range manifest.Entries {
		if entry.Depth < len(dirStack) {
			dirStack = dirStack[:entry.Depth]
		}
		var fullPath string
		if len(dirStack) > 0 {
			fullPath = strings.Join(dirStack, "") + entry.Name
		} else {
			fullPath = entry.Name
		}
		if fullPath == subPath {
			return entry
		}
		if entry.IsDir() {
			for len(dirStack) <= entry.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[entry.Depth] = entry.Name
		}
	}
	return nil
}
