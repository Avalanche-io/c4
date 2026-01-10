package scan

import (
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/cmd/c4/internal/bundle"
)

// Type aliases for c4m types to simplify migration
type Entry = c4m.Entry
type Manifest = c4m.Manifest
type Bundle = bundle.Bundle

// Function aliases
var NewManifest = c4m.NewManifest
var NewDecoder = c4m.NewDecoder
var NewEncoder = c4m.NewEncoder

// NaturalLess is an alias for c4m.NaturalLess
var NaturalLess = c4m.NaturalLess

// FileSource wraps a filesystem path for use with c4m.Source interface
type FileSource struct {
	Path      string
	Generator *Generator
}

// ToManifest implements c4m.Source
func (fs FileSource) ToManifest() (*c4m.Manifest, error) {
	return fs.Generator.GenerateFromPath(fs.Path)
}
