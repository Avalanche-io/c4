package chunk

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"
)

func TestOpTypeString(t *testing.T) {
	tests := []struct {
		op   OpType
		want string
	}{
		{OpRef, "REF"},
		{OpLiteral, "LITERAL"},
		{OpType('X'), "UNKNOWN(X)"},
	}

	for _, tt := range tests {
		if got := tt.op.String(); got != tt.want {
			t.Errorf("OpType(%c).String() = %q, want %q", tt.op, got, tt.want)
		}
	}
}

func TestDeltaAddRef(t *testing.T) {
	d := &Delta{}
	d.AddRef(100, 500)

	if len(d.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(d.Operations))
	}

	op := d.Operations[0]
	if op.Type != OpRef {
		t.Errorf("expected OpRef, got %v", op.Type)
	}
	if op.Offset != 100 {
		t.Errorf("expected offset 100, got %d", op.Offset)
	}
	if op.Size != 500 {
		t.Errorf("expected size 500, got %d", op.Size)
	}
	if d.RefCount() != 1 {
		t.Errorf("expected RefCount 1, got %d", d.RefCount())
	}
	if d.RefBytes() != 500 {
		t.Errorf("expected RefBytes 500, got %d", d.RefBytes())
	}
}

func TestDeltaAddLiteral(t *testing.T) {
	d := &Delta{}
	data := []byte("hello world")
	d.AddLiteral(data)

	if len(d.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(d.Operations))
	}

	op := d.Operations[0]
	if op.Type != OpLiteral {
		t.Errorf("expected OpLiteral, got %v", op.Type)
	}
	if op.Size != len(data) {
		t.Errorf("expected size %d, got %d", len(data), op.Size)
	}
	if !bytes.Equal(op.Data, data) {
		t.Errorf("data mismatch")
	}
	if d.LiteralCount() != 1 {
		t.Errorf("expected LiteralCount 1, got %d", d.LiteralCount())
	}
	if d.LiteralBytes() != int64(len(data)) {
		t.Errorf("expected LiteralBytes %d, got %d", len(data), d.LiteralBytes())
	}
}

func TestDeltaEfficiency(t *testing.T) {
	tests := []struct {
		name       string
		refs       int64
		literals   int64
		wantEff    float64
		wantTarget int64
	}{
		{"all refs", 1000, 0, 1.0, 1000},
		{"all literals", 0, 1000, 0.0, 1000},
		{"50/50", 500, 500, 0.5, 1000},
		{"75% refs", 750, 250, 0.75, 1000},
		{"empty", 0, 0, 0.0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Delta{}
			if tt.refs > 0 {
				d.AddRef(0, int(tt.refs))
			}
			if tt.literals > 0 {
				d.AddLiteral(make([]byte, tt.literals))
			}

			if got := d.Efficiency(); got != tt.wantEff {
				t.Errorf("Efficiency() = %v, want %v", got, tt.wantEff)
			}
			if got := d.TargetSize(); got != tt.wantTarget {
				t.Errorf("TargetSize() = %v, want %v", got, tt.wantTarget)
			}
		})
	}
}

func TestDeltaApplySimple(t *testing.T) {
	base := []byte("hello world, this is a test")

	d := &Delta{}
	d.AddRef(0, 5)           // "hello"
	d.AddLiteral([]byte("!")) // "!"
	d.AddRef(11, 16)          // ", this is a test"

	result, err := d.Apply(base)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	expected := []byte("hello!, this is a test")
	if !bytes.Equal(result, expected) {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestDeltaApplyAllRefs(t *testing.T) {
	base := []byte("the quick brown fox")

	d := &Delta{}
	d.AddRef(0, len(base))

	result, err := d.Apply(base)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	if !bytes.Equal(result, base) {
		t.Errorf("got %q, want %q", result, base)
	}
}

func TestDeltaApplyAllLiterals(t *testing.T) {
	base := []byte("old content")
	newData := []byte("completely new content")

	d := &Delta{}
	d.AddLiteral(newData)

	result, err := d.Apply(base)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	if !bytes.Equal(result, newData) {
		t.Errorf("got %q, want %q", result, newData)
	}
}

func TestDeltaApplyErrors(t *testing.T) {
	base := []byte("short")

	t.Run("ref out of bounds", func(t *testing.T) {
		d := &Delta{}
		d.AddRef(0, 100) // base is only 5 bytes

		_, err := d.Apply(base)
		if err == nil {
			t.Error("expected error for out of bounds ref")
		}
	})

	t.Run("negative offset", func(t *testing.T) {
		d := &Delta{}
		d.Operations = append(d.Operations, DeltaOp{
			Type:   OpRef,
			Offset: -1,
			Size:   5,
		})

		_, err := d.Apply(base)
		if err == nil {
			t.Error("expected error for negative offset")
		}
	})

	t.Run("literal size mismatch", func(t *testing.T) {
		d := &Delta{}
		d.Operations = append(d.Operations, DeltaOp{
			Type: OpLiteral,
			Size: 100, // declared 100
			Data: []byte("small"), // actual 5
		})

		_, err := d.Apply(base)
		if err == nil {
			t.Error("expected error for size mismatch")
		}
	})

	t.Run("unknown op type", func(t *testing.T) {
		d := &Delta{}
		d.Operations = append(d.Operations, DeltaOp{
			Type: OpType('X'),
			Size: 5,
		})

		_, err := d.Apply(base)
		if err == nil {
			t.Error("expected error for unknown op type")
		}
	})
}

func TestComputeDeltaIdentical(t *testing.T) {
	data := make([]byte, 100*KB)
	rand.Read(data)

	config := Config{MinSize: 4 * KB, AvgSize: 16 * KB, MaxSize: 64 * KB}
	chunker := NewFastCDC(config)
	chunks := chunker.ChunkBytes(data)
	sig := NewSignature(chunks)

	delta := ComputeDelta(sig, data, config)

	// Identical content should produce all refs
	if delta.LiteralCount() != 0 {
		t.Errorf("expected 0 literals for identical data, got %d", delta.LiteralCount())
	}
	if delta.Efficiency() != 1.0 {
		t.Errorf("expected 100%% efficiency for identical data, got %.1f%%", delta.Efficiency()*100)
	}

	// Verify roundtrip
	result, err := delta.Apply(data)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !bytes.Equal(result, data) {
		t.Error("roundtrip failed for identical data")
	}
}

func TestComputeDeltaCompletlyDifferent(t *testing.T) {
	base := make([]byte, 100*KB)
	target := make([]byte, 100*KB)
	rand.Read(base)
	rand.Read(target)

	config := Config{MinSize: 4 * KB, AvgSize: 16 * KB, MaxSize: 64 * KB}
	chunker := NewFastCDC(config)
	chunks := chunker.ChunkBytes(base)
	sig := NewSignature(chunks)

	delta := ComputeDelta(sig, target, config)

	// Completely different content should produce all literals
	if delta.RefCount() != 0 {
		t.Errorf("expected 0 refs for different data, got %d", delta.RefCount())
	}
	if delta.Efficiency() != 0.0 {
		t.Errorf("expected 0%% efficiency for different data, got %.1f%%", delta.Efficiency()*100)
	}

	// Verify roundtrip
	result, err := delta.Apply(base)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !bytes.Equal(result, target) {
		t.Error("roundtrip failed for different data")
	}
}

func TestComputeDeltaPartialChange(t *testing.T) {
	// Create base data
	base := make([]byte, 200*KB)
	rand.Read(base)

	// Create target with 75% unchanged, 25% changed
	target := make([]byte, 200*KB)
	copy(target, base[:150*KB])    // First 75% identical
	rand.Read(target[150*KB:])     // Last 25% different

	config := Config{MinSize: 4 * KB, AvgSize: 16 * KB, MaxSize: 64 * KB}
	chunker := NewFastCDC(config)
	chunks := chunker.ChunkBytes(base)
	sig := NewSignature(chunks)

	delta := ComputeDelta(sig, target, config)

	// Should have some refs and some literals
	if delta.RefCount() == 0 {
		t.Error("expected some refs for partially identical data")
	}
	if delta.LiteralCount() == 0 {
		t.Error("expected some literals for partially changed data")
	}

	// Efficiency should be between 0 and 1
	eff := delta.Efficiency()
	if eff <= 0 || eff >= 1 {
		t.Errorf("expected efficiency between 0 and 1, got %.2f", eff)
	}

	// Verify roundtrip
	result, err := delta.Apply(base)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !bytes.Equal(result, target) {
		t.Error("roundtrip failed for partial change")
	}
}

func TestComputeDeltaFromReader(t *testing.T) {
	base := make([]byte, 100*KB)
	target := make([]byte, 100*KB)
	rand.Read(base)
	copy(target, base[:80*KB])
	rand.Read(target[80*KB:])

	config := Config{MinSize: 4 * KB, AvgSize: 16 * KB, MaxSize: 64 * KB}
	chunker := NewFastCDC(config)
	chunks := chunker.ChunkBytes(base)
	sig := NewSignature(chunks)

	delta, err := ComputeDeltaFromReader(sig, bytes.NewReader(target), config)
	if err != nil {
		t.Fatalf("ComputeDeltaFromReader error: %v", err)
	}

	// Verify roundtrip
	result, err := delta.Apply(base)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !bytes.Equal(result, target) {
		t.Error("roundtrip failed")
	}
}

func TestDeltaApplyToWriter(t *testing.T) {
	base := []byte("hello world, this is a longer test string for verification")
	// base is 58 chars: positions 0-57

	d := &Delta{}
	d.AddRef(0, 12)             // "hello world," (positions 0-11)
	d.AddLiteral([]byte(" MODIFIED"))
	d.AddRef(22, 36)            // " longer test string for verification" (positions 22-57)

	var buf bytes.Buffer
	err := d.ApplyToWriter(bytes.NewReader(base), &buf)
	if err != nil {
		t.Fatalf("ApplyToWriter error: %v", err)
	}

	expected := "hello world, MODIFIED longer test string for verification"
	if buf.String() != expected {
		t.Errorf("got %q, want %q", buf.String(), expected)
	}
}

func TestDeltaOptimize(t *testing.T) {
	t.Run("merge adjacent refs", func(t *testing.T) {
		d := &Delta{}
		d.AddRef(0, 100)
		d.AddRef(100, 100)
		d.AddRef(200, 100)

		d.Optimize()

		if len(d.Operations) != 1 {
			t.Errorf("expected 1 merged operation, got %d", len(d.Operations))
		}
		if d.Operations[0].Size != 300 {
			t.Errorf("expected merged size 300, got %d", d.Operations[0].Size)
		}
	})

	t.Run("merge adjacent literals", func(t *testing.T) {
		d := &Delta{}
		d.AddLiteral([]byte("hello"))
		d.AddLiteral([]byte(" "))
		d.AddLiteral([]byte("world"))

		d.Optimize()

		if len(d.Operations) != 1 {
			t.Errorf("expected 1 merged operation, got %d", len(d.Operations))
		}
		if string(d.Operations[0].Data) != "hello world" {
			t.Errorf("expected merged data 'hello world', got %q", d.Operations[0].Data)
		}
	})

	t.Run("non-adjacent refs not merged", func(t *testing.T) {
		d := &Delta{}
		d.AddRef(0, 100)
		d.AddRef(200, 100) // Gap at 100-200

		d.Optimize()

		if len(d.Operations) != 2 {
			t.Errorf("expected 2 operations (non-adjacent), got %d", len(d.Operations))
		}
	})

	t.Run("mixed operations", func(t *testing.T) {
		d := &Delta{}
		d.AddRef(0, 100)
		d.AddRef(100, 100)
		d.AddLiteral([]byte("new"))
		d.AddLiteral([]byte("data"))
		d.AddRef(300, 100)

		d.Optimize()

		if len(d.Operations) != 3 {
			t.Errorf("expected 3 operations, got %d", len(d.Operations))
		}
	})
}

func TestQuickDelta(t *testing.T) {
	// Create large random base and target with most content identical
	base := make([]byte, 2*MB)
	rand.Read(base)

	target := make([]byte, 2*MB)
	copy(target, base)
	// Change ~5% of the content at the end
	rand.Read(target[1900*KB:])

	delta := QuickDelta(base, target)

	// With 95% identical content, expect reasonable efficiency
	if delta.Efficiency() < 0.5 {
		t.Errorf("expected >50%% efficiency for 95%% identical data, got %.1f%%", delta.Efficiency()*100)
	}

	// Verify roundtrip
	result, err := QuickApply(base, delta)
	if err != nil {
		t.Fatalf("QuickApply error: %v", err)
	}
	if !bytes.Equal(result, target) {
		t.Error("roundtrip failed")
	}
}

func TestVerifyDelta(t *testing.T) {
	base := []byte("original content here")
	target := []byte("modified content here")

	delta := QuickDelta(base, target)

	if !VerifyDelta(base, delta, target) {
		t.Error("VerifyDelta should return true for valid delta")
	}

	// Wrong target should fail
	if VerifyDelta(base, delta, []byte("wrong")) {
		t.Error("VerifyDelta should return false for wrong target")
	}
}

func TestDeltaBuilderStreaming(t *testing.T) {
	base := make([]byte, 100*KB)
	target := make([]byte, 100*KB)
	rand.Read(base)
	copy(target, base[:75*KB])
	rand.Read(target[75*KB:])

	config := Config{MinSize: 4 * KB, AvgSize: 16 * KB, MaxSize: 64 * KB}

	// Create base signature
	chunker := NewFastCDC(config)
	baseChunks := chunker.ChunkBytes(base)
	sig := NewSignature(baseChunks)

	// Use builder for streaming
	builder := NewDeltaBuilder(sig, config)
	targetChunks := chunker.ChunkBytes(target)
	for _, c := range targetChunks {
		builder.AddChunk(c)
	}
	delta := builder.Build()

	// Verify roundtrip
	result, err := delta.Apply(base)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !bytes.Equal(result, target) {
		t.Error("roundtrip failed with DeltaBuilder")
	}
}

func TestDeltaString(t *testing.T) {
	d := &Delta{}
	d.AddRef(0, 1000)
	d.AddLiteral(make([]byte, 500))

	s := d.String()
	if !strings.Contains(s, "ops=2") {
		t.Errorf("String should contain op count, got %s", s)
	}
	if !strings.Contains(s, "refs=1") {
		t.Errorf("String should contain ref count, got %s", s)
	}
	if !strings.Contains(s, "literals=1") {
		t.Errorf("String should contain literal count, got %s", s)
	}
}

// ----------------------------------------------------------------------------
// Benchmarks
// ----------------------------------------------------------------------------

func BenchmarkComputeDeltaIdentical(b *testing.B) {
	data := make([]byte, 1*MB)
	rand.Read(data)

	config := DefaultConfig()
	chunker := NewFastCDC(config)
	chunks := chunker.ChunkBytes(data)
	sig := NewSignature(chunks)

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ComputeDelta(sig, data, config)
	}
}

func BenchmarkComputeDelta10PercentChange(b *testing.B) {
	base := make([]byte, 1*MB)
	rand.Read(base)

	target := make([]byte, 1*MB)
	copy(target, base[:900*KB])
	rand.Read(target[900*KB:])

	config := DefaultConfig()
	chunker := NewFastCDC(config)
	chunks := chunker.ChunkBytes(base)
	sig := NewSignature(chunks)

	b.SetBytes(int64(len(target)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ComputeDelta(sig, target, config)
	}
}

func BenchmarkDeltaApply(b *testing.B) {
	base := make([]byte, 1*MB)
	rand.Read(base)

	target := make([]byte, 1*MB)
	copy(target, base[:500*KB])
	rand.Read(target[500*KB:])

	delta := QuickDelta(base, target)

	b.SetBytes(int64(len(target)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = delta.Apply(base)
	}
}

func BenchmarkDeltaOptimize(b *testing.B) {
	// Create delta with many small adjacent refs
	d := &Delta{}
	for i := 0; i < 1000; i++ {
		d.AddRef(int64(i*100), 100)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d2 := &Delta{Operations: make([]DeltaOp, len(d.Operations))}
		copy(d2.Operations, d.Operations)
		d2.Optimize()
	}
}
