### C4 ID - Universal Identification

[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](../LICENSE)
[![GoDoc](https://godoc.org/github.com/Avalanche-io/c4?status.svg)](https://godoc.org/github.com/Avalanche-io/c4)
[![Stories in Ready](https://badge.waffle.io/Avalanche-io/c4.png?label=ready&title=Ready)](https://waffle.io/Avalanche-io/c4)
[![Build Status](https://travis-ci.org/Avalanche-io/c4.svg?branch=master)](https://travis-ci.org/Avalanche-io/c4)
[![Coverage Status](https://coveralls.io/repos/github/Avalanche-io/c4/badge.svg?branch=master)](https://coveralls.io/github/Avalanche-io/c4?branch=master)

Go package and cli for c4.

### c4id Command line tool
See [c4 command line tool](https://github.com/Avalanche-io/c4/tree/master/cmd/c4id)

#### C4 ID Features

- C4 IDs can be used in filenames and URLs.
- C4 IDs can represent any number of files.
- C4 IDs can be recognized out of context without labeling.
- C4 IDs are easer to work with, double click and `select word` selects the entire id.
- C4 IDs are easy to parse with regular expressions `c4[1-9A-HJ-NP-Za-km-z]{88}`.
- C4 IDs are easily differentiated (1).
- C4 IDs deduplication data for free when used as a file name or an object storage key. 
- C4 IDs can represent any block of data or any number of non-contiguous blocks of data.
- C4 IDs are random, secure, and completely unforgeable.
- C4 ID sort in the same order as the raw SHA-512 data.

1. In a sorted list of C4 IDs duplicates are completely obvious even at very fast scrolling speeds. C4 IDs with more than 5 identical characters in a row appear less than once per 38 billion ids. It is not necessary to compare more than a few digits before considering two C4 IDs to be identical.

#### Technical Details
A C4 ID is a standardized encoding of a SHA-512 digest one of the fastest securing hashing algorithms on 64 bit hardware (faster than SHA-256). The encoding has carefully designed features to enhance usability and discoverability.

C4 IDs have the following properties:

- **Always 90 characters long**
- **Starts with `c4`**
- **No non-alphanumeric characters**
- **URL, Filename, and database safe**

This is the C4 ID of an empty string.

```
c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT
```

#### C4 ID, vs Sha256 and Sha512 Hex
There are no universal standard encodings for common cryptographic hashes. Labeled hex representations (i.e. sha512-cf83...) seem to be a tad more popular at the moment.

C4 IDs are 33% shorter then hex SHA-512, and only 20% longer than hex SHA-256.

```yaml
# Comparison
sha-256: sha256-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
c4:      c44jVTEz8y7wCiJcXvsX66BHhZEUdmtf7TNcZPy1jdM6S14qqrzsiLyoZRSvRGcAMLnKn4zVBvAFimNg14NFKp46cC
sha-512: sha512-cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e
```

The only encoding of a SHA-512 digest that is shorter than a C4 ID is an unlabeled Base64 string. It is only 4 characters shorter, and has none of the features of C4 IDs.

```yaml
# Base64 vs C4

unlabeled: 9/u6bgY2+JDlb7vzKD5STG+jIErimDgtYkdB0NxmODJuKCxBvl5CVNiCB3LFUYosWowMf37aGVlKfrU5RT4e1w
c4:        c45xZeXwMSpqXjpDumcHMA6mhoAmGHkUo7r9WmN2UgSEQzj9KjgseaQdkEJ11fGb5S1WEENcV3q8RFWwEeVpC7Fjk2
labeled:   sha512-9/u6bgY2+JDlb7vzKD5STG+jIErimDgtYkdB0NxmODJuKCxBvl5CVNiCB3LFUYosWowMf37aGVlKfrU5RT4e1w
hex:       sha512-cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e
```

If you double click on each of the ids above you'll find that only C4 IDs allow you to quickly cut and paste.  Double clicking on the other representations only selects a portion of the id.

The c4 id, only 4 characters longer than the shortest possible encoding, includes a label, can be used in file names and URLs, and is easer to select and copy.


### Example
Example usage to generate an c4 ID for a file.

```go
package main

import (
  "fmt"
  "io"
  "os"

  c4 "github.com/Avalanche-io/c4/id"
)

func main() {
  file := "main.go"
  f, err := os.Open(file)
  if err != nil {
    panic(err)
  }
  defer f.Close()

  // create a ID encoder.
  e := c4.NewEncoder()
  // the encoder is an io.Writer
  _, err = io.Copy(e, f)
  if err != nil {
    panic(err)
  }
  // ID will return a *c4.ID.
  // Be sure to be done writing bytes before calling ID()
  id := e.ID()
  // use the *c4.ID String method to get the c4id string
  fmt.Printf("C4id of \"%s\": %s\n", file, id)
  return
}

```

Output:

```bash
>go run main.go 
C4id of "main.go": c45oF5Jdtx29kuyxGt1vV9rALbAbhhgZde51FYHxJHDNB1rnFjmgzvJCgFH61ChV8MMcmnPuiDthiva7LYgAbhuy1X
```


### Releases 

Current release: [v0.6.0](https://github.com/Avalanche-io/c4/tree/v0.6.0)

Check the `release` branch for the latest release, or tags to find a specific release.  The `master` branch holds current development.

### License
This software is released under the MIT license.  See [LICENSE](./LICENSE) for more information.
