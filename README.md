
# C4 - The Cinema Content Creation Cloud

[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)
[![GoDoc](https://godoc.org/github.com/Avalanche-io/c4?status.svg)](https://godoc.org/github.com/avalanche-io/c4)
[![Stories in Ready](https://badge.waffle.io/Avalanche-io/c4.png?label=ready&title=Ready)](https://waffle.io/Avalanche-io/c4)
[![Build Status](https://travis-ci.org/Avalanche-io/c4.svg?branch=master)](https://travis-ci.org/Avalanche-io/c4)


C4 the Cinema Content Creation Cloud is an open source framework for content creation using remote resources.

This repos holds the reference implementation of the C4 framework and source for the command line tools.

Command line tools are built from this repository, see releases for information about the latest builds.

---

## C4 ID 
The `id` package is a go implementation of the C4 ID standard.  It contains tools for generating and parsing C4 IDs, C4 Digests, and C4 ID Tree structures. 

For documentation see the [godocs](https://godoc.org/github.com/Avalanche-io/c4/id).

To import the package:

```go
import "github.com/Avalanche-io/c4/id"
```

### C4 ID - Universally Unique and Consistent Identification

- Videos:
  - [C4 Framework Universal Asset ID](https://youtu.be/ZHQY0WYmGYU)
  - [The Magic of C4](https://youtu.be/vzh0JzKhY4o)

[C4 ID Whitepaper](http://www.cccc.io/downloads/C4ID%20ETC%20Whitepaper_u2.pdf)

C4 ID, is a SMPTE standard for data identification that is a standardized encoding of a SHA512 hash, and a process for deriving a single C4 ID for any set of C4 IDs. 

The C4 ID system provides an unambiguous, universally unique id for any file or block of data. Unlike other mechanisms of identification, file paths, URLs, UUID, GUID, etc., C4 IDs are not only universally unique, but also universally consistent. This means that users at different locations and times will independently agree on the C4 ID of any data. This allows for consistent and unambiguous identification of data between parties or isolated systems without prior agreement or communication of any kind.

This 'agreement without communication' is the key to enabling interoperability for globally distributed workflows.

See the [README.md](id/README.md) in the `id` package for more information.

---

### Releases 

Current release: [v0.7.0](https://github.com/Avalanche-io/c4/tree/v0.7.0)

#### Previous Releases:

**Warning this version uses an incorrect character set for C4 IDs**
[v0.6.0](https://github.com/Avalanche-io/c4/tree/v0.6.0) 

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
