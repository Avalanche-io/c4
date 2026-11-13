package c4m

import (
	"fmt"
	"io"
	"sort"

	"github.com/Avalanche-io/c4"
)

// rangeDataFor returns the ID lists, taken from lists, of the ranges among
// entries: one per folded sequence whose list some input carries, and
// nothing else. A list no input carries stays absent; nothing is invented
// (design/range-data-through-operations.md). skip, when non-nil, leaves an
// entry out (a patch's removal lines name ranges the result does not hold).
// Lists are keyed by their own C4 ID, so the same key in two inputs is the
// same list and the order of lists does not matter. The result is nil when
// no range has a list.
func rangeDataFor(entries []*Entry, skip func(*Entry) bool, lists ...map[c4.ID]string) map[c4.ID]string {
	var out map[c4.ID]string
	for _, e := range entries {
		if !e.IsSequence || e.C4ID.IsNil() || (skip != nil && skip(e)) {
			continue
		}
		for _, l := range lists {
			list, ok := l[e.C4ID]
			if !ok {
				continue
			}
			if out == nil {
				out = make(map[c4.ID]string)
			}
			out[e.C4ID] = list
			break
		}
	}
	return out
}

// keepRangeData sets m.RangeData to the lists, from lists, of the ranges m
// holds. Every operation that writes a c4m from c4m inputs calls it on its
// output, so the output carries the list of every range it contains and no
// list for a range it does not.
func keepRangeData(m *Manifest, lists ...map[c4.ID]string) {
	m.RangeData = rangeDataFor(m.Entries, nil, lists...)
}

// writeRangeData writes inline ID-list lines, sorted by their C4 IDs for
// deterministic output (SPECIFICATION.md, "Inline Range Data"). They follow
// the entries of the section they belong to.
func writeRangeData(w io.Writer, rd map[c4.ID]string) error {
	if len(rd) == 0 {
		return nil
	}
	keys := make([]string, 0, len(rd))
	byKey := make(map[string]string, len(rd))
	for id, list := range rd {
		k := id.String()
		keys = append(keys, k)
		byKey[k] = list
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, err := fmt.Fprintf(w, "%s\n", byKey[k]); err != nil {
			return err
		}
	}
	return nil
}
