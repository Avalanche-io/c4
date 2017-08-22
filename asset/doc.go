// Package asset contains Go implementations C4 asset identification.
//
// Creating IDs
//
// To create a C4 ID, use the IDEncoder type as a hash writer.
//
//     e := asset.NewIDEncoder()
//     io.Copy(e, src)
//     id := e.ID()
//
// Parsing IDs
//
// To parse an ID from a string, use the ParseID function.
//
//     asset.ParseID(src)
package asset
