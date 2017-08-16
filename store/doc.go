// Package store implements a c4 based file storage system.
//
// The `store` API presents a simple key/value interface that maps
// keys to files. The API provides typical file access operations similar to
// the `os` package `Open`, `Create`, etc. When a file is created or opened for
// writing the data is written to a temporary location, and C4 identification
// happens as data is written to the file.  When the file is closed the data
// is moved into the storage location and named by it's C4 ID.
// The C4 ID is then associated with the Key in the key/value database.
//
// The file data is stored only once per C4 ID which implicitly de-duplicates
// the data. When files are deleted, they are not deleted immediately but
// rather marked for deletion. Once space is needed `store` will reclaim space
// by deleting the oldest files marked for deletion. A 'destroy' method is
// provided for removing files immediately.
//
package store
