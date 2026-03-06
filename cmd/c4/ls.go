package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/establish"
	"github.com/Avalanche-io/c4/cmd/c4/internal/pathspec"
	"github.com/Avalanche-io/c4/cmd/c4/internal/scan"
	flag "github.com/spf13/pflag"
)

// runLs implements "c4 ls" — list contents via colon notation.
//
//	c4 ls project.c4m:               # list c4m file root
//	c4 ls project.c4m:renders/       # list subtree
//	c4 ls                            # equivalent to c4 .
func runLs(args []string) {
	fs := flag.NewFlagSet("ls", flag.ExitOnError)
	pretty := fs.BoolP("pretty", "p", false, "Pretty-print c4m")
	id := fs.BoolP("id", "i", false, "Output bare C4 ID")
	fs.Parse(args)

	if fs.NArg() == 0 {
		// No argument = list current directory (same as c4 .)
		gen := scan.NewGeneratorWithOptions(scan.WithC4IDs(true))
		manifest, err := gen.GenerateFromPath(".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if *id {
			fmt.Println(manifest.ComputeC4ID())
			return
		}
		enc := c4m.NewEncoder(os.Stdout)
		if *pretty {
			enc.SetPretty(true)
		}
		enc.Encode(manifest)
		return
	}

	target := fs.Arg(0)

	spec, err := pathspec.Parse(target, establish.IsLocationEstablished)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	switch spec.Type {
	case pathspec.Local:
		// Local path — scan it
		gen := scan.NewGeneratorWithOptions(scan.WithC4IDs(true))
		manifest, err := gen.GenerateFromPath(spec.Source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if *id {
			fmt.Println(manifest.ComputeC4ID())
			return
		}
		enc := c4m.NewEncoder(os.Stdout)
		if *pretty {
			enc.SetPretty(true)
		}
		enc.Encode(manifest)

	case pathspec.Capsule:
		// c4m file — read and list its contents
		manifest, err := loadManifest(spec.Source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", spec.Source, err)
			os.Exit(1)
		}

		if spec.SubPath != "" {
			manifest = filterBySubpath(manifest, spec.SubPath)
			if len(manifest.Entries) == 0 {
				fmt.Fprintf(os.Stderr, "Error: no entries match %s\n", target)
				os.Exit(1)
			}
		}

		if *id {
			fmt.Println(manifest.ComputeC4ID())
			return
		}
		enc := c4m.NewEncoder(os.Stdout)
		if *pretty {
			enc.SetPretty(true)
		}
		enc.Encode(manifest)

	default:
		fmt.Fprintf(os.Stderr, "Error: %s not yet supported for ls\n", spec.Type)
		os.Exit(1)
	}
}

// filterBySubpath filters a manifest to entries under the given subpath,
// adjusting depth to make the subtree appear root-relative.
func filterBySubpath(manifest *c4m.Manifest, subPath string) *c4m.Manifest {
	// Reconstruct full paths to find entries under subpath
	type resolvedEntry struct {
		fullPath string
		entry    *c4m.Entry
	}
	var resolved []resolvedEntry
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
		resolved = append(resolved, resolvedEntry{fullPath: fullPath, entry: entry})
		if entry.IsDir() {
			for len(dirStack) <= entry.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[entry.Depth] = entry.Name
		}
	}

	// Filter and adjust depths
	prefixDepth := strings.Count(strings.TrimSuffix(subPath, "/"), "/") + 1
	result := c4m.NewManifest()
	for _, re := range resolved {
		if !strings.HasPrefix(re.fullPath, subPath) {
			continue
		}
		// Skip the directory entry itself
		if re.fullPath == subPath {
			continue
		}
		e := *re.entry
		e.Depth -= prefixDepth
		result.AddEntry(&e)
	}

	return result
}
