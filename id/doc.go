// ## C4 ID
//
// To create a C4 ID for a block of contiguous data there are two interfaces.
// Encoder implements the io.Writer interface best for identifying streaming data without
// having to hold all the data in ram at once.
//
//     e := c4.NewEncoder()
//     io.Copy(e, src)
//     id := e.ID()
//
// The Encoder can be reused by calling it's Reset() methods.
//
// The other mechanism for Identifying contiguous blocks of data is the Identify
// convenience method. It takes an io.Reader and returns a C4 ID, it handles creating
// an encoder and copying bytes to be encoded.
//
// To identify non-contiguous blocks of data create a DigestSlice and the C4 Digest
// for each item to be identified (i.e. 10 Digests for 10 files in a folder).
//
// When all Digests have been added the Digest() method of a DigestSlice will return
// the Digest for the set.  Call the ID() methods on Digest to get the actual C4 ID.
//
//
//     var digests c4.DigestSlice
//     for digest := range digest_chan {
//         digests.Insert(digest)
//			}
//      digest_id := digests.Digest().ID()
//
//
// Parsing IDs
//
// To parse an ID from a string, use the Parse function.
//
//     c4.Parse(c4_id_string)
//
package id
