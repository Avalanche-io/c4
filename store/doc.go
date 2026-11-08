// github.com/Avalanch-io/c4/store - is a package for representing generic c4
// storage. A C4 store abstracts away the details of data management allowing
// c4 data consumers and producers to store and retreave c4 identified data
// using the c4 id alone.
//
// A c4 store could represent an object storage bucket, a local filesystem,
// or the agrigation of many c4 stores. A c4 store can also be used to abstract
// processes like encryption, creating distributed copies, and on the fly
// validation.
//
// # Incomplete ingests
//
// A local store publishes an object at its hash name only when its bytes
// are durable; until then the bytes live in a temp file in the store root
// named .ingest.<host>.<pid>.<random>. A writer that dies before its
// barrier leaves those files behind, and nothing can replay them: the name
// carries no ID, so a complete temp and a torn one look alike. The rule
// (Joshua, 2026-11-08) is that on boot a system garbage-collects incomplete
// transactions that cannot, or should not, be replayed to completion.
// Opening a store is its boot: NewTreeStore removes the temp files of
// writers on this host that are no longer running, keeps a live writer's
// and another host's, and never touches an object. Every implementation of
// a local store should do the same: record the writer in the temp name,
// sweep dead writers' temps on open, never an object. See
// design/store-ingest-sweep.md.
package store
