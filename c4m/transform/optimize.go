package transform

import (
	"math"
	"sort"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// CostFunc calculates the cost of an operation
type CostFunc func(op Operation) float64

// CostConfig defines cost functions for different operation types
type CostConfig struct {
	// MoveCost calculates cost to move a file (typically cheap, same filesystem)
	MoveCost CostFunc

	// CopyCost calculates cost to copy (uses local data)
	CopyCost CostFunc

	// AddCost calculates cost to add new content (network transfer)
	AddCost CostFunc

	// DeleteCost calculates cost to delete
	DeleteCost CostFunc

	// ModifyCost calculates cost to modify (network transfer for new content)
	ModifyCost CostFunc
}

// DefaultCostConfig returns a size-based cost configuration
func DefaultCostConfig() *CostConfig {
	return &CostConfig{
		MoveCost: func(op Operation) float64 {
			// Moves are very cheap (just rename)
			return 1.0
		},
		CopyCost: func(op Operation) float64 {
			// Copies use local I/O, proportional to size
			if op.Entry == nil {
				return 10.0
			}
			return 10.0 + float64(op.Entry.Size)/1024/1024 // Base + MB
		},
		AddCost: func(op Operation) float64 {
			// Adds require network transfer, expensive
			if op.Entry == nil {
				return 100.0
			}
			return 100.0 + float64(op.Entry.Size)/1024/1024*10 // Base + 10x per MB
		},
		DeleteCost: func(op Operation) float64 {
			// Deletes are cheap
			return 0.5
		},
		ModifyCost: func(op Operation) float64 {
			// Modifies require network transfer like adds
			if op.Entry == nil {
				return 100.0
			}
			return 100.0 + float64(op.Entry.Size)/1024/1024*10
		},
	}
}

// BandwidthAwareCostConfig creates a config that considers bandwidth constraints
func BandwidthAwareCostConfig(localBandwidthMBps, networkBandwidthMBps float64) *CostConfig {
	return &CostConfig{
		MoveCost: func(op Operation) float64 {
			return 0.001 // Nearly instant
		},
		CopyCost: func(op Operation) float64 {
			if op.Entry == nil {
				return 1.0
			}
			sizeMB := float64(op.Entry.Size) / 1024 / 1024
			return sizeMB / localBandwidthMBps // Time in seconds
		},
		AddCost: func(op Operation) float64 {
			if op.Entry == nil {
				return 10.0
			}
			sizeMB := float64(op.Entry.Size) / 1024 / 1024
			return sizeMB / networkBandwidthMBps // Time in seconds
		},
		DeleteCost: func(op Operation) float64 {
			return 0.001
		},
		ModifyCost: func(op Operation) float64 {
			if op.Entry == nil {
				return 10.0
			}
			sizeMB := float64(op.Entry.Size) / 1024 / 1024
			return sizeMB / networkBandwidthMBps
		},
	}
}

// OptimizedTransformer uses graph algorithms to find minimum-cost transformations
type OptimizedTransformer struct {
	config     *Config
	costConfig *CostConfig
}

// NewOptimizedTransformer creates a transformer with cost optimization
func NewOptimizedTransformer(config *Config, costConfig *CostConfig) *OptimizedTransformer {
	if config == nil {
		config = DefaultConfig()
	}
	if costConfig == nil {
		costConfig = DefaultCostConfig()
	}
	return &OptimizedTransformer{
		config:     config,
		costConfig: costConfig,
	}
}

// OptimalPlan generates a minimum-cost transformation plan
func (t *OptimizedTransformer) OptimalPlan(source, target *c4m.Manifest) (*Plan, float64) {
	// Build indices
	sourceByID := make(map[c4.ID][]*c4m.Entry)
	sourceByPath := make(map[string]*c4m.Entry)
	targetByPath := make(map[string]*c4m.Entry)

	for _, e := range source.Entries {
		sourceByPath[e.Name] = e
		if !e.C4ID.IsNil() {
			sourceByID[e.C4ID] = append(sourceByID[e.C4ID], e)
		}
	}
	for _, e := range target.Entries {
		targetByPath[e.Name] = e
	}

	plan := &Plan{
		Operations: make([]Operation, 0),
	}
	var totalCost float64

	// Find target entries that could be satisfied by moves or copies
	moveCandidates := t.findMoveCandidates(source, target, sourceByID, sourceByPath, targetByPath)

	// Use Hungarian algorithm to find optimal assignment
	assignment, cost := t.hungarianMatch(moveCandidates)
	totalCost += cost

	// Apply the optimal assignment
	usedSources := make(map[string]bool)
	satisfiedTargets := make(map[string]bool)

	for targetPath, match := range assignment {
		if match.opType == OpMove {
			op := Operation{
				Type:   OpMove,
				Source: match.sourcePath,
				Target: targetPath,
				Entry:  targetByPath[targetPath],
			}
			plan.Operations = append(plan.Operations, op)
			t.updateStats(&plan.Stats, op)
			usedSources[match.sourcePath] = true
			satisfiedTargets[targetPath] = true
		} else if match.opType == OpCopy {
			op := Operation{
				Type:   OpCopy,
				Source: match.sourcePath,
				Target: targetPath,
				Entry:  targetByPath[targetPath],
			}
			plan.Operations = append(plan.Operations, op)
			t.updateStats(&plan.Stats, op)
			satisfiedTargets[targetPath] = true
		}
	}

	// Handle unchanged files (same path, same content)
	for path, targetEntry := range targetByPath {
		if satisfiedTargets[path] {
			continue
		}
		if sourceEntry, exists := sourceByPath[path]; exists {
			if !sourceEntry.C4ID.IsNil() && sourceEntry.C4ID == targetEntry.C4ID {
				usedSources[path] = true
				satisfiedTargets[path] = true
				continue
			}
		}
	}

	// Handle modifications (same path, different content)
	for path, targetEntry := range targetByPath {
		if satisfiedTargets[path] {
			continue
		}
		if _, exists := sourceByPath[path]; exists {
			if !usedSources[path] {
				op := Operation{
					Type:   OpModify,
					Target: path,
					Entry:  targetEntry,
				}
				plan.Operations = append(plan.Operations, op)
				t.updateStats(&plan.Stats, op)
				totalCost += t.costConfig.ModifyCost(op)
				usedSources[path] = true
				satisfiedTargets[path] = true
			}
		}
	}

	// Handle additions
	for path, entry := range targetByPath {
		if satisfiedTargets[path] {
			continue
		}
		op := Operation{
			Type:   OpAdd,
			Target: path,
			Entry:  entry,
		}
		plan.Operations = append(plan.Operations, op)
		t.updateStats(&plan.Stats, op)
		totalCost += t.costConfig.AddCost(op)
	}

	// Handle deletions
	for path, entry := range sourceByPath {
		if usedSources[path] {
			continue
		}
		op := Operation{
			Type:   OpDelete,
			Target: path,
			Entry:  entry,
		}
		plan.Operations = append(plan.Operations, op)
		t.updateStats(&plan.Stats, op)
		totalCost += t.costConfig.DeleteCost(op)
	}

	// Optimize operation order
	t.optimizeOperations(plan)

	return plan, totalCost
}

// moveCandidate represents a potential move or copy
type moveCandidate struct {
	sourcePath string
	targetPath string
	c4id       c4.ID
	cost       float64
	opType     OpType
}

// matchResult stores the result of matching
type matchResult struct {
	sourcePath string
	opType     OpType
	cost       float64
}

// findMoveCandidates finds all possible moves and copies
func (t *OptimizedTransformer) findMoveCandidates(
	source, target *c4m.Manifest,
	sourceByID map[c4.ID][]*c4m.Entry,
	sourceByPath, targetByPath map[string]*c4m.Entry,
) []moveCandidate {
	var candidates []moveCandidate

	for _, targetEntry := range target.Entries {
		if targetEntry.C4ID.IsNil() {
			continue
		}

		// Skip if already satisfied (same path, same content)
		if sourceEntry, exists := sourceByPath[targetEntry.Name]; exists {
			if sourceEntry.C4ID == targetEntry.C4ID {
				continue
			}
		}

		// Find sources with same C4 ID
		sources := sourceByID[targetEntry.C4ID]
		for _, sourceEntry := range sources {
			// Determine if this should be a move or copy
			_, sourceStillNeeded := targetByPath[sourceEntry.Name]

			var opType OpType
			var cost float64

			if sourceStillNeeded {
				// Source path still exists in target, must be a copy
				opType = OpCopy
				cost = t.costConfig.CopyCost(Operation{
					Type:   OpCopy,
					Source: sourceEntry.Name,
					Target: targetEntry.Name,
					Entry:  targetEntry,
				})
			} else {
				// Source path not in target, can be a move
				opType = OpMove
				cost = t.costConfig.MoveCost(Operation{
					Type:   OpMove,
					Source: sourceEntry.Name,
					Target: targetEntry.Name,
					Entry:  targetEntry,
				})
			}

			candidates = append(candidates, moveCandidate{
				sourcePath: sourceEntry.Name,
				targetPath: targetEntry.Name,
				c4id:       targetEntry.C4ID,
				cost:       cost,
				opType:     opType,
			})
		}
	}

	return candidates
}

// hungarianMatch uses the Hungarian algorithm for optimal assignment
func (t *OptimizedTransformer) hungarianMatch(candidates []moveCandidate) (map[string]matchResult, float64) {
	if len(candidates) == 0 {
		return make(map[string]matchResult), 0
	}

	// Group candidates by target
	targetToSources := make(map[string][]moveCandidate)
	for _, c := range candidates {
		targetToSources[c.targetPath] = append(targetToSources[c.targetPath], c)
	}

	// For each target, track which sources can provide it
	// For moves: each source can only be used once
	// For copies: source can be reused

	// Build the bipartite graph
	targets := make([]string, 0, len(targetToSources))
	for t := range targetToSources {
		targets = append(targets, t)
	}
	sort.Strings(targets)

	// Collect all unique sources that could be moved (not copied)
	movableSources := make(map[string]bool)
	for _, cands := range targetToSources {
		for _, c := range cands {
			if c.opType == OpMove {
				movableSources[c.sourcePath] = true
			}
		}
	}

	sources := make([]string, 0, len(movableSources))
	for s := range movableSources {
		sources = append(sources, s)
	}
	sort.Strings(sources)

	// Build cost matrix for moves only (copies handled separately)
	n := len(targets)
	m := len(sources)

	if m == 0 || n == 0 {
		// No moves possible, just use copies where available
		return t.greedyCopyAssignment(targetToSources)
	}

	// Create cost matrix (n x m) with infinity for impossible assignments
	costMatrix := make([][]float64, n)
	candidateMap := make(map[string]map[string]moveCandidate) // target -> source -> candidate

	for i := range costMatrix {
		costMatrix[i] = make([]float64, m)
		for j := range costMatrix[i] {
			costMatrix[i][j] = math.Inf(1) // Default to impossible
		}
	}

	for _, c := range candidates {
		if c.opType != OpMove {
			continue
		}
		targetIdx := indexOf(targets, c.targetPath)
		sourceIdx := indexOf(sources, c.sourcePath)
		if targetIdx >= 0 && sourceIdx >= 0 {
			costMatrix[targetIdx][sourceIdx] = c.cost
			if candidateMap[c.targetPath] == nil {
				candidateMap[c.targetPath] = make(map[string]moveCandidate)
			}
			candidateMap[c.targetPath][c.sourcePath] = c
		}
	}

	// Run Hungarian algorithm
	assignment := hungarian(costMatrix)

	result := make(map[string]matchResult)
	var totalCost float64

	usedSources := make(map[string]bool)

	// Apply move assignments
	for targetIdx, sourceIdx := range assignment {
		if sourceIdx < 0 || sourceIdx >= m {
			continue
		}
		targetPath := targets[targetIdx]
		sourcePath := sources[sourceIdx]

		if math.IsInf(costMatrix[targetIdx][sourceIdx], 1) {
			continue // No valid assignment
		}

		if cand, ok := candidateMap[targetPath][sourcePath]; ok {
			result[targetPath] = matchResult{
				sourcePath: sourcePath,
				opType:     OpMove,
				cost:       cand.cost,
			}
			totalCost += cand.cost
			usedSources[sourcePath] = true
		}
	}

	// For targets not assigned a move, try to find a copy
	for targetPath, cands := range targetToSources {
		if _, assigned := result[targetPath]; assigned {
			continue
		}

		// Find best copy option
		var bestCopy *moveCandidate
		for i := range cands {
			c := &cands[i]
			if c.opType == OpCopy {
				if bestCopy == nil || c.cost < bestCopy.cost {
					bestCopy = c
				}
			}
		}

		if bestCopy != nil {
			result[targetPath] = matchResult{
				sourcePath: bestCopy.sourcePath,
				opType:     OpCopy,
				cost:       bestCopy.cost,
			}
			totalCost += bestCopy.cost
		}
	}

	return result, totalCost
}

// greedyCopyAssignment handles case when only copies are available
func (t *OptimizedTransformer) greedyCopyAssignment(targetToSources map[string][]moveCandidate) (map[string]matchResult, float64) {
	result := make(map[string]matchResult)
	var totalCost float64

	for targetPath, cands := range targetToSources {
		var best *moveCandidate
		for i := range cands {
			c := &cands[i]
			if best == nil || c.cost < best.cost {
				best = c
			}
		}
		if best != nil {
			result[targetPath] = matchResult{
				sourcePath: best.sourcePath,
				opType:     best.opType,
				cost:       best.cost,
			}
			totalCost += best.cost
		}
	}

	return result, totalCost
}

// indexOf returns the index of s in slice, or -1 if not found
func indexOf(slice []string, s string) int {
	for i, v := range slice {
		if v == s {
			return i
		}
	}
	return -1
}

// hungarian implements the Hungarian algorithm for minimum cost assignment
// Returns assignment[i] = j meaning row i is assigned to column j
// Returns -1 for unassigned rows
func hungarian(costMatrix [][]float64) []int {
	n := len(costMatrix)
	if n == 0 {
		return nil
	}
	m := len(costMatrix[0])

	// Make the matrix square by padding
	size := n
	if m > size {
		size = m
	}

	// Create padded square matrix
	matrix := make([][]float64, size)
	for i := range matrix {
		matrix[i] = make([]float64, size)
		for j := range matrix[i] {
			if i < n && j < m {
				matrix[i][j] = costMatrix[i][j]
			} else {
				matrix[i][j] = 0 // Dummy assignments have zero cost
			}
		}
	}

	// Step 1: Subtract row minimum from each row
	for i := 0; i < size; i++ {
		minVal := matrix[i][0]
		for j := 1; j < size; j++ {
			if matrix[i][j] < minVal {
				minVal = matrix[i][j]
			}
		}
		if !math.IsInf(minVal, 1) {
			for j := 0; j < size; j++ {
				if !math.IsInf(matrix[i][j], 1) {
					matrix[i][j] -= minVal
				}
			}
		}
	}

	// Step 2: Subtract column minimum from each column
	for j := 0; j < size; j++ {
		minVal := matrix[0][j]
		for i := 1; i < size; i++ {
			if matrix[i][j] < minVal {
				minVal = matrix[i][j]
			}
		}
		if !math.IsInf(minVal, 1) {
			for i := 0; i < size; i++ {
				if !math.IsInf(matrix[i][j], 1) {
					matrix[i][j] -= minVal
				}
			}
		}
	}

	// Use augmenting path method for assignment
	rowAssign := make([]int, size)
	colAssign := make([]int, size)
	for i := range rowAssign {
		rowAssign[i] = -1
		colAssign[i] = -1
	}

	for i := 0; i < size; i++ {
		// Try to find augmenting path from row i
		visited := make([]bool, size)
		augment(matrix, i, rowAssign, colAssign, visited)
	}

	// Return only the original assignments
	result := make([]int, n)
	for i := 0; i < n; i++ {
		if rowAssign[i] < m {
			result[i] = rowAssign[i]
		} else {
			result[i] = -1 // Assigned to dummy column
		}
	}

	return result
}

// augment tries to find an augmenting path from row u
func augment(matrix [][]float64, u int, rowAssign, colAssign []int, visited []bool) bool {
	size := len(matrix)

	for v := 0; v < size; v++ {
		if visited[v] || matrix[u][v] != 0 {
			continue
		}
		visited[v] = true

		if colAssign[v] == -1 || augment(matrix, colAssign[v], rowAssign, colAssign, visited) {
			rowAssign[u] = v
			colAssign[v] = u
			return true
		}
	}
	return false
}

// updateStats updates plan statistics
func (t *OptimizedTransformer) updateStats(stats *Stats, op Operation) {
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

// optimizeOperations reorders operations for efficiency
func (t *OptimizedTransformer) optimizeOperations(plan *Plan) {
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
		return plan.Operations[i].Target < plan.Operations[j].Target
	})
}

// ComparePlans compares two plans and returns the cost difference
func ComparePlans(plan1, plan2 *Plan, costConfig *CostConfig) (cost1, cost2 float64) {
	if costConfig == nil {
		costConfig = DefaultCostConfig()
	}

	for _, op := range plan1.Operations {
		switch op.Type {
		case OpMove:
			cost1 += costConfig.MoveCost(op)
		case OpCopy:
			cost1 += costConfig.CopyCost(op)
		case OpAdd:
			cost1 += costConfig.AddCost(op)
		case OpDelete:
			cost1 += costConfig.DeleteCost(op)
		case OpModify:
			cost1 += costConfig.ModifyCost(op)
		}
	}

	for _, op := range plan2.Operations {
		switch op.Type {
		case OpMove:
			cost2 += costConfig.MoveCost(op)
		case OpCopy:
			cost2 += costConfig.CopyCost(op)
		case OpAdd:
			cost2 += costConfig.AddCost(op)
		case OpDelete:
			cost2 += costConfig.DeleteCost(op)
		case OpModify:
			cost2 += costConfig.ModifyCost(op)
		}
	}

	return cost1, cost2
}

// OptimalSyncPlan creates an optimized plan for syncing between two locations
// considering that some content may already exist at the destination
type SyncOptimizer struct {
	costConfig *CostConfig
}

// NewSyncOptimizer creates a sync optimizer
func NewSyncOptimizer(costConfig *CostConfig) *SyncOptimizer {
	if costConfig == nil {
		costConfig = DefaultCostConfig()
	}
	return &SyncOptimizer{costConfig: costConfig}
}

// SyncPlan represents an optimized sync plan
type SyncPlan struct {
	// Content that needs to be transferred from remote
	TransferFromRemote []*c4m.Entry

	// Content that can be copied locally
	LocalCopies []Operation

	// Content that can be moved locally
	LocalMoves []Operation

	// Content that needs to be deleted
	Deletions []Operation

	// Total estimated cost
	TotalCost float64

	// Estimated transfer size in bytes
	TransferBytes int64

	// Estimated local operation size in bytes
	LocalBytes int64
}

// Optimize creates an optimal sync plan
func (s *SyncOptimizer) Optimize(local, remote *c4m.Manifest) *SyncPlan {
	plan := &SyncPlan{}

	// Build indices
	localByID := make(map[c4.ID][]*c4m.Entry)
	localByPath := make(map[string]*c4m.Entry)

	for _, e := range local.Entries {
		localByPath[e.Name] = e
		if !e.C4ID.IsNil() {
			localByID[e.C4ID] = append(localByID[e.C4ID], e)
		}
	}

	remoteByPath := make(map[string]*c4m.Entry)
	for _, e := range remote.Entries {
		remoteByPath[e.Name] = e
	}

	// Track what we need and what we have
	processedLocal := make(map[string]bool)
	satisfiedRemote := make(map[string]bool)

	// First pass: find exact matches (no operation needed)
	for _, remoteEntry := range remote.Entries {
		if localEntry, exists := localByPath[remoteEntry.Name]; exists {
			if !localEntry.C4ID.IsNil() && localEntry.C4ID == remoteEntry.C4ID {
				processedLocal[remoteEntry.Name] = true
				satisfiedRemote[remoteEntry.Name] = true
			}
		}
	}

	// Second pass: find moves (same content, different path)
	for _, remoteEntry := range remote.Entries {
		if satisfiedRemote[remoteEntry.Name] || remoteEntry.C4ID.IsNil() {
			continue
		}

		localEntries := localByID[remoteEntry.C4ID]
		for _, localEntry := range localEntries {
			if processedLocal[localEntry.Name] {
				continue
			}

			// Check if local path is still needed in remote
			if _, needed := remoteByPath[localEntry.Name]; !needed {
				// Can move
				op := Operation{
					Type:   OpMove,
					Source: localEntry.Name,
					Target: remoteEntry.Name,
					Entry:  remoteEntry,
				}
				plan.LocalMoves = append(plan.LocalMoves, op)
				plan.LocalBytes += remoteEntry.Size
				plan.TotalCost += s.costConfig.MoveCost(op)
				processedLocal[localEntry.Name] = true
				satisfiedRemote[remoteEntry.Name] = true
				break
			}
		}
	}

	// Third pass: find copies
	for _, remoteEntry := range remote.Entries {
		if satisfiedRemote[remoteEntry.Name] || remoteEntry.C4ID.IsNil() {
			continue
		}

		localEntries := localByID[remoteEntry.C4ID]
		if len(localEntries) > 0 {
			// Can copy from any local source
			op := Operation{
				Type:   OpCopy,
				Source: localEntries[0].Name,
				Target: remoteEntry.Name,
				Entry:  remoteEntry,
			}
			plan.LocalCopies = append(plan.LocalCopies, op)
			plan.LocalBytes += remoteEntry.Size
			plan.TotalCost += s.costConfig.CopyCost(op)
			satisfiedRemote[remoteEntry.Name] = true
		}
	}

	// Fourth pass: transfers needed (content not available locally)
	for _, remoteEntry := range remote.Entries {
		if satisfiedRemote[remoteEntry.Name] {
			continue
		}
		plan.TransferFromRemote = append(plan.TransferFromRemote, remoteEntry)
		plan.TransferBytes += remoteEntry.Size
		plan.TotalCost += s.costConfig.AddCost(Operation{
			Type:  OpAdd,
			Entry: remoteEntry,
		})
	}

	// Fifth pass: deletions
	for path, localEntry := range localByPath {
		if processedLocal[path] {
			continue
		}
		if _, existsInRemote := remoteByPath[path]; !existsInRemote {
			op := Operation{
				Type:   OpDelete,
				Target: path,
				Entry:  localEntry,
			}
			plan.Deletions = append(plan.Deletions, op)
			plan.TotalCost += s.costConfig.DeleteCost(op)
		}
	}

	return plan
}
