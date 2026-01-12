package chunk

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestMarshalUnmarshalEmpty(t *testing.T) {
	d := &Delta{}

	data, err := Marshal(d)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	d2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(d2.Operations) != 0 {
		t.Errorf("expected 0 operations, got %d", len(d2.Operations))
	}
}

func TestMarshalUnmarshalRefs(t *testing.T) {
	d := &Delta{}
	d.AddRef(0, 1000)
	d.AddRef(2000, 500)
	d.AddRef(10000, 100)

	data, err := Marshal(d)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	d2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(d2.Operations) != 3 {
		t.Fatalf("expected 3 operations, got %d", len(d2.Operations))
	}

	// Verify first op
	if d2.Operations[0].Type != OpRef {
		t.Error("expected OpRef")
	}
	if d2.Operations[0].Offset != 0 {
		t.Errorf("expected offset 0, got %d", d2.Operations[0].Offset)
	}
	if d2.Operations[0].Size != 1000 {
		t.Errorf("expected size 1000, got %d", d2.Operations[0].Size)
	}

	// Verify stats
	if d2.RefCount() != 3 {
		t.Errorf("expected RefCount 3, got %d", d2.RefCount())
	}
	if d2.RefBytes() != 1600 {
		t.Errorf("expected RefBytes 1600, got %d", d2.RefBytes())
	}
}

func TestMarshalUnmarshalLiterals(t *testing.T) {
	d := &Delta{}
	d.AddLiteral([]byte("hello world"))
	d.AddLiteral([]byte("second literal with more data"))

	data, err := Marshal(d)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	d2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(d2.Operations) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(d2.Operations))
	}

	if string(d2.Operations[0].Data) != "hello world" {
		t.Errorf("first literal mismatch: %q", d2.Operations[0].Data)
	}
	if string(d2.Operations[1].Data) != "second literal with more data" {
		t.Errorf("second literal mismatch: %q", d2.Operations[1].Data)
	}
}

func TestMarshalUnmarshalMixed(t *testing.T) {
	d := &Delta{}
	d.AddRef(0, 1000)
	d.AddLiteral([]byte("inserted"))
	d.AddRef(1000, 2000)
	d.AddLiteral([]byte("more"))
	d.AddRef(5000, 500)

	data, err := Marshal(d)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	d2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(d2.Operations) != 5 {
		t.Fatalf("expected 5 operations, got %d", len(d2.Operations))
	}

	// Check types
	expectedTypes := []OpType{OpRef, OpLiteral, OpRef, OpLiteral, OpRef}
	for i, expected := range expectedTypes {
		if d2.Operations[i].Type != expected {
			t.Errorf("op %d: expected %v, got %v", i, expected, d2.Operations[i].Type)
		}
	}

	// Check target size preserved
	if d2.TargetSize() != d.TargetSize() {
		t.Errorf("target size mismatch: %d vs %d", d2.TargetSize(), d.TargetSize())
	}
}

func TestMarshalUnmarshalLargeData(t *testing.T) {
	// Create delta with large literal
	largeData := make([]byte, 1*MB)
	rand.Read(largeData)

	d := &Delta{}
	d.AddRef(0, 500*KB)
	d.AddLiteral(largeData)
	d.AddRef(1*MB, 500*KB)

	data, err := Marshal(d)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	d2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if !bytes.Equal(d2.Operations[1].Data, largeData) {
		t.Error("large literal data mismatch")
	}
}

func TestMarshalUnmarshalRoundtrip(t *testing.T) {
	// Create a realistic delta from actual chunking
	base := make([]byte, 500*KB)
	target := make([]byte, 500*KB)
	rand.Read(base)
	copy(target, base[:400*KB])
	rand.Read(target[400*KB:])

	original := QuickDelta(base, target)

	// Marshal/Unmarshal
	data, err := Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	decoded, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Verify both deltas produce same result
	result1, _ := original.Apply(base)
	result2, _ := decoded.Apply(base)

	if !bytes.Equal(result1, result2) {
		t.Error("roundtrip produced different results")
	}
	if !bytes.Equal(result1, target) {
		t.Error("results don't match target")
	}
}

func TestEncoderDecoder(t *testing.T) {
	d := &Delta{}
	d.AddRef(100, 200)
	d.AddLiteral([]byte("test data"))
	d.AddRef(500, 300)

	var buf bytes.Buffer

	// Encode
	enc := NewEncoder(&buf)
	if err := enc.Encode(d); err != nil {
		t.Fatalf("Encode error: %v", err)
	}

	// Decode
	dec := NewDecoder(&buf)
	d2, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}

	if len(d2.Operations) != 3 {
		t.Errorf("expected 3 operations, got %d", len(d2.Operations))
	}
}

func TestStreamingEncoder(t *testing.T) {
	var buf bytes.Buffer

	// Calculate expected target size
	targetSize := int64(1000 + 11 + 2000) // ref + literal + ref

	enc, err := NewStreamingEncoder(&buf, 3, targetSize)
	if err != nil {
		t.Fatalf("NewStreamingEncoder error: %v", err)
	}

	if err := enc.WriteRef(0, 1000); err != nil {
		t.Fatalf("WriteRef error: %v", err)
	}
	if err := enc.WriteLiteral([]byte("hello world")); err != nil {
		t.Fatalf("WriteLiteral error: %v", err)
	}
	if err := enc.WriteRef(2000, 2000); err != nil {
		t.Fatalf("WriteRef error: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	// Decode and verify
	dec := NewDecoder(&buf)
	d, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}

	if len(d.Operations) != 3 {
		t.Errorf("expected 3 operations, got %d", len(d.Operations))
	}
	if d.TargetSize() != targetSize {
		t.Errorf("target size mismatch: %d vs %d", d.TargetSize(), targetSize)
	}
}

func TestStreamingDecoder(t *testing.T) {
	// Create and encode a delta
	d := &Delta{}
	d.AddRef(0, 1000)
	d.AddLiteral([]byte("test"))
	d.AddRef(2000, 500)

	data, _ := Marshal(d)

	// Decode streaming
	dec, err := NewStreamingDecoder(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("NewStreamingDecoder error: %v", err)
	}

	header := dec.Header()
	if header.OpCount != 3 {
		t.Errorf("expected 3 ops in header, got %d", header.OpCount)
	}

	// Read ops one by one
	ops := make([]DeltaOp, 0, 3)
	for {
		op, err := dec.Next()
		if err != nil {
			t.Fatalf("Next error: %v", err)
		}
		if op == nil {
			break
		}
		ops = append(ops, *op)
	}

	if len(ops) != 3 {
		t.Errorf("expected 3 ops, got %d", len(ops))
	}
	if dec.OpsRemaining() != 0 {
		t.Errorf("expected 0 remaining, got %d", dec.OpsRemaining())
	}
}

func TestDecodeErrors(t *testing.T) {
	t.Run("invalid magic", func(t *testing.T) {
		data := []byte("XXXX" + "\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00")
		_, err := Unmarshal(data)
		if err == nil || err != ErrInvalidMagic {
			t.Errorf("expected ErrInvalidMagic, got %v", err)
		}
	})

	t.Run("invalid version", func(t *testing.T) {
		data := []byte("C4DL\x99\x00\x00\x00\x00\x00\x00\x00\x00\x00")
		_, err := Unmarshal(data)
		if err == nil {
			t.Error("expected error for invalid version")
		}
	})

	t.Run("truncated header", func(t *testing.T) {
		data := []byte("C4DL\x01")
		_, err := Unmarshal(data)
		if err == nil {
			t.Error("expected error for truncated header")
		}
	})

	t.Run("invalid op type", func(t *testing.T) {
		d := &Delta{}
		d.Operations = append(d.Operations, DeltaOp{Type: OpType('X'), Size: 10})
		d.stats.targetSize = 10

		_, err := Marshal(d)
		if err == nil {
			t.Error("expected error for invalid op type")
		}
	})
}

func TestWireSize(t *testing.T) {
	d := &Delta{}
	d.AddRef(0, 100)
	d.AddLiteral([]byte("hello"))
	d.AddRef(200, 50)

	estimated := WireSize(d)
	actual, _ := Marshal(d)

	// Estimated should be close to actual
	if estimated < len(actual) {
		t.Errorf("estimated %d < actual %d", estimated, len(actual))
	}
	// Should not overestimate by much
	if estimated > len(actual)+10 {
		t.Errorf("estimated %d too far from actual %d", estimated, len(actual))
	}
}

func TestVarintEncoding(t *testing.T) {
	// Test with large offsets and sizes
	d := &Delta{}
	d.AddRef(1<<40, 1<<20) // Large offset (1TB), large size (1MB)
	d.AddRef(0, 1)         // Small values

	data, err := Marshal(d)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	d2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if d2.Operations[0].Offset != 1<<40 {
		t.Errorf("large offset mismatch: %d", d2.Operations[0].Offset)
	}
	if d2.Operations[0].Size != 1<<20 {
		t.Errorf("large size mismatch: %d", d2.Operations[0].Size)
	}
}

func TestHeaderFields(t *testing.T) {
	d := &Delta{}
	d.AddRef(0, 1000)
	d.AddLiteral(make([]byte, 500))
	d.AddRef(2000, 2000)

	data, _ := Marshal(d)

	// Check magic
	if string(data[:4]) != WireMagic {
		t.Errorf("magic mismatch: %q", data[:4])
	}

	// Check version
	if data[4] != WireVersion {
		t.Errorf("version mismatch: %d", data[4])
	}
}

func TestStreamingEncoderClosed(t *testing.T) {
	var buf bytes.Buffer

	enc, _ := NewStreamingEncoder(&buf, 1, 100)
	enc.WriteRef(0, 100)
	enc.Close()

	// Writing after close should error
	err := enc.WriteRef(100, 100)
	if err == nil {
		t.Error("expected error writing after close")
	}
}

func TestEmptyLiteral(t *testing.T) {
	d := &Delta{}
	d.AddLiteral([]byte{})
	d.AddRef(0, 100)

	data, err := Marshal(d)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	d2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(d2.Operations[0].Data) != 0 {
		t.Error("empty literal should have 0-length data")
	}
}

// ----------------------------------------------------------------------------
// Benchmarks
// ----------------------------------------------------------------------------

func BenchmarkMarshal(b *testing.B) {
	// Create realistic delta
	base := make([]byte, 1*MB)
	target := make([]byte, 1*MB)
	rand.Read(base)
	copy(target, base[:800*KB])
	rand.Read(target[800*KB:])

	d := QuickDelta(base, target)
	totalBytes := d.TargetSize()

	b.SetBytes(totalBytes)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = Marshal(d)
	}
}

func BenchmarkUnmarshal(b *testing.B) {
	// Create and marshal delta
	base := make([]byte, 1*MB)
	target := make([]byte, 1*MB)
	rand.Read(base)
	copy(target, base[:800*KB])
	rand.Read(target[800*KB:])

	d := QuickDelta(base, target)
	data, _ := Marshal(d)

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = Unmarshal(data)
	}
}

func BenchmarkStreamingDecode(b *testing.B) {
	// Create and marshal delta
	d := &Delta{}
	for i := 0; i < 1000; i++ {
		d.AddRef(int64(i*1000), 1000)
	}
	data, _ := Marshal(d)

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dec, _ := NewStreamingDecoder(bytes.NewReader(data))
		for {
			op, _ := dec.Next()
			if op == nil {
				break
			}
		}
	}
}

func BenchmarkWireSize(b *testing.B) {
	d := &Delta{}
	for i := 0; i < 100; i++ {
		d.AddRef(int64(i*1000), 1000)
		d.AddLiteral(make([]byte, 100))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = WireSize(d)
	}
}
