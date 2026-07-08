//go:build !darwin

package store

import "os"

// fsyncData flushes file data to the device. Outside darwin,
// os.File.Sync is already plain fsync.
func fsyncData(f *os.File) error {
	return f.Sync()
}
