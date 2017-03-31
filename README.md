
# C4 - The Cinema Content Creation Cloud

[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)
[![GoDoc](https://godoc.org/github.com/Avalanche-io/c4?status.svg)](https://godoc.org/github.com/avalanche-io/c4)
[![Stories in Ready](https://badge.waffle.io/Avalanche-io/c4.png?label=ready&title=Ready)](https://waffle.io/Avalanche-io/c4)
[![Build Status](https://travis-ci.org/Avalanche-io/c4.svg?branch=master)](https://travis-ci.org/Avalanche-io/c4)


### The C4 framework
C4 the Cinema Content Creation Cloud is an open source framework for content creation using remote resources.

- Videos:
  - [C4 Framework Universal Asset ID](https://youtu.be/ZHQY0WYmGYU)
  - [The Magic of C4](https://youtu.be/vzh0JzKhY4o)

- Web: http://www.cccc.io/
- Mailing list: https://groups.google.com/forum/#!forum/c4-framework
- Twitter: https://twitter.com/CinemaC4

---

### C4 ID - Universal Identification

[C4 ID Whitepaper](http://www.cccc.io/downloads/C4ID%20ETC%20Whitepaper_u2.pdf)

The C4 ID system provides an unambiguous, universally unique id for any file or block of data. Unlike other mechanisms of identification, file paths, URLs, UUID, GUID, etc., C4 IDs are not only universally unique, but also universally consistent. This means that users at different locations and times will independently agree on the C4 ID of any data. This allows for consistent and unambiguous communication without prior agreement or communication of any kind.

This 'agreement without communication' is the key to enabling interoperability for globally distributed workflows.

See the [README.md](id/README.md) in `./id` for more information.

---

### C4 Lang - Domain Language for Wokflows

The C4 Domain Specific Language (DSL) is a declarative language that represents a dependency graph of operations that are repeatable and verifiable.  C4 Lang make it possible to describe processes that must work across ad-hoc and unreliable networks.

Like the [Bitcoin][1] block chain, the [git][2] source code management system, and CoreOS's [etcd][3], C4 is at it's core a way to address the [consensus problem][4] of distributed computing. 

In the context of media production computational work can be reduced to a graph of dependencies where each node represents a process that transforms inputs into outputs. C4 Lang identifies inputs and outputs by C4 IDs which allows systems to consider processing and storage to be interchangeable. 

This property extends to any combination of processes leading to the same C4 ID. This means that any dependency graph can be collapsed if the C4 ID of it's results are available or can be deduced. This fungibility of computation and storage is a very powerful property of the language that reduces much of the complexity of distributed computation by eliminating the need for message passing or synchronous remote procedure calls.

Both the results of a C4 Lang workflow and the dependency graph itself are immutable. By making workflows unmodifiable all systems are guaranteed to have a consistent view of any given workflow across all points in time. Until that workflow is superseded work can progress indefinitely to reduce it to it's fix and immutable results, surviving network outages, reboots, extended downtime and even storage failures. 

At any point in time it is unambiguous what work has been done and what work needs to be done, active synchronization is not required. If dependencies can be locally satisfied for pending tasks work progresses without knowledge of past events or the state of remote systems. When information from remote systems does arrive it can be combined with the local state in a lock free, conflict free merge. Systems needn't track where or when updates occur they need only ever consider the static state of the current environment at the moment of evaluation.

[1]: https://en.bitcoin.it/wiki/Block_chain
[2]: https://git-scm.com
[3]: https://coreos.com/etcd
[4]: https://en.wikipedia.org/wiki/Consensus_(computer_science)

---

### C4 PKI - Identity and Access Management

Under the C4 Public Key Infrastructure model there are no logins (other than a user on their own device). C4 PKI employs a public key infrastructure with mutually authenticated certificates to establish federated access between systems without needing an active network connection. C4 PKI eliminates the need for account logins, or identity providers like LDAP and OAuth. 

C4 PKI allows access to be structured hierarchically. An organization like a studio can grant access to sub organizations like a post house, and directly to individuals.  Two or more sub organizations with access to the same production, as granted by a signed certificate from the studio, can share data and interact with each other over secure connections. Encrypted communication is still possible and security is maintained even when an Internet connection is not available because users are authenticated by signed certificate not by user name and password.

---

##### Regular Expressions for C4 IDs
Here are some options for regular expressions with varying precision.
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

### Releases 

Current release: [v0.6.0](https://github.com/Avalanche-io/c4/tree/v0.6.0)

Check the `release` branch for the latest release, or tags to find a specific release.  The `master` branch holds current development.

### License
This software is released under the MIT license.  See [LICENSE](./LICENSE) for more information.
