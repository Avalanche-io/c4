// ## C4 ID
//
// The c4 id package implements the C4 ID standard. C4 IDs are base 58 encoded
// SHA-512 hashes.  C4 IDs represent a universally unique and consistent
// identification system for all observers world wide without the need for
// a central registry.  The standardized encoding makes the IDs easer to use
// in common contexts like filenames, or keys in an object store.
//
// A single C4 IDs may represent one or more files (or bocks of data), by
// hashing the contents of the file(s). This makes identifying collections of
// files much easer. C4 IDs are not effect by filename, location, modification
// time or other metadata.
//
// ### Types
//
// This implementation exports the basic types ID and Digest, and Slice
// collections for both. The Slice types support an Insert method to insure
// proper ordering for identification of a collection.
//
// ### Single File Identification
//
// To create a C4 ID for a file (or any block of contiguous data) there are two
// interfaces.
//
// #### Encoder
//
// Encoder implements io.Writer and is best for identifying streaming data
// without having to hold all the data in ram at once.
//
//     e := c4.NewEncoder()
//     io.Copy(e, src)
//     id := e.ID()
//
// Every time ID() is called the ID will be the data written to the encoder so
// for. The Encoder can be cleared and re-used for different data by calling
// Reset().
//
// #### Identify
//
// The second interface for identifying contiguous data is the Identify method.
// It takes an io.Reader and returns a C4 ID, if there is an error other than
// io.EOF from the Reader Identify will return nil.
//
// ### Multiple File Identification
//
// For multiple files or other types of non-contiguous data first using the
// process above generate the C4 ID of each file in the collection and
// use the Insert method to add them to a Slice, or DigestSlice. Insert insures
// the proper order for identification.
//
// For a Slice the ID() method returns the C4 ID of the Slice. For DigestSlice
// the Digest method returns the C4 Digest.
//
//     var digests c4.DigestSlice
//     for digest := range inputDigests {
//         digests.Insert(digest)
//			}
//      collectionDigest := digests.Digest()
//      collectionID := collectionDigest.ID()
//
// ### Parsing IDs
//
// To parse an ID string, use the Parse function.
//
//     c4.Parse(c4_id_string)
//
package id
