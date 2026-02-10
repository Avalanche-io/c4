package transform

import (
	"sort"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// OpType represents the type of transformation operation
type OpType int

const (
	OpAdd OpType = iota
	OpDelete
	OpModify
	OpMove
	OpCopy
)

func (o OpType) String() string {
	switch o {
	case OpAdd:
		return "ADD"
	case OpDelete:
		return "DELETE"
	case OpModify:
		return "MODIFY"
	case OpMove:
		return "MOVE"
	case OpCopy:
		return "COPY"
	default:
		return "UNKNOWN"
	}
}

// Operation represents a single transformation operation
type Operation struct {
	Type   OpType
	Source string      // Source path (for move/copy)
	Target string      // Target path
	Entry  *c4m.Entry  // Entry data
}

// Stats contains statistics about the transformation
type Stats struct {
	TotalOps    int
	Adds        int
	Deletes     int
	Modifies    int
	Moves       int
	Copies      int
	BytesToAdd  int64 // Bytes requiring transfer (adds + modifies)
	BytesToMove int64 // Bytes that can be moved locally
}

// Plan represents a transformation plan from source to target
type Plan struct {
	Operations []Operation
	Stats      Stats
}

// Config contains transformation configuration
type Config struct {
	// DetectMoves enables move detection using C4 IDs
	DetectMoves bool

	// DetectCopies enables copy detection (same content in multiple locations)
	DetectCopies bool
}

// DefaultConfig returns a default configuration with all detection enabled
func DefaultConfig() *Config {
	return &Config{
		DetectMoves:  true,
		DetectCopies: true,
	}
}

// Transformer handles manifest transformations
type Transformer struct {
	config *Config

	// Consolidated indices built once and reused
	sourceByID   map[c4.ID][]*c4m.Entry // Source entries by C4 ID
	sourceByPath map[string]*c4m.Entry  // Source entries by path
	targetByID   map[c4.ID][]*c4m.Entry // Target entries by C4 ID
	targetByPath map[string]*c4m.Entry  // Target entries by path
}

// NewTransformer creates a new transformer with the given configuration
func NewTransformer(config *Config) *Transformer {
	if config == nil {
		config = DefaultConfig()
	}
	return &Transformer{
		config: config,
	}
}

// Transform generates a transformation plan from source to target manifest
func (t *Transformer) Transform(source, target *c4m.Manifest) (*Plan, error) {
	// Build all indices once
	t.buildIndices(source, target)

	plan := &Plan{
		Operations: make([]Operation, 0),
	}

	// Track which entries have been processed
	processedSource := make(map[string]bool)
	processedTarget := make(map[string]bool)

	// Phase 1: Detect moves (same content, different path)
	if t.config.DetectMoves {
		moves := t.detectMoves(processedSource, processedTarget)
		for _, op := range moves {
			plan.Operations = append(plan.Operations, op)
			t.updateStats(&plan.Stats, op)
		}
	}

	// Phase 2: Detect modifications (same path, different content)
	mods := t.detectModifications(processedSource, processedTarget)
	for _, op := range mods {
		plan.Operations = append(plan.Operations, op)
		t.updateStats(&plan.Stats, op)
	}

	// Phase 3: Detect copies (content exists in source, appears at new location)
	if t.config.DetectCopies {
		copies := t.detectCopies(processedSource, processedTarget)
		for _, op := range copies {
			plan.Operations = append(plan.Operations, op)
			t.updateStats(&plan.Stats, op)
		}
	}

	// Phase 4: Detect pure additions and deletions
	adds, dels := t.detectAddsDeletes(processedSource, processedTarget)
	for _, op := range adds {
		plan.Operations = append(plan.Operations, op)
		t.updateStats(&plan.Stats, op)
	}
	for _, op := range dels {
		plan.Operations = append(plan.Operations, op)
		t.updateStats(&plan.Stats, op)
	}

	// Phase 5: Optimize operation order for efficiency
	t.optimizeOperations(plan)

	return plan, nil
}

// buildIndices builds all lookup indices once for efficient processing
func (t *Transformer) buildIndices(source, target *c4m.Manifest) {
	// Initialize maps
	t.sourceByID = make(map[c4.ID][]*c4m.Entry)
	t.sourceByPath = make(map[string]*c4m.Entry)
	t.targetByID = make(map[c4.ID][]*c4m.Entry)
	t.targetByPath = make(map[string]*c4m.Entry)

	// Index source entries
	for _, entry := range source.Entries {
		t.sourceByPath[entry.Name] = entry
		if !entry.C4ID.IsNil() {
			t.sourceByID[entry.C4ID] = append(t.sourceByID[entry.C4ID], entry)
		}
	}

	// Index target entries
	for _, entry := range target.Entries {
		t.targetByPath[entry.Name] = entry
		if !entry.C4ID.IsNil() {
			t.targetByID[entry.C4ID] = append(t.targetByID[entry.C4ID], entry)
		}
	}
}

// detectMoves finds files that moved (same C4 ID, different path)
func (t *Transformer) detectMoves(processedSource, processedTarget map[string]bool) []Operation {
	var ops []Operation

	// For each target entry with a C4 ID
	for _, targetEntry := range t.targetByPath {
		if targetEntry.C4ID.IsNil() || processedTarget[targetEntry.Name] {
			continue
		}

		// Skip if there's an exact match (same path, same content)
		if sourceEntry, exists := t.sourceByPath[targetEntry.Name]; exists {
			if !sourceEntry.C4ID.IsNil() && sourceEntry.C4ID == targetEntry.C4ID {
				continue // Not a move, just unchanged
			}
		}

		// Look for source entries with same C4 ID at different path
		sourceEntries, hasSource := t.sourceByID[targetEntry.C4ID]
		if !hasSource {
			continue
		}

		// Find an unprocessed source entry to use as move source
		for _, sourceEntry := range sourceEntries {
			if processedSource[sourceEntry.Name] {
				continue
			}

			// Check if source path still exists in target (would make this a copy, not move)
			if _, existsInTarget := t.targetByPath[sourceEntry.Name]; existsInTarget {
				continue // Source path still used, this should be a copy
			}

			// This is a move
			ops = append(ops, Operation{
				Type:   OpMove,
				Source: sourceEntry.Name,
				Target: targetEntry.Name,
				Entry:  targetEntry,
			})
			processedSource[sourceEntry.Name] = true
			processedTarget[targetEntry.Name] = true
			break
		}
	}

	return ops
}

// detectModifications finds files with same path but different content
func (t *Transformer) detectModifications(processedSource, processedTarget map[string]bool) []Operation {
	var ops []Operation

	for path, targetEntry := range t.targetByPath {
		if processedTarget[path] {
			continue
		}

		sourceEntry, exists := t.sourceByPath[path]
		if !exists || processedSource[path] {
			continue
		}

		// Check if content changed
		if !sourceEntry.C4ID.IsNil() && !targetEntry.C4ID.IsNil() {
			// Direct C4 ID comparison (Phase 1 fix: no String() conversion)
			if sourceEntry.C4ID != targetEntry.C4ID {
				ops = append(ops, Operation{
					Type:   OpModify,
					Target: targetEntry.Name,
					Entry:  targetEntry,
				})
				processedSource[path] = true
				processedTarget[path] = true
			} else {
				// Same content, same path - mark as processed but no operation
				processedSource[path] = true
				processedTarget[path] = true
			}
		} else {
			// Fallback to size comparison if C4 IDs not available
			if sourceEntry.Size != targetEntry.Size {
				ops = append(ops, Operation{
					Type:   OpModify,
					Target: targetEntry.Name,
					Entry:  targetEntry,
				})
				processedSource[path] = true
				processedTarget[path] = true
			} else {
				// Assume same if size matches and no C4 IDs
				processedSource[path] = true
				processedTarget[path] = true
			}
		}
	}

	return ops
}

// detectCopies finds files where content from source appears at new locations
func (t *Transformer) detectCopies(processedSource, processedTarget map[string]bool) []Operation {
	var ops []Operation

	for _, targetEntry := range t.targetByPath {
		if targetEntry.C4ID.IsNil() || processedTarget[targetEntry.Name] {
			continue
		}

		// Check if this C4 ID exists in source
		sourceEntries, hasSource := t.sourceByID[targetEntry.C4ID]
		if !hasSource {
			continue
		}

		// Find a source to copy from (prefer one that's still in target at same path)
		var copySource *c4m.Entry
		for _, se := range sourceEntries {
			// Prefer source that still exists at same path in target
			if te, exists := t.targetByPath[se.Name]; exists && te.C4ID == se.C4ID {
				copySource = se
				break
			}
		}
		if copySource == nil {
			// Use any available source
			copySource = sourceEntries[0]
		}

		ops = append(ops, Operation{
			Type:   OpCopy,
			Source: copySource.Name,
			Target: targetEntry.Name,
			Entry:  targetEntry,
		})
		processedTarget[targetEntry.Name] = true
	}

	return ops
}

// detectAddsDeletes finds pure additions and deletions
func (t *Transformer) detectAddsDeletes(processedSource, processedTarget map[string]bool) ([]Operation, []Operation) {
	var adds, dels []Operation

	// Find additions (in target but not processed)
	for path, entry := range t.targetByPath {
		if !processedTarget[path] {
			adds = append(adds, Operation{
				Type:   OpAdd,
				Target: entry.Name,
				Entry:  entry,
			})
		}
	}

	// Find deletions (in source but not processed)
	for path, entry := range t.sourceByPath {
		if !processedSource[path] {
			dels = append(dels, Operation{
				Type:   OpDelete,
				Target: entry.Name,
				Entry:  entry,
			})
		}
	}

	return adds, dels
}

// optimizeOperations reorders operations for efficiency
func (t *Transformer) optimizeOperations(plan *Plan) {
	// Sort by priority:
	// 1. Deletes (free space first)
	// 2. Moves (cheap local operation)
	// 3. Copies (can use local data)
	// 4. Modifies (need new data)
	// 5. Adds (need new data)
	priority := map[OpType]int{
		OpDelete: 1,
		OpMove:   2,
		OpCopy:   3,
		OpModify: 4,
		OpAdd:    5,
	}

	sort.Slice(plan.Operations, func(i, j int) bool {
		pi := priority[plan.Operations[i].Type]
		pj := priority[plan.Operations[j].Type]
		if pi != pj {
			return pi < pj
		}
		// Secondary sort by path for determinism
		return plan.Operations[i].Target < plan.Operations[j].Target
	})
}

// updateStats updates transformation statistics
func (t *Transformer) updateStats(stats *Stats, op Operation) {
	stats.TotalOps++
	size := int64(0)
	if op.Entry != nil {
		size = op.Entry.Size
	}

	switch op.Type {
	case OpAdd:
		stats.Adds++
		stats.BytesToAdd += size
	case OpDelete:
		stats.Deletes++
	case OpModify:
		stats.Modifies++
		stats.BytesToAdd += size
	case OpMove:
		stats.Moves++
		stats.BytesToMove += size
	case OpCopy:
		stats.Copies++
		stats.BytesToMove += size
	}
}

// FindMissing returns entries that are in target but not in source (by C4 ID)
// This is useful for determining what content needs to be fetched
func FindMissing(source, target *c4m.Manifest) *c4m.Manifest {
	missing := c4m.NewManifest()

	// Build source C4 ID set
	sourceIDs := make(map[c4.ID]bool)
	for _, entry := range source.Entries {
		if !entry.C4ID.IsNil() {
			sourceIDs[entry.C4ID] = true
		}
	}

	// Find target entries with C4 IDs not in source
	for _, entry := range target.Entries {
		if !entry.C4ID.IsNil() && !sourceIDs[entry.C4ID] {
			missing.AddEntry(entry)
		}
	}

	missing.SortEntries()
	return missing
}

// FindExtra returns entries that are in source but not in target (by C4 ID)
// This is useful for determining what content can be cleaned up
func FindExtra(source, target *c4m.Manifest) *c4m.Manifest {
	return FindMissing(target, source)
}

// FindCommon returns entries with C4 IDs present in both manifests
func FindCommon(source, target *c4m.Manifest) *c4m.Manifest {
	common := c4m.NewManifest()

	// Build source C4 ID set
	sourceIDs := make(map[c4.ID]bool)
	for _, entry := range source.Entries {
		if !entry.C4ID.IsNil() {
			sourceIDs[entry.C4ID] = true
		}
	}

	// Find target entries with C4 IDs in source
	for _, entry := range target.Entries {
		if !entry.C4ID.IsNil() && sourceIDs[entry.C4ID] {
			common.AddEntry(entry)
		}
	}

	common.SortEntries()
	return common
}
