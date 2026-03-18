package c4m

import (
	"os"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
)

// ManifestBuilder provides a fluent API for building manifests with correct hierarchy
type ManifestBuilder struct {
	manifest *Manifest
	errs     []error  // Accumulated validation errors
	warnings []string // Non-fatal warnings
}

// NewBuilder creates a new ManifestBuilder for constructing manifests
func NewBuilder() *ManifestBuilder {
	return &ManifestBuilder{
		manifest: NewManifest(),
	}
}

// Builder returns a ManifestBuilder for an existing manifest
func (m *Manifest) Builder() *ManifestBuilder {
	return &ManifestBuilder{
		manifest: m,
	}
}

// AddFile adds a file entry at the root level (depth 0)
func (b *ManifestBuilder) AddFile(name string, opts ...EntryOption) *ManifestBuilder {
	entry := &Entry{
		Name:  name,
		Depth: 0,
	}
	for _, opt := range opts {
		opt(entry)
	}
	b.manifest.AddEntry(entry)
	return b
}

// AddDir adds a directory at the root level and returns a DirBuilder for adding children
func (b *ManifestBuilder) AddDir(name string, opts ...EntryOption) *DirBuilder {
	// Ensure trailing slash
	if !strings.HasSuffix(name, "/") {
		name += "/"
	}

	entry := &Entry{
		Name:  name,
		Depth: 0,
		Mode:  os.ModeDir | 0755,
	}
	for _, opt := range opts {
		opt(entry)
	}
	b.manifest.AddEntry(entry)

	return &DirBuilder{
		root:   b,
		parent: nil,
		entry:  entry,
	}
}

// Build constructs the manifest and returns any validation errors
// The manifest is always returned, even if there are errors
func (b *ManifestBuilder) Build() (*Manifest, error) {
	if len(b.errs) > 0 {
		return b.manifest, b.errs[0]
	}
	return b.manifest, nil
}

// MustBuild is like Build but panics on error
func (b *ManifestBuilder) MustBuild() *Manifest {
	m, err := b.Build()
	if err != nil {
		panic(err)
	}
	return m
}

// Warnings returns any non-fatal warnings accumulated during building
func (b *ManifestBuilder) Warnings() []string {
	return b.warnings
}

// DirBuilder provides a fluent API for adding entries to a directory
type DirBuilder struct {
	root   *ManifestBuilder
	parent *DirBuilder // nil for root-level dirs
	entry  *Entry
}

// AddFile adds a file entry as a child of this directory
func (d *DirBuilder) AddFile(name string, opts ...EntryOption) *DirBuilder {
	entry := &Entry{
		Name:  name,
		Depth: d.entry.Depth + 1,
	}
	for _, opt := range opts {
		opt(entry)
	}
	d.root.manifest.AddEntry(entry)
	return d
}

// AddDir adds a subdirectory and returns a DirBuilder for adding children to it
func (d *DirBuilder) AddDir(name string, opts ...EntryOption) *DirBuilder {
	// Ensure trailing slash
	if !strings.HasSuffix(name, "/") {
		name += "/"
	}

	entry := &Entry{
		Name:  name,
		Depth: d.entry.Depth + 1,
		Mode:  os.ModeDir | 0755,
	}
	for _, opt := range opts {
		opt(entry)
	}
	d.root.manifest.AddEntry(entry)

	return &DirBuilder{
		root:   d.root,
		parent: d,
		entry:  entry,
	}
}

// End returns to the ManifestBuilder (use after finishing a root-level directory)
func (d *DirBuilder) End() *ManifestBuilder {
	return d.root
}

// EndDir returns to the parent DirBuilder (use after finishing a subdirectory)
func (d *DirBuilder) EndDir() *DirBuilder {
	if d.parent != nil {
		return d.parent
	}
	// If no parent, create a temporary DirBuilder that still allows End()
	return d
}

// EntryOption is a functional option for configuring entries
type EntryOption func(*Entry)

// WithC4ID sets the C4 ID of an entry
func WithC4ID(id c4.ID) EntryOption {
	return func(e *Entry) {
		e.C4ID = id
	}
}

// WithSize sets the size of an entry
func WithSize(size int64) EntryOption {
	return func(e *Entry) {
		e.Size = size
	}
}

// WithMode sets the file mode of an entry
func WithMode(mode os.FileMode) EntryOption {
	return func(e *Entry) {
		e.Mode = mode
	}
}

// WithTimestamp sets the timestamp of an entry
func WithTimestamp(t time.Time) EntryOption {
	return func(e *Entry) {
		e.Timestamp = t
	}
}

// WithTarget sets the symlink target of an entry
func WithTarget(target string) EntryOption {
	return func(e *Entry) {
		e.Target = target
		e.Mode |= os.ModeSymlink
	}
}

// WithFlowOutbound sets outbound flow to a location.
func WithFlowOutbound(target string) EntryOption {
	return func(e *Entry) {
		e.FlowDirection = FlowOutbound
		e.FlowTarget = target
	}
}

// WithFlowInbound sets inbound flow from a location.
func WithFlowInbound(target string) EntryOption {
	return func(e *Entry) {
		e.FlowDirection = FlowInbound
		e.FlowTarget = target
	}
}

// WithFlowSync sets bidirectional flow with a location.
func WithFlowSync(target string) EntryOption {
	return func(e *Entry) {
		e.FlowDirection = FlowBidirectional
		e.FlowTarget = target
	}
}

// WithAttrs sets multiple attributes at once
func WithAttrs(c4id c4.ID, size int64, mode os.FileMode, timestamp time.Time) EntryOption {
	return func(e *Entry) {
		e.C4ID = c4id
		e.Size = size
		e.Mode = mode
		e.Timestamp = timestamp
	}
}
