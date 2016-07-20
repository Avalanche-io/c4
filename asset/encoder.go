package asset

import (
	"bytes"
	"crypto/sha512"
	"hash"
	"io"
	"io/ioutil"
	"math/big"
)

// IDEncoder generates C4 Asset IDs.
type IDEncoder struct {
	err error
	h   hash.Hash
	wr  []io.Writer
}

// NewIDEncoder makes a new IDEncoder.
func NewIDEncoder(w_op ...io.Writer) *IDEncoder {
	w := w_op

	if len(w) <= 0 {
		w = []io.Writer{ioutil.Discard}
	}

	return &IDEncoder{
		wr: w,
		h:  sha512.New(),
	}
}

// Write writes bytes to the hash that makes up the ID.
func (e *IDEncoder) Write(b []byte) (int, error) {
	w := make([]io.Writer, len(e.wr)+1)
	w = append(e.wr, e.h)
	c, err := io.Copy(io.MultiWriter(w...), bytes.NewReader(b))
	return int(c), err
}

// ID gets the ID for the written bytes.
func (e *IDEncoder) ID() *ID {
	b := new(big.Int)
	b.SetBytes(e.h.Sum(nil))
	id := ID(*b)
	return &id
}

// Reset the encoder so it can identify a new block of data.
func (e *IDEncoder) Reset() {
	e.h.Reset()
}
