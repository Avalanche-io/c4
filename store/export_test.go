package store

// Test-only bridges for the external store_test package, which can import
// c4m (c4m imports store, so the internal tests can't).

// ObserveDirSyncs calls f with each directory the barrier syncs (both the
// device flush and the plain entry sync), until restore is called.
func ObserveDirSyncs(f func(dir string)) (restore func()) {
	oldFull, oldEntries := syncDirFull, syncDirEntries
	syncDirFull = func(d string) error {
		err := oldFull(d)
		f(d)
		return err
	}
	syncDirEntries = func(d string) error {
		err := oldEntries(d)
		f(d)
		return err
	}
	return func() { syncDirFull, syncDirEntries = oldFull, oldEntries }
}

// DirsWithinRoot is dirsWithinRoot: dir and each ancestor inside root.
func DirsWithinRoot(root, dir string) []string { return dirsWithinRoot(root, dir) }
