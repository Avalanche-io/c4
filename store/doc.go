// Package store implements a c4 based file storage system.
//
// A key/value store is used to map file paths to C4 IDs.
// In storage files are named by their C4 ID so the key can be mapped to the actual file
// data. Store functions as a file system, and users need not interact with C4 IDs at all.
//
// Store implements a copy-on-write model, and implicitly de-duplicates data.
//
// All files are stored read only. When files are opened for writing they are opened
// in a temporary path. On close these files are moved to the storage path and renamed to
// their C4 ID if it does not already exist.
//
// Identification is done progressively whenever data is written to a file.
//
// When new paths are added an internally managed 'directory' file is automatically
// updated to add or remove names.
//
// Store also stores the filesystem metadata of each file, and has the ability to store
// any amount of additional metadata.
//
// Store does not implement linking since links have no meaning in a de-duplicated file
// system.
//
package store
