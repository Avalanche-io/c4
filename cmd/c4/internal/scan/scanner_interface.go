package scan

import (
	"context"
	"io"
)

// Scanner is a generic interface for scanning sources to produce metadata
// This abstraction allows scanning from filesystems, S3, archives, Git, etc.
type Scanner interface {
	// Scan performs the scanning operation
	Scan(ctx context.Context) (*ScanResult, error)
	
	// SetProgressCallback sets a callback for progress updates
	SetProgressCallback(func(current, total int64))
}

// FilesystemScanner scans local filesystem paths
type FilesystemScanner interface {
	Scanner
	
	// SetPath sets the path to scan
	SetPath(path string)
	
	// SetOptions configures scanning behavior
	SetOptions(opts ScanOptions)
}

// ScanOptions configures scanning behavior
type ScanOptions struct {
	ComputeC4IDs    bool
	FollowSymlinks  bool
	IncludeHidden   bool
	DetectSequences bool
	Gitignore       bool
	MaxDepth        int
	Exclude         []string // Patterns to exclude
}

// StreamingScanner can output results as they're discovered
type StreamingScanner interface {
	Scanner
	
	// SetOutputCallback sets a callback that receives metadata as it's discovered
	// The callback should return an error to stop scanning
	SetOutputCallback(func(FileMetadata) error)
}

// ManifestBuilder converts scan results to manifests
type ManifestBuilder struct {
	options BuildOptions
}

// BuildOptions configures manifest building
type BuildOptions struct {
	DetectSequences bool
	SortEntries     bool
	ComputeRootC4ID bool
}

// NewManifestBuilder creates a new manifest builder
func NewManifestBuilder(opts BuildOptions) *ManifestBuilder {
	return &ManifestBuilder{options: opts}
}

// BuildFromScanResult creates a manifest from scan results
func (mb *ManifestBuilder) BuildFromScanResult(result *ScanResult) *Manifest {
	manifest := NewManifest()
	
	// Convert all metadata to entries
	for _, md := range result.AllFiles {
		entry := MetadataToEntry(md)
		manifest.AddEntry(entry)
	}
	
	// Sort if requested
	if mb.options.SortEntries {
		manifest.SortEntries()
	}
	
	// Detect sequences if requested
	if mb.options.DetectSequences {
		// This would use the existing sequence detection logic
		// For now, we'll leave it as a TODO
	}
	
	return manifest
}

// WriterScanner can write results directly to an io.Writer
type WriterScanner interface {
	Scanner
	
	// ScanToWriter scans and writes the manifest directly to a writer
	ScanToWriter(ctx context.Context, w io.Writer) error
}

// ScannerFactory creates scanners for different sources
type ScannerFactory struct{}

// NewFilesystemScanner creates a scanner for local filesystems
func (sf *ScannerFactory) NewFilesystemScanner(path string, opts ScanOptions) FilesystemScanner {
	// This would return the appropriate scanner implementation
	// For now, we can use the existing Generator as a bridge
	gen := NewGeneratorWithOptions(
		WithC4IDs(opts.ComputeC4IDs),
		WithSymlinks(opts.FollowSymlinks),
		WithHidden(opts.IncludeHidden),
		WithSequenceDetection(opts.DetectSequences),
		WithGitignore(opts.Gitignore),
	)
	
	return &generatorAdapter{
		generator: gen,
		path:      path,
	}
}

// generatorAdapter adapts the existing Generator to the Scanner interface
type generatorAdapter struct {
	generator *Generator
	path      string
}

func (ga *generatorAdapter) Scan(ctx context.Context) (*ScanResult, error) {
	manifest, err := ga.generator.GenerateFromPath(ga.path)
	if err != nil {
		return nil, err
	}
	
	// Convert manifest entries back to metadata
	var allFiles []FileMetadata
	for _, entry := range manifest.Entries {
		md := EntryToMetadata(entry)
		allFiles = append(allFiles, md)
	}
	
	return &ScanResult{
		Root:     nil, // Could determine root from entries
		AllFiles: allFiles,
	}, nil
}

func (ga *generatorAdapter) SetPath(path string) {
	ga.path = path
}

func (ga *generatorAdapter) SetOptions(opts ScanOptions) {
	ga.generator.computeC4IDs = opts.ComputeC4IDs
	ga.generator.followSymlinks = opts.FollowSymlinks
	ga.generator.includeHidden = opts.IncludeHidden
	ga.generator.detectSequences = opts.DetectSequences
	ga.generator.respectGitignore = opts.Gitignore
}

func (ga *generatorAdapter) SetProgressCallback(func(current, total int64)) {
	// Generator doesn't support progress callbacks yet
}