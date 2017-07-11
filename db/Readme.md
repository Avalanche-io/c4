# C4 DB
c4/db is a go package implementing a C4 ID key/value database.  

This package has been recently updated to improve the interface, functionality, and performance. 

A database built on the C4 framework is at it's core very simple.  A key is associated with a C4 ID, and a reverse index provides the inverse mapping of a C4 ID to a list of keys that refer to it.

Typically one finds the C4 ID of an asset and stores it's ID along with it's filepath or other 'assigned' id as the key. C4 IDs can also be associated together with an arbitrary string identifying the type of relationship.  So, the type and meaning of relationships is up to the user, but here's an example for saving a file's ID and the ID for some metadata about the file.

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    c4db "github.com/Avalanche-io/c4/db"
    c4 "github.com/Avalanche-io/c4/id"
    "io"
    "os"
    "path/filepath"
)

func assert(is_true bool) {
    if !is_true {
        panic("assertion not true")
    }
}
func stopOnError(err error) {
    if err != nil {
        fmt.Errorf("error %s\n", err)
        os.Exit(-1)
    }
}

func main() {
    // Open or create a C4 database
    db, err := c4db.Open("test.c4db", nil)
    stopOnError(err)
    defer db.Close()

    // get the current working directory and set main.go as the input
    expath, err := os.Executable()
    stopOnError(err)
    cwd := filepath.Dir(expath)
    inputname := "main.go"
    inputpath := filepath.Join(cwd, inputname)

    // open the input file
    fin, err := os.Open(inputname)
    stopOnError(err)
    defer fin.Close()

    // get os.FileInfo metadata about the file
    info, err := fin.Stat()
    stopOnError(err)

    // marshal the metadata
    info_data, err := json.Marshal(info)
    stopOnError(err)

    // Find the C4 ID of the file and of the metadata
    main_id := c4.Identify(fin)
    info_id := c4.Identify(bytes.NewReader(info_data))

    // Save the metadata in some file, and close.
    outputpath := filepath.Join(cwd, "main.info")
    fout, err := os.Create(outputpath)
    stopOnError(err)
    _, err = io.Copy(fout, bytes.NewReader(info_data))
    stopOnError(err)
    fout.Close()

    // save the main_id with it's path as the key
    _, err = db.KeySet(inputpath, main_id.Digest())
    stopOnError(err)
    // save the info_id with it's path as the key
    _, err = db.KeySet(outputpath, info_id.Digest())
    stopOnError(err)

    // link the main_id to the info_id as 'metadata'
    err = db.LinkSet("metadata", main_id.Digest(), info_id.Digest())
    stopOnError(err)

    // Get the main_id from the key:
    main_id2, err := db.KeyGet(inputpath)
    stopOnError(err)
    _ = main_id2

    // Get all keys and IDs under a key prefix:
    count := 0
    for en := range db.KeyGetAll(cwd) {
        key := en.Key()
        digest := en.Value()
        fmt.Printf("Key: %q\n", key)
        fmt.Printf("ID: %s\n", digest.ID())
        en.Close()
        count++
    }
    assert(count == 2)

    // Find keys from IDs
    keys := db.KeyFind(main_id.Digest())
    assert(keys[0] == inputpath)

    // Find "metadata" links from the main_id
    for en := range db.LinkGet("metadata", main_id.Digest()) {
        source_digest := en.Source()
        target_digest := en.Target()
        fmt.Printf("main.go id: %s\n", source_digest.ID())
        fmt.Printf("main.info id: %s\n", target_digest.ID())
        en.Close()
    }

}
```

