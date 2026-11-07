package c4

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

// failAfter yields n bytes of data, then fails with err: a file whose
// read fails partway, as a non-local file on a mount does with EIO.
type failAfter struct {
	data []byte
	n    int
	err  error
}

func (f *failAfter) Read(p []byte) (int, error) {
	if f.n <= 0 {
		return 0, f.err
	}
	k := copy(p, f.data[:f.n])
	f.n -= k
	f.data = f.data[k:]
	return k, nil
}

// A read error never becomes a C4 ID: not the ID of the bytes read so
// far, and not the ID of empty input (which shasum prints in this case,
// measured by the storage lead on 2026-11-07). The result is the nil ID
// and, from IdentifyReader, the error itself.
func TestReadErrorNeverBecomesAnID(t *testing.T) {
	boom := errors.New("read: input/output error")
	payload := []byte("hello world, this file is twelve kilobytes in a real case")

	for _, n := range []int{0, 5, len(payload) - 1} {
		id, err := IdentifyReader(&failAfter{data: payload, n: n, err: boom})
		if !errors.Is(err, boom) {
			t.Fatalf("after %d bytes: error %v, want the read error", n, err)
		}
		if !id.IsNil() {
			t.Fatalf("after %d bytes: ID %s, want the nil ID", n, id)
		}
		if got := Identify(&failAfter{data: payload, n: n, err: boom}); !got.IsNil() {
			t.Fatalf("Identify after %d bytes: %s, want the nil ID", n, got)
		}
	}

	// The two things a failure must never be confused with.
	partial := Identify(bytes.NewReader(payload[:5]))
	empty := Identify(bytes.NewReader(nil))
	if partial.IsNil() || empty.IsNil() || partial == empty {
		t.Fatalf("the partial-input and empty-input IDs must be real and distinct")
	}
	whole, err := IdentifyReader(bytes.NewReader(payload))
	if err != nil || whole != Identify(bytes.NewReader(payload)) {
		t.Fatalf("a reader that ends with EOF identifies normally: %v", err)
	}
	var _ io.Reader = &failAfter{}
}
