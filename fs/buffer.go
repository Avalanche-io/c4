package fs

import (
	"bytes"
	"io"
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

func (b *Buffer) Reset(n ...uint64) {
	if len(n) == 0 || n[0] == 0 {
		b.buf = b.buf[:0]
		return
	}
	b.buf = make([]byte, n[0])
}

func (b *Buffer) Shrink(n uint64) {
	b.buf = b.buf[:n]
}

func (b *Buffer) Len() int {
	return len(b.buf)
}

func (b *Buffer) Cap() int {
	return cap(b.buf)
}

type MultiTaskBuffer struct {
	Ch    chan *Buffer
	count int
	total int
}

func NewMTB(capacity uint64) *MultiTaskBuffer {
	m := MultiTaskBuffer{make(chan *Buffer, capacity), 0, 0}
	return &m
}

func (mtb *MultiTaskBuffer) Release(b *Buffer) {
	select {
	case mtb.Ch <- b:
	default: //Discard the buffer if the pool is full.
		mtb.count -= 1
	}
}

func (mtb *MultiTaskBuffer) Get(n uint64) (b *Buffer) {
	select {
	case b = <-mtb.Ch:
		b.Reset(n)
	default:
		mtb.count += 1
		buf := make([]byte, n)
		if buf == nil {
			panic(Red("MultiTaskBuffer Get failed to allocated a buffer."))
		}
		bb := Buffer{mtb, buf}
		b = &bb
	}
	if b == nil {
		panic(Red("b unexpected nil!!!"))
	}
	return b
}
