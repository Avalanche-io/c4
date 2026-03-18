# C4 Store

[![GoDoc](https://godoc.org/github.com/Avalanche-io/c4/store?status.svg)](https://godoc.org/github.com/avalanche-io/c4/store)

```go
import "github.com/Avalanche-io/c4/store"
```

C4 store is a package for content-addressed blob storage. It provides
interfaces and implementations for storing and retrieving data by C4 ID.

## Store Types

- **Folder** — Flat directory with files named by C4 ID. Simple, fast.
- **TreeStore** — Adaptive trie-sharded directory. Splits automatically when
  a directory exceeds 4096 files. Scales to billions of objects.
- **RAM** — In-memory store for testing and caching.

## TreeStore

The `TreeStore` uses adaptive trie sharding. Every C4 ID starts with `c4`,
so the store has one top-level directory. When a leaf directory exceeds the
split threshold, files redistribute into 2-char subdirectories:

```
store/
  c4/
    5Y/
      c45Yxzabc...    ← content at depth 2
    8K/
      c48Kdef123...
```

```go
s, err := store.NewTreeStore("/data/c4store")
// Put computes the C4 ID and stores content in one pass.
id, err := s.Put(file)
// Has checks existence.
s.Has(id)
// Open retrieves content.
rc, err := s.Open(id)
```

## Configuration

The `c4` CLI and ecosystem tools share store configuration:

1. `C4_STORE` environment variable
2. `~/.c4/config` file (`store = /path/to/store`)

```go
s, err := store.OpenConfigured()
```
