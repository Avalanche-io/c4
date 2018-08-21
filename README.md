
# C4 ID - Universally Unique and Consistent Identification

[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)
[![GoDoc](https://godoc.org/github.com/Avalanche-io/c4?status.svg)](https://godoc.org/github.com/avalanche-io/c4)
[![Stories in Ready](https://badge.waffle.io/Avalanche-io/c4.png?label=ready&title=Ready)](https://waffle.io/Avalanche-io/c4)
[![Build Status](https://travis-ci.org/Avalanche-io/c4.svg?branch=master)](https://travis-ci.org/Avalanche-io/c4)

_**Note:** The C4 ID package has recently moved from `github.com/Avalanche-io/c4/id` to `github.com/Avalanche-io/c4`, and the API has been cleaned up considerably to perform better and be easier to use. While deprecated the previous C4 ID API will continue to work and remain unchanged at the current `c4/id` path to give users time to switch._

This is a Go package that implements the C4 ID system **SMPTE standard ST 2114:2017**. C4 IDs are universally unique and consistent identifiers that standardize the derivation and formatting of data identification so that all users independently agree on the identification of any block or set of blocks of data.

C4 IDs are 90 character long strings suitable for use in filenames, URLs, database fields, or anywhere else that a string identifier might normally be used. In ram C4 IDs are represented in a 64 byte "digest" format.

The `id` package is a go implementation of the C4 ID standard.  It contains tools for generating and parsing C4 IDs, and C4 ID Tree structures.

### Example Usage

```go
package main

import (
  "fmt"
  "strings"

  "github.com/Avalanche-io/c4"
)

func main() {

  // Generate a C4 ID for any contiguous block of data...
  id := c4.Identify(strings.NewReader("alfa"))
  fmt.Println(id)
  // output: c43zYcLni5LF9rR4Lg4B8h3Jp8SBwjcnyyeh4bc6gTPHndKuKdjUWx1kJPYhZxYt3zV6tQXpDs2shPsPYjgG81wZM1

  // Generate a C4 ID for any number of non-contiguous blocks...
  var ids c4.IDs
  var inputs = []string{"alfa", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india"}
  for _, input := range inputs {
    ids = append(ids, c4.Identify(strings.NewReader(input)))
  }
  fmt.Println(ids.ID())
  // output: c435RzTWWsjWD1Fi7dxS3idJ7vFgPVR96oE95RfDDT5ue7hRSPENePDjPDJdnV46g7emDzWK8LzJUjGESMG5qzuXqq
}
```

---

### Releases

Current release: [v0.7.0](https://github.com/Avalanche-io/c4/tree/v0.7.0)

#### Previous Releases:

**Warning this version uses an incorrect character set for C4 IDs**
[v0.6.0](https://github.com/Avalanche-io/c4/tree/v0.6.0)


### Links

Videos:
  - [C4 Framework Universal Asset ID](https://youtu.be/ZHQY0WYmGYU)
  - [The Magic of C4](https://youtu.be/vzh0JzKhY4o)

[C4 ID Whitepaper](http://www.cccc.io/downloads/C4ID%20ETC%20Whitepaper_u2.pdf)

### Contributing

Contributions are welcome. Following are some general guidelines for project organization. If you have questions please open an issue.

The `master` branch holds the current release, older releases can be found by their version number. The `dev` branch represents the development branch from which bug and feature branches should be taken. Pull requests that are accepted will be merged against the `dev` branch and then pushed to versioned releases as appropriate.

Feature and bug branches should follow the github integrated naming convention.  Features should be given the `new` tag, and bugs the `bug` tag.  Here is an example of checking out a feature branch:

```bash
> git checkout dev
Switched to branch 'dev'
Your branch is up-to-date with 'origin/dev'.
> git checkout -b new/#99_some_github_issue
...
```

If a branch for an issue is already listed in this repository then check it out and work from it.

### License
This software is released under the MIT license.  See [LICENSE](./LICENSE) for more information.
