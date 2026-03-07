package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/cmd/c4/internal/container"
	"github.com/Avalanche-io/c4/cmd/c4/internal/establish"
	"github.com/Avalanche-io/c4/cmd/c4/internal/pathspec"
)

// runCat implements "c4 cat" — output content bytes to stdout.
//
//	c4 cat project.c4m:README.md     # file content from c4m
//	c4 cat c4abc...                  # content by C4 ID from c4d
func runCat(args []string) {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: c4 cat <target>\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  c4 cat project.c4m:README.md   # file from c4m\n")
		fmt.Fprintf(os.Stderr, "  c4 cat c4abc...                # content by C4 ID\n")
		os.Exit(1)
	}

	target := args[0]

	// Check if target is a bare C4 ID (90 chars, starts with "c4")
	if looksLikeC4ID(target) {
		catFromC4d(target)
		return
	}

	// Try parsing as a pathspec
	spec, err := pathspec.Parse(target, establish.IsLocationEstablished)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	switch spec.Type {
	case pathspec.C4m:
		if spec.SubPath == "" {
			fmt.Fprintf(os.Stderr, "Error: c4 cat requires a file path within the c4m\n")
			fmt.Fprintf(os.Stderr, "Usage: c4 cat %s:<path>\n", spec.Source)
			os.Exit(1)
		}
		catFromC4m(spec.Source, spec.SubPath)

	case pathspec.Local:
		// For local files, just output their content (like regular cat)
		f, err := os.Open(spec.Source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		io.Copy(os.Stdout, f)

	case pathspec.Container:
		if spec.SubPath == "" {
			fmt.Fprintf(os.Stderr, "Error: c4 cat requires a file path within the archive\n")
			fmt.Fprintf(os.Stderr, "Usage: c4 cat %s:<path>\n", spec.Source)
			os.Exit(1)
		}
		rc, err := container.CatFile(spec.Source, pathspec.ContainerFormat(spec.Source), spec.SubPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		defer rc.Close()
		io.Copy(os.Stdout, rc)

	case pathspec.Managed:
		catFromManaged(spec.SubPath)

	default:
		fmt.Fprintf(os.Stderr, "Error: %s not yet supported for cat\n", spec.Type)
		os.Exit(1)
	}
}

// looksLikeC4ID checks if a string looks like a C4 ID (90 chars, starts with "c4").
func looksLikeC4ID(s string) bool {
	if len(s) != 90 || !strings.HasPrefix(s, "c4") {
		return false
	}
	// Check base58 characters
	for _, ch := range s[2:] {
		if !isBase58(byte(ch)) {
			return false
		}
	}
	return true
}

func isBase58(b byte) bool {
	return (b >= '1' && b <= '9') ||
		(b >= 'A' && b <= 'H') ||
		(b >= 'J' && b <= 'N') ||
		(b >= 'P' && b <= 'Z') ||
		(b >= 'a' && b <= 'k') ||
		(b >= 'm' && b <= 'z')
}

// catFromC4d fetches content by C4 ID from c4d and writes to stdout.
func catFromC4d(idStr string) {
	resp, err := c4dClient.Get(c4dAddr() + "/" + idStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: c4d not reachable: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		fmt.Fprintf(os.Stderr, "Error: content not found for %s\n", idStr)
		os.Exit(1)
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Error: c4d returned %s\n", resp.Status)
		os.Exit(1)
	}

	io.Copy(os.Stdout, resp.Body)
}

// catFromManaged outputs content from a managed directory reference.
// SubPath format: "~release-v1/config.yaml" or "src/main.go" or empty.
func catFromManaged(subPath string) {
	if subPath == "" {
		fmt.Fprintf(os.Stderr, "Error: c4 cat requires a file path within the managed directory\n")
		fmt.Fprintf(os.Stderr, "Usage: c4 cat :<path> or c4 cat :~<ref>/<path>\n")
		os.Exit(1)
	}

	// Get the manifest for the managed reference
	manifest := getManagedManifest(subPath)

	// For managed subpaths like :~release-v1/config.yaml, the getManagedManifest
	// handles the ~ref part and returns a filtered manifest. But for cat we need
	// a specific file's content, not a manifest listing.
	// If the manifest was filtered by subpath (via getManagedManifest), the entry
	// should be at depth 0 as a single file.
	if len(manifest.Entries) == 0 {
		fmt.Fprintf(os.Stderr, "Error: %s not found in managed directory\n", subPath)
		os.Exit(1)
	}

	// Find the leaf file entry
	for _, entry := range manifest.Entries {
		if entry.IsDir() {
			continue
		}
		if entry.C4ID.IsNil() {
			return // empty or nil-ID file
		}
		catFromC4d(entry.C4ID.String())
		return
	}

	fmt.Fprintf(os.Stderr, "Error: %s is a directory, use c4 ls\n", subPath)
	os.Exit(1)
}

// catFromC4m extracts a file's content from a c4m file via c4d.
func catFromC4m(c4mPath, subPath string) {
	manifest, err := loadManifest(c4mPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", c4mPath, err)
		os.Exit(1)
	}

	// Resolve subPath to find the entry
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

		if entry.IsDir() {
			for len(dirStack) <= entry.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[entry.Depth] = entry.Name
		}

		if fullPath == subPath {
			if entry.IsDir() {
				fmt.Fprintf(os.Stderr, "Error: %s is a directory, use c4 ls\n", subPath)
				os.Exit(1)
			}
			if entry.C4ID == (c4.ID{}) || entry.C4ID.IsNil() {
				// Empty or nil-ID file — output nothing
				return
			}
			catFromC4d(entry.C4ID.String())
			return
		}
	}

	fmt.Fprintf(os.Stderr, "Error: %s not found in %s\n", subPath, c4mPath)
	os.Exit(1)
}
