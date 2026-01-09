package bundle

import (
	"fmt"
	"os"
	"time"

	"github.com/Avalanche-io/c4/c4m"
)

// Type aliases for c4m types to simplify migration
type Entry = c4m.Entry
type Manifest = c4m.Manifest

// Function aliases
var NewManifest = c4m.NewManifest
var NewParser = c4m.NewParser
var NaturalLess = c4m.NaturalLess

// GenerateFromReader creates a manifest by parsing an existing C4M
func GenerateFromReader(r interface{ Read([]byte) (int, error) }) (*Manifest, error) {
	parser := c4m.NewParser(r)
	return parser.ParseAll()
}

// Timing helpers for progress output
var startTime = time.Now()

// TimedPrintln prints a message with elapsed time
func TimedPrintln(msg string) {
	elapsed := time.Since(startTime)
	fmt.Fprintf(os.Stderr, "[%s] %s\n", elapsed.Round(time.Millisecond), msg)
}

// TimedPrintf prints a formatted message with elapsed time
func TimedPrintf(format string, args ...interface{}) {
	elapsed := time.Since(startTime)
	fmt.Fprintf(os.Stderr, "[%s] "+format, append([]interface{}{elapsed.Round(time.Millisecond)}, args...)...)
}
