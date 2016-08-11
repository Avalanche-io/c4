
# C4 - The Cinema Content Creation Cloud
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)
[![GoDoc](https://godoc.org/github.com/etcenter/c4go?status.svg)](https://godoc.org/github.com/etcenter/c4)
[![Stories in Ready](https://badge.waffle.io/etcenter/c4.png?label=ready&title=Ready)](https://waffle.io/etcenter/c4)
[![Build Status](https://travis-ci.org/etcenter/c4.svg?branch=master)](https://travis-ci.org/etcenter/c4)
[![Coverage Status](https://coveralls.io/repos/github/etcenter/c4/badge.svg?branch=master)](https://coveralls.io/github/etcenter/c4?branch=master)

Go package and cli for c4.
### Command line tool
See [c4 command line tool](https://github.com/etcenter/c4/tree/master/cmd/c4)

C4 is the Cinema Content Creation Cloud.  This repo contains the c4 command line interface,
and the c4 demon.  We are in the process of rolling out the following features:

- [x] Identify any file or block of data.
- [x] Identify folders and arbitrarily complex filesystems.
- [x] Key/c4 id store
- [ ] Threaded multi-target copy and id
- [ ] Optimized remote file sync ("the rsync killer").
- [ ] Dependency graph/workflow language
- [ ] Fuse based file system.
- [ ] PKI Security Model

### The C4 framework
There are 3 major parts of the C4 Framework.  The ID system, the Domian Spacific Language,
and the Security Model.

### C4 ID

The C4 ID system is a standardized encoding of a SHA-512 hash.  It provides an unambiguous, universally
unique id for any file or block of data.  However, not only is the C4 ID universally unique, it is also
universally consistent.  This means given identical files in two different organizations, both
organizations would independently agree on the C4 ID, without the need for a central registry or any
other shared information.  This allows organizations to communicate with others about assets
consistently and unambiguously, while managing assets internally in anyway they choose.  This can be
done without prior agreement or communication of any kind.

This 'agreement without communication' is an essential feature of C4 IDs and a key differentiator
between it and other identification systems. It enables interoperability between human beings,
organizations, databases, software applications, and networks, and it is essential to the globally
distributed workflows of media production.

A C4 ID is a 90 character alphanumeric number with the following properties:

- **Always 90 characters long**
- **Starts with `c4`**
- **URL, Filename, and database safe**
- **No non-alphanumeric characters**

Here is an example of a c4 id:

```
c44jVTEz8y7wCiJcXvsX66BHhZEUdmtf7TNcZPy1jdM6S14qqrzsiLyoZRSvRGcAMLnKn4zVBvAFimNg14NFKp46cC
```



### C4 Lang
The C4 Domain Specific Language (DSL) is a declarative language that is designed to represent a
dependency graph of operations that are repeatable and verifiable. C4lang can describe processes that
span any number of physical domains, making it much easer to design and reason about distributed
workflows.  

All data in c4lang is immutable, operations are idempotent.  With these constraints, and the cashing
of results of any non-deterministic processes, a given c4lang graph node will eventually be reduced to
a fixed and immutable result.  This provides "strong eventual consistency" and abstracts
compute vs storage making them identical as far as the language is concerned. This fungibility of
computation and storage is a very powerful property of the language and reduces much of the complexity
of distributed computation for media production.

### C4 PKI
Under the C4 Public Key Infrastructure model there are no logins (other than a user on their own device).
Identity is automatically federated without the need for an "Identity Provider" provider
(i.e. the OAuth model).  Instead a standard x.509 certificate chain is used to validate *both* sides of
all communications.  This system works automatically via strong cryptography. It even works in off-line
environments such as productions in remote locations.

x.509 has a much longer history than OAuth, and is a well vetted component of standard secure web traffic.
OAuth is a system designed around the idea that some identity providers want to 'own' a user's account.
In media production, however, a more robust system that does not require a trusted 3rd party is required.

### Go Package
Example usage to generate an c4 ID for a file.

```go
package main

import (
  "fmt"
  "io"
  "os"

  // import 'asset' asset identification
  "github.com/etcenter/c4/asset"
)

func main() {
  file := "main.go"
  f, err := os.Open(file)
  if err != nil {
    panic(err)
  }
  defer f.Close()

  // create a ID encoder.
  e := asset.NewIDEncoder()
  // the encoder is an io.Writer
  _, err = io.Copy(e, f)
  if err != nil {
    panic(err)
  }
  // ID will return an *asset.ID.
  // Be sure to be done writing bytes before calling ID()
  id := e.ID()
  // use the *asset.ID String method to get the c4id string
  fmt.Printf("C4id of \"%s\": %s\n", file, id.String())
  return
}

```

Output:

```bash
>go run main.go 
C4id of "main.go": c44jVTEz8y7wCiJcXvsX66BHhZEUdmtf7TNcZPy1jdM6S14qqrzsiLyoZRSvRGcAMLnKn4zVBvAFimNg14NFKp46cC
```

### Releases 

Current release: [v0.6.0](https://github.com/etcenter/c4/tree/v0.6.0)

Check the `release` branch for the latest release, or tags to find a specific release.  The `master` branch holds currently development.

### License
This software is released under the MIT license.  See LICENSE for more information.
