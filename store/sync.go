package store

import "os"

// SyncMode selects how store writes reach stable storage.
type SyncMode int

const (
	// SyncEach flushes every object to stable storage before it is
	// renamed into place (F_FULLFSYNC on darwin), then syncs the name's
	// directory chain and flushes the device before Put or Create's Close
	// returns; a Put that skips a present object does the same for that
	// object's name. Each object is durable the moment it lands. The
	// default. (Through 2026-11-16 the names were never synced; see
	// TreeStore.holdName.)
	SyncEach SyncMode = iota

	// SyncBatch hands each object to the device with a cheap data
	// flush and defers the expensive device-cache flush to a single
	// Sync call at the end of the batch. Objects are always complete
	// or absent; a crash before Sync may lose recent objects. Sync also
	// makes durable the name of every object the batch skipped because
	// it was present, so a claim made after Sync rests only on durable
	// names.
	SyncBatch

	// SyncNone skips flushing entirely. Fastest; after a power failure
	// recently written objects may be lost. Objects are still complete
	// or absent (atomic, not durable), by stated choice.
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
