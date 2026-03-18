# C4 Store

[![GoDoc](https://godoc.org/github.com/Avalanche-io/c4/store?status.svg)](https://godoc.org/github.com/avalanche-io/c4/store)

```go
import "github.com/Avalanche-io/c4/store"
```

C4 store is a package for content-addressed blob storage. It provides
interfaces and implementations for storing and retrieving data by C4 ID.

## Store Types

- **TreeStore** — Adaptive trie-sharded directory. Splits automatically when
  a directory exceeds 4096 files. Scales to billions of objects.
- **S3Store** — S3-compatible object store. Works with AWS S3, MinIO,
  Backblaze B2, Wasabi, Ceph, or any S3-compatible endpoint.
- **MultiStore** — Combines multiple stores. Writes to the first, reads
  from all in order.
- **Folder** — Flat directory with files named by C4 ID.
- **RAM** — In-memory store for testing and caching.

## Configuration

The `c4` CLI and ecosystem tools share store configuration:

```bash
# Single local store
C4_STORE=/data/c4store

# Single S3 store
C4_STORE=s3://bucket/prefix?region=us-west-2

# Multiple stores — writes go to the first, reads check all
C4_STORE=/fast/ssd,s3://bucket/c4?region=us-west-2,/mnt/archive
```

Or in `~/.c4/config`:

```
store = /fast/ssd
store = s3://bucket/c4?region=us-west-2
store = /mnt/archive
```

```go
s, err := store.OpenStore()  // returns Store interface (single or multi)
```

## S3 Configuration

S3 stores use standard AWS credentials:

```bash
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
export C4_STORE=s3://mybucket/c4?region=us-west-2
```

For non-AWS endpoints:

```bash
export C4_STORE=s3://mybucket/c4?endpoint=minio.local:9000
# or
export C4_S3_ENDPOINT=minio.local:9000
```

## TreeStore

The `TreeStore` uses adaptive trie sharding. Every C4 ID starts with `c4`,
so the store has one top-level directory. When a leaf directory exceeds the
split threshold, files redistribute into 2-char subdirectories:

```
store/
  c4/
    5Y/
      c45Yxzabc...
    8K/
      c48Kdef123...
```

```go
s, err := store.NewTreeStore("/data/c4store")
id, err := s.Put(file)       // compute C4 ID + store in one pass
exists := s.Has(id)           // check existence
rc, err := s.Open(id)         // retrieve content
```
