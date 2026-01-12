package chunk

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Wire protocol format for delta transfer
//
// Header (16 bytes):
//   Magic:      4 bytes  "C4DL" (C4 Delta)
//   Version:    1 byte   (currently 1)
//   Flags:      1 byte   (reserved)
//   OpCount:    2 bytes  (number of operations, little-endian)
//   TargetSize: 8 bytes  (total target size, little-endian)
//
// Operations (variable length):
//   Type:   1 byte  ('R' for REF, 'L' for LITERAL)
//   Size:   varint  (size of data)
//   Offset: varint  (REF only: offset in base)
//   Data:   []byte  (LITERAL only: the actual data)
//
// The protocol is designed for:
//   - Compact encoding (varints for sizes)
//   - Streaming (operations can be processed as received)
//   - Verification (target size for validation)

const (
	// WireMagic identifies a C4 delta stream
	WireMagic = "C4DL"

	// WireVersion is the current protocol version
	WireVersion = 1

	// Maximum sizes for safety
	maxOpCount    = 1 << 24 // 16M operations
	maxTargetSize = 1 << 40 // 1 TB
	maxLiteralSize = 1 << 28 // 256 MB per literal
)

// Wire format errors
var (
	ErrInvalidMagic   = errors.New("invalid magic bytes")
	ErrInvalidVersion = errors.New("unsupported protocol version")
	ErrTooManyOps     = errors.New("too many operations")
	ErrTargetTooLarge = errors.New("target size too large")
	ErrLiteralTooLarge = errors.New("literal size too large")
	ErrUnexpectedEOF  = errors.New("unexpected end of stream")
	ErrInvalidOpType  = errors.New("invalid operation type")
)

// WireHeader represents the delta stream header
type WireHeader struct {
	Version    uint8
	Flags      uint8
	OpCount    uint16
	TargetSize uint64
}

// Encoder writes deltas to a wire format stream
type Encoder struct {
	w   io.Writer
	bw  *bufio.Writer
	err error
}

// NewEncoder creates a new wire format encoder
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{
		w:  w,
		bw: bufio.NewWriter(w),
	}
}

// Encode writes a complete delta to the stream
func (e *Encoder) Encode(d *Delta) error {
	if e.err != nil {
		return e.err
	}

	// Validate
	if len(d.Operations) > maxOpCount {
		return ErrTooManyOps
	}
	if d.TargetSize() > maxTargetSize {
		return ErrTargetTooLarge
	}

	// Write header
	if err := e.writeHeader(d); err != nil {
		e.err = err
		return err
	}

	// Write operations
	for _, op := range d.Operations {
		if err := e.writeOp(op); err != nil {
			e.err = err
			return err
		}
	}

	// Flush
	if err := e.bw.Flush(); err != nil {
		e.err = err
		return err
	}

	return nil
}

func (e *Encoder) writeHeader(d *Delta) error {
	// Magic
	if _, err := e.bw.WriteString(WireMagic); err != nil {
		return err
	}

	// Version and flags
	if err := e.bw.WriteByte(WireVersion); err != nil {
		return err
	}
	if err := e.bw.WriteByte(0); err != nil { // flags reserved
		return err
	}

	// OpCount (2 bytes, little-endian)
	var buf [8]byte
	binary.LittleEndian.PutUint16(buf[:2], uint16(len(d.Operations)))
	if _, err := e.bw.Write(buf[:2]); err != nil {
		return err
	}

	// TargetSize (8 bytes, little-endian)
	binary.LittleEndian.PutUint64(buf[:8], uint64(d.TargetSize()))
	if _, err := e.bw.Write(buf[:8]); err != nil {
		return err
	}

	return nil
}

func (e *Encoder) writeOp(op DeltaOp) error {
	// Type byte
	if err := e.bw.WriteByte(byte(op.Type)); err != nil {
		return err
	}

	// Size (varint)
	if err := e.writeVarint(uint64(op.Size)); err != nil {
		return err
	}

	switch op.Type {
	case OpRef:
		// Offset (varint)
		if err := e.writeVarint(uint64(op.Offset)); err != nil {
			return err
		}

	case OpLiteral:
		// Data
		if op.Size > maxLiteralSize {
			return ErrLiteralTooLarge
		}
		if _, err := e.bw.Write(op.Data); err != nil {
			return err
		}

	default:
		return fmt.Errorf("unknown op type: %c", op.Type)
	}

	return nil
}

func (e *Encoder) writeVarint(v uint64) error {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], v)
	_, err := e.bw.Write(buf[:n])
	return err
}

// Decoder reads deltas from a wire format stream
type Decoder struct {
	r   io.Reader
	br  *bufio.Reader
	err error
}

// NewDecoder creates a new wire format decoder
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{
		r:  r,
		br: bufio.NewReader(r),
	}
}

// Decode reads a complete delta from the stream
func (d *Decoder) Decode() (*Delta, error) {
	if d.err != nil {
		return nil, d.err
	}

	// Read header
	header, err := d.readHeader()
	if err != nil {
		d.err = err
		return nil, err
	}

	// Read operations
	delta := &Delta{
		Operations: make([]DeltaOp, 0, header.OpCount),
	}

	for i := uint16(0); i < header.OpCount; i++ {
		op, err := d.readOp()
		if err != nil {
			d.err = err
			return nil, fmt.Errorf("reading operation %d: %w", i, err)
		}

		// Update stats manually since we're not using Add methods
		switch op.Type {
		case OpRef:
			delta.stats.refCount++
			delta.stats.refBytes += int64(op.Size)
		case OpLiteral:
			delta.stats.literalCount++
			delta.stats.literalBytes += int64(op.Size)
		}
		delta.stats.targetSize += int64(op.Size)

		delta.Operations = append(delta.Operations, op)
	}

	// Verify target size matches
	if uint64(delta.TargetSize()) != header.TargetSize {
		return nil, fmt.Errorf("target size mismatch: header says %d, operations sum to %d",
			header.TargetSize, delta.TargetSize())
	}

	return delta, nil
}

func (d *Decoder) readHeader() (*WireHeader, error) {
	// Magic
	magic := make([]byte, 4)
	if _, err := io.ReadFull(d.br, magic); err != nil {
		if err == io.EOF {
			return nil, ErrUnexpectedEOF
		}
		return nil, err
	}
	if string(magic) != WireMagic {
		return nil, ErrInvalidMagic
	}

	// Version
	version, err := d.br.ReadByte()
	if err != nil {
		return nil, err
	}
	if version != WireVersion {
		return nil, fmt.Errorf("%w: got %d, expected %d", ErrInvalidVersion, version, WireVersion)
	}

	// Flags (ignored for now)
	if _, err := d.br.ReadByte(); err != nil {
		return nil, err
	}

	// OpCount
	var buf [8]byte
	if _, err := io.ReadFull(d.br, buf[:2]); err != nil {
		return nil, err
	}
	opCount := binary.LittleEndian.Uint16(buf[:2])

	// TargetSize
	if _, err := io.ReadFull(d.br, buf[:8]); err != nil {
		return nil, err
	}
	targetSize := binary.LittleEndian.Uint64(buf[:8])

	if targetSize > maxTargetSize {
		return nil, ErrTargetTooLarge
	}

	return &WireHeader{
		Version:    version,
		Flags:      0,
		OpCount:    opCount,
		TargetSize: targetSize,
	}, nil
}

func (d *Decoder) readOp() (DeltaOp, error) {
	var op DeltaOp

	// Type
	typeByte, err := d.br.ReadByte()
	if err != nil {
		return op, err
	}
	op.Type = OpType(typeByte)

	// Size
	size, err := binary.ReadUvarint(d.br)
	if err != nil {
		return op, err
	}
	op.Size = int(size)

	switch op.Type {
	case OpRef:
		// Offset
		offset, err := binary.ReadUvarint(d.br)
		if err != nil {
			return op, err
		}
		op.Offset = int64(offset)

	case OpLiteral:
		if op.Size > maxLiteralSize {
			return op, ErrLiteralTooLarge
		}
		op.Data = make([]byte, op.Size)
		if _, err := io.ReadFull(d.br, op.Data); err != nil {
			return op, err
		}

	default:
		return op, fmt.Errorf("%w: %c", ErrInvalidOpType, typeByte)
	}

	return op, nil
}

// StreamingDecoder provides operation-by-operation decoding for large deltas
type StreamingDecoder struct {
	d       *Decoder
	header  *WireHeader
	opsRead uint16
	err     error
}

// NewStreamingDecoder creates a decoder that yields operations one at a time
func NewStreamingDecoder(r io.Reader) (*StreamingDecoder, error) {
	d := NewDecoder(r)

	header, err := d.readHeader()
	if err != nil {
		return nil, err
	}

	return &StreamingDecoder{
		d:      d,
		header: header,
	}, nil
}

// Header returns the decoded header
func (s *StreamingDecoder) Header() *WireHeader {
	return s.header
}

// Next returns the next operation, or nil when done
func (s *StreamingDecoder) Next() (*DeltaOp, error) {
	if s.err != nil {
		return nil, s.err
	}

	if s.opsRead >= s.header.OpCount {
		return nil, nil // Done
	}

	op, err := s.d.readOp()
	if err != nil {
		s.err = err
		return nil, err
	}

	s.opsRead++
	return &op, nil
}

// OpsRemaining returns how many operations are left
func (s *StreamingDecoder) OpsRemaining() int {
	return int(s.header.OpCount - s.opsRead)
}

// StreamingEncoder provides operation-by-operation encoding for large deltas
type StreamingEncoder struct {
	e          *Encoder
	targetSize int64
	opCount    int
	closed     bool
}

// NewStreamingEncoder creates an encoder for streaming operations
// The opCount and targetSize must be known in advance for the header
func NewStreamingEncoder(w io.Writer, opCount int, targetSize int64) (*StreamingEncoder, error) {
	if opCount > maxOpCount {
		return nil, ErrTooManyOps
	}
	if targetSize > maxTargetSize {
		return nil, ErrTargetTooLarge
	}

	e := NewEncoder(w)

	// Write header manually
	if _, err := e.bw.WriteString(WireMagic); err != nil {
		return nil, err
	}
	if err := e.bw.WriteByte(WireVersion); err != nil {
		return nil, err
	}
	if err := e.bw.WriteByte(0); err != nil {
		return nil, err
	}

	var buf [8]byte
	binary.LittleEndian.PutUint16(buf[:2], uint16(opCount))
	if _, err := e.bw.Write(buf[:2]); err != nil {
		return nil, err
	}

	binary.LittleEndian.PutUint64(buf[:8], uint64(targetSize))
	if _, err := e.bw.Write(buf[:8]); err != nil {
		return nil, err
	}

	return &StreamingEncoder{
		e:          e,
		targetSize: targetSize,
		opCount:    opCount,
	}, nil
}

// WriteOp writes a single operation
func (s *StreamingEncoder) WriteOp(op DeltaOp) error {
	if s.closed {
		return errors.New("encoder closed")
	}
	return s.e.writeOp(op)
}

// WriteRef writes a REF operation
func (s *StreamingEncoder) WriteRef(offset int64, size int) error {
	return s.WriteOp(DeltaOp{
		Type:   OpRef,
		Offset: offset,
		Size:   size,
	})
}

// WriteLiteral writes a LITERAL operation
func (s *StreamingEncoder) WriteLiteral(data []byte) error {
	return s.WriteOp(DeltaOp{
		Type: OpLiteral,
		Size: len(data),
		Data: data,
	})
}

// Close flushes any buffered data
func (s *StreamingEncoder) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.e.bw.Flush()
}

// Marshal encodes a delta to bytes
func Marshal(d *Delta) ([]byte, error) {
	// Estimate size: header + ops
	estimatedSize := 16 + len(d.Operations)*16 + int(d.LiteralBytes())
	buf := make([]byte, 0, estimatedSize)

	w := &bytesWriter{buf: buf}
	enc := NewEncoder(w)

	if err := enc.Encode(d); err != nil {
		return nil, err
	}

	return w.buf, nil
}

// Unmarshal decodes a delta from bytes
func Unmarshal(data []byte) (*Delta, error) {
	r := &bytesReader{data: data}
	dec := NewDecoder(r)
	return dec.Decode()
}

// bytesWriter is a simple io.Writer that appends to a byte slice
type bytesWriter struct {
	buf []byte
}

func (w *bytesWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}

// bytesReader is a simple io.Reader that reads from a byte slice
type bytesReader struct {
	data []byte
	pos  int
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// WireSize returns the approximate encoded size of a delta
func WireSize(d *Delta) int {
	size := 16 // header

	for _, op := range d.Operations {
		size += 1 // type byte
		size += varintSize(uint64(op.Size))

		switch op.Type {
		case OpRef:
			size += varintSize(uint64(op.Offset))
		case OpLiteral:
			size += op.Size
		}
	}

	return size
}

func varintSize(v uint64) int {
	size := 1
	for v >= 0x80 {
		v >>= 7
		size++
	}
	return size
}
