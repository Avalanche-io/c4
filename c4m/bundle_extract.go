package c4m

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExtractBundleToSingleManifest reads a C4M bundle and outputs all entries as a single manifest
// Note: This produces a simple concatenation of chunks. For large directories that were collapsed,
// the output may have files and directories interleaved (not strictly sorted).
func ExtractBundleToSingleManifest(bundlePath string, output io.Writer) error {
	// Read header.c4 to get the header manifest ID
	headerPath := filepath.Join(bundlePath, "header.c4")
	headerData, err := os.ReadFile(headerPath)
	if err != nil {
		return fmt.Errorf("cannot read header.c4: %w", err)
	}
	
	headerID := strings.TrimSpace(string(headerData))
	if !strings.HasPrefix(headerID, "c4") {
		return fmt.Errorf("invalid C4 ID in header.c4")
	}
	
	// Read the header manifest
	headerManifestPath := filepath.Join(bundlePath, "c4", headerID)
	headerManifest, err := os.ReadFile(headerManifestPath)
	if err != nil {
		return fmt.Errorf("cannot read header manifest: %w", err)
	}
	
	// Extract chunk IDs from header manifest
	chunkIDs := extractChunkIDsFromManifest(string(headerManifest))
	
	// Write the C4M header
	if _, err := fmt.Fprintln(output, "@c4m 1.0"); err != nil {
		return err
	}
	
	// Process and write all chunks
	for i, chunkID := range chunkIDs {
		chunkPath := filepath.Join(bundlePath, "c4", chunkID)
		if err := appendChunkContents(chunkPath, output, i == 0); err != nil {
			return fmt.Errorf("error processing chunk %s: %w", chunkID, err)
		}
	}
	
	return nil
}

// extractChunkIDsFromManifest finds all C4 IDs for .c4m files in a manifest
func extractChunkIDsFromManifest(manifestContent string) []string {
	var chunkIDs []string
	lines := strings.Split(manifestContent, "\n")
	
	for _, line := range lines {
		// Skip empty lines and directives
		if line == "" || strings.HasPrefix(line, "@") {
			continue
		}
		
		// Look for .c4m files
		if strings.Contains(line, ".c4m ") {
			// Extract the C4 ID at the end of the line
			fields := strings.Fields(line)
			for _, field := range fields {
				if strings.HasPrefix(field, "c4") && len(field) > 10 {
					chunkIDs = append(chunkIDs, field)
					break
				}
			}
		}
	}
	
	return chunkIDs
}

// appendChunkContents reads a chunk file and appends its contents (minus header) to output
func appendChunkContents(chunkPath string, output io.Writer, includeBlankLine bool) error {
	file, err := os.Open(chunkPath)
	if err != nil {
		return err
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer for long lines
	
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		
		// Skip the @c4m header line in chunks
		if lineNum == 1 && strings.HasPrefix(line, "@c4m") {
			continue
		}
		
		// Write all other lines
		if _, err := fmt.Fprintln(output, line); err != nil {
			return err
		}
	}
	
	return scanner.Err()
}

// ExtractBundleToFile is a convenience function that extracts a bundle to a file
func ExtractBundleToFile(bundlePath, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()
	
	return ExtractBundleToSingleManifest(bundlePath, file)
}

// ExtractBundlePretty extracts a bundle with pretty-printed formatting
func ExtractBundlePretty(bundlePath string, output io.Writer) error {
	// First, load all entries into a manifest
	manifest, err := LoadBundleAsManifest(bundlePath)
	if err != nil {
		return err
	}
	
	// Use the manifest's WritePretty method
	_, err = manifest.WritePretty(output)
	return err
}

// ExtractBundlePrettyToFile extracts a bundle to a file with pretty formatting
func ExtractBundlePrettyToFile(bundlePath, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()
	
	return ExtractBundlePretty(bundlePath, file)
}

// LoadBundleAsManifest loads a bundle and returns it as a Manifest struct
func LoadBundleAsManifest(bundlePath string) (*Manifest, error) {
	// Read header.c4 to get the header manifest ID
	headerPath := filepath.Join(bundlePath, "header.c4")
	headerData, err := os.ReadFile(headerPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read header.c4: %w", err)
	}
	
	headerID := strings.TrimSpace(string(headerData))
	if !strings.HasPrefix(headerID, "c4") {
		return nil, fmt.Errorf("invalid C4 ID in header.c4")
	}
	
	// Read the header manifest
	headerManifestPath := filepath.Join(bundlePath, "c4", headerID)
	headerManifest, err := os.ReadFile(headerManifestPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read header manifest: %w", err)
	}
	
	// Extract chunk IDs from header manifest
	chunkIDs := extractChunkIDsFromManifest(string(headerManifest))
	
	// Create a combined manifest
	combined := NewManifest()
	combined.Version = "1.0"
	
	// Load all chunks and merge their entries
	for _, chunkID := range chunkIDs {
		chunkPath := filepath.Join(bundlePath, "c4", chunkID)
		chunkFile, err := os.Open(chunkPath)
		if err != nil {
			return nil, fmt.Errorf("cannot open chunk %s: %w", chunkID, err)
		}
		
		// Parse the chunk
		chunkManifest, err := GenerateFromReader(chunkFile)
		chunkFile.Close()
		if err != nil {
			return nil, fmt.Errorf("cannot parse chunk %s: %w", chunkID, err)
		}
		
		// Merge entries
		combined.Entries = append(combined.Entries, chunkManifest.Entries...)
	}
	
	return combined, nil
}