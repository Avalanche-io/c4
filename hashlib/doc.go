// Package hashlib provides data structures optimized for C4 IDs.
//
// Every key is a SHA-512 hash — 512 bits of uniformly distributed,
// cryptographically strong randomness. Structures in this package
// exploit this property directly: hash maps skip hashing, cuckoo
// filters use byte windows as independent hash functions, tries
// are balanced by construction, and probabilistic structures derive
// all needed randomness from the key itself.
//
// All persistent structures (HAMTMap, HAMTSet, PatriciaTrie) use
// copy-on-write semantics. Mutations return new values; originals
// are never modified. This enables O(1) snapshots via pointer copy
// and safe concurrent reads without locking.
package hashlib
