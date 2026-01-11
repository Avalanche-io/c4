package transform

import (
	"fmt"
	"path"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// TreeNode represents a node in the filesystem tree
type TreeNode struct {
	Name     string
	Entry    *c4m.Entry
	Children []*TreeNode
	Parent   *TreeNode
	Depth    int
}

// BuildTree constructs a tree representation from a manifest
func BuildTree(manifest *c4m.Manifest) *TreeNode {
	root := &TreeNode{
		Name:     "",
		Children: make([]*TreeNode, 0),
		Depth:    0,
	}

	// Map paths to nodes for efficient lookup
	pathNodes := make(map[string]*TreeNode)
	pathNodes[""] = root

	for _, entry := range manifest.Entries {
		parts := splitPath(entry.Name)
		currentPath := ""
		parent := root

		for i, part := range parts {
			if currentPath == "" {
				currentPath = part
			} else {
				currentPath = path.Join(currentPath, part)
			}

			if node, exists := pathNodes[currentPath]; exists {
				parent = node
			} else {
				node := &TreeNode{
					Name:     part,
					Children: make([]*TreeNode, 0),
					Parent:   parent,
					Depth:    i + 1,
				}

				// If this is the last part, attach the entry
				if i == len(parts)-1 {
					node.Entry = entry
				}

				parent.Children = append(parent.Children, node)
				pathNodes[currentPath] = node
				parent = node
			}
		}
	}

	return root
}

// splitPath splits a path into components
func splitPath(p string) []string {
	if p == "" || p == "/" {
		return []string{}
	}

	// Remove leading slash
	if len(p) > 0 && p[0] == '/' {
		p = p[1:]
	}

	// Remove trailing slash for directories
	if len(p) > 0 && p[len(p)-1] == '/' {
		p = p[:len(p)-1]
	}

	if p == "" {
		return []string{}
	}

	return strings.Split(p, "/")
}

// TreeEditDistance computes the edit distance between two filesystem trees
// Uses dynamic programming with memoization for efficiency
type TreeEditCalculator struct {
	cache map[string]int
}

// NewTreeEditCalculator creates a new calculator with memoization
func NewTreeEditCalculator() *TreeEditCalculator {
	return &TreeEditCalculator{
		cache: make(map[string]int),
	}
}

// cacheKey generates a unique key for a pair of nodes
func (c *TreeEditCalculator) cacheKey(source, target *TreeNode) string {
	sourceKey := ""
	targetKey := ""
	if source != nil {
		sourceKey = fmt.Sprintf("%p", source)
	}
	if target != nil {
		targetKey = fmt.Sprintf("%p", target)
	}
	return sourceKey + ":" + targetKey
}

// Compute calculates the tree edit distance with memoization
func (c *TreeEditCalculator) Compute(source, target *TreeNode) int {
	// Check cache
	key := c.cacheKey(source, target)
	if cached, ok := c.cache[key]; ok {
		return cached
	}

	result := c.compute(source, target)
	c.cache[key] = result
	return result
}

func (c *TreeEditCalculator) compute(source, target *TreeNode) int {
	// Base cases
	if source == nil && target == nil {
		return 0
	}
	if source == nil {
		return countNodes(target)
	}
	if target == nil {
		return countNodes(source)
	}

	// Cost of relabeling this node
	cost := 0
	if !nodesEqual(source, target) {
		cost = 1
	}

	// Compute child edit distances using DP
	sourceChildren := source.Children
	targetChildren := target.Children
	m := len(sourceChildren)
	n := len(targetChildren)

	// DP table for matching children
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	// Initialize base cases
	for i := 1; i <= m; i++ {
		dp[i][0] = dp[i-1][0] + countNodes(sourceChildren[i-1])
	}
	for j := 1; j <= n; j++ {
		dp[0][j] = dp[0][j-1] + countNodes(targetChildren[j-1])
	}

	// Fill DP table
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			// Cost of matching children[i-1] with children[j-1]
			matchCost := c.Compute(sourceChildren[i-1], targetChildren[j-1])

			// Cost of deleting source child
			deleteCost := dp[i-1][j] + countNodes(sourceChildren[i-1])

			// Cost of inserting target child
			insertCost := dp[i][j-1] + countNodes(targetChildren[j-1])

			// Take minimum
			dp[i][j] = min(matchCost+dp[i-1][j-1], min(deleteCost, insertCost))
		}
	}

	return cost + dp[m][n]
}

// ComputeTreeEditDistance is a convenience function that creates a calculator
// and computes the distance
func ComputeTreeEditDistance(source, target *TreeNode) int {
	calc := NewTreeEditCalculator()
	return calc.Compute(source, target)
}

// nodesEqual checks if two tree nodes represent the same content
func nodesEqual(a, b *TreeNode) bool {
	if a == nil || b == nil {
		return a == b
	}

	// Compare by C4 ID if available (direct comparison, no String())
	if a.Entry != nil && b.Entry != nil {
		if !a.Entry.C4ID.IsNil() && !b.Entry.C4ID.IsNil() {
			return a.Entry.C4ID == b.Entry.C4ID
		}
	}

	// Fallback to name comparison
	return a.Name == b.Name
}

// countNodes counts the total number of nodes in a tree
func countNodes(node *TreeNode) int {
	if node == nil {
		return 0
	}
	count := 1
	for _, child := range node.Children {
		count += countNodes(child)
	}
	return count
}

// TreeSimilarity computes a similarity score between two trees (0.0 to 1.0)
func TreeSimilarity(source, target *TreeNode) float64 {
	sourceCount := countNodes(source)
	targetCount := countNodes(target)

	if sourceCount == 0 && targetCount == 0 {
		return 1.0
	}

	distance := ComputeTreeEditDistance(source, target)
	maxNodes := max(sourceCount, targetCount)

	if maxNodes == 0 {
		return 1.0
	}

	// Similarity = 1 - (distance / maxNodes)
	similarity := 1.0 - float64(distance)/float64(maxNodes)
	if similarity < 0 {
		similarity = 0
	}
	return similarity
}

// FindMovedDirectories detects directories that moved as a unit
func FindMovedDirectories(source, target *c4m.Manifest) []Operation {
	var moves []Operation

	sourceTree := BuildTree(source)
	targetTree := BuildTree(target)

	// Build map of source directories by content hash
	sourceDirs := make(map[c4.ID]*TreeNode)
	collectDirsWithHash(sourceTree, sourceDirs)

	// Find matching directories in target
	targetDirs := make(map[c4.ID]*TreeNode)
	collectDirsWithHash(targetTree, targetDirs)

	// Find directories with same content hash but different path
	for id, sourceNode := range sourceDirs {
		if targetNode, exists := targetDirs[id]; exists {
			sourcePath := nodePath(sourceNode)
			targetPath := nodePath(targetNode)
			if sourcePath != targetPath {
				moves = append(moves, Operation{
					Type:   OpMove,
					Source: sourcePath,
					Target: targetPath,
				})
			}
		}
	}

	return moves
}

// collectDirsWithHash collects directories that have a computed hash
func collectDirsWithHash(node *TreeNode, result map[c4.ID]*TreeNode) {
	if node == nil {
		return
	}

	// If this is a directory with a hash (from manifest), add it
	if node.Entry != nil && node.Entry.IsDir() && !node.Entry.C4ID.IsNil() {
		result[node.Entry.C4ID] = node
	}

	for _, child := range node.Children {
		collectDirsWithHash(child, result)
	}
}

// nodePath returns the full path to a node
func nodePath(node *TreeNode) string {
	if node == nil || node.Parent == nil {
		return node.Name
	}

	parts := []string{}
	for n := node; n != nil && n.Name != ""; n = n.Parent {
		parts = append([]string{n.Name}, parts...)
	}
	return strings.Join(parts, "/")
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// String returns a string representation of the plan
func (p *Plan) String() string {
	var sb strings.Builder

	sb.WriteString("Transformation Plan:\n")
	sb.WriteString(fmt.Sprintf("  Total Operations: %d\n", p.Stats.TotalOps))
	sb.WriteString(fmt.Sprintf("  Adds: %d, Deletes: %d, Modifies: %d, Moves: %d, Copies: %d\n",
		p.Stats.Adds, p.Stats.Deletes, p.Stats.Modifies, p.Stats.Moves, p.Stats.Copies))
	sb.WriteString(fmt.Sprintf("  Bytes to transfer: %d\n", p.Stats.BytesToAdd))
	sb.WriteString(fmt.Sprintf("  Bytes to move locally: %d\n", p.Stats.BytesToMove))
	sb.WriteString("\nOperations:\n")

	for _, op := range p.Operations {
		switch op.Type {
		case OpAdd:
			sb.WriteString(fmt.Sprintf("  ADD: %s\n", op.Target))
		case OpDelete:
			sb.WriteString(fmt.Sprintf("  DELETE: %s\n", op.Target))
		case OpModify:
			sb.WriteString(fmt.Sprintf("  MODIFY: %s\n", op.Target))
		case OpMove:
			sb.WriteString(fmt.Sprintf("  MOVE: %s -> %s\n", op.Source, op.Target))
		case OpCopy:
			sb.WriteString(fmt.Sprintf("  COPY: %s -> %s\n", op.Source, op.Target))
		}
	}

	return sb.String()
}
