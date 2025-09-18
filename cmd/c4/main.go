package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	flag "github.com/spf13/pflag"
)

const version = "1.0.0"

var (
	// Core flags
	versionFlag  bool
	manifestFlag bool
	recursiveFlag bool
	
	// Output flags
	verboseFlag  bool
	quietFlag    bool
	absoluteFlag bool
	pathsFlag    bool
	emptyFlag    bool
	prettyFlag   bool
	
	// Behavior flags
	followFlag   bool
	depthFlag    int
	noIDsFlag    bool
	formatFlag   string
	progressiveFlag bool
	slowModeFlag bool  // Development flag for testing progress display
	
	// Bundle flags
	bundleFlag   bool
	resumeFlag   bool
	devModeFlag  bool

	// Sequence flags
	expandSequencesFlag     bool
	standaloneSequencesFlag bool
	collapseSequencesFlag   bool
	minSequenceLengthFlag   int

	// Long-form aliases
	helpFlag bool
)

func init() {
	flag.Usage = usage
	
	// Core flags
	flag.BoolVarP(&versionFlag, "version", "", false, "Show version information")
	flag.BoolVarP(&manifestFlag, "manifest", "m", false, "Output C4M manifest format")
	flag.BoolVarP(&recursiveFlag, "recursive", "r", false, "Process recursively")
	
	// Output flags
	flag.BoolVarP(&verboseFlag, "verbose", "v", false, "Verbose output")
	flag.BoolVarP(&quietFlag, "quiet", "q", false, "Quiet mode")
	flag.BoolVarP(&absoluteFlag, "absolute", "a", false, "Use absolute paths")
	flag.BoolVarP(&pathsFlag, "paths", "p", false, "Output paths only")
	flag.BoolVar(&emptyFlag, "empty", false, "Exit 0 if empty, 1 if content")
	flag.BoolVar(&prettyFlag, "pretty", false, "Pretty-print manifest with aligned columns and formatted sizes")
	
	// Behavior flags
	flag.BoolVarP(&followFlag, "follow", "L", false, "Follow symbolic links")
	flag.IntVarP(&depthFlag, "depth", "d", -1, "Max depth for recursive processing")
	flag.BoolVarP(&noIDsFlag, "no-ids", "n", false, "Don't compute C4 IDs (faster)")
	flag.StringVar(&formatFlag, "format", "c4m", "Output format: c4m, paths, json")
	flag.BoolVar(&progressiveFlag, "progressive", false, "Progressive scan with interrupt support (Ctrl+T for status on macOS)")
	flag.BoolVar(&slowModeFlag, "slow", false, "Add artificial delays for testing progress display (dev mode)")
	
	// Bundle flags
	flag.BoolVar(&bundleFlag, "bundle", false, "Create/use C4M bundle for unbounded scans")
	flag.BoolVar(&resumeFlag, "resume", false, "Resume incomplete bundle scan")
	flag.BoolVar(&devModeFlag, "dev", false, "Use development mode (small chunks)")

	// Sequence flags
	flag.BoolVar(&expandSequencesFlag, "expand-sequences", false, "Expand sequence notation into individual entries")
	flag.BoolVar(&standaloneSequencesFlag, "standalone-sequences", false, "Output sequence expansions to separate manifest")
	flag.BoolVar(&collapseSequencesFlag, "collapse-sequences", false, "Detect and collapse file sequences into notation")
	flag.IntVar(&minSequenceLengthFlag, "min-sequence", 3, "Minimum number of files to form a sequence")

	// Help flag
	flag.BoolVar(&helpFlag, "help", false, "Show help message")
}

func usage() {
	fmt.Fprintf(os.Stderr, `c4 - Content-addressable identification using C4 IDs

Usage: 
  c4 [options] [path...]           # Generate C4 IDs or manifests
  c4 diff <source> <target>         # Compare manifests or paths
  c4 union <inputs...>              # Combine manifests
  c4 intersect <inputs...>          # Find common elements
  c4 subtract <from> <remove>       # Set subtraction
  c4 validate <file|bundle>         # Validate C4M manifest or bundle
  c4 extract <bundle> [output]      # Extract bundle to single C4M file

Examples:
  c4 file.txt                       # C4 ID of file
  c4 .                              # C4 ID of directory (from canonical C4M)
  c4 -m .                           # Show C4M manifest (one level)
  c4 -mr .                          # Show full recursive C4M
  c4 -m --pretty .                  # Pretty-print manifest with aligned columns
  c4 --progressive .                # Progressive scan (Ctrl+C stop, Ctrl+T status on macOS)
  c4 --bundle /path                 # Create bundle for unbounded scan
  c4 --bundle --resume scan.c4m_bundle  # Resume incomplete bundle
  echo "data" | c4                  # C4 ID from piped input
  
  c4 diff old.c4m new.c4m           # Compare two manifests
  c4 subtract needed.c4m . > todo.c4m  # Find missing files

Options:
`)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nVersion: %s\n", version)
}

func main() {
	// Check for operations first
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "diff":
			runDiff(os.Args[2:])
			return
		case "union":
			runUnion(os.Args[2:])
			return
		case "intersect":
			runIntersect(os.Args[2:])
			return
		case "subtract":
			runSubtract(os.Args[2:])
			return
		case "validate":
			runValidate(os.Args[2:])
			return
		case "extract":
			runExtract(os.Args[2:])
			return
		}
	}
	
	flag.Parse()
	
	if versionFlag {
		fmt.Printf("c4 version %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}
	
	if helpFlag {
		flag.Usage()
		os.Exit(0)
	}
	
	paths := flag.Args()
	
	if len(paths) == 0 {
		// Read from stdin
		processStdin()
	} else {
		// Process files/directories
		processFiles(paths)
	}
}

func processStdin() {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		flag.Usage()
		os.Exit(1)
	}
	
	// Check if stdin is C4M format
	scanner := bufio.NewScanner(os.Stdin)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	
	input := strings.Join(lines, "\n")
	if strings.HasPrefix(input, "@c4m ") {
		// Parse as C4M and compute its ID
		manifest, err := c4m.GenerateFromReader(strings.NewReader(input))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing C4M: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(manifest.ComputeC4ID())
	} else {
		// Compute C4 ID of raw input
		id := c4.Identify(strings.NewReader(input))
		fmt.Println(id)
	}
}

func processFiles(paths []string) {
	// Handle bundle mode specially
	if bundleFlag {
		if len(paths) != 1 {
			fmt.Fprintf(os.Stderr, "Error: bundle mode requires exactly one path\n")
			os.Exit(1)
		}
		if err := runBundleScan(paths[0]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	
	for _, path := range paths {
		if err := processPath(path); err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", path, err)
			os.Exit(1)
		}
	}
}

func processPath(path string) error {
	// Check if path is a C4M file
	if strings.HasSuffix(path, ".c4m") {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			// It's a C4M file, compute its C4 ID as a file
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			
			if manifestFlag {
				// Output the manifest contents
				_, err = io.Copy(os.Stdout, file)
				return err
			} else {
				// Output the C4 ID of the file
				id := c4.Identify(file)
				outputID(id, path)
				return nil
			}
		}
	}
	
	// Regular file/directory processing
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	
	if info.IsDir() {
		return processDirectory(path)
	} else {
		return processFile(path, info)
	}
}

func runBundleScan(path string) error {
	// Configure bundle
	var config *c4m.BundleConfig
	if devModeFlag {
		config = c4m.DevBundleConfig()
		fmt.Fprintln(os.Stderr, "# Using development configuration (small chunks)")
	} else {
		config = c4m.DefaultBundleConfig()
	}
	
	// Use simple CLI with directory-aware chunking
	cli := c4m.NewSimpleBundleCLI(config, verboseFlag)
	
	// Execute command
	if resumeFlag {
		return cli.ResumeBundle(path)
	}
	return cli.CreateBundle(path)
}

func runProgressiveScan(dirPath string) error {
	// Create CLI options based on flags
	var cliOpts []c4m.CLIOption
	
	cliOpts = append(cliOpts, c4m.WithOutput(os.Stdout, os.Stderr))
	cliOpts = append(cliOpts, c4m.WithVerbose(verboseFlag))
	cliOpts = append(cliOpts, c4m.WithProgress(!quietFlag))
	
	if slowModeFlag {
		cliOpts = append(cliOpts, c4m.WithSlowMode(true))
	}
	
	if followFlag {
		// Note: progressive scanner doesn't have follow symlinks yet
		// This would need to be added to the scanner
	}
	
	// Show instructions
	if !quietFlag {
		fmt.Fprintf(os.Stderr, "# Starting progressive scan of: %s\n", dirPath)
		fmt.Fprintf(os.Stderr, "# Press Ctrl+C to stop and output results\n")
		if runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd" {
			fmt.Fprintf(os.Stderr, "# Press Ctrl+T for status update\n")
		} else {
			fmt.Fprintf(os.Stderr, "# Send USR1 signal for status: kill -USR1 %d\n", os.Getpid())
		}
		if slowModeFlag {
			fmt.Fprintf(os.Stderr, "# SLOW MODE: Adding artificial delays for testing\n")
		}
		fmt.Fprintf(os.Stderr, "#\n")
	}
	
	cli := c4m.NewProgressiveCLI(dirPath, cliOpts...)
	return cli.Run()
}

func processDirectory(dirPath string) error {
	// Check if progressive mode is requested
	if progressiveFlag {
		return runProgressiveScan(dirPath)
	}
	
	// Generate manifest for the directory
	opts := []c4m.GeneratorOption{
		c4m.WithC4IDs(!noIDsFlag),
		c4m.WithSymlinks(followFlag),
	}
	
	generator := c4m.NewGeneratorWithOptions(opts...)
	
	// For directories, we need different behavior based on flags
	if manifestFlag {
		// Output the manifest
		if recursiveFlag {
			// Full recursive manifest
			manifest, err := generator.GenerateFromPath(dirPath)
			if err != nil {
				return err
			}
			outputManifest(manifest)
		} else {
			// One level only
			manifest, err := generateOneLevel(dirPath, generator)
			if err != nil {
				return err
			}
			outputManifest(manifest)
		}
	} else {
		// Output the C4 ID of the directory (from its canonical C4M)
		// Use one-level manifest with subdirectory C4 IDs
		manifest, err := generateOneLevel(dirPath, generator)
		if err != nil {
			return err
		}
		id := manifest.ComputeC4ID()
		outputID(id, dirPath)
	}
	
	return nil
}

func generateOneLevel(dirPath string, generator *c4m.Generator) (*c4m.Manifest, error) {
	manifest := c4m.NewManifest()
	
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		
		e := &c4m.Entry{
			Mode:      info.Mode(),
			Timestamp: info.ModTime().UTC(),
			Size:      info.Size(),
			Name:      entry.Name(),
		}
		
		if info.IsDir() {
			e.Name += "/"
			// For directories, compute their C4 ID from their manifest
			subPath := filepath.Join(dirPath, entry.Name())
			subManifest, err := generator.GenerateFromPath(subPath)
			if err == nil {
				e.C4ID = subManifest.ComputeC4ID()
			}
		} else if !noIDsFlag && info.Mode().IsRegular() {
			// Compute file C4 ID
			filePath := filepath.Join(dirPath, entry.Name())
			file, err := os.Open(filePath)
			if err == nil {
				e.C4ID = c4.Identify(file)
				file.Close()
			}
		}
		
		manifest.AddEntry(e)
	}
	
	manifest.SortSiblingsHierarchically()
	return manifest, nil
}

func processFile(path string, info os.FileInfo) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	
	id := c4.Identify(file)
	outputID(id, path)
	
	return nil
}

func outputID(id c4.ID, path string) {
	displayPath := path
	if absoluteFlag {
		if absPath, err := filepath.Abs(path); err == nil {
			displayPath = absPath
		}
	}
	
	if quietFlag || (pathsFlag && len(flag.Args()) == 1) {
		fmt.Println(id)
	} else if verboseFlag || len(flag.Args()) > 1 {
		fmt.Printf("%s %s\n", id, displayPath)
	} else {
		fmt.Println(id)
	}
}

func outputManifest(manifest *c4m.Manifest) {
	// Apply sequence collapse if requested
	if collapseSequencesFlag {
		manifest = c4m.CollapseToSequences(manifest, minSequenceLengthFlag)
	}

	// Apply sequence expansion if requested
	if expandSequencesFlag {
		mode := c4m.SequenceEmbedded
		if standaloneSequencesFlag {
			mode = c4m.SequenceStandalone
		}

		expandedManifest, err := c4m.ProcessManifestWithSequences(manifest, mode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error expanding sequences: %v\n", err)
		} else {
			manifest = expandedManifest
		}
	}

	switch formatFlag {
	case "paths":
		for _, path := range manifest.PathList() {
			fmt.Println(path)
		}
	case "c4m":
		if prettyFlag {
			manifest.WritePretty(os.Stdout)
		} else {
			manifest.WriteTo(os.Stdout)
		}
	default:
		if prettyFlag {
			manifest.WritePretty(os.Stdout)
		} else {
			manifest.WriteTo(os.Stdout)
		}
	}
}

// Operation handlers
func runDiff(args []string) {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
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
	
	if emptyFlag {
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

func runUnion(args []string) {
	fs := flag.NewFlagSet("union", flag.ExitOnError)
	fs.Parse(args)
	
	if fs.NArg() < 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 union <input1> <input2> [...]\n")
		os.Exit(1)
	}
	
	var sources []c4m.Source
	for i := 0; i < fs.NArg(); i++ {
		sources = append(sources, getSource(fs.Arg(i)))
	}
	
	result, err := c4m.Union(sources...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	
	outputManifest(result)
}

func runIntersect(args []string) {
	fs := flag.NewFlagSet("intersect", flag.ExitOnError)
	fs.Parse(args)
	
	if fs.NArg() < 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 intersect <input1> <input2> [...]\n")
		os.Exit(1)
	}
	
	var sources []c4m.Source
	for i := 0; i < fs.NArg(); i++ {
		sources = append(sources, getSource(fs.Arg(i)))
	}
	
	result, err := c4m.Intersect(sources...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	
	outputManifest(result)
}

func runSubtract(args []string) {
	fs := flag.NewFlagSet("subtract", flag.ExitOnError)
	fs.Parse(args)
	
	if fs.NArg() != 2 {
		fmt.Fprintf(os.Stderr, "Usage: c4 subtract <from> <remove>\n")
		os.Exit(1)
	}
	
	from := getSource(fs.Arg(0))
	remove := getSource(fs.Arg(1))
	
	result, err := c4m.Subtract(from, remove)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	
	outputManifest(result)
}

func getSource(path string) c4m.Source {
	// Check if it's stdin
	if path == "-" {
		scanner := bufio.NewScanner(os.Stdin)
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		input := strings.Join(lines, "\n")
		manifest, err := c4m.GenerateFromReader(strings.NewReader(input))
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
			manifest, err := c4m.GenerateFromReader(file)
			if err == nil {
				return c4m.ManifestSource{Manifest: manifest}
			}
		}
	}
	
	// Treat as filesystem path
	return c4m.FileSource{
		Path: path,
		Generator: c4m.NewGeneratorWithOptions(
			c4m.WithC4IDs(!noIDsFlag),
			c4m.WithSymlinks(followFlag),
		),
	}
}

func runValidate(args []string) {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Error: validate requires exactly one argument\n")
		fmt.Fprintf(os.Stderr, "Usage: c4 validate <file.c4m | bundle_dir>\n")
		os.Exit(1)
	}
	
	path := args[0]
	strict := true // Always strict validation
	
	// Create validator
	validator := c4m.NewValidator(strict)
	
	// Check if it's a bundle directory
	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot access %s: %v\n", path, err)
		os.Exit(1)
	}
	
	var validationErr error
	if info.IsDir() {
		// Validate as bundle
		fmt.Printf("Validating bundle: %s\n", path)
		validationErr = validator.ValidateBundle(path)
	} else {
		// Validate as manifest file
		fmt.Printf("Validating manifest: %s\n", path)
		file, err := os.Open(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot open file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
		validationErr = validator.ValidateManifest(file)
	}
	
	// Report results
	errors := validator.GetErrors()
	warnings := validator.GetWarnings()
	stats := validator.GetStats()
	
	// Display statistics
	fmt.Printf("\nStatistics:\n")
	fmt.Printf("  Total entries in manifests: %d\n", stats.TotalEntries)
	fmt.Printf("  Files: %d\n", stats.Files)
	fmt.Printf("  Directories: %d\n", stats.Directories)
	fmt.Printf("  Symlinks: %d\n", stats.Symlinks)
	if stats.SpecialFiles > 0 {
		fmt.Printf("  Special files: %d\n", stats.SpecialFiles)
	}
	fmt.Printf("  Total size: %d bytes\n", stats.TotalSize)
	if !stats.OldestTime.IsZero() {
		fmt.Printf("  Oldest: %s\n", stats.OldestTime.Format(time.RFC3339))
	}
	if !stats.NewestTime.IsZero() {
		fmt.Printf("  Newest: %s\n", stats.NewestTime.Format(time.RFC3339))
	}
	if stats.NullTimes > 0 {
		fmt.Printf("  Null timestamps: %d\n", stats.NullTimes)
	}
	if stats.NullSizes > 0 {
		fmt.Printf("  Null sizes: %d\n", stats.NullSizes)
	}
	if stats.Layers > 0 {
		fmt.Printf("  Layers: %d\n", stats.Layers)
	}
	if stats.ChunkedManifests > 0 {
		fmt.Printf("  Chunked manifests: %d\n", stats.ChunkedManifests)
	}
	if len(stats.CollapsedDirs) > 0 {
		fmt.Printf("  Collapsed directories: %v\n", stats.CollapsedDirs)
	}
	fmt.Printf("  Max depth: %d\n", stats.MaxDepth)
	
	// Note about collapsed directories
	if stats.ChunkedManifests > 0 {
		fmt.Printf("\nNote: Large directories (>70K entries) are stored in separate chunks.\n")
		fmt.Printf("The scan originally processed many more entries that are referenced in the chunks.\n")
	}
	
	if len(warnings) > 0 {
		fmt.Printf("\nWarnings (%d):\n", len(warnings))
		for _, w := range warnings {
			fmt.Printf("  %s\n", w.Error())
		}
	}
	
	if validationErr != nil {
		fmt.Printf("\nErrors (%d):\n", len(errors))
		for _, e := range errors {
			fmt.Printf("  %s\n", e.Error())
		}
		fmt.Printf("\n✗ Validation failed\n")
		os.Exit(1)
	}
	
	fmt.Printf("\n✓ Validation passed\n")
}

func runExtract(args []string) {
	// Create a flag set for the extract command
	fs := flag.NewFlagSet("extract", flag.ExitOnError)
	pretty := fs.Bool("pretty", false, "Pretty-print manifest with aligned columns")
	v2 := fs.Bool("v2", false, "Use V2 extraction algorithm with proper @base chain following")
	fs.Parse(args)
	
	if fs.NArg() < 1 || fs.NArg() > 2 {
		fmt.Fprintf(os.Stderr, "Error: extract requires a bundle path and optional output file\n")
		fmt.Fprintf(os.Stderr, "Usage: c4 extract [--pretty] <bundle_dir> [output.c4m]\n")
		os.Exit(1)
	}
	
	bundlePath := fs.Arg(0)
	
	// Check if bundle directory exists
	info, err := os.Stat(bundlePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot access %s: %v\n", bundlePath, err)
		os.Exit(1)
	}
	if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: bundle path is not a directory\n")
		os.Exit(1)
	}
	
	// Extract to file or stdout
	if fs.NArg() == 2 {
		outputPath := fs.Arg(1)
		fmt.Printf("Extracting bundle to: %s\n", outputPath)
		if *v2 {
			// Use V2 extraction (always pretty)
			if err := c4m.ExtractBundlePrettyToFileV2(bundlePath, outputPath); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		} else if *pretty {
			if err := c4m.ExtractBundlePrettyToFile(bundlePath, outputPath); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		} else {
			if err := c4m.ExtractBundleToFile(bundlePath, outputPath); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
		fmt.Printf("✓ Extraction complete\n")
	} else {
		// Extract to stdout
		if *v2 {
			// Use V2 extraction (always pretty)
			if err := c4m.ExtractBundlePrettyV2(bundlePath, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		} else if *pretty {
			if err := c4m.ExtractBundlePretty(bundlePath, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		} else {
			if err := c4m.ExtractBundleToSingleManifest(bundlePath, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
	}
}