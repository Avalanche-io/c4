package progscan

import (
	"fmt"
	"os"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// Fixed field widths for padded lines. These widths guarantee that
// phase 1 (metadata) and phase 2 (C4 ID) updates are in-place byte
// overwrites with no change in line length.
const (
	modeWidth  = 10 // "-rwxr-xr-x" or "d---------"
	tsWidth    = 21 // null: "- " + 19 spaces; real: RFC3339(20) + space
	sizeWidth  = 16 // null: "-" + 15 spaces; real: up to 15-digit number
	c4idWidth  = 90 // null: "-" + 89 spaces; real: 90-char base58
	indentStep = 2  // spaces per depth level
)

// LineLen returns the total byte length of a padded line (including newline).
func LineLen(depth, nameLen int) int {
	// indent + mode + sp + ts_region + size_region + sp + name + sp + c4id_region + nl
	return depth*indentStep + modeWidth + 1 + tsWidth + sizeWidth + 1 + nameLen + 1 + c4idWidth + 1
}

// Field byte offsets within a line (from line start).
func tsOff(depth int) int            { return depth*indentStep + modeWidth + 1 }
func sizeOff(depth int) int          { return tsOff(depth) + tsWidth }
func nameOff(depth int) int          { return sizeOff(depth) + sizeWidth + 1 }
func c4idOff(depth, nameLen int) int { return nameOff(depth) + nameLen + 1 }
func modeOff(depth int) int          { return depth * indentStep }

// PaddedLine produces a fixed-width c4m entry line. All fields are padded
// to their target width so the line length is constant across resolution
// levels. The c4m parser's whitespace-skipping handles the padding.
func PaddedLine(depth int, mode os.FileMode, ts time.Time, size int64, name string, id c4.ID) []byte {
	w := LineLen(depth, len(name))
	buf := make([]byte, w)
	// Fill with spaces (padding default).
	for i := range buf {
		buf[i] = ' '
	}

	pos := depth * indentStep

	// Mode (10 chars, always same width).
	copy(buf[pos:], renderMode(mode))
	pos += modeWidth
	buf[pos] = ' '
	pos++

	// Timestamp region (21 chars total).
	if isNullTS(ts) {
		buf[pos] = '-'
		// buf[pos+1] is already space, remaining 19 spaces already filled.
	} else {
		copy(buf[pos:], ts.UTC().Format("2006-01-02T15:04:05Z"))
		// buf[pos+20] is already space.
	}
	pos += tsWidth

	// Size region (16 chars total, left-aligned).
	if size < 0 {
		buf[pos] = '-'
		// remaining 15 spaces already filled.
	} else {
		s := fmt.Sprintf("%d", size)
		copy(buf[pos:], s)
		// remaining spaces already filled.
	}
	pos += sizeWidth

	buf[pos] = ' '
	pos++

	// Name (variable width, but fixed per entry).
	copy(buf[pos:], name)
	pos += len(name)
	buf[pos] = ' '
	pos++

	// C4 ID region (90 chars total).
	if id.IsNil() {
		buf[pos] = '-'
		// remaining 89 spaces already filled.
	} else {
		copy(buf[pos:], id.String())
	}
	pos += c4idWidth

	buf[pos] = '\n'
	return buf
}

// ModeBytes returns the 10-byte mode field for in-place overwrite.
func ModeBytes(mode os.FileMode) []byte {
	return []byte(renderMode(mode))
}

// TSBytes returns the 21-byte timestamp region for in-place overwrite.
func TSBytes(ts time.Time) []byte {
	buf := make([]byte, tsWidth)
	for i := range buf {
		buf[i] = ' '
	}
	if isNullTS(ts) {
		buf[0] = '-'
	} else {
		copy(buf, ts.UTC().Format("2006-01-02T15:04:05Z"))
	}
	return buf
}

// SizeBytes returns the 16-byte size region for in-place overwrite.
func SizeBytes(size int64) []byte {
	buf := make([]byte, sizeWidth)
	for i := range buf {
		buf[i] = ' '
	}
	if size < 0 {
		buf[0] = '-'
	} else {
		s := fmt.Sprintf("%d", size)
		copy(buf, s)
	}
	return buf
}

// C4IDBytes returns the 90-byte C4 ID region for in-place overwrite.
func C4IDBytes(id c4.ID) []byte {
	buf := make([]byte, c4idWidth)
	for i := range buf {
		buf[i] = ' '
	}
	if id.IsNil() {
		buf[0] = '-'
	} else {
		copy(buf, id.String())
	}
	return buf
}

func isNullTS(ts time.Time) bool {
	return ts.IsZero() || ts.Equal(c4m.NullTimestamp())
}

func renderMode(mode os.FileMode) string {
	var buf [10]byte
	for i := range buf {
		buf[i] = '-'
	}

	switch {
	case mode.IsDir():
		buf[0] = 'd'
	case mode&os.ModeSymlink != 0:
		buf[0] = 'l'
	case mode&os.ModeNamedPipe != 0:
		buf[0] = 'p'
	case mode&os.ModeSocket != 0:
		buf[0] = 's'
	case mode&os.ModeDevice != 0:
		if mode&os.ModeCharDevice != 0 {
			buf[0] = 'c'
		} else {
			buf[0] = 'b'
		}
	}

	perm := mode.Perm()
	if perm&0400 != 0 {
		buf[1] = 'r'
	}
	if perm&0200 != 0 {
		buf[2] = 'w'
	}
	if perm&0100 != 0 {
		buf[3] = 'x'
	}
	if perm&040 != 0 {
		buf[4] = 'r'
	}
	if perm&020 != 0 {
		buf[5] = 'w'
	}
	if perm&010 != 0 {
		buf[6] = 'x'
	}
	if perm&04 != 0 {
		buf[7] = 'r'
	}
	if perm&02 != 0 {
		buf[8] = 'w'
	}
	if perm&01 != 0 {
		buf[9] = 'x'
	}
	return string(buf[:])
}

// typeOnlyMode returns an os.FileMode with only the type bits set
// (dir, symlink, etc.) and no permission bits. This is what readdir
// gives us without a stat call.
func typeOnlyMode(typ os.FileMode) os.FileMode {
	return typ & os.ModeType
}
