package c4m

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
)

// Bundle represents a C4M bundle directory structure
type Bundle struct {
	Path      string
	ScanPath  string
	Scans     []*BundleScan
}

// BundleScan represents a single scan within a bundle
type BundleScan struct {
	Number       int
	StartTime    time.Time
	CompletedAt  *time.Time
	Path         string
	ProgressChunks []string
	SnapshotID   *c4.ID
}

// BundleConfig configures bundle chunking behavior
type BundleConfig struct {
	MaxEntriesPerChunk int
	MaxBytesPerChunk   int64
	MaxChunkInterval   time.Duration
	CompactThreshold   int
	BundleDir         string
}

// DefaultBundleConfig returns default configuration
func DefaultBundleConfig() *BundleConfig {
	return &BundleConfig{
		MaxEntriesPerChunk: 100000,
		MaxBytesPerChunk:   100 * 1024 * 1024, // 100MB
		MaxChunkInterval:   30 * time.Second,
		CompactThreshold:   50000,
	}
}

// DevBundleConfig returns development configuration with smaller limits
func DevBundleConfig() *BundleConfig {
	return &BundleConfig{
		MaxEntriesPerChunk: 10,
		MaxBytesPerChunk:   1024 * 1024, // 1MB
		MaxChunkInterval:   5 * time.Second,
		CompactThreshold:   50,
	}
}

// CreateBundle creates a new bundle for scanning the given path
func CreateBundle(scanPath string, config *BundleConfig) (*Bundle, error) {
	if config == nil {
		config = DefaultBundleConfig()
	}

	// Create bundle directory name
	baseName := filepath.Base(scanPath)
	if baseName == "/" || baseName == "." {
		baseName = "root"
	}
	timestamp := time.Now().Format("20060102-150405")
	bundleName := fmt.Sprintf("%s_%s.c4m_bundle", baseName, timestamp)
	
	bundlePath := filepath.Join(config.BundleDir, bundleName)
	if config.BundleDir == "" {
		bundlePath = bundleName
	}

	// Create bundle directory structure
	if err := os.MkdirAll(bundlePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create bundle directory: %w", err)
	}

	c4Dir := filepath.Join(bundlePath, "c4")
	if err := os.MkdirAll(c4Dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create c4 directory: %w", err)
	}

	bundle := &Bundle{
		Path:     bundlePath,
		ScanPath: scanPath,
		Scans:    []*BundleScan{},
	}

	// Initialize header
	if err := bundle.writeHeader(); err != nil {
		return nil, fmt.Errorf("failed to write header: %w", err)
	}

	return bundle, nil
}

// OpenBundle opens an existing bundle
func OpenBundle(bundlePath string) (*Bundle, error) {
	// Check if bundle exists
	if _, err := os.Stat(bundlePath); err != nil {
		return nil, fmt.Errorf("bundle not found: %w", err)
	}

	// Read header.c4
	headerPath := filepath.Join(bundlePath, "header.c4")
	headerContent, err := os.ReadFile(headerPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read header.c4: %w", err)
	}

	headerIDStr := strings.TrimSpace(string(headerContent))
	headerID, err := c4.Parse(headerIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid header ID: %w", err)
	}

	// Read header manifest
	manifestPath := filepath.Join(bundlePath, "c4", headerID.String())
	manifestContent, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read header manifest: %w", err)
	}

	// Parse header manifest to extract scans
	bundle := &Bundle{
		Path:  bundlePath,
		Scans: []*BundleScan{},
	}

	if err := bundle.parseHeaderManifest(string(manifestContent)); err != nil {
		return nil, fmt.Errorf("failed to parse header manifest: %w", err)
	}

	return bundle, nil
}

// NewScan starts a new scan in the bundle
func (b *Bundle) NewScan(scanPath string) (*BundleScan, error) {
	scanNum := len(b.Scans) + 1
	
	scan := &BundleScan{
		Number:         scanNum,
		StartTime:      time.Now(),
		Path:           scanPath,
		ProgressChunks: []string{},
	}

	b.Scans = append(b.Scans, scan)

	// Create scan directory structure in header manifest
	if err := b.writeHeader(); err != nil {
		return nil, fmt.Errorf("failed to update header: %w", err)
	}

	// Write path.txt
	pathFile := b.getPathForScan(scan, "path.txt")
	if _, err := b.writeFile(pathFile, []byte(scanPath)); err != nil {
		return nil, fmt.Errorf("failed to write path.txt: %w", err)
	}

	return scan, nil
}

// ResumeScan resumes an incomplete scan
func (b *Bundle) ResumeScan() (*BundleScan, error) {
	// Find the last incomplete scan
	for i := len(b.Scans) - 1; i >= 0; i-- {
		if b.Scans[i].CompletedAt == nil {
			return b.Scans[i], nil
		}
	}
	return nil, fmt.Errorf("no incomplete scan found")
}

// writeHeader writes the header manifest and header.c4
func (b *Bundle) writeHeader() error {
	// Generate header manifest content
	manifest := b.generateHeaderManifest()
	
	// Compute C4 ID of manifest
	id := c4.Identify(strings.NewReader(manifest))
	
	// Write manifest to c4 directory
	manifestPath := filepath.Join(b.Path, "c4", id.String())
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	// Write header.c4 pointing to manifest
	headerPath := filepath.Join(b.Path, "header.c4")
	if err := os.WriteFile(headerPath, []byte(id.String()+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write header.c4: %w", err)
	}

	return nil
}

// generateHeaderManifest creates the header manifest content
func (b *Bundle) generateHeaderManifest() string {
	var sb strings.Builder
	sb.WriteString("@c4m 1.0\n")
	sb.WriteString("d--------- - - scans/\n")

	for _, scan := range b.Scans {
		// Scan directory
		scanTime := scan.StartTime.Format(time.RFC3339)
		sb.WriteString(fmt.Sprintf("  d--------- %s - %d/\n", scanTime, scan.Number))
		
		// path.txt
		if pathID := b.getFileID("path.txt", scan); pathID != nil {
			sb.WriteString(fmt.Sprintf("    ---------- %s %d path.txt %s\n", 
				scanTime, len(scan.Path), pathID))
		}
		
		// Progress chunks
		if len(scan.ProgressChunks) > 0 {
			sb.WriteString(fmt.Sprintf("    d--------- %s - progress/\n", scanTime))
			for i, chunkID := range scan.ProgressChunks {
				sb.WriteString(fmt.Sprintf("      ---------- - - %d.c4m %s\n", i+1, chunkID))
			}
		}
		
		// Snapshot if complete
		if scan.SnapshotID != nil {
			completedTime := "-"
			if scan.CompletedAt != nil {
				completedTime = scan.CompletedAt.Format(time.RFC3339)
			}
			sb.WriteString(fmt.Sprintf("    ---------- %s - snapshot.c4m %s\n", 
				completedTime, scan.SnapshotID))
		}
	}

	return sb.String()
}

// parseHeaderManifest parses the header manifest to populate bundle state
func (b *Bundle) parseHeaderManifest(content string) error {
	// Basic parsing - extract scan path from first scan's path.txt
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.Contains(line, "path.txt") {
			// Extract C4 ID
			parts := strings.Fields(line)
			if len(parts) >= 5 {
				idStr := parts[len(parts)-1]
				id, err := c4.Parse(idStr)
				if err == nil {
					// Read path.txt content
					pathFile := filepath.Join(b.Path, "c4", id.String())
					pathContent, err := os.ReadFile(pathFile)
					if err == nil {
						b.ScanPath = strings.TrimSpace(string(pathContent))
						break
					}
				}
			}
		}
	}
	
	// TODO: Parse full scan structure
	return nil
}

// getPathForScan returns the virtual path for a file in a scan
func (b *Bundle) getPathForScan(scan *BundleScan, filename string) string {
	return fmt.Sprintf("scans/%d/%s", scan.Number, filename)
}

// getFileID computes the C4 ID for a file's content
func (b *Bundle) getFileID(filename string, scan *BundleScan) *c4.ID {
	// This would look up or compute the ID for the given file
	// For now, return nil
	return nil
}

// writeFile writes a file to the bundle and returns its C4 ID
func (b *Bundle) writeFile(virtualPath string, content []byte) (*c4.ID, error) {
	// Compute C4 ID
	id := c4.Identify(strings.NewReader(string(content)))
	
	// Write to c4 directory
	filePath := filepath.Join(b.Path, "c4", id.String())
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}
	
	return &id, nil
}

// AddProgressChunk adds a new progress chunk to the current scan
func (b *Bundle) AddProgressChunk(scan *BundleScan, manifest *Manifest) error {
	// Generate chunk content with @base if not first chunk
	var content strings.Builder
	content.WriteString("@c4m 1.0\n")
	
	if len(scan.ProgressChunks) > 0 {
		lastChunkID := scan.ProgressChunks[len(scan.ProgressChunks)-1]
		content.WriteString(fmt.Sprintf("@base %s\n", lastChunkID))
	}
	
	// Add manifest entries
	content.WriteString(manifest.Canonical())
	
	// Write chunk to bundle
	chunkID, err := b.writeFile(
		fmt.Sprintf("scans/%d/progress/%d.c4m", scan.Number, len(scan.ProgressChunks)+1),
		[]byte(content.String()),
	)
	if err != nil {
		return err
	}
	
	scan.ProgressChunks = append(scan.ProgressChunks, chunkID.String())
	
	// Update header
	return b.writeHeader()
}

// CompleteS can marks a scan as complete with a snapshot
func (b *Bundle) CompleteScan(scan *BundleScan) error {
	if len(scan.ProgressChunks) == 0 {
		return fmt.Errorf("no progress chunks to snapshot")
	}
	
	// Create snapshot pointing to last chunk
	lastChunkID := scan.ProgressChunks[len(scan.ProgressChunks)-1]
	snapshot := fmt.Sprintf("@c4m 1.0\n@base %s\n", lastChunkID)
	
	snapshotID, err := b.writeFile(
		fmt.Sprintf("scans/%d/snapshot.c4m", scan.Number),
		[]byte(snapshot),
	)
	if err != nil {
		return err
	}
	
	now := time.Now()
	scan.CompletedAt = &now
	scan.SnapshotID = snapshotID
	
	return b.writeHeader()
}