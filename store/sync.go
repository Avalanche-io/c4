package store

import "os"

// SyncMode selects how store writes reach stable storage.
type SyncMode int

const (
	// SyncEach flushes every object to stable storage before it is
	// renamed into place (F_FULLFSYNC on darwin). Each object is
	// durable the moment it lands. The default.
	SyncEach SyncMode = iota

	// SyncBatch hands each object to the device with a cheap data
	// flush and defers the expensive device-cache flush to a single
	// Sync call at the end of the batch. Objects are always complete
	// or absent; a crash before Sync may lose recent objects.
	SyncBatch

	// SyncNone skips flushing entirely. Fastest; after a power failure
	// recently written objects may be lost.
	SyncNone
)

// Syncer is the optional interface for stores that support a batch
// durability barrier.
type Syncer interface {
	// Sync makes every object written so far durable.
	Sync() error
}

// flushFile pushes written file data toward stable storage per mode:
// all the way for SyncEach, to the device for SyncBatch (a later
// barrier flushes its cache), not at all for SyncNone.
func flushFile(f *os.File, mode SyncMode) error {
	switch mode {
	case SyncEach:
		return f.Sync()
	case SyncBatch:
		return fsyncData(f)
	}
	return nil
}
