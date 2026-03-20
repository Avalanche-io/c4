# FAQ

## Why SHA-512? Why not algorithm agility?

A C4 ID is a permanent name for content. It is not a checksum you
verify and discard. Two people who have never met, working decades
apart, produce the same name for the same content. No coordination,
no registry, no versioning of the naming scheme.

This only works if the algorithm never changes.

If SHA-512 were replaced, every existing C4 ID becomes an "old format"
ID. You would need to know which algorithm produced any given ID. The
property that makes C4 IDs useful -- "I found this ID, I know what it
means" -- breaks the moment there are two possible meanings.

**Concrete examples:**

- A delivery manifest created in 2026 can be verified in 2046 without
  knowing what software created it. The algorithm is fixed. The ID is
  self-describing.
- A C4 ID found in a database, email, filename, or metadata field
  needs no version prefix, no algorithm tag, no context. It is always
  SHA-512, always base58, always 90 characters starting with `c4`.
- Two facilities that have never communicated can discover they have
  identical content by comparing IDs. This works because there is
  exactly one way to compute a C4 ID.

**On quantum computing:**

Even under Grover's algorithm, SHA-512's collision resistance degrades
from 2^256 to 2^128 -- still stronger than any classical attack on any
hash function in widespread use today. There is no credible threat
model requiring SHA-512 replacement in the foreseeable future. If that
changes, we will have decades of warning and the entire industry will
be rebuilding, not just C4.

**On IPFS multihash:**

IPFS chose algorithm agility. C4 chose algorithm permanence. Both are
valid designs optimizing for different things. Multihash optimizes for
cryptographic flexibility -- the ability to upgrade the hash function
without breaking the addressing scheme. C4 optimizes for universal
discoverability across time and space -- the guarantee that an ID
means the same thing everywhere, forever, with no coordination.

## Why is the c4m format not an external standard?

C4 IDs are standardized as [SMPTE ST 2114:2017](https://ieeexplore.ieee.org/document/7971777).
The c4m format builds on this standard, adding filesystem metadata
(permissions, timestamps, sizes, names) around C4 IDs.

The format is deliberately simple: one line per entry, fields in
predictable positions, plain text throughout. Independent
implementations are straightforward -- [libc4](https://github.com/Avalanche-io/libc4)
(C/C++) and [c4py](https://github.com/Avalanche-io/c4py) (Python)
both implement the format, producing byte-identical output to the Go
reference implementation.

External standardization may be pursued as adoption grows. The format
is stable and documented in the [c4m specification](../c4m/SPECIFICATION.md).

## How does the content store scale?

The content store uses adaptive trie sharding based on C4 ID prefixes.

When a directory contains fewer than 4096 objects, files are stored
flat -- one file per C4 ID. When the count exceeds 4096, the store
automatically splits into 2-character prefix subdirectories derived
from the C4 ID. This keeps directory listings fast on any filesystem.

```
# Flat (< 4096 objects):
store/c43Q4j81SxGkV9FhbeW23YrMTj6...
store/c44iCq6un9W47x7ydjJSWp4arMJ...

# Sharded (>= 4096 objects):
store/c4/3Q4j81SxGkV9FhbeW23YrMTj6...
store/c4/4iCq6un9W47x7ydjJSWp4arMJ...
store/c5/...
```

The same layout is used by all ecosystem tools (`c4`, `c4sh`, `c4git`,
`c4py`). Content stored by any tool is immediately available to all
others.

The design can handle billions of objects. At 4096 files per
subdirectory with 58^2 = 3364 possible prefixes, each level supports
roughly 13 million objects before a second level of sharding would be
needed. In practice, this has not yet been tested at that scale in
production -- we are honest about this. The architecture is sound, but
billion-object production deployments remain ahead of us.
