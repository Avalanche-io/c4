package transform

import (
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// Enhanced operation types for directory and attribute operations
const (
	OpMkdir  OpType = iota + 10 // Create directory
	OpRmdir                     // Remove empty directory
	OpChmod                     // Permission change (same content)
	OpTouch                     // Timestamp update only
	OpSeqAdd                    // Add frames to sequence
	OpSeqDel                    // Remove frames from sequence
)

func init() {
	// Extend String() for new types
}

// ExtendedOpString returns string for all operation types including enhanced ones
func ExtendedOpString(op OpType) string {
	switch op {
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
	case OpMkdir:
		return "MKDIR"
	case OpRmdir:
		return "RMDIR"
	case OpChmod:
		return "CHMOD"
	case OpTouch:
		return "TOUCH"
	case OpSeqAdd:
		return "SEQ_ADD"
	case OpSeqDel:
		return "SEQ_DEL"
	default:
		return "UNKNOWN"
	}
}

// EnhancedConfig extends Config with additional detection options
type EnhancedConfig struct {
	Config

	// DetectDirOps enables directory-level operation detection
	DetectDirOps bool

	// DetectAttributes enables mode/timestamp change detection
	DetectAttributes bool

	// DetectSequences enables sequence-aware operations
	DetectSequences bool

	// SequencePatterns are regex patterns for sequence detection
	// Default: common patterns like name.%04d.ext, name_####.ext
	SequencePatterns []string
}

// DefaultEnhancedConfig returns config with all enhanced features enabled
func DefaultEnhancedConfig() *EnhancedConfig {
	return &EnhancedConfig{
		Config: Config{
			DetectMoves:  true,
			DetectCopies: true,
		},
		DetectDirOps:     true,
		DetectAttributes: true,
		DetectSequences:  true,
		SequencePatterns: []string{
			`^(.+)\.(\d+)\.([^.]+)$`,       // name.0001.ext
			`^(.+)_(\d+)\.([^.]+)$`,        // name_0001.ext
			`^(.+)\.(\d+)$`,                // name.0001
			`^(.+)(\d{4,})\.([^.]+)$`,      // name0001.ext (4+ digits)
		},
	}
}

// EnhancedTransformer extends Transformer with additional capabilities
type EnhancedTransformer struct {
	*Transformer
	enhancedConfig *EnhancedConfig
	seqPatterns    []*regexp.Regexp
}

// NewEnhancedTransformer creates a transformer with enhanced features
func NewEnhancedTransformer(config *EnhancedConfig) *EnhancedTransformer {
	if config == nil {
		config = DefaultEnhancedConfig()
	}

	patterns := make([]*regexp.Regexp, 0, len(config.SequencePatterns))
	for _, p := range config.SequencePatterns {
		if re, err := regexp.Compile(p); err == nil {
			patterns = append(patterns, re)
		}
	}

	return &EnhancedTransformer{
		Transformer:    NewTransformer(&config.Config),
		enhancedConfig: config,
		seqPatterns:    patterns,
	}
}

// TransformEnhanced generates a plan with enhanced operation types
func (t *EnhancedTransformer) TransformEnhanced(source, target *c4m.Manifest) (*Plan, error) {
	// Get base plan
	plan, err := t.Transformer.Transform(source, target)
	if err != nil {
		return nil, err
	}

	// Enhance with directory operations
	if t.enhancedConfig.DetectDirOps {
		t.enhanceWithDirOps(plan, source, target)
	}

	// Enhance with attribute detection
	if t.enhancedConfig.DetectAttributes {
		t.enhanceWithAttributes(plan, source, target)
	}

	// Enhance with sequence operations
	if t.enhancedConfig.DetectSequences {
		t.enhanceWithSequences(plan, source, target)
	}

	// Re-optimize operation order
	t.optimizeEnhancedOperations(plan)

	return plan, nil
}

// enhanceWithDirOps adds directory-specific operations
func (t *EnhancedTransformer) enhanceWithDirOps(plan *Plan, source, target *c4m.Manifest) {
	// Collect directories from both manifests
	sourceDirs := make(map[string]*c4m.Entry)
	targetDirs := make(map[string]*c4m.Entry)

	for _, e := range source.Entries {
		if e.IsDir() {
			sourceDirs[e.Name] = e
		}
	}
	for _, e := range target.Entries {
		if e.IsDir() {
			targetDirs[e.Name] = e
		}
	}

	// Find directory moves (all children moved together)
	dirMoves := t.detectDirMoves(source, target)
	for _, move := range dirMoves {
		// Remove individual file moves that are part of dir move
		plan.Operations = filterChildOps(plan.Operations, move.Source, move.Target)
		plan.Operations = append(plan.Operations, move)
	}

	// Add mkdir for new directories
	for path, entry := range targetDirs {
		if _, exists := sourceDirs[path]; !exists {
			// Check if not already covered by a move
			if !isChildOfMove(path, dirMoves) {
				plan.Operations = append(plan.Operations, Operation{
					Type:   OpMkdir,
					Target: path,
					Entry:  entry,
				})
				plan.Stats.TotalOps++
			}
		}
	}

	// Add rmdir for removed directories (only if empty after other ops)
	for path, entry := range sourceDirs {
		if _, exists := targetDirs[path]; !exists {
			if !isChildOfMove(path, dirMoves) && t.isDirEmptyAfterOps(path, plan) {
				plan.Operations = append(plan.Operations, Operation{
					Type:   OpRmdir,
					Target: path,
					Entry:  entry,
				})
				plan.Stats.TotalOps++
			}
		}
	}
}

// detectDirMoves finds directories that moved as a unit
func (t *EnhancedTransformer) detectDirMoves(source, target *c4m.Manifest) []Operation {
	var moves []Operation

	// Build C4 ID index for directories
	sourceDirsByID := make(map[c4.ID]*c4m.Entry)
	for _, e := range source.Entries {
		if e.IsDir() && !e.C4ID.IsNil() {
			sourceDirsByID[e.C4ID] = e
		}
	}

	// Find matching directories in target
	for _, targetEntry := range target.Entries {
		if !targetEntry.IsDir() || targetEntry.C4ID.IsNil() {
			continue
		}

		if sourceEntry, exists := sourceDirsByID[targetEntry.C4ID]; exists {
			if sourceEntry.Name != targetEntry.Name {
				moves = append(moves, Operation{
					Type:   OpMove,
					Source: sourceEntry.Name,
					Target: targetEntry.Name,
					Entry:  targetEntry,
				})
			}
		}
	}

	return moves
}

// filterChildOps removes operations on children of a moved directory
func filterChildOps(ops []Operation, sourceDir, targetDir string) []Operation {
	result := make([]Operation, 0, len(ops))
	sourcePrefix := sourceDir + "/"
	targetPrefix := targetDir + "/"

	for _, op := range ops {
		// Skip if this is a child of the moved directory
		if strings.HasPrefix(op.Source, sourcePrefix) && strings.HasPrefix(op.Target, targetPrefix) {
			continue
		}
		result = append(result, op)
	}

	return result
}

// isChildOfMove checks if a path is a child of any moved directory
func isChildOfMove(p string, moves []Operation) bool {
	for _, move := range moves {
		if strings.HasPrefix(p, move.Source+"/") || strings.HasPrefix(p, move.Target+"/") {
			return true
		}
	}
	return false
}

// isDirEmptyAfterOps checks if a directory will be empty after operations
func (t *EnhancedTransformer) isDirEmptyAfterOps(dir string, plan *Plan) bool {
	prefix := dir + "/"

	for _, op := range plan.Operations {
		// If any file remains in this directory, it's not empty
		if op.Type != OpDelete && op.Type != OpMove {
			if strings.HasPrefix(op.Target, prefix) {
				return false
			}
		}
	}

	return true
}

// enhanceWithAttributes detects permission and timestamp changes
func (t *EnhancedTransformer) enhanceWithAttributes(plan *Plan, source, target *c4m.Manifest) {
	sourceByPath := make(map[string]*c4m.Entry)
	for _, e := range source.Entries {
		sourceByPath[e.Name] = e
	}

	// Look at entries that appear unchanged (same path, same content)
	for _, targetEntry := range target.Entries {
		sourceEntry, exists := sourceByPath[targetEntry.Name]
		if !exists {
			continue
		}

		// Same content?
		if sourceEntry.C4ID.IsNil() || targetEntry.C4ID.IsNil() {
			continue
		}
		if sourceEntry.C4ID != targetEntry.C4ID {
			continue // Already handled as modify
		}

		// Check for mode change
		if sourceEntry.Mode != targetEntry.Mode && targetEntry.Mode != 0 {
			plan.Operations = append(plan.Operations, Operation{
				Type:   OpChmod,
				Target: targetEntry.Name,
				Entry:  targetEntry,
			})
			plan.Stats.TotalOps++
			continue
		}

		// Check for timestamp-only change
		if !sourceEntry.Timestamp.IsZero() && !targetEntry.Timestamp.IsZero() {
			if !sourceEntry.Timestamp.Equal(targetEntry.Timestamp) {
				plan.Operations = append(plan.Operations, Operation{
					Type:   OpTouch,
					Target: targetEntry.Name,
					Entry:  targetEntry,
				})
				plan.Stats.TotalOps++
			}
		}
	}
}

// Sequence represents a detected frame sequence
type Sequence struct {
	Pattern    string   // Printf-style pattern (e.g., "render.%04d.exr")
	Directory  string   // Directory containing the sequence
	Prefix     string   // Name prefix
	Suffix     string   // Name suffix (extension)
	Frames     []int    // Frame numbers present
	FrameToID  map[int]c4.ID // Frame number to C4 ID
	TotalSize  int64    // Total size of all frames
}

// FrameRange returns the start and end frame numbers
func (s *Sequence) FrameRange() (start, end int) {
	if len(s.Frames) == 0 {
		return 0, 0
	}
	return s.Frames[0], s.Frames[len(s.Frames)-1]
}

// MissingFrames returns frame numbers that are gaps in the sequence
func (s *Sequence) MissingFrames() []int {
	if len(s.Frames) < 2 {
		return nil
	}

	var missing []int
	for i := 1; i < len(s.Frames); i++ {
		for f := s.Frames[i-1] + 1; f < s.Frames[i]; f++ {
			missing = append(missing, f)
		}
	}
	return missing
}

// enhanceWithSequences detects and optimizes sequence operations
func (t *EnhancedTransformer) enhanceWithSequences(plan *Plan, source, target *c4m.Manifest) {
	sourceSeqs := t.detectSequences(source)
	targetSeqs := t.detectSequences(target)

	// Match sequences by pattern and directory
	for key, targetSeq := range targetSeqs {
		sourceSeq, exists := sourceSeqs[key]
		if !exists {
			continue
		}

		// Detect frame additions and deletions
		sourceFrameSet := make(map[int]bool)
		for _, f := range sourceSeq.Frames {
			sourceFrameSet[f] = true
		}

		targetFrameSet := make(map[int]bool)
		for _, f := range targetSeq.Frames {
			targetFrameSet[f] = true
		}

		// Frames added
		var addedFrames []int
		for _, f := range targetSeq.Frames {
			if !sourceFrameSet[f] {
				addedFrames = append(addedFrames, f)
			}
		}

		// Frames removed
		var removedFrames []int
		for _, f := range sourceSeq.Frames {
			if !targetFrameSet[f] {
				removedFrames = append(removedFrames, f)
			}
		}

		// Add sequence operations if we have ranges
		if len(addedFrames) > 0 {
			plan.Operations = append(plan.Operations, Operation{
				Type:   OpSeqAdd,
				Target: targetSeq.Pattern,
				Entry: &c4m.Entry{
					Name: formatFrameRange(addedFrames),
				},
			})
			plan.Stats.TotalOps++
		}

		if len(removedFrames) > 0 {
			plan.Operations = append(plan.Operations, Operation{
				Type:   OpSeqDel,
				Target: sourceSeq.Pattern,
				Entry: &c4m.Entry{
					Name: formatFrameRange(removedFrames),
				},
			})
			plan.Stats.TotalOps++
		}
	}
}

// detectSequences finds frame sequences in a manifest
func (t *EnhancedTransformer) detectSequences(m *c4m.Manifest) map[string]*Sequence {
	sequences := make(map[string]*Sequence)

	for _, entry := range m.Entries {
		if entry.IsDir() {
			continue
		}

		dir := path.Dir(entry.Name)
		base := path.Base(entry.Name)

		for _, re := range t.seqPatterns {
			matches := re.FindStringSubmatch(base)
			if matches == nil {
				continue
			}

			// Extract frame number (second capture group)
			frameNum, err := strconv.Atoi(matches[2])
			if err != nil {
				continue
			}

			// Build pattern key
			var pattern string
			if len(matches) == 4 {
				// name.####.ext
				pattern = path.Join(dir, matches[1]+".%0"+strconv.Itoa(len(matches[2]))+"d."+matches[3])
			} else if len(matches) == 3 {
				// name.####
				pattern = path.Join(dir, matches[1]+".%0"+strconv.Itoa(len(matches[2]))+"d")
			}

			key := pattern

			seq, exists := sequences[key]
			if !exists {
				seq = &Sequence{
					Pattern:   pattern,
					Directory: dir,
					Prefix:    matches[1],
					FrameToID: make(map[int]c4.ID),
				}
				if len(matches) == 4 {
					seq.Suffix = matches[3]
				}
				sequences[key] = seq
			}

			seq.Frames = append(seq.Frames, frameNum)
			seq.FrameToID[frameNum] = entry.C4ID
			seq.TotalSize += entry.Size
			break // Only match first pattern
		}
	}

	// Sort frames in each sequence
	for _, seq := range sequences {
		sort.Ints(seq.Frames)
	}

	return sequences
}

// formatFrameRange formats frame numbers as ranges (e.g., "1-10,15,20-25")
func formatFrameRange(frames []int) string {
	if len(frames) == 0 {
		return ""
	}

	sort.Ints(frames)

	var parts []string
	start := frames[0]
	end := frames[0]

	for i := 1; i < len(frames); i++ {
		if frames[i] == end+1 {
			end = frames[i]
		} else {
			parts = append(parts, rangeString(start, end))
			start = frames[i]
			end = frames[i]
		}
	}
	parts = append(parts, rangeString(start, end))

	return strings.Join(parts, ",")
}

func rangeString(start, end int) string {
	if start == end {
		return strconv.Itoa(start)
	}
	return strconv.Itoa(start) + "-" + strconv.Itoa(end)
}

// optimizeEnhancedOperations orders all operation types
func (t *EnhancedTransformer) optimizeEnhancedOperations(plan *Plan) {
	priority := map[OpType]int{
		OpRmdir:  0, // Remove empty dirs first
		OpDelete: 1,
		OpSeqDel: 2,
		OpMove:   3,
		OpCopy:   4,
		OpMkdir:  5, // Create dirs before files
		OpModify: 6,
		OpAdd:    7,
		OpSeqAdd: 8,
		OpChmod:  9,
		OpTouch:  10,
	}

	sort.Slice(plan.Operations, func(i, j int) bool {
		pi := priority[plan.Operations[i].Type]
		pj := priority[plan.Operations[j].Type]
		if pi != pj {
			return pi < pj
		}
		return plan.Operations[i].Target < plan.Operations[j].Target
	})
}

// DetectSequences analyzes a manifest and returns detected sequences
func DetectSequences(m *c4m.Manifest) map[string]*Sequence {
	t := NewEnhancedTransformer(nil)
	return t.detectSequences(m)
}

// SequenceDiff compares two sequences and returns frame differences
type SequenceDiff struct {
	Pattern       string
	AddedFrames   []int
	RemovedFrames []int
	ModifiedFrames []int // Same frame number, different content
}

// CompareSequences compares sequences between two manifests
func CompareSequences(source, target *c4m.Manifest) []SequenceDiff {
	t := NewEnhancedTransformer(nil)
	sourceSeqs := t.detectSequences(source)
	targetSeqs := t.detectSequences(target)

	var diffs []SequenceDiff

	// Compare matching sequences
	for key, targetSeq := range targetSeqs {
		sourceSeq, exists := sourceSeqs[key]
		if !exists {
			// Entire sequence is new
			diffs = append(diffs, SequenceDiff{
				Pattern:     targetSeq.Pattern,
				AddedFrames: targetSeq.Frames,
			})
			continue
		}

		diff := SequenceDiff{Pattern: targetSeq.Pattern}

		sourceFrameSet := make(map[int]c4.ID)
		for f, id := range sourceSeq.FrameToID {
			sourceFrameSet[f] = id
		}

		for _, f := range targetSeq.Frames {
			sourceID, inSource := sourceFrameSet[f]
			if !inSource {
				diff.AddedFrames = append(diff.AddedFrames, f)
			} else if sourceID != targetSeq.FrameToID[f] {
				diff.ModifiedFrames = append(diff.ModifiedFrames, f)
			}
		}

		targetFrameSet := make(map[int]bool)
		for _, f := range targetSeq.Frames {
			targetFrameSet[f] = true
		}

		for _, f := range sourceSeq.Frames {
			if !targetFrameSet[f] {
				diff.RemovedFrames = append(diff.RemovedFrames, f)
			}
		}

		if len(diff.AddedFrames) > 0 || len(diff.RemovedFrames) > 0 || len(diff.ModifiedFrames) > 0 {
			diffs = append(diffs, diff)
		}
	}

	// Find removed sequences
	for key, sourceSeq := range sourceSeqs {
		if _, exists := targetSeqs[key]; !exists {
			diffs = append(diffs, SequenceDiff{
				Pattern:       sourceSeq.Pattern,
				RemovedFrames: sourceSeq.Frames,
			})
		}
	}

	return diffs
}
