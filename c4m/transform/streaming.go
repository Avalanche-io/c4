package transform

import (
	"sync"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// StreamingTransformer handles large manifest transformations using
// memory-efficient streaming and parallel processing.
type StreamingTransformer struct {
	config      *Config
	parallelism int
}

// NewStreamingTransformer creates a streaming transformer with the given config.
// Parallelism controls how many goroutines to use for parallel operations.
func NewStreamingTransformer(config *Config, parallelism int) *StreamingTransformer {
	if config == nil {
		config = DefaultConfig()
	}
	if parallelism < 1 {
		parallelism = 1
	}
	return &StreamingTransformer{
		config:      config,
		parallelism: parallelism,
	}
}

// StreamingPlan represents a transformation plan that can be consumed incrementally.
type StreamingPlan struct {
	ops   chan Operation
	done  chan struct{}
	stats Stats
	mu    sync.Mutex
	err   error
}

// Operations returns a channel of operations to be consumed.
func (p *StreamingPlan) Operations() <-chan Operation {
	return p.ops
}

// Wait blocks until all operations have been generated.
func (p *StreamingPlan) Wait() error {
	<-p.done
	return p.err
}

// Stats returns the current statistics (may be incomplete until Wait returns).
func (p *StreamingPlan) Stats() Stats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stats
}

// TransformStreaming generates operations incrementally without loading
// the full plan into memory. Operations are sent to the returned channel.
func (t *StreamingTransformer) TransformStreaming(source, target *c4m.Manifest) *StreamingPlan {
	plan := &StreamingPlan{
		ops:  make(chan Operation, 100),
		done: make(chan struct{}),
	}

	go func() {
		defer close(plan.ops)
		defer close(plan.done)

		// Build indices (still needed for efficient lookup)
		sourceByID := make(map[c4.ID][]*c4m.Entry)
		sourceByPath := make(map[string]*c4m.Entry)
		targetByID := make(map[c4.ID][]*c4m.Entry)
		targetByPath := make(map[string]*c4m.Entry)

		for _, entry := range source.Entries {
			sourceByPath[entry.Name] = entry
			if !entry.C4ID.IsNil() {
				sourceByID[entry.C4ID] = append(sourceByID[entry.C4ID], entry)
			}
		}

		for _, entry := range target.Entries {
			targetByPath[entry.Name] = entry
			if !entry.C4ID.IsNil() {
				targetByID[entry.C4ID] = append(targetByID[entry.C4ID], entry)
			}
		}

		processedSource := make(map[string]bool)
		processedTarget := make(map[string]bool)

		// Phase 1: Stream moves
		if t.config.DetectMoves {
			t.streamMoves(sourceByID, sourceByPath, targetByID, targetByPath,
				processedSource, processedTarget, plan)
		}

		// Phase 2: Stream modifications
		t.streamModifications(sourceByPath, targetByPath,
			processedSource, processedTarget, plan)

		// Phase 3: Stream copies
		if t.config.DetectCopies {
			t.streamCopies(sourceByID, targetByPath,
				processedSource, processedTarget, plan)
		}

		// Phase 4: Stream adds and deletes
		t.streamAddsDeletes(sourceByPath, targetByPath,
			processedSource, processedTarget, plan)
	}()

	return plan
}

func (t *StreamingTransformer) streamMoves(
	sourceByID map[c4.ID][]*c4m.Entry,
	sourceByPath map[string]*c4m.Entry,
	targetByID map[c4.ID][]*c4m.Entry,
	targetByPath map[string]*c4m.Entry,
	processedSource, processedTarget map[string]bool,
	plan *StreamingPlan,
) {
	for _, targetEntry := range targetByPath {
		if targetEntry.C4ID.IsNil() || processedTarget[targetEntry.Name] {
			continue
		}

		// Skip exact matches
		if sourceEntry, exists := sourceByPath[targetEntry.Name]; exists {
			if !sourceEntry.C4ID.IsNil() && sourceEntry.C4ID == targetEntry.C4ID {
				continue
			}
		}

		sourceEntries, hasSource := sourceByID[targetEntry.C4ID]
		if !hasSource {
			continue
		}

		for _, sourceEntry := range sourceEntries {
			if processedSource[sourceEntry.Name] {
				continue
			}

			if _, existsInTarget := targetByPath[sourceEntry.Name]; existsInTarget {
				continue
			}

			op := Operation{
				Type:   OpMove,
				Source: sourceEntry.Name,
				Target: targetEntry.Name,
				Entry:  targetEntry,
			}
			plan.ops <- op
			t.updateStats(plan, op)
			processedSource[sourceEntry.Name] = true
			processedTarget[targetEntry.Name] = true
			break
		}
	}
}

func (t *StreamingTransformer) streamModifications(
	sourceByPath, targetByPath map[string]*c4m.Entry,
	processedSource, processedTarget map[string]bool,
	plan *StreamingPlan,
) {
	for path, targetEntry := range targetByPath {
		if processedTarget[path] {
			continue
		}

		sourceEntry, exists := sourceByPath[path]
		if !exists || processedSource[path] {
			continue
		}

		if !sourceEntry.C4ID.IsNil() && !targetEntry.C4ID.IsNil() {
			if sourceEntry.C4ID != targetEntry.C4ID {
				op := Operation{
					Type:   OpModify,
					Target: targetEntry.Name,
					Entry:  targetEntry,
				}
				plan.ops <- op
				t.updateStats(plan, op)
				processedSource[path] = true
				processedTarget[path] = true
			} else {
				processedSource[path] = true
				processedTarget[path] = true
			}
		} else if sourceEntry.Size != targetEntry.Size {
			op := Operation{
				Type:   OpModify,
				Target: targetEntry.Name,
				Entry:  targetEntry,
			}
			plan.ops <- op
			t.updateStats(plan, op)
			processedSource[path] = true
			processedTarget[path] = true
		} else {
			processedSource[path] = true
			processedTarget[path] = true
		}
	}
}

func (t *StreamingTransformer) streamCopies(
	sourceByID map[c4.ID][]*c4m.Entry,
	targetByPath map[string]*c4m.Entry,
	processedSource, processedTarget map[string]bool,
	plan *StreamingPlan,
) {
	for _, targetEntry := range targetByPath {
		if targetEntry.C4ID.IsNil() || processedTarget[targetEntry.Name] {
			continue
		}

		sourceEntries, hasSource := sourceByID[targetEntry.C4ID]
		if !hasSource {
			continue
		}

		// Find best copy source
		var copySource *c4m.Entry
		for _, se := range sourceEntries {
			if te, exists := targetByPath[se.Name]; exists && te.C4ID == se.C4ID {
				copySource = se
				break
			}
		}
		if copySource == nil {
			copySource = sourceEntries[0]
		}

		op := Operation{
			Type:   OpCopy,
			Source: copySource.Name,
			Target: targetEntry.Name,
			Entry:  targetEntry,
		}
		plan.ops <- op
		t.updateStats(plan, op)
		processedTarget[targetEntry.Name] = true
	}
}

func (t *StreamingTransformer) streamAddsDeletes(
	sourceByPath, targetByPath map[string]*c4m.Entry,
	processedSource, processedTarget map[string]bool,
	plan *StreamingPlan,
) {
	// Deletions first (frees space)
	for path, entry := range sourceByPath {
		if processedSource[path] {
			continue
		}
		op := Operation{
			Type:   OpDelete,
			Target: entry.Name,
			Entry:  entry,
		}
		plan.ops <- op
		t.updateStats(plan, op)
	}

	// Then additions
	for path, entry := range targetByPath {
		if processedTarget[path] {
			continue
		}
		op := Operation{
			Type:   OpAdd,
			Target: entry.Name,
			Entry:  entry,
		}
		plan.ops <- op
		t.updateStats(plan, op)
	}
}

func (t *StreamingTransformer) updateStats(plan *StreamingPlan, op Operation) {
	plan.mu.Lock()
	defer plan.mu.Unlock()

	plan.stats.TotalOps++
	size := int64(0)
	if op.Entry != nil {
		size = op.Entry.Size
	}

	switch op.Type {
	case OpAdd:
		plan.stats.Adds++
		plan.stats.BytesToAdd += size
	case OpDelete:
		plan.stats.Deletes++
	case OpModify:
		plan.stats.Modifies++
		plan.stats.BytesToAdd += size
	case OpMove:
		plan.stats.Moves++
		plan.stats.BytesToMove += size
	case OpCopy:
		plan.stats.Copies++
		plan.stats.BytesToMove += size
	}
}

// ParallelExecutor executes operations in parallel where safe.
type ParallelExecutor struct {
	parallelism int
	handler     OperationHandler
}

// OperationHandler processes a single operation.
type OperationHandler func(op Operation) error

// NewParallelExecutor creates an executor with the given parallelism.
func NewParallelExecutor(parallelism int, handler OperationHandler) *ParallelExecutor {
	if parallelism < 1 {
		parallelism = 1
	}
	return &ParallelExecutor{
		parallelism: parallelism,
		handler:     handler,
	}
}

// Execute processes operations from the plan in parallel where safe.
// Operations of the same type can run in parallel, but type groups are sequential.
func (e *ParallelExecutor) Execute(plan *Plan) error {
	// Group by type
	groups := make(map[OpType][]Operation)
	for _, op := range plan.Operations {
		groups[op.Type] = append(groups[op.Type], op)
	}

	// Execute in priority order
	typeOrder := []OpType{OpDelete, OpMove, OpCopy, OpModify, OpAdd}

	for _, opType := range typeOrder {
		ops := groups[opType]
		if len(ops) == 0 {
			continue
		}

		if err := e.executeParallel(ops); err != nil {
			return err
		}
	}

	return nil
}

func (e *ParallelExecutor) executeParallel(ops []Operation) error {
	if len(ops) == 0 {
		return nil
	}

	// Use semaphore for parallelism control
	sem := make(chan struct{}, e.parallelism)
	errChan := make(chan error, len(ops))
	var wg sync.WaitGroup

	for _, op := range ops {
		wg.Add(1)
		sem <- struct{}{} // Acquire

		go func(op Operation) {
			defer wg.Done()
			defer func() { <-sem }() // Release

			if err := e.handler(op); err != nil {
				errChan <- err
			}
		}(op)
	}

	wg.Wait()
	close(errChan)

	// Return first error if any
	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
}

// ExecuteStreaming processes operations from a streaming plan as they arrive.
func (e *ParallelExecutor) ExecuteStreaming(plan *StreamingPlan) error {
	// Buffer operations by type
	currentType := OpType(-1)
	var currentBatch []Operation

	for op := range plan.Operations() {
		if op.Type != currentType {
			// Flush previous batch
			if len(currentBatch) > 0 {
				if err := e.executeParallel(currentBatch); err != nil {
					return err
				}
			}
			currentType = op.Type
			currentBatch = nil
		}
		currentBatch = append(currentBatch, op)
	}

	// Flush final batch
	if len(currentBatch) > 0 {
		if err := e.executeParallel(currentBatch); err != nil {
			return err
		}
	}

	return plan.Wait()
}

// DiffIterator provides memory-efficient iteration over manifest differences.
type DiffIterator struct {
	source *c4m.Manifest
	target *c4m.Manifest

	sourceIdx int
	targetIdx int

	sourceByPath map[string]*c4m.Entry
	targetByPath map[string]*c4m.Entry
}

// NewDiffIterator creates an iterator for comparing two manifests.
func NewDiffIterator(source, target *c4m.Manifest) *DiffIterator {
	sourceByPath := make(map[string]*c4m.Entry)
	targetByPath := make(map[string]*c4m.Entry)

	for _, e := range source.Entries {
		sourceByPath[e.Name] = e
	}
	for _, e := range target.Entries {
		targetByPath[e.Name] = e
	}

	return &DiffIterator{
		source:       source,
		target:       target,
		sourceByPath: sourceByPath,
		targetByPath: targetByPath,
	}
}

// DiffEntry represents a single difference between manifests.
type DiffEntry struct {
	Type   DiffType
	Path   string
	Source *c4m.Entry
	Target *c4m.Entry
}

// DiffType indicates the type of difference.
type DiffType int

const (
	DiffAdded DiffType = iota
	DiffRemoved
	DiffModified
	DiffUnchanged
)

func (d DiffType) String() string {
	switch d {
	case DiffAdded:
		return "ADDED"
	case DiffRemoved:
		return "REMOVED"
	case DiffModified:
		return "MODIFIED"
	case DiffUnchanged:
		return "UNCHANGED"
	default:
		return "UNKNOWN"
	}
}

// All returns all differences as a slice (for small manifests).
func (d *DiffIterator) All() []DiffEntry {
	var diffs []DiffEntry
	seen := make(map[string]bool)

	// Check all source entries
	for _, entry := range d.source.Entries {
		seen[entry.Name] = true
		targetEntry, inTarget := d.targetByPath[entry.Name]

		if !inTarget {
			diffs = append(diffs, DiffEntry{
				Type:   DiffRemoved,
				Path:   entry.Name,
				Source: entry,
			})
		} else if !entry.C4ID.IsNil() && !targetEntry.C4ID.IsNil() {
			if entry.C4ID != targetEntry.C4ID {
				diffs = append(diffs, DiffEntry{
					Type:   DiffModified,
					Path:   entry.Name,
					Source: entry,
					Target: targetEntry,
				})
			}
		} else if entry.Size != targetEntry.Size {
			diffs = append(diffs, DiffEntry{
				Type:   DiffModified,
				Path:   entry.Name,
				Source: entry,
				Target: targetEntry,
			})
		}
	}

	// Check for additions
	for _, entry := range d.target.Entries {
		if !seen[entry.Name] {
			diffs = append(diffs, DiffEntry{
				Type:   DiffAdded,
				Path:   entry.Name,
				Target: entry,
			})
		}
	}

	return diffs
}

// OnlyChanges returns only added, removed, and modified entries.
func (d *DiffIterator) OnlyChanges() []DiffEntry {
	all := d.All()
	var changes []DiffEntry
	for _, diff := range all {
		if diff.Type != DiffUnchanged {
			changes = append(changes, diff)
		}
	}
	return changes
}
