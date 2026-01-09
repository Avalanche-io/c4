package c4m

import "time"

// PropagateMetadata resolves null values in entries by propagating from children
// This is used for directory entries to compute size and timestamp from contents
func PropagateMetadata(entries []*Entry) {
	// Find directory entries with null values
	for i := range entries {
		entry := entries[i]

		if entry.IsDir() && entry.HasNullValues() {
			// Get children of this directory
			children := getDirectoryChildren(entries, entry)

			// Propagate size if null
			if entry.Size < 0 {
				entry.Size = calculateDirectorySize(children)
			}

			// Propagate timestamp if null
			if entry.Timestamp.Unix() == 0 {
				entry.Timestamp = getMostRecentModtime(children)
			}
		}
	}
}

// getDirectoryChildren returns all entries that are direct children of a directory
func getDirectoryChildren(entries []*Entry, dir *Entry) []*Entry {
	var children []*Entry
	dirDepth := dir.Depth

	// Find entries at depth+1 that appear after this directory
	collecting := false
	for _, e := range entries {
		if e == dir {
			collecting = true
			continue
		}
		if collecting {
			if e.Depth == dirDepth+1 {
				children = append(children, e)
			} else if e.Depth <= dirDepth {
				// Reached next sibling or parent, stop
				break
			}
		}
	}

	return children
}

// calculateDirectorySize computes the total size of all entries
// This is the sum of all file sizes recursively, excluding null sizes
func calculateDirectorySize(entries []*Entry) int64 {
	var total int64
	for _, e := range entries {
		if e.Size >= 0 { // Skip null sizes (-1)
			total += e.Size
		}
	}
	return total
}

// getMostRecentModtime finds the most recent modification time among entries
// Returns current time if no valid timestamps found
func getMostRecentModtime(entries []*Entry) time.Time {
	var mostRecent time.Time

	for _, e := range entries {
		// Skip null timestamps (epoch)
		if e.Timestamp.Unix() > 0 && e.Timestamp.After(mostRecent) {
			mostRecent = e.Timestamp
		}
	}

	// If no valid timestamps found, return current time
	if mostRecent.IsZero() {
		return time.Now().UTC()
	}

	return mostRecent
}
