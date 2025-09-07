package c4m

import (
	"os"
	"time"

	"github.com/Avalanche-io/c4"
)

// FileMetadata represents generic file metadata that implements os.FileInfo
// This allows scanners to work with any source (filesystem, S3, archives, etc.)
// without being tied to the c4m.Entry type
type FileMetadata interface {
	os.FileInfo

	// ID returns the C4 ID if available, or nil ID if not yet computed
	ID() c4.ID

	// Path returns the full path to the file
	Path() string

	// Target returns the symlink target, or empty string if not a symlink
	Target() string

	// Depth returns the depth in the hierarchy (0 for root)
	Depth() int

	// SetID sets the C4 ID after computation
	SetID(id c4.ID)

	// Children returns child metadata for directories
	Children() []FileMetadata
}

// BasicFileMetadata is a concrete implementation of FileMetadata
type BasicFileMetadata struct {
	path     string
	name     string
	size     int64
	mode     os.FileMode
	modTime  time.Time
	isDir    bool
	target   string
	depth    int
	c4id     c4.ID
	children []FileMetadata
}

// NewFileMetadata creates a new BasicFileMetadata from os.FileInfo
func NewFileMetadata(path string, info os.FileInfo, depth int) FileMetadata {
	return &BasicFileMetadata{
		path:    path,
		name:    info.Name(),
		size:    info.Size(),
		mode:    info.Mode(),
		modTime: info.ModTime(),
		isDir:   info.IsDir(),
		depth:   depth,
	}
}

// os.FileInfo interface implementation

func (m *BasicFileMetadata) Name() string       { return m.name }
func (m *BasicFileMetadata) Size() int64        { return m.size }
func (m *BasicFileMetadata) Mode() os.FileMode  { return m.mode }
func (m *BasicFileMetadata) ModTime() time.Time { return m.modTime }
func (m *BasicFileMetadata) IsDir() bool        { return m.isDir }
func (m *BasicFileMetadata) Sys() interface{}   { return nil }

// FileMetadata interface implementation

func (m *BasicFileMetadata) ID() c4.ID           { return m.c4id }
func (m *BasicFileMetadata) Path() string        { return m.path }
func (m *BasicFileMetadata) Target() string      { return m.target }
func (m *BasicFileMetadata) Depth() int          { return m.depth }
func (m *BasicFileMetadata) SetID(id c4.ID)      { m.c4id = id }
func (m *BasicFileMetadata) Children() []FileMetadata { return m.children }

// SetTarget sets the symlink target
func (m *BasicFileMetadata) SetTarget(target string) {
	m.target = target
}

// AddChild adds a child metadata entry (for directories)
func (m *BasicFileMetadata) AddChild(child FileMetadata) {
	m.children = append(m.children, child)
}

// MetadataToEntry converts FileMetadata to a c4m.Entry
func MetadataToEntry(md FileMetadata) *Entry {
	entry := &Entry{
		Mode:      md.Mode(),
		Timestamp: md.ModTime().UTC(),
		Size:      md.Size(),
		Name:      md.Name(),
		Target:    md.Target(),
		C4ID:      md.ID(),
		Depth:     md.Depth(),
	}

	// For directories, add trailing slash to name
	if md.IsDir() {
		entry.Name = entry.Name + "/"
	}

	return entry
}

// EntryToMetadata converts a c4m.Entry to FileMetadata
func EntryToMetadata(entry *Entry) FileMetadata {
	name := entry.Name
	isDir := entry.IsDir()
	
	// Remove trailing slash for internal representation
	if isDir && len(name) > 0 && name[len(name)-1] == '/' {
		name = name[:len(name)-1]
	}

	return &BasicFileMetadata{
		path:    name, // In Entry, Name often serves as path
		name:    name,
		size:    entry.Size,
		mode:    entry.Mode,
		modTime: entry.Timestamp,
		isDir:   isDir,
		target:  entry.Target,
		depth:   entry.Depth,
		c4id:    entry.C4ID,
	}
}

// ScanResult represents the result of a scan operation
// This allows scanners to return metadata without knowing about c4m types
type ScanResult struct {
	Root     FileMetadata
	AllFiles []FileMetadata
}

// ToManifest converts a ScanResult to a c4m.Manifest
func (sr *ScanResult) ToManifest() *Manifest {
	manifest := NewManifest()
	
	for _, md := range sr.AllFiles {
		entry := MetadataToEntry(md)
		manifest.AddEntry(entry)
	}
	
	return manifest
}