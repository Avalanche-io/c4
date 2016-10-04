package fs

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

type Buffer struct {
	MTB *MultiTaskBuffer
	buf []byte
}

func (b *Buffer) Bytes() []byte {
	return b.buf
}

func (b *Buffer) Reader() io.Reader {
	return bytes.NewReader(b.buf)
}

func (b *Buffer) Release() {
	b.MTB.Release(b)
}

func (b *Buffer) Reset() {
	b.buf = b.buf[:0]
}

func (b *Buffer) Shrink(n uint64) {
	b.buf = b.buf[:n]
}

type MultiTaskBuffer struct {
	Used     uint64
	Capacity uint64
	Peak     uint64
	Ch       chan *Buffer
}

func NewMTB(capacity uint64) *MultiTaskBuffer {
	m := MultiTaskBuffer{
		Used:     0,
		Capacity: capacity,
		Ch:       make(chan *Buffer, 200),
	}
	return &m
}

func (mtb *MultiTaskBuffer) Release(b *Buffer) {
	select {
	case mtb.Ch <- b:
	default: //Discard the buffer if the pool is full.
	}
}

func (mtb *MultiTaskBuffer) Get(n uint64) (b *Buffer) {
	previusUsed := mtb.Used
	select {
	case b = <-mtb.Ch:

		c := uint64(len(b.Bytes()))
		if c < n {
			b = nil
			bb := Buffer{mtb, make([]byte, n)}
			b = &bb
		} else {
			b.Shrink(n)
		}
		previusUsed = mtb.Used
		mtb.Used = previusUsed + n - c
	default:
		bb := Buffer{mtb, make([]byte, n)}
		previusUsed = mtb.Used
		mtb.Used = previusUsed + n
		b = &bb
	}

	if mtb.Used > mtb.Capacity {
		fmt.Fprintf(os.Stderr, "MultiTaskBuffer Previous Size: %d, n: %d, Size: %d, Capacity: %d\n", previusUsed, n, mtb.Used, mtb.Capacity)
		panic("MultiTaskBuffer over allocation")
	}
	if mtb.Used > mtb.Peak {
		mtb.Peak = mtb.Used
	}
	return
}
