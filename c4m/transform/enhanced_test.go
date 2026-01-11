package transform

import (
	"os"
	"testing"
	"time"

	"github.com/Avalanche-io/c4/c4m"
)

func TestExtendedOpString(t *testing.T) {
	tests := []struct {
		op       OpType
		expected string
	}{
		{OpAdd, "ADD"},
		{OpDelete, "DELETE"},
		{OpModify, "MODIFY"},
		{OpMove, "MOVE"},
		{OpCopy, "COPY"},
		{OpMkdir, "MKDIR"},
		{OpRmdir, "RMDIR"},
		{OpChmod, "CHMOD"},
		{OpTouch, "TOUCH"},
		{OpSeqAdd, "SEQ_ADD"},
		{OpSeqDel, "SEQ_DEL"},
		{OpType(99), "UNKNOWN"},
	}

	for _, test := range tests {
		if got := ExtendedOpString(test.op); got != test.expected {
			t.Errorf("ExtendedOpString(%d) = %s, want %s", test.op, got, test.expected)
		}
	}
}

func TestDefaultEnhancedConfig(t *testing.T) {
	config := DefaultEnhancedConfig()

	if !config.DetectMoves {
		t.Error("DetectMoves should be true")
	}
	if !config.DetectCopies {
		t.Error("DetectCopies should be true")
	}
	if !config.DetectDirOps {
		t.Error("DetectDirOps should be true")
	}
	if !config.DetectAttributes {
		t.Error("DetectAttributes should be true")
	}
	if !config.DetectSequences {
		t.Error("DetectSequences should be true")
	}
	if len(config.SequencePatterns) == 0 {
		t.Error("Should have default sequence patterns")
	}
}

func testDirEntry(name string, c4idSuffix string) *c4m.Entry {
	entry := &c4m.Entry{
		Name: name,
		Mode: os.ModeDir | 0755,
	}
	if c4idSuffix != "" {
		entry.C4ID = testEntry(name, 0, c4idSuffix).C4ID
	}
	return entry
}

func TestEnhancedTransformMkdir(t *testing.T) {
	source := testManifest()
	target := testManifest(
		testDirEntry("newdir/", "dir1"),
		testEntry("newdir/file.txt", 100, "content"),
	)

	et := NewEnhancedTransformer(nil)
	plan, err := et.TransformEnhanced(source, target)
	if err != nil {
		t.Fatalf("TransformEnhanced failed: %v", err)
	}

	hasMkdir := false
	for _, op := range plan.Operations {
		if op.Type == OpMkdir {
			hasMkdir = true
			break
		}
	}

	if !hasMkdir {
		t.Error("Expected MKDIR operation for new directory")
	}
}

func TestEnhancedTransformRmdir(t *testing.T) {
	source := testManifest(
		testDirEntry("olddir/", "dir1"),
	)
	target := testManifest()

	et := NewEnhancedTransformer(nil)
	plan, err := et.TransformEnhanced(source, target)
	if err != nil {
		t.Fatalf("TransformEnhanced failed: %v", err)
	}

	hasRmdir := false
	for _, op := range plan.Operations {
		if op.Type == OpRmdir {
			hasRmdir = true
			break
		}
	}

	if !hasRmdir {
		t.Error("Expected RMDIR operation for removed directory")
	}
}

func TestEnhancedTransformChmod(t *testing.T) {
	entry1 := testEntry("file.txt", 100, "same-content")
	entry1.Mode = 0644

	entry2 := testEntry("file.txt", 100, "same-content")
	entry2.Mode = 0755

	source := testManifest(entry1)
	target := testManifest(entry2)

	et := NewEnhancedTransformer(nil)
	plan, err := et.TransformEnhanced(source, target)
	if err != nil {
		t.Fatalf("TransformEnhanced failed: %v", err)
	}

	hasChmod := false
	for _, op := range plan.Operations {
		if op.Type == OpChmod {
			hasChmod = true
			break
		}
	}

	if !hasChmod {
		t.Error("Expected CHMOD operation for permission change")
	}
}

func TestEnhancedTransformTouch(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Hour)

	entry1 := testEntry("file.txt", 100, "same-content")
	entry1.Timestamp = now

	entry2 := testEntry("file.txt", 100, "same-content")
	entry2.Timestamp = later

	source := testManifest(entry1)
	target := testManifest(entry2)

	et := NewEnhancedTransformer(nil)
	plan, err := et.TransformEnhanced(source, target)
	if err != nil {
		t.Fatalf("TransformEnhanced failed: %v", err)
	}

	hasTouch := false
	for _, op := range plan.Operations {
		if op.Type == OpTouch {
			hasTouch = true
			break
		}
	}

	if !hasTouch {
		t.Error("Expected TOUCH operation for timestamp change")
	}
}

func TestSequenceDetection(t *testing.T) {
	manifest := testManifest(
		testEntry("render/frame.0001.exr", 1000, "f1"),
		testEntry("render/frame.0002.exr", 1000, "f2"),
		testEntry("render/frame.0003.exr", 1000, "f3"),
		testEntry("render/frame.0005.exr", 1000, "f5"), // Gap at 4
	)

	sequences := DetectSequences(manifest)

	if len(sequences) != 1 {
		t.Fatalf("Expected 1 sequence, got %d", len(sequences))
	}

	var seq *Sequence
	for _, s := range sequences {
		seq = s
	}

	if len(seq.Frames) != 4 {
		t.Errorf("Expected 4 frames, got %d", len(seq.Frames))
	}

	start, end := seq.FrameRange()
	if start != 1 || end != 5 {
		t.Errorf("Expected range 1-5, got %d-%d", start, end)
	}

	missing := seq.MissingFrames()
	if len(missing) != 1 || missing[0] != 4 {
		t.Errorf("Expected missing frame 4, got %v", missing)
	}
}

func TestSequencePatterns(t *testing.T) {
	tests := []struct {
		filename string
		isSeq    bool
	}{
		{"render.0001.exr", true},
		{"render_0001.exr", true},
		{"render.0001", true},
		{"render0001.exr", true},
		{"render.exr", false},
		{"render_v001.exr", false}, // Only 3 digits
	}

	for _, test := range tests {
		manifest := testManifest(
			testEntry(test.filename, 100, "content"),
		)
		sequences := DetectSequences(manifest)

		if test.isSeq && len(sequences) == 0 {
			t.Errorf("%s should be detected as sequence", test.filename)
		}
		if !test.isSeq && len(sequences) > 0 {
			t.Errorf("%s should NOT be detected as sequence", test.filename)
		}
	}
}

func TestSequenceDiff(t *testing.T) {
	source := testManifest(
		testEntry("seq/frame.0001.exr", 100, "f1"),
		testEntry("seq/frame.0002.exr", 100, "f2"),
		testEntry("seq/frame.0003.exr", 100, "f3"),
	)
	target := testManifest(
		testEntry("seq/frame.0002.exr", 100, "f2"),
		testEntry("seq/frame.0003.exr", 100, "f3-modified"), // Modified
		testEntry("seq/frame.0004.exr", 100, "f4"),          // Added
		testEntry("seq/frame.0005.exr", 100, "f5"),          // Added
	)

	diffs := CompareSequences(source, target)

	if len(diffs) != 1 {
		t.Fatalf("Expected 1 sequence diff, got %d", len(diffs))
	}

	diff := diffs[0]

	if len(diff.RemovedFrames) != 1 || diff.RemovedFrames[0] != 1 {
		t.Errorf("Expected frame 1 removed, got %v", diff.RemovedFrames)
	}

	if len(diff.AddedFrames) != 2 {
		t.Errorf("Expected 2 frames added, got %v", diff.AddedFrames)
	}

	if len(diff.ModifiedFrames) != 1 || diff.ModifiedFrames[0] != 3 {
		t.Errorf("Expected frame 3 modified, got %v", diff.ModifiedFrames)
	}
}

func TestFormatFrameRange(t *testing.T) {
	tests := []struct {
		frames   []int
		expected string
	}{
		{[]int{}, ""},
		{[]int{1}, "1"},
		{[]int{1, 2, 3}, "1-3"},
		{[]int{1, 2, 3, 5, 6, 10}, "1-3,5-6,10"},
		{[]int{1, 3, 5, 7}, "1,3,5,7"},
	}

	for _, test := range tests {
		result := formatFrameRange(test.frames)
		if result != test.expected {
			t.Errorf("formatFrameRange(%v) = %s, want %s", test.frames, result, test.expected)
		}
	}
}

func TestEnhancedOperationOrder(t *testing.T) {
	source := testManifest(
		testDirEntry("olddir/", "d1"),
		testEntry("olddir/file.txt", 100, "f1"),
	)
	target := testManifest(
		testDirEntry("newdir/", "d2"),
		testEntry("newdir/file.txt", 100, "f2"),
	)

	et := NewEnhancedTransformer(nil)
	plan, _ := et.TransformEnhanced(source, target)

	// Verify order: rmdir before mkdir, deletes before adds
	priority := map[OpType]int{
		OpRmdir:  0,
		OpDelete: 1,
		OpSeqDel: 2,
		OpMove:   3,
		OpCopy:   4,
		OpMkdir:  5,
		OpModify: 6,
		OpAdd:    7,
		OpSeqAdd: 8,
		OpChmod:  9,
		OpTouch:  10,
	}

	lastPriority := -1
	for _, op := range plan.Operations {
		p := priority[op.Type]
		if p < lastPriority {
			t.Errorf("Operation %s came after higher priority operation", ExtendedOpString(op.Type))
		}
		lastPriority = p
	}
}

func TestEnhancedConfigDisabled(t *testing.T) {
	config := &EnhancedConfig{
		Config: Config{
			DetectMoves:  true,
			DetectCopies: true,
		},
		DetectDirOps:     false,
		DetectAttributes: false,
		DetectSequences:  false,
	}

	entry1 := testEntry("file.txt", 100, "same-content")
	entry1.Mode = 0644

	entry2 := testEntry("file.txt", 100, "same-content")
	entry2.Mode = 0755

	source := testManifest(entry1)
	target := testManifest(entry2)

	et := NewEnhancedTransformer(config)
	plan, _ := et.TransformEnhanced(source, target)

	// Should not have CHMOD when attribute detection disabled
	for _, op := range plan.Operations {
		if op.Type == OpChmod {
			t.Error("CHMOD should not be detected when DetectAttributes is false")
		}
	}
}

// Benchmarks

func BenchmarkEnhancedTransform(b *testing.B) {
	source := testManifest()
	target := testManifest()

	for i := 0; i < 100; i++ {
		source.AddEntry(testEntry("dir/file"+string(rune('0'+i%10))+".txt", int64(i), "s"+string(rune('0'+i%10))))
		target.AddEntry(testEntry("dir/file"+string(rune('0'+i%10))+".txt", int64(i), "t"+string(rune('0'+i%10))))
	}

	et := NewEnhancedTransformer(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		et.TransformEnhanced(source, target)
	}
}

func BenchmarkSequenceDetection(b *testing.B) {
	manifest := testManifest()
	for i := 1; i <= 100; i++ {
		name := "render/frame." + padNumber(i, 4) + ".exr"
		manifest.AddEntry(testEntry(name, 1000, "f"+string(rune('0'+i%10))))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DetectSequences(manifest)
	}
}

func padNumber(n, width int) string {
	s := ""
	for i := 0; i < width; i++ {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func BenchmarkCompareSequences(b *testing.B) {
	source := testManifest()
	target := testManifest()

	for i := 1; i <= 100; i++ {
		name := "seq/frame." + padNumber(i, 4) + ".exr"
		source.AddEntry(testEntry(name, 1000, "s"+padNumber(i, 4)))
		if i > 10 { // Remove first 10, add last 10 different
			target.AddEntry(testEntry(name, 1000, "t"+padNumber(i, 4)))
		}
	}
	for i := 101; i <= 110; i++ {
		name := "seq/frame." + padNumber(i, 4) + ".exr"
		target.AddEntry(testEntry(name, 1000, "t"+padNumber(i, 4)))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CompareSequences(source, target)
	}
}
