# Encrypted Content Delivery in C4

## Conclusion

Untrusted relay encryption cannot be justified under the C4 philosophy. Every
viable design either breaks source independence, doubles storage, requires
pre-encryption of all content, or introduces formats and protocols that fail
the "every layer must justify its existence" test.

The right answer: **relays must be trusted.** Trusted relays + TLS + content
addressing already provide confidentiality in transit, integrity verification,
and authentication. No new formats, no cipher tables, no protocol changes.

The remainder of this document preserves the analysis that led to this
conclusion.

---

## The Question

c4m files describe filesystem content using plaintext C4 IDs. When content
traverses relays (NAT-traversing intermediaries), could an untrusted relay
read the content or correlate content identities? Should c4 encrypt content
for relay transit?

## Constraints (from C4 Philosophy)

**"A thing is what it contains."** Content-addressed storage means the C4 ID
is the hash of the bytes. Anything stored under a C4 ID must hash to that ID.
Relays, endpoints, and any storage system can verify integrity by recomputing
the hash. This property cannot be sacrificed.

**"Every layer of abstraction must justify its existence."** Encryption adds
complexity. The format, the mechanism, the key management — each piece must
earn its place. If any part can be eliminated without losing a necessary
property, it should be.

**"Knowing about data and having data are different things."** The c4m file is
the description. Content is the data. Encryption belongs on the data side, not
the description side. C4M files should remain human-readable, diffable,
email-able text.

**"Simplicity is the goal, not a tradeoff."** The solution should have the
fewest concepts that cover the necessary cases.

**Source independence.** Receivers can pull content from any available source —
the original sender, a peer, a relay, an S3 bucket, a USB drive. The C4 ID is
sufficient to request and verify content from any source. This property is
fundamental to c4's pull model.

## Approaches Explored

### Approach 1: Transport-Layer Encryption

Encrypt content at the HTTP boundary. Endpoints serve encrypted content on GET,
decrypt on receipt. Relays handle opaque ciphertext.

**Result:** Broke when multiple workspace keys were involved. Cipher map was
1:1 but needed 1:N. Content type was ambiguous (relay couldn't distinguish
plaintext from ciphertext). Scenario analysis revealed 10+ broken cases.
Abandoned.

### Approach 2: Package Format (Cipher Table)

Encrypt content independently. Each ciphertext has its own C4 ID (hash of
ciphertext bytes). A cipher table maps plaintext IDs to cipher IDs. Recipients
request encrypted content by cipher ID from relays.

**Result:** Preserves content addressing and source independence. But requires:

- **Pre-encryption of all content** to compute cipher IDs for the table.
  Cryptographic analysis confirmed this is unavoidable — Hash(ciphertext)
  cannot be derived from (key, Hash(plaintext)) without encrypting. This is
  fundamental to the nonlinearity of cryptographic hash functions. Every
  production encrypted content-addressed system (Tahoe-LAFS, ERIS, Storj)
  faces the same constraint.

- **A cipher table artifact** communicated alongside the c4m file. For a
  sequence-heavy VFX project (47k files), the cipher table is 6–9 MB — an
  order of magnitude larger than the c4m file. This is a new layer that must
  justify its existence.

- **Storage or CPU overhead.** Either store both plaintext and ciphertext
  (doubles storage), or encrypt-hash-discard then re-encrypt on serve
  (doubles CPU per file).

### Approach 3: Derivable Locators (Tahoe-LAFS Pattern)

Separate routing from verification. Recipients compute HMAC(key, plaintextID)
as a locator. Relays maintain a locator → ciphertext hash index. Senders
encrypt on-the-fly when content is first requested.

**Result:** Eliminates the cipher table and pre-encryption. But **breaks
source independence.** Only endpoints with the workspace key can respond to
locator requests. A relay can serve cached content, but an S3 bucket, USB
drive, or peer without the key cannot fulfill a locator request. Source
independence — the ability to pull from any available source — is a core C4
property that cannot be sacrificed.

### Approach 4: Derived Cipher IDs (No Encryption Needed)

Define cipher IDs as a deterministic function of key + plaintext ID:
`cipherID = SHA-512(key || plaintextID)`. Recipients compute cipher IDs
locally from the c4m file.

**Result:** Simplest possible design. But the cipher ID is not the hash of
the ciphertext — relays store content under an ID that doesn't match the
content hash. This breaks content-addressed verification. Relays cannot verify
integrity by hashing. This violates "trust should be mathematical."

## Why Every Approach Fails

| Approach | Breaks |
|----------|--------|
| Transport-layer | Multi-key scenarios, content type ambiguity |
| Cipher table | Requires pre-encryption, storage/CPU doubling, new artifact |
| Derivable locators | Source independence |
| Derived cipher IDs | Content-addressed verification |

The fundamental tension: encrypting content for untrusted relays requires
either (a) knowing the ciphertext hash upfront (impossible without encrypting)
or (b) using non-content-addressed identifiers (breaks the trust model or
source independence). There is no design that preserves all of C4's core
properties while adding untrusted relay encryption.

## The Right Answer: Trust Relays

If relays are trusted, the problem disappears:

- **Confidentiality in transit:** TLS between all nodes.
- **Integrity:** Content addressing. Relay cannot serve corrupted data — hash
  verification catches it.
- **Authentication:** PKI certificates (existing design in
  STORAGE_AND_SECURITY_ARCHITECTURE.md).
- **Source independence:** Fully preserved. Content flows as plaintext C4 IDs.
  Any source can serve any content.
- **Simplicity:** No new formats, no cipher tables, no locator indexes, no
  protocol changes. The c4m file is still just a c4m file.

Trusting relays is reasonable:

- You choose which relays to use.
- PKI authenticates them via certificate chains.
- You trust them the way you trust your cloud provider or studio IT.
- A compromised relay can attempt to serve bad data, but content addressing
  catches it (hash mismatch = reject).
- If at-rest protection matters on the relay, the relay encrypts its own store
  with its own key — a local concern, not a protocol concern.

The only property not covered: a relay operator (or someone who compromises a
relay) can read content stored on the relay. This is the same trust model as
S3, GCS, or any cloud storage without client-side encryption. For most
workflows, this is acceptable. For workflows where it isn't, the answer is not
a protocol-level encryption layer that sacrifices core properties — the answer
is to not use untrusted relays for that content.

## What We Keep

The `internal/encryption` package (XChaCha20-Poly1305, deterministic nonce
from C4 ID) remains in the c4d codebase. It is not part of the relay protocol,
the c4m format, or the transfer system.

## Future: Encrypt at Rest

Encrypt at rest on individual nodes is a plausible future feature and avoids
every problem that relay encryption creates. The store layer would transparently
encrypt on write and decrypt on read using a node-local key.

- **C4 IDs still refer to plaintext.** The encryption is invisible to
  everything above the store — HTTP layer, transfer system, c4m files, peers.
- **No cipher tables.** The ID is always the plaintext hash. The store just
  happens to encrypt bytes on disk.
- **No source independence issues.** Content is served decrypted. Any node
  serves any C4 ID normally.
- **No protocol changes.** Purely a local storage concern.
- **No pre-encryption problem.** Encrypt on write, decrypt on read, single
  pass.
- **Node-local key.** Each node manages its own key. No distribution problem.

This is conceptually equivalent to dm-crypt/LUKS or FileVault at the
application layer. The existing `internal/encryption` package provides
everything needed. Implementation would be a thin wrapper in `internal/store`
that encrypts before writing to disk and decrypts after reading — a future
enhancement, not a protocol decision.

## References

- Tahoe-LAFS: storage index (derivable) + ciphertext hash tree (verifiable)
- ERIS: block reference + encrypted block hash
- Storj: erasure-coded encrypted pieces + piece hashes
- All production encrypted content-addressed systems separate routing from
  verification, and all require pre-encryption to compute ciphertext hashes.
