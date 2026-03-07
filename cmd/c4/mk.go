package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/establish"
	"github.com/Avalanche-io/c4/cmd/c4/internal/managed"
	"github.com/Avalanche-io/c4/cmd/c4/internal/pathspec"
	"github.com/Avalanche-io/c4/cmd/c4/internal/scan"
)

// runMk implements "c4 mk" — establish a c4m file or location for writing.
//
//	c4 mk project.c4m:                    # c4m file
//	c4 mk studio: cloud.example.com:7433  # location
func runMk(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  c4 mk <name>.c4m:              # establish c4m file for writing\n")
		fmt.Fprintf(os.Stderr, "  c4 mk <name>: <host:port>      # establish location for writing\n")
		os.Exit(1)
	}

	target := args[0]

	// Bare colon = managed directory
	if target == ":" {
		// Collect --exclude flags
		var excludes []string
		for i := 1; i < len(args); i++ {
			if args[i] == "--exclude" && i+1 < len(args) {
				excludes = append(excludes, args[i+1])
				i++
			}
		}

		if managed.IsManaged(".") {
			if len(excludes) > 0 {
				d, err := managed.Open(".")
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				if err := d.AddIgnorePatterns(excludes); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				fmt.Println("added exclude patterns")
			} else {
				fmt.Fprintf(os.Stderr, ": already established\n")
			}
			os.Exit(0)
		}

		d, err := managed.Init(".", excludes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		n, _ := d.HistoryLen()
		m, _ := d.Current()
		fmt.Printf("established : (%d entries, snapshot 0)\n", len(m.Entries))
		_ = n
		return
	}

	// Must end with colon
	if !strings.HasSuffix(target, ":") {
		fmt.Fprintf(os.Stderr, "Error: target must end with colon (e.g. project.c4m: or studio: or :)\n")
		os.Exit(1)
	}

	name := strings.TrimSuffix(target, ":")

	if strings.HasSuffix(name, ".c4m") {
		// c4m file establishment
		if establish.IsC4mEstablished(name) {
			fmt.Fprintf(os.Stderr, "%s already established\n", target)
			os.Exit(0)
		}
		if err := establish.EstablishC4m(name); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("established %s\n", target)
	} else {
		// Location establishment — requires address argument
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: location requires address argument\n")
			fmt.Fprintf(os.Stderr, "Usage: c4 mk %s <host:port>\n", target)
			os.Exit(1)
		}
		address := args[1]
		if establish.IsLocationEstablished(name) {
			existing := establish.GetLocation(name)
			if existing != nil && existing.Address == address {
				fmt.Fprintf(os.Stderr, "%s already established at %s\n", target, address)
				os.Exit(0)
			}
			// Update address
		}
		if err := establish.EstablishLocation(name, address); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("established %s → %s\n", target, address)
	}
}

// runRm implements "c4 rm" — remove entries, registrations, or tracking.
//
//	c4 rm studio:                   # remove location
//	c4 rm project.c4m:              # remove c4m file establishment
//	c4 rm project.c4m:renders/old/  # remove entry from c4m
//	c4 rm :                         # stop tracking, remove history
//	c4 rm :~.ignore/data/           # remove ignore pattern
//	c4 rm :~tagname                 # remove tag
func runRm(args []string) {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: c4 rm <target>\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  c4 rm studio:                  # remove location\n")
		fmt.Fprintf(os.Stderr, "  c4 rm project.c4m:renders/     # remove entry from c4m\n")
		fmt.Fprintf(os.Stderr, "  c4 rm :                        # stop tracking\n")
		fmt.Fprintf(os.Stderr, "  c4 rm :~.ignore/data/          # remove ignore pattern\n")
		fmt.Fprintf(os.Stderr, "  c4 rm :~tagname                # remove tag\n")
		os.Exit(1)
	}

	target := args[0]

	spec, err := pathspec.Parse(target, establish.IsLocationEstablished)
	if err != nil {
		// Fall back to legacy colon-suffix handling for bare "name:"
		if strings.HasSuffix(target, ":") {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	switch spec.Type {
	case pathspec.Managed:
		rmManaged(spec)

	case pathspec.C4m:
		if spec.SubPath != "" {
			rmC4mEntry(spec)
		} else {
			// Bare c4m: → remove establishment
			if !establish.IsC4mEstablished(spec.Source) {
				fmt.Fprintf(os.Stderr, "%s: is not established\n", spec.Source)
				os.Exit(1)
			}
			if err := establish.RemoveC4mEstablishment(spec.Source); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("removed establishment for %s:\n", spec.Source)
		}

	case pathspec.Location:
		if !establish.IsLocationEstablished(spec.Source) {
			fmt.Fprintf(os.Stderr, "%s is not a known location\n", spec.Source)
			os.Exit(1)
		}
		if err := establish.RemoveLocation(spec.Source); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("removed %s:\n", spec.Source)

	default:
		fmt.Fprintf(os.Stderr, "Error: c4 rm does not support %s paths\n", spec.Type)
		os.Exit(1)
	}
}

// rmManaged handles c4 rm for managed directory targets.
func rmManaged(spec pathspec.PathSpec) {
	// Bare : → tear down tracking
	if spec.SubPath == "" {
		if !managed.IsManaged(".") {
			fmt.Fprintf(os.Stderr, "Error: not a managed directory\n")
			os.Exit(1)
		}
		d, err := managed.Open(".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := d.Teardown(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("removed tracking for :")
		return
	}

	// :~.ignore/pattern → remove ignore pattern
	if strings.HasPrefix(spec.SubPath, "~.ignore/") {
		pattern := strings.TrimPrefix(spec.SubPath, "~.ignore/")
		d, err := managed.Open(".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := d.RemoveIgnorePattern(pattern); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("removed ignore pattern: %s\n", pattern)
		return
	}

	// :~tagname → remove tag
	if strings.HasPrefix(spec.SubPath, "~") {
		tagName := spec.SubPath[1:]
		d, err := managed.Open(".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := d.RemoveTag(tagName); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("removed tag :~%s\n", tagName)
		return
	}

	fmt.Fprintf(os.Stderr, "Error: c4 rm :%s not supported\n", spec.SubPath)
	os.Exit(1)
}

// rmC4MEntry removes an entry from a c4m file.
func rmC4mEntry(spec pathspec.PathSpec) {
	if !establish.IsC4mEstablished(spec.Source) {
		fmt.Fprintf(os.Stderr, "Error: %s: is not established for writing\n", spec.Source)
		fmt.Fprintf(os.Stderr, "Run: c4 mk %s:\n", spec.Source)
		os.Exit(1)
	}

	manifest, err := loadManifest(spec.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	entry := findEntry(manifest, spec.SubPath)
	if entry == nil {
		fmt.Fprintf(os.Stderr, "Error: %s not found in %s\n", spec.SubPath, spec.Source)
		os.Exit(1)
	}

	// Remove the entry (and children if directory)
	removed := removeEntry(manifest, spec.SubPath)

	manifest.SortEntries()
	scan.PropagateMetadata(manifest.Entries)

	if err := writeManifest(spec.Source, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("removed %d entries from %s:\n", removed, spec.Source)
}

// removeEntry removes an entry (and its children if a directory) from a manifest.
// Returns the number of entries removed.
func removeEntry(manifest *c4m.Manifest, subPath string) int {
	// Resolve full paths to find the target and its children
	var dirStack []string
	type indexedPath struct {
		index    int
		fullPath string
	}
	var resolved []indexedPath

	for i, entry := range manifest.Entries {
		if entry.Depth < len(dirStack) {
			dirStack = dirStack[:entry.Depth]
		}
		var fullPath string
		if len(dirStack) > 0 {
			fullPath = strings.Join(dirStack, "") + entry.Name
		} else {
			fullPath = entry.Name
		}
		resolved = append(resolved, indexedPath{index: i, fullPath: fullPath})
		if entry.IsDir() {
			for len(dirStack) <= entry.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[entry.Depth] = entry.Name
		}
	}

	// Find indices to remove: the entry itself plus any children (prefix match)
	var toRemove []int
	for _, rp := range resolved {
		if rp.fullPath == subPath || strings.HasPrefix(rp.fullPath, subPath) {
			toRemove = append(toRemove, rp.index)
		}
	}

	// Remove in reverse order to preserve indices
	for i := len(toRemove) - 1; i >= 0; i-- {
		idx := toRemove[i]
		manifest.Entries = append(manifest.Entries[:idx], manifest.Entries[idx+1:]...)
	}
	manifest.InvalidateIndex()

	return len(toRemove)
}
