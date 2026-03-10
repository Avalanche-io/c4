package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"strconv"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/container"
	"github.com/Avalanche-io/c4/cmd/c4/internal/establish"
	"github.com/Avalanche-io/c4/cmd/c4/internal/managed"
	"github.com/Avalanche-io/c4/cmd/c4/internal/pathspec"
	"github.com/Avalanche-io/c4/cmd/c4/internal/scan"
	flag "github.com/spf13/pflag"
)

const version = "1.0.0"

var (
	// Global flags
	versionFlag  bool
	idFlag       bool
	prettyFlag   bool
	helpFlag     bool
	sequenceFlag bool
	progressFlag bool
)

func init() {
	flag.Usage = usage

	flag.BoolVarP(&versionFlag, "version", "", false, "Show version information")
	flag.BoolVarP(&idFlag, "id", "i", false, "Output bare C4 ID(s) instead of c4m")
	flag.BoolVarP(&prettyFlag, "pretty", "p", false, "Pretty-print c4m with aligned columns")
	flag.BoolVarP(&sequenceFlag, "sequence", "s", false, "Detect and fold file sequences into range notation")
	flag.BoolVarP(&progressFlag, "progress", "P", false, "Show progressive scan with stage progress bars")
	flag.BoolVar(&helpFlag, "help", false, "Show help message")
}

func usage() {
	fmt.Fprintf(os.Stderr, `c4 - Content-addressable identification using C4 IDs

Usage:
  c4 [options] [path...]           # Identify files or directories
  c4 version                       # Show c4 and c4d mesh versions
  c4 ls <target>                   # List contents via colon notation
  c4 cat <target>                  # Output content bytes to stdout
  c4 diff <source> <target>        # Compare manifests or paths
  c4 cp <source> <dest>            # Copy between local, c4m, locations
  c4 mv <source> <dest>            # Move/rename within c4m file
  c4 ln [-s] <source> <dest>      # Link (hard or symbolic) in c4m
  c4 mk <target>: [address]         # Establish for writing (c4m, location, or :)
  c4 rm <target>:                  # Remove establishment or entries
  c4 mkdir [-p] <target>           # Create directory in c4m file
  c4 patch <source> <target>       # Apply c4m patch or target state
  c4 undo :                        # Revert last operation on managed dir
  c4 redo :                        # Re-apply undone operation
  c4 unrm :                        # List/recover removed items
  c4 du                            # Show store disk usage
  c4 scan <path>                   # Progressive filesystem scan
  c4 compact <file.c4m>            # Remove padding from c4m file

Examples:
  c4 file.txt                      # c4m entry for file
  c4 myproject/                    # Full recursive c4m listing
  c4 -i file.txt                   # Bare C4 ID of file
  c4 -i myproject/                 # Bare C4 ID of directory
  echo "data" | c4                 # Bare C4 ID from stdin
  c4 -p myproject/                 # Pretty-print c4m listing
  c4 ls project.c4m:               # List c4m file contents
  c4 ls project.c4m:renders/       # List subtree
  c4 cat project.c4m:README.md     # File content from c4m to stdout
  c4 cat c4abc...                  # Content by C4 ID from c4d
  c4 diff old.c4m new.c4m          # Compare two manifests

Options:
`)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nVersion: %s\n", version)
}

func main() {
	// Check for subcommands first
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			runVersion(os.Args[2:])
			return
		case "ls":
			runLs(os.Args[2:])
			return
		case "cat":
			runCat(os.Args[2:])
			return
		case "diff":
			runDiff(os.Args[2:])
			return
		case "cp":
			runCp(os.Args[2:])
			return
		case "mk":
			runMk(os.Args[2:])
			return
		case "rm":
			runRm(os.Args[2:])
			return
		case "mkdir":
			runMkdir(os.Args[2:])
			return
		case "mv":
			runMv(os.Args[2:])
			return
		case "ln":
			runLn(os.Args[2:])
			return
		case "patch":
			runPatch(os.Args[2:])
			return
		case "undo":
			runUndo(os.Args[2:])
			return
		case "redo":
			runRedo(os.Args[2:])
			return
		case "unrm":
			runUnrm(os.Args[2:])
			return
		case "du":
			runDU(os.Args[2:])
			return
		case "scan":
			runScan(os.Args[2:])
			return
		case "compact":
			runCompact(os.Args[2:])
			return
		}
	}

	flag.Parse()

	if versionFlag {
		runVersion(nil)
		os.Exit(0)
	}

	if helpFlag {
		flag.Usage()
		os.Exit(0)
	}

	paths := flag.Args()

	if len(paths) == 0 {
		processStdin()
	} else {
		processFiles(paths)
	}
}

func processStdin() {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		flag.Usage()
		os.Exit(1)
	}

	id := c4.Identify(os.Stdin)
	fmt.Println(id)
}

func processFiles(paths []string) {
	for _, path := range paths {
		if err := processPath(path); err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", path, err)
			os.Exit(1)
		}
	}
}

func processPath(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return processDirectory(path)
	}
	return processFile(path, info)
}

func processDirectory(dirPath string) error {
	if progressFlag {
		return processDirectoryProgressive(dirPath)
	}

	opts := []scan.GeneratorOption{scan.WithC4IDs(true)}
	if sequenceFlag {
		opts = append(opts, scan.WithSequenceDetection(true))
	}
	gen := scan.NewGeneratorWithOptions(opts...)
	manifest, err := gen.GenerateFromPath(dirPath)
	if err != nil {
		return err
	}

	if idFlag {
		fmt.Println(manifest.ComputeC4ID())
		return nil
	}

	outputManifest(manifest)
	return nil
}

func processDirectoryProgressive(dirPath string) error {
	cli := scan.NewProgressiveCLI(dirPath,
		scan.WithProgress(true),
	)
	return cli.Run()
}

func processFile(path string, info os.FileInfo) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	id := c4.Identify(file)

	if idFlag {
		fmt.Println(id)
		return nil
	}

	// Output as c4m entry line
	entry := &c4m.Entry{
		Mode:      info.Mode(),
		Timestamp: info.ModTime().UTC(),
		Size:      info.Size(),
		Name:      filepath.Base(path),
		C4ID:      id,
	}

	m := c4m.NewManifest()
	m.AddEntry(entry)

	outputManifest(m)
	return nil
}

func outputManifest(manifest *c4m.Manifest) {
	enc := c4m.NewEncoder(os.Stdout)
	if prettyFlag {
		enc.SetPretty(true)
	}
	enc.Encode(manifest)
}

// Operation handlers

func runDiff(args []string) {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	pretty := fs.BoolP("pretty", "p", false, "Pretty-print c4m")
	id := fs.BoolP("id", "i", false, "Output bare C4 ID of diff result")
	empty := fs.Bool("empty", false, "Exit 0 if empty, 1 if content")
	fs.Parse(args)

	if fs.NArg() != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 diff [-p] <source> <target>\n")
		os.Exit(1)
	}

	oldManifest := getManifest(fs.Arg(0))
	newManifest := getManifest(fs.Arg(1))

	result := c4m.PatchDiff(oldManifest, newManifest)

	if *empty {
		if result.IsEmpty() {
			os.Exit(0)
		} else {
			os.Exit(1)
		}
	}

	if *id {
		if result.IsEmpty() {
			return
		}
		fmt.Println(result.NewID)
		return
	}

	if result.IsEmpty() {
		return
	}

	// Output patch format: prior C4 ID → patch entries → new C4 ID
	fmt.Println(result.OldID)

	enc := c4m.NewEncoder(os.Stdout)
	if *pretty {
		enc.SetPretty(true)
	}
	enc.Encode(result.Patch)

	fmt.Println(result.NewID)
}

// getManifest resolves a path argument to a manifest.
// Handles: stdin (-), managed dirs (:), c4m files, containers, local paths.
func getManifest(pathArg string) *c4m.Manifest {
	if pathArg == "-" {
		manifest, err := c4m.NewDecoder(os.Stdin).Decode()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}
		return manifest
	}

	spec, err := pathspec.Parse(pathArg, establish.IsLocationEstablished)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	switch spec.Type {
	case pathspec.C4m:
		manifest, err := loadManifest(spec.Source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", spec.Source, err)
			os.Exit(1)
		}
		if spec.SubPath != "" {
			manifest = filterBySubpath(manifest, spec.SubPath)
		}
		return manifest

	case pathspec.Container:
		manifest, err := container.ReadManifest(spec.Source, pathspec.ContainerFormat(spec.Source))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", spec.Source, err)
			os.Exit(1)
		}
		if spec.SubPath != "" {
			manifest = filterBySubpath(manifest, spec.SubPath)
		}
		return manifest

	case pathspec.Managed:
		return getManagedManifest(spec.SubPath)

	case pathspec.Location:
		return getLocationManifest(spec)

	default:
		// Local filesystem path — scan it
		gen := scan.NewGeneratorWithOptions(scan.WithC4IDs(true))
		manifest, err := gen.GenerateFromPath(spec.Source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error scanning %s: %v\n", spec.Source, err)
			os.Exit(1)
		}
		return manifest
	}
}

// getLocationManifest fetches a manifest from a remote location via the local c4d proxy.
// Route: GET {c4dAddr}/~{location}/mnt/{subpath}
func getLocationManifest(spec pathspec.PathSpec) *c4m.Manifest {
	addr := c4dAddr()
	url := fmt.Sprintf("%s/~%s/mnt/", addr, spec.Source)
	if spec.SubPath != "" {
		url += spec.SubPath
	}

	resp, err := c4dClient.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: c4d not reachable at %s\n", addr)
		fmt.Fprintf(os.Stderr, "Location pathspecs require a running c4d.\n")
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Error: c4d returned %s for %s:%s\n", resp.Status, spec.Source, spec.SubPath)
		os.Exit(1)
	}

	manifest, err := c4m.NewDecoder(resp.Body).Decode()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding response from %s: %v\n", spec.Source, err)
		os.Exit(1)
	}
	return manifest
}

// getManagedManifest resolves a managed directory reference (:, :~1, :~name).
func getManagedManifest(subPath string) *c4m.Manifest {
	d, err := managed.Open(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var manifest *c4m.Manifest

	switch {
	case subPath == "":
		// : → current managed state
		manifest, err = d.Current()

	case subPath == "~":
		// :~ → snapshot history (not a manifest, error)
		fmt.Fprintf(os.Stderr, "Error: :~ (history list) is not valid as a manifest source; use c4 ls :~\n")
		os.Exit(1)

	case strings.HasPrefix(subPath, "~"):
		// :~N or :~name
		ref := subPath[1:]
		if n, nerr := strconv.Atoi(ref); nerr == nil {
			manifest, err = d.GetSnapshot(n)
		} else {
			manifest, err = d.GetTag(ref)
		}

	default:
		// :path/ → subpath within current managed state
		manifest, err = d.Current()
		if err == nil && subPath != "" {
			manifest = filterBySubpath(manifest, subPath)
		}
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	return manifest
}
