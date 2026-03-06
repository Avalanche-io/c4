package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/scan"
	flag "github.com/spf13/pflag"
)

const version = "1.0.0"

var (
	// Global flags
	versionFlag bool
	idFlag      bool
	prettyFlag  bool
	helpFlag    bool
)

func init() {
	flag.Usage = usage

	flag.BoolVarP(&versionFlag, "version", "", false, "Show version information")
	flag.BoolVarP(&idFlag, "id", "i", false, "Output bare C4 ID(s) instead of c4m")
	flag.BoolVarP(&prettyFlag, "pretty", "p", false, "Pretty-print c4m with aligned columns")
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
  c4 mk <name>: [address]          # Establish for writing
  c4 rm <name>:                    # Remove establishment
  c4 mkdir [-p] <target>           # Create directory in c4m file

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
	gen := scan.NewGeneratorWithOptions(scan.WithC4IDs(true))
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
	empty := fs.Bool("empty", false, "Exit 0 if empty, 1 if content")
	fs.Parse(args)

	if fs.NArg() != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 diff <source> <target>\n")
		os.Exit(1)
	}

	source := getSource(fs.Arg(0))
	target := getSource(fs.Arg(1))

	result, err := c4m.Diff(source, target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if *empty {
		if result.IsEmpty() {
			os.Exit(0)
		} else {
			os.Exit(1)
		}
	}

	// Output diff results
	if len(result.Added.Entries) > 0 {
		fmt.Println("# Added:")
		for _, e := range result.Added.Entries {
			fmt.Printf("+ %s\n", e.Name)
		}
	}

	if len(result.Removed.Entries) > 0 {
		fmt.Println("# Removed:")
		for _, e := range result.Removed.Entries {
			fmt.Printf("- %s\n", e.Name)
		}
	}

	if len(result.Modified.Entries) > 0 {
		fmt.Println("# Modified:")
		for _, e := range result.Modified.Entries {
			fmt.Printf("M %s\n", e.Name)
		}
	}
}

func getSource(path string) c4m.Source {
	// Check if it's stdin
	if path == "-" {
		manifest, err := c4m.NewDecoder(os.Stdin).Decode()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}
		return c4m.ManifestSource{Manifest: manifest}
	}

	// Check if it's a C4M file
	if strings.HasSuffix(path, ".c4m") {
		file, err := os.Open(path)
		if err == nil {
			defer file.Close()
			manifest, err := c4m.NewDecoder(file).Decode()
			if err == nil {
				return c4m.ManifestSource{Manifest: manifest}
			}
		}
	}

	// Treat as filesystem path
	return scan.FileSource{
		Path:      path,
		Generator: scan.NewGeneratorWithOptions(scan.WithC4IDs(true)),
	}
}
