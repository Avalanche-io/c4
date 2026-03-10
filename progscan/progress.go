package progscan

import (
	"fmt"
	"os"
	"strings"

	"github.com/Avalanche-io/c4/c4m"
)

// Progress reports resolution statistics for a c4m file.
type Progress struct {
	Total      int   // total entries (files + dirs)
	Files      int
	Dirs       int
	HasMeta    int   // entries with resolved timestamp (phase 1 done)
	HasC4ID    int   // file entries with resolved C4 ID (phase 2 done)
	TotalBytes int64 // sum of known file sizes (complete after phase 1)
}

// Phase returns the highest fully-completed phase (0, 1, or 2),
// or -1 if the file has no entries.
func (p *Progress) Phase() int {
	if p.Total == 0 {
		return -1
	}
	if p.Files > 0 && p.HasC4ID == p.Files {
		return 2
	}
	if p.Files == 0 && p.HasMeta == p.Total {
		return 2 // dirs only, no hashing needed
	}
	if p.HasMeta == p.Total {
		return 1
	}
	return 0
}

// Fraction returns overall progress as a value in [0, 1].
func (p *Progress) Fraction() float64 {
	if p.Total == 0 {
		return 1
	}
	mw, hw := p.weights()
	total := mw + hw
	if total == 0 {
		return 1
	}

	metaDone := float64(p.HasMeta) / float64(p.Total) * mw
	var hashDone float64
	if p.Files > 0 {
		hashDone = float64(p.HasC4ID) / float64(p.Files) * hw
	}
	return (metaDone + hashDone) / total
}

// Bar renders a progress bar of the given width (not counting brackets).
// The bar is split into a metadata segment and a hash segment, with a
// pipe divider showing the boundary. Segment widths reflect the estimated
// cost ratio: stat calls are cheap, hashing is proportional to data size.
//
//	[████|██████████░░░░░░░░░░░░░] 45%
//	 meta  hash
func (p *Progress) Bar(width int) string {
	if width < 4 {
		width = 4
	}

	mw, hw := p.weights()
	total := mw + hw
	if total == 0 {
		// Everything done or empty.
		return "[" + strings.Repeat("█", width) + "] 100%"
	}

	// Allocate cells: 1 cell for divider, rest proportional.
	inner := width - 1 // reserve 1 for '|'
	metaCells := int(float64(inner) * mw / total)
	if metaCells < 1 {
		metaCells = 1
	}
	hashCells := inner - metaCells

	// Fill within each segment.
	var metaFill int
	if p.Total > 0 {
		metaFill = metaCells * p.HasMeta / p.Total
	}
	var hashFill int
	if p.Files > 0 {
		hashFill = hashCells * p.HasC4ID / p.Files
	}

	pct := int(p.Fraction() * 100)

	var b strings.Builder
	b.WriteByte('[')
	b.WriteString(strings.Repeat("█", metaFill))
	b.WriteString(strings.Repeat("░", metaCells-metaFill))
	b.WriteByte('|')
	b.WriteString(strings.Repeat("█", hashFill))
	b.WriteString(strings.Repeat("░", hashCells-hashFill))
	b.WriteByte(']')
	b.WriteString(fmt.Sprintf(" %d%%", pct))
	return b.String()
}

// weights returns the relative cost of metadata resolution vs content
// hashing, as comparable float64 values.
//
// Heuristic: 1 stat call ≈ hashing 20KB of data.
//   - meta_weight = Total (one stat per entry)
//   - hash_weight = TotalBytes / 20480
//
// When file sizes are unknown (before phase 1), we estimate average
// file size at 100KB, giving hash_weight ≈ Files * 5.
func (p *Progress) weights() (meta, hash float64) {
	meta = float64(p.Total)
	if p.TotalBytes > 0 {
		hash = float64(p.TotalBytes) / 20480
	} else if p.HasMeta < p.Total {
		// Sizes unknown — estimate 100KB average.
		hash = float64(p.Files) * 5
	} else {
		// Phase 1 complete but TotalBytes is 0 (empty files only).
		hash = float64(p.Files)
	}
	return meta, hash
}

// ReadProgress decodes a c4m file and returns resolution statistics.
func ReadProgress(path string) (*Progress, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	m, err := c4m.NewDecoder(f).Decode()
	if err != nil {
		return nil, err
	}

	return ManifestProgress(m), nil
}

// ManifestProgress computes resolution stats from a decoded manifest.
func ManifestProgress(m *c4m.Manifest) *Progress {
	p := &Progress{}
	for _, e := range m.Entries {
		p.Total++
		if e.IsDir() {
			p.Dirs++
		} else {
			p.Files++
			if !e.C4ID.IsNil() {
				p.HasC4ID++
			}
			if e.Size >= 0 {
				p.TotalBytes += e.Size
			}
		}
		if !e.Timestamp.IsZero() && !e.Timestamp.Equal(c4m.NullTimestamp()) {
			p.HasMeta++
		}
	}
	return p
}
