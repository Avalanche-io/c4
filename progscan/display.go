package progscan

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// C4IDColumn is the fixed column position where C4 IDs are placed in
// display output. Lines shorter than this get padded; lines longer
// overflow with a single space gap. The constant avoids any need to
// pre-scan entries, so output can begin immediately.
const C4IDColumn = 80

// DisplayWrite writes a manifest in aligned display format: canonical c4m
// field rendering with C4 IDs starting at a fixed column. All padding sits
// between the name and the C4 ID, making it trivial to canonicalize with
// standard tools (collapse whitespace before the last column).
func DisplayWrite(w io.Writer, m *c4m.Manifest) error {
	m.SortEntries()
	for _, e := range m.Entries {
		full := e.Format(2, false)
		lastSpace := strings.LastIndex(full, " ")
		pfx := full[:lastSpace]
		id := full[lastSpace+1:]

		pad := C4IDColumn - len(pfx)
		if pad < 1 {
			pad = 1
		}
		if _, err := fmt.Fprintf(w, "%s%s%s\n", pfx, strings.Repeat(" ", pad), id); err != nil {
			return err
		}
	}
	return nil
}

// DisplayLine returns a display-format line for a single entry with
// the C4 ID aligned at C4IDColumn. Used for streaming output during
// Phase 0 where entries are emitted as they're discovered.
func DisplayLine(depth int, mode os.FileMode, ts time.Time, size int64, name string, id c4.ID) string {
	indent := strings.Repeat(" ", depth*indentStep)
	modeStr := renderMode(mode)

	var tsStr string
	if isNullTS(ts) {
		tsStr = "-"
	} else {
		tsStr = ts.UTC().Format("2006-01-02T15:04:05Z")
	}

	var sizeStr string
	if size < 0 {
		sizeStr = "-"
	} else {
		sizeStr = fmt.Sprintf("%d", size)
	}

	var idStr string
	if id.IsNil() {
		idStr = "-"
	} else {
		idStr = id.String()
	}

	pfx := indent + modeStr + " " + tsStr + " " + sizeStr + " " + name
	pad := C4IDColumn - len(pfx)
	if pad < 1 {
		pad = 1
	}
	return pfx + strings.Repeat(" ", pad) + idStr + "\n"
}
