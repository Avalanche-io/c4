# C4 Store

[![GoDoc](https://godoc.org/github.com/Avalanche-io/c4/store?status.svg)](https://godoc.org/github.com/avalanche-io/c4/store)

```go
import "github.com/Avalanche-io/c4/store"
```

C4 store is a package for representing generic c4 storage. A C4 Store abstracts
the details of data management allowing c4 data consumers and producers to store
and retreave c4 identified data easily using the c4 id alone.

Examples of c4 stores include object storage, filesystems, archive files,
databases, bittorrent networks, web service endpoints, etc. C4 stores are also
useful for encapsulating processes like encryption, creating distributed copies,
and on the fly validation.
