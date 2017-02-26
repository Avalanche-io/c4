
# C4 - The Cinema Content Creation Cloud
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)
[![GoDoc](https://godoc.org/github.com/Avalanche-io/c4?status.svg)](https://godoc.org/github.com/Avalanche-io/c4)
[![Stories in Ready](https://badge.waffle.io/Avalanche-io/c4.png?label=ready&title=Ready)](https://waffle.io/Avalanche-io/c4)
[![Build Status](https://travis-ci.org/Avalanche-io/c4.svg?branch=master)](https://travis-ci.org/Avalanche-io/c4)
[![Coverage Status](https://coveralls.io/repos/github/Avalanche-io/c4/badge.svg?branch=master)](https://coveralls.io/github/Avalanche-io/c4?branch=master)


### The C4 framework
C4 the Cinema Content Creation Cloud is an open source framework for content creation using remote resources. It consists of C4 ID, C4 Lang, and C4 PKI. **C4 ID** is a universal ID system. **C4 Lang** is a domain language for workflows. **C4 PKI** is a federated security model.

- Videos:
  - [C4 Framework Universal Asset ID](https://youtu.be/ZHQY0WYmGYU)
  - [The Magic of C4](https://youtu.be/vzh0JzKhY4o)

- Web: http://www.cccc.io/
- Mailing list: https://groups.google.com/forum/#!forum/c4-framework
- Twitter: https://twitter.com/CinemaC4

---

Go package and cli for c4.

### c4id Command line tool
See [c4 command line tool](https://github.com/Avalanche-io/c4/tree/master/cmd/c4id)


---

### C4 ID (c4id)

#### Intro

[C4 ID Whitepaper](http://www.cccc.io/downloads/C4ID%20ETC%20Whitepaper_u2.pdf)

The C4 ID system is a standardized encoding of a SHA-512 hash. It provides an unambiguous, universally unique id for any file or block of data. Unlike all other id systems in use today, file path, URI, UUID, GUID, C4 IDs are not only universally unique, but also universally consistent. This means a given regardless of where it's found who or when it is identified it will always have the same C4 ID. Different organizations will independently agree on the C4 ID, without the need for a central registry or any other shared information. This allows for consistent and unambiguous communication, while retaining the freedom to move and rename files any any way.  This can all be done without prior agreement or communication of any kind.

This 'agreement without communication' is an essential feature of C4 IDs and a key differentiator between it and other identification systems. It enables interoperability between human beings, organizations, databases, software applications, and networks, and it is essential for globally
distributed workflows.

#### Details
A C4 ID is a 90 character alphanumeric number with the following properties:

- **Always 90 characters long**
- **Starts with `c4`**
- **URL, Filename, and database safe**
- **No non-alphanumeric characters**

Here is an example of a c4 id the c4id of "" (empty string):

```
c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT
```

#### C4 ID, vs Sha256 and Sha512 Hex
There are no universal standard encodings for common cryptographic hashes.  Labeled hex representations (i.e. sha512-cf83...) seem to be a tad more popular at the moment, so those are used for comparison below.  Note that c4 is a sha-512, yet it's only 19 characters longer than a hex encoded sha-256, and also faster to compute (on 64 bit hardware).

```yaml
# Comparison
sha-256: sha256-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
c4:      c44jVTEz8y7wCiJcXvsX66BHhZEUdmtf7TNcZPy1jdM6S14qqrzsiLyoZRSvRGcAMLnKn4zVBvAFimNg14NFKp46cC
sha-512: sha512-cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e
```

#### C4 ID, vs Sha512 Base64
It might seem like Base64 would be better because it would make the string shorter.  However Base64 only saves 4 characters even if we cheat and remove the label.

```yaml
# Base64 vs C4

c4:      c45XyDwWmrPQwJPdULBhma6LGNaLghKtN7R9vLn2tFrepZJ9jJFSDzpCKei11EgA5r1veenBu3Q8qfvWeDuPc7fJK2
Sha-512: 9/u6bgY2+JDlb7vzKD5STG+jIErimDgtYkdB0NxmODJuKCxBvl5CVNiCB3LFUYosWowMf37aGVlKfrU5RT4e1w
```

The c4 id, only 4 characters longer, includes a label and is easily selectable by double clicking. If you double click on the above sha-512 you'll see that you don't select the entire string.

#### C4 ID Features

- C4 IDs can be used in filenames, urls, and database fields.
- If used as a filename de-duplication comes for free.
- C4 IDs contain only alphanumeric, no special characters.
- Double clicking selects an entire c4 id.
- Multiple files can be identified by a single C4 ID.
- Any block of data can be identified, not just files.
- C4 IDs can be recognized without labeling.
- Simple to capture with a regular expression (1).
- Differentiating IDs is easy (2).
- Computed vs stored values are interchangeable (see c4dg).
- IDs are uniformly distributed so avoid 'hot-spots' in object storage.

1. Here are some options for regular expressions with varying precision.
  - Non-overlapping matches.
    - `c4\w{88}`
    - `c4[1-9A-Za-z]{88}`
    - `c4[1-9A-HJ-NP-Za-km-z]{88}`
    - `c4[1-6][1-9A-HJ-NP-Za-km-z]{87}`
    - `c46[1-7][1-9A-HJ-NP-Za-km-z]{86}|c4[1-5][1-9A-HJ-NP-Za-km-z]{87}`
  - Overlapping matches.
    - `(?=(c4\w{88}))`
    - `(?=(c4[1-9A-Za-z]{88}))`
    - `(?=(c4[1-9A-HJ-NP-Za-km-z]{88}))`
    - `(?=(c4[1-6][1-9A-HJ-NP-Za-km-z]{87}))`
    - `(?=(c46[1-7][1-9A-HJ-NP-Za-km-z]{86}|c4[1-5][1-9A-HJ-NP-Za-km-z]{87}))`
2. In practice C4 IDs are not likely to be similar by more than 5 characters. IDs with 6 identical characters in sequence appear only once per 38 billion. Further if a sets of IDs are in sorted order the right most digits are statistically unlikely to ever match unless the IDs are identical. Because of this, matching IDs are very easy to detect visually especially in large blocks of IDs.  When comparing IDs visually that are in different locations (i.e. paper and screen), compare the right most digits first for best results.

### C4 Dependency Graph (c4dg)

*(Whitepaper coming soon)*

The C4 Domain Specific Language (DSL) is a declarative language that is designed to represent a dependency graph of operations that are repeatable and verifiable. C4lang can describe processes that span any number of physical domains, making it much easer to design and reason about distributed workflows.

All data in c4lang is immutable, operations are idempotent.  With these constraints, and the cashing of results of any non-deterministic processes, a given c4lang graph node will eventually be reduced to a fixed and immutable result.  This provides "strong eventual consistency" and abstracts compute vs storage making them identical as far as the language is concerned. This fungibility of computation and storage is a very powerful property of the language and reduces much of the complexity of distributed computation for media production.

### C4 Security (c4pki)

Under the C4 Public Key Infrastructure model there are no logins (other than a user on their own device). Identity is automatically federated without the need for an "Identity Provider" provider (i.e. the OAuth model).  Instead a standard x.509 certificate chain is used to validate *both* sides of all communications.  This system works automatically via strong cryptography. It even works in off-line environments such as productions in remote locations.

x.509 has a much longer history than OAuth, and is a well vetted component of standard secure web traffic. OAuth is a system designed around the idea that some identity providers want to 'own' a user's account. In media production, however, a more robust system that does not require a trusted 3rd party is required.

### Go Package
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
  e := c4.NewIDEncoder()
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
C4id of "main.go": c44skR7eEM9VPEop3QNNZjMWYUmKLwQDu4zq3Kh1LyrbdxX2zNb1j1YRCKGViNb4V1hzCrb4z1dCp7S53JMZ7Ao9U9
```

### Releases 

Current release: [v0.6.0](https://github.com/Avalanche-io/c4/tree/v0.6.0)

Check the `release` branch for the latest release, or tags to find a specific release.  The `master` branch holds current development.

### License
This software is released under the MIT license.  See [LICENSE](./LICENSE) for more information.
