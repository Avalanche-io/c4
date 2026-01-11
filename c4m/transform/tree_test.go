package transform

import (
	"bytes"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

func TestSplitPath(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", []string{}},
		{"/", []string{}},
		{"file.txt", []string{"file.txt"}},
		{"/file.txt", []string{"file.txt"}},
		{"dir/file.txt", []string{"dir", "file.txt"}},
		{"/dir/file.txt", []string{"dir", "file.txt"}},
		{"a/b/c/d.txt", []string{"a", "b", "c", "d.txt"}},
		{"/a/b/c/", []string{"a", "b", "c"}},
	}

	for _, test := range tests {
		result := splitPath(test.input)
		if len(result) != len(test.expected) {
			t.Errorf("splitPath(%q) = %v, want %v", test.input, result, test.expected)
			continue
		}
		for i, v := range result {
			if v != test.expected[i] {
				t.Errorf("splitPath(%q)[%d] = %q, want %q", test.input, i, v, test.expected[i])
			}
		}
	}
}

func TestBuildTreeEmpty(t *testing.T) {
	manifest := c4m.NewManifest()
	tree := BuildTree(manifest)

	if tree == nil {
		t.Fatal("BuildTree returned nil")
	}
	if tree.Name != "" {
		t.Errorf("Root name should be empty, got %q", tree.Name)
	}
	if len(tree.Children) != 0 {
		t.Errorf("Empty manifest should produce empty tree, got %d children", len(tree.Children))
	}
}

func TestBuildTreeSingleFile(t *testing.T) {
	manifest := c4m.NewManifest()
	manifest.AddEntry(testEntry("file.txt", 100, "content"))

	tree := BuildTree(manifest)

	if len(tree.Children) != 1 {
		t.Fatalf("Expected 1 child, got %d", len(tree.Children))
	}
	if tree.Children[0].Name != "file.txt" {
		t.Errorf("Expected file.txt, got %s", tree.Children[0].Name)
	}
	if tree.Children[0].Entry == nil {
		t.Error("Entry should not be nil")
	}
}

func TestBuildTreeNestedStructure(t *testing.T) {
	manifest := c4m.NewManifest()
	manifest.AddEntry(testEntry("a/b/file.txt", 100, "content"))
	manifest.AddEntry(testEntry("a/other.txt", 50, "other"))

	tree := BuildTree(manifest)

	// Root should have 1 child: "a"
	if len(tree.Children) != 1 {
		t.Fatalf("Expected 1 root child, got %d", len(tree.Children))
	}
	aNode := tree.Children[0]
	if aNode.Name != "a" {
		t.Errorf("Expected 'a', got %s", aNode.Name)
	}

	// "a" should have 2 children: "b" and "other.txt"
	if len(aNode.Children) != 2 {
		t.Fatalf("Expected 2 children of 'a', got %d", len(aNode.Children))
	}
}

func TestCountNodes(t *testing.T) {
	tests := []struct {
		name     string
		node     *TreeNode
		expected int
	}{
		{"nil", nil, 0},
		{"single", &TreeNode{Name: "a"}, 1},
		{"with children", &TreeNode{
			Name: "root",
			Children: []*TreeNode{
				{Name: "child1"},
				{Name: "child2"},
			},
		}, 3},
	}

	for _, test := range tests {
		result := countNodes(test.node)
		if result != test.expected {
			t.Errorf("%s: countNodes() = %d, want %d", test.name, result, test.expected)
		}
	}
}

func TestNodesEqual(t *testing.T) {
	id1 := c4.Identify(bytes.NewReader([]byte("content1")))
	id2 := c4.Identify(bytes.NewReader([]byte("content2")))

	tests := []struct {
		name     string
		a        *TreeNode
		b        *TreeNode
		expected bool
	}{
		{"both nil", nil, nil, true},
		{"a nil", nil, &TreeNode{Name: "a"}, false},
		{"b nil", &TreeNode{Name: "a"}, nil, false},
		{"same name no entry", &TreeNode{Name: "a"}, &TreeNode{Name: "a"}, true},
		{"different name", &TreeNode{Name: "a"}, &TreeNode{Name: "b"}, false},
		{
			"same c4id",
			&TreeNode{Entry: &c4m.Entry{C4ID: id1}},
			&TreeNode{Entry: &c4m.Entry{C4ID: id1}},
			true,
		},
		{
			"different c4id",
			&TreeNode{Entry: &c4m.Entry{C4ID: id1}},
			&TreeNode{Entry: &c4m.Entry{C4ID: id2}},
			false,
		},
	}

	for _, test := range tests {
		result := nodesEqual(test.a, test.b)
		if result != test.expected {
			t.Errorf("%s: nodesEqual() = %v, want %v", test.name, result, test.expected)
		}
	}
}

func TestTreeEditDistanceIdentical(t *testing.T) {
	manifest := c4m.NewManifest()
	manifest.AddEntry(testEntry("a/b.txt", 100, "content"))

	tree1 := BuildTree(manifest)
	tree2 := BuildTree(manifest)

	distance := ComputeTreeEditDistance(tree1, tree2)
	if distance != 0 {
		t.Errorf("Identical trees should have distance 0, got %d", distance)
	}
}

func TestTreeEditDistanceAddition(t *testing.T) {
	m1 := c4m.NewManifest()
	m1.AddEntry(testEntry("a.txt", 100, "a"))

	m2 := c4m.NewManifest()
	m2.AddEntry(testEntry("a.txt", 100, "a"))
	m2.AddEntry(testEntry("b.txt", 100, "b"))

	tree1 := BuildTree(m1)
	tree2 := BuildTree(m2)

	distance := ComputeTreeEditDistance(tree1, tree2)
	if distance != 1 {
		t.Errorf("Adding one node should have distance 1, got %d", distance)
	}
}

func TestTreeEditCalculatorMemoization(t *testing.T) {
	m1 := c4m.NewManifest()
	m1.AddEntry(testEntry("a/b/c.txt", 100, "c"))

	m2 := c4m.NewManifest()
	m2.AddEntry(testEntry("a/b/c.txt", 100, "c"))

	tree1 := BuildTree(m1)
	tree2 := BuildTree(m2)

	calc := NewTreeEditCalculator()

	// First computation
	d1 := calc.Compute(tree1, tree2)

	// Second should use cache
	d2 := calc.Compute(tree1, tree2)

	if d1 != d2 {
		t.Errorf("Memoized results should be identical: %d != %d", d1, d2)
	}

	// Check cache was used
	if len(calc.cache) == 0 {
		t.Error("Cache should not be empty after computation")
	}
}

func TestTreeSimilarityIdentical(t *testing.T) {
	manifest := c4m.NewManifest()
	manifest.AddEntry(testEntry("file.txt", 100, "content"))

	tree1 := BuildTree(manifest)
	tree2 := BuildTree(manifest)

	similarity := TreeSimilarity(tree1, tree2)
	if similarity != 1.0 {
		t.Errorf("Identical trees should have similarity 1.0, got %f", similarity)
	}
}

func TestTreeSimilarityEmpty(t *testing.T) {
	tree1 := &TreeNode{}
	tree2 := &TreeNode{}

	similarity := TreeSimilarity(tree1, tree2)
	if similarity != 1.0 {
		t.Errorf("Empty trees should have similarity 1.0, got %f", similarity)
	}
}

func TestNodePath(t *testing.T) {
	root := &TreeNode{Name: ""}
	a := &TreeNode{Name: "a", Parent: root}
	b := &TreeNode{Name: "b", Parent: a}
	c := &TreeNode{Name: "c.txt", Parent: b}

	tests := []struct {
		node     *TreeNode
		expected string
	}{
		{root, ""},
		{a, "a"},
		{b, "a/b"},
		{c, "a/b/c.txt"},
	}

	for _, test := range tests {
		result := nodePath(test.node)
		if result != test.expected {
			t.Errorf("nodePath() = %q, want %q", result, test.expected)
		}
	}
}

func TestMinMax(t *testing.T) {
	if min(1, 2) != 1 {
		t.Error("min(1, 2) should be 1")
	}
	if min(2, 1) != 1 {
		t.Error("min(2, 1) should be 1")
	}
	if max(1, 2) != 2 {
		t.Error("max(1, 2) should be 2")
	}
	if max(2, 1) != 2 {
		t.Error("max(2, 1) should be 2")
	}
}

// Benchmark tree operations

func BenchmarkBuildTree(b *testing.B) {
	manifest := c4m.NewManifest()
	for i := 0; i < 100; i++ {
		manifest.AddEntry(testEntry("dir"+string(rune('0'+i%10))+"/file"+string(rune('0'+i%10))+".txt", int64(i), "c"+string(rune('0'+i%10))))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildTree(manifest)
	}
}

func BenchmarkTreeEditDistance(b *testing.B) {
	m1 := c4m.NewManifest()
	m2 := c4m.NewManifest()

	for i := 0; i < 50; i++ {
		m1.AddEntry(testEntry("d"+string(rune('0'+i%10))+"/f"+string(rune('0'+i%10))+".txt", int64(i), "a"+string(rune('0'+i%10))))
		m2.AddEntry(testEntry("d"+string(rune('0'+i%10))+"/f"+string(rune('0'+i%10))+".txt", int64(i), "b"+string(rune('0'+i%10))))
	}

	tree1 := BuildTree(m1)
	tree2 := BuildTree(m2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputeTreeEditDistance(tree1, tree2)
	}
}

func BenchmarkTreeSimilarity(b *testing.B) {
	m1 := c4m.NewManifest()
	m2 := c4m.NewManifest()

	for i := 0; i < 50; i++ {
		m1.AddEntry(testEntry("dir/file"+string(rune('0'+i%10))+".txt", int64(i), "content"))
		m2.AddEntry(testEntry("dir/file"+string(rune('0'+i%10))+".txt", int64(i), "content"))
	}

	tree1 := BuildTree(m1)
	tree2 := BuildTree(m2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		TreeSimilarity(tree1, tree2)
	}
}
