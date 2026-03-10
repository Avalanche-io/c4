# Universal Filename Encoding Specification

**A Three-Tier Encoding for Faithful Byte-Level Filename Transport**

Draft Specification v0.1 — March 2026

---

## 1. Introduction

This document specifies a text-based encoding for arbitrary byte sequences representing filesystem names. The encoding is designed for use in configuration files, serialization formats, formal specifications, and any context where filesystem identifiers must be represented as human-readable text.

The encoding provides three properties simultaneously: faithful round-trip transport of all legal byte values, human readability of common filenames, and visual salience of anomalous or non-printable bytes.

## 2. Design Philosophy

### 2.1. Identifiers Are Not Display Strings

A filename is an opaque byte-sequence identifier. The historical conflation of filesystem identifiers with user-interface display strings is the root cause of encoding fragility across the ecosystem. This specification treats the distinction between transport and interpretation as foundational.

This conflation is visible even at the kernel level. The Linux VFS layer defines a maximally permissive contract — filenames are opaque byte strings with only `/` and `\0` forbidden — yet the kernel's own internal filesystem constructions (procfs, sysfs, devtmpfs) implicitly assume printable ASCII names. The contract permits what the implementation quietly declines to exercise. The fragility is not in the permissiveness of the contract but in the failure of consumers to honor it, precisely because they treat identifiers as display strings.

A filename occupies the same conceptual category as a memory pointer or a hash value: it is an opaque handle to data. No system would restrict memory addresses to values that render legibly in a terminal. The same principle applies here.

### 2.2. Maximally Permissive Transport

This encoding accepts any byte sequence that does not contain the zero byte (0x00). The sole forbidden value is 0x00, which serves as the universal path delimiter (see Section 2.3). All other byte values, including those conventionally classified as control characters, whitespace, or non-UTF-8 sequences, are legal and must survive a round trip intact.

The encoding layer does not interpret, sanitize, normalize, or reject byte sequences. If a caller provides bytes satisfying the single constraint, the encoding guarantees their preservation.

### 2.3. Path Delimiter Mapping

The conventional path separator `/` (0x2F) is mapped to the zero byte (0x00) at the abstraction layer. This yields a single forbidden value for both path delimiting and string termination, and reclaims 0x2F as a legal byte within name components. Path splitting reduces to splitting on zero, the most primitive possible operation.

### 2.4. Truthful Error Reporting

The encoding layer's obligations are faithful transport and honest error reporting. When a backing filesystem cannot round-trip a given byte sequence (due to normalization, case folding, character substitution, or other limitations), the encoding layer surfaces this as a backend error. It does not silently absorb data loss or preemptively restrict the input domain.

Filesystems that round-trip faithfully are conformant backends. Filesystems that do not are lossy, and this layer tells the truth about that rather than concealing it.

## 3. Encoding Specification

The encoding uses three tiers, each handling a class of byte values with increasing levels of transformation. In the common case, most bytes pass through as readable text. Anomalous bytes receive visually distinct encoding.

### 3.1. Tier 1: Printable Unicode Passthrough

Byte sequences that form valid UTF-8 encodings of printable Unicode code points pass through the encoding unchanged, with one exception: the generic currency sign ¤ (U+00A4) is reserved as the Tier 3 escape character and must not pass through literally.

This tier covers the vast majority of real-world filenames. Latin, CJK, Cyrillic, Arabic, Devanagari, emoji, and all other printable Unicode renders directly as readable text.

### 3.2. Tier 2: Conventional Control Character Escapes

Common control characters that have well-established backslash escape representations use those conventions:

| Byte Value | Escape Sequence | Description            |
|------------|-----------------|------------------------|
| 0x00       | `\0`            | Null / path delimiter  |
| 0x09       | `\t`            | Horizontal tab         |
| 0x0A       | `\n`            | Line feed              |
| 0x0D       | `\r`            | Carriage return        |
| 0x5C       | `\\`            | Literal backslash      |

These escapes operate at the character level. They are syntactic sugar over Tier 3 byte encoding and exist solely for human readability.

### 3.3. Tier 3: Byte-Level Encoding via Braille Block

All byte values not handled by Tier 1 or Tier 2 are encoded using a dedicated escape character followed by one or more Unicode Braille Pattern characters.

**Escape Character:** ¤ (U+00A4, Generic Currency Sign)

This code point was chosen for the following properties: it is a printable, widely-supported character present in Latin-1; it has effectively zero real-world usage (having been superseded by specific currency symbols); and its original semantic — "placeholder for a context-dependent value" — aligns with its function here.

**Encoding Block:** Unicode Braille Patterns, U+2800–U+28FF

This block contains exactly 256 code points, providing a one-to-one mapping with all possible byte values (0x00–0xFF). A byte with value *N* is represented by the code point U+2800 + *N*.

**Range Encoding:** Consecutive non-printable bytes are encoded within symmetric delimiters.

A single encoded byte is written as: `¤⟨Braille⟩¤`

A range of consecutive encoded bytes is written as: `¤⟨Braille⟩⟨Braille⟩...⟨Braille⟩¤`

The opening ¤ enters byte-encoding mode. Each subsequent Braille Pattern character represents one byte. The closing ¤ exits byte-encoding mode. This symmetric structure ensures that byte range boundaries are self-contained and verifiable without reference to surrounding context.

## 4. Formal Grammar

The encoding is defined by the following grammar, expressed in extended BNF:

```
encoded-name  = { segment } ;
segment       = tier1-char | tier2-escape | tier3-range ;

tier1-char    = <any printable UTF-8 code point except U+00A4 and U+005C> ;

tier2-escape  = '\\' | '\n' | '\t' | '\r' | '\0' ;

tier3-range   = CURRENCY braille-char { braille-char } CURRENCY ;

braille-char  = <any code point in U+2800..U+28FF> ;

CURRENCY      = U+00A4 ;
```

**Decoding rule:** The parser operates in one of two modes. In *text mode* (the default), characters are interpreted as Tier 1 literals or Tier 2 escapes. Upon encountering ¤, the parser enters *byte mode* and interprets each subsequent character as a Braille-encoded byte until a second ¤ is encountered, which returns the parser to text mode.

## 5. Self-Escape Mechanics

### 5.1. Literal Currency Sign

The character ¤ (U+00A4) is never a literal in the encoding. It always functions as a byte-mode delimiter. To represent a filename containing a literal ¤, encode its constituent UTF-8 bytes (0xC2, 0xA4) through Tier 3:

```
Literal ¤ in filename  →  ¤⣂⢤¤
```

Where: 0xC2 → U+28C2 (⣂) and 0xA4 → U+28A4 (⢤). Note that because the ground truth of this encoding is bytes, and ¤ is excluded from Tier 1, its UTF-8 byte components are always encoded at the byte level. No special case is required; the rule follows mechanically.

### 5.2. Literal Backslash

The backslash character (0x5C) is encoded as `\\` per Tier 2 conventions. This is the sole self-escape in Tier 2.

## 6. Correctness Property: Round-Trip Identity

**Claim:** For every byte sequence *B* not containing 0x00, decode(encode(*B*)) = *B*.

**Sketch:** The encoding partitions the input byte sequence into maximal spans, where each span is classified into exactly one tier. Tier 1 spans are UTF-8 code point sequences that pass through unchanged; identity holds trivially. Tier 2 substitutions are bijective single-byte replacements; the decode function reverses each escape to its original byte. Tier 3 maps each byte *N* to a unique Braille code point U+2800+*N*; this is an injection from [0x00, 0xFF] to [U+2800, U+28FF] and the decode function subtracts U+2800 to recover *N*. The delimiters ¤ and `\` are unambiguous mode transitions that consume no payload bytes. No two tiers claim the same input byte (the partition is exhaustive and disjoint), so no ambiguity arises during decoding.

**Canonicalization:** The encoding produces a canonical form. For any input *B*, encode(*B*) is unique. Bytes eligible for Tier 1 must be encoded via Tier 1 (the highest-priority tier). Bytes eligible for Tier 2 must use Tier 2. All remaining bytes use Tier 3. This priority ordering eliminates representational ambiguity.

## 7. Security Properties

### 7.1. Anomalous Byte Salience

A critical property of this encoding is that anomalous bytes are visually conspicuous. Because Tier 1 preserves all printable Unicode literally, the encoding does not generate visual noise around legitimate characters. The Braille escape sequences are visually alien against readable text, which means that injected or unexpected bytes are immediately apparent to human inspection.

This contrasts with encodings such as percent-encoding or C-style hex escapes, which mangle all non-ASCII bytes uniformly. In those schemes, a malicious 0x01 byte hiding among escaped accented characters is camouflaged by the noise the encoding itself creates. In this encoding, legitimate Unicode passes through readable while anomalous bytes receive distinctive visual markers.

### 7.2. Self-Contained Range Boundaries

The use of symmetric ¤ delimiters for Tier 3 ranges ensures that the boundaries of a byte-encoded region are determinable without reference to surrounding context. An attacker who controls part of a filename cannot influence where the parser believes a byte range begins or ends. This property would be absent with an implicit-exit parsing strategy where byte mode terminates upon encountering a non-Braille character.

### 7.3. Non-Conflation of Encoding Levels

Tier 2 (character-level escapes) and Tier 3 (byte-level encoding) use distinct syntactic markers: backslash and ¤ respectively. This prevents confusion between character semantics and byte semantics in the encoded output.

## 8. Efficiency Analysis

### 8.1. Character-Level Density

The encoding's character-level overhead depends on the tier:

| Tier              | Overhead     | Notes                                  |
|-------------------|--------------|----------------------------------------|
| Tier 1            | 1:1          | Printable Unicode passes through       |
| Tier 2            | 2:1          | One payload byte per two-char escape   |
| Tier 3 (single)   | 3:1          | ¤ + Braille + ¤ for one byte           |
| Tier 3 (range of n) | (n+2) : n | Approaches 1:1 as n grows             |

### 8.2. Comparison With Other Schemes (Pathological Case)

For a pathological input of 8 non-printable bytes (`01 02 FF FE 0A 0D 1B 7F`):

| Scheme                              | Encoded Characters | Ratio   |
|-------------------------------------|--------------------|---------|
| This specification (Tier 3 range)   | 12                 | 1.5 : 1 |
| Percent-encoding                    | 24                 | 3 : 1   |
| C-style hex escapes (`\xHH`)       | 32                 | 4 : 1   |
| JSON unicode escapes (`\uHHHH`)    | 38                 | ~5 : 1  |

### 8.3. Mixed Content Advantage

For filenames containing mostly valid Unicode with isolated anomalous bytes (the typical real-world case for corrupted or adversarial filenames), this encoding offers a qualitative advantage beyond density: legitimate content remains readable. Percent-encoding and hex-escape schemes destroy the readability of non-ASCII Unicode (such as accented Latin, CJK, or Cyrillic characters) even though these are perfectly valid, penalizing all non-ASCII bytes equally. This encoding only penalizes bytes that genuinely require it.

## 9. UTF-8 Wire Representation

When serialized as UTF-8, the Braille code points U+2800–U+28FF each occupy three bytes (E2 A0–A3 80–BF), and the ¤ delimiter occupies two bytes (C2 A4). The byte-level overhead of Tier 3 is therefore 3:1 per encoded byte plus constant delimiter cost.

This overhead is effectively neutralized by compression. The Braille block's UTF-8 encoding has high structural regularity: a fixed first byte (0xE2) and only four possible second bytes (0xA0–0xA3). LZ-family and Huffman-based compressors exploit this redundancy efficiently, reducing the effective cost to near the information-theoretic minimum. In practice, this structure compresses more efficiently than hex-encoded alternatives, which distribute payload across 16 distinct ASCII characters with no structural redundancy.

For systems where compressed storage or transport is standard (and the specification assumes this is the common case), the UTF-8 wire overhead is a non-issue.

## 10. Examples

### 10.1. Simple Filename (Pure Tier 1)

```
Bytes:    68 65 6C 6C 6F 2E 74 78 74
Encoded:  hello.txt
```

### 10.2. Embedded Tab (Tiers 1 + 2)

```
Bytes:    6B 65 79 09 76 61 6C 75 65
Encoded:  key\tvalue
```

### 10.3. Embedded Newline (Tiers 1 + 2)

```
Bytes:    6C 69 6E 65 31 0A 6C 69 6E 65 32
Encoded:  line1\nline2
```

### 10.4. Raw Non-Printable Bytes (Tiers 1 + 3)

```
Bytes:    01 64 61 74 61 FF
Encoded:  ¤⠁¤data¤⣿¤
```

Where: 0x01 → U+2801 (⠁) and 0xFF → U+28FF (⣿)

### 10.5. Contiguous Non-Printable Range (Tier 3 Range)

```
Bytes:    01 02 FF FE 0A 0D 1B 7F
Encoded:  ¤⠁⠂⣿⣾⠊⠍⠛⡿¤
```

Note: 0x0A and 0x0D could alternatively be encoded via Tier 2 as `\n` and `\r`, splitting the range. The choice between a single Tier 3 range and interleaved Tier 2 escapes is an implementation decision; both decode to the same byte sequence.

### 10.6. Literal Currency Sign (Tier 3 Self-Escape)

```
Bytes:    70 72 69 63 65 C2 A4 35 30   (UTF-8 for "price¤50")
Encoded:  price¤⣂⢤¤50
```

### 10.7. Full Mix (All Tiers)

```
Bytes:    48 69 09 1B 5B 33 31 6D C2 A4 0A FF
Encoded:  Hi\t¤⠛¤[31m¤⣂⢤¤\n¤⣿¤
```

## 11. Backend Conformance Model

This encoding specifies a transport layer. It explicitly does not specify how backing filesystems store the underlying bytes. The relationship between this encoding and a filesystem backend is governed by the following conformance model.

**Conformant backend:** A filesystem that, for every byte sequence accepted by this encoding, stores and retrieves the identical byte sequence. The round-trip identity property is preserved end-to-end.

**Lossy backend:** A filesystem that alters byte sequences during storage or retrieval (through Unicode normalization, case folding, character substitution, or restriction). When a lossy backend is detected, the encoding layer must surface an error to the caller indicating that the backend could not fulfill the transport contract. The encoding layer must never silently absorb data loss.

This model places the responsibility for data integrity with the backend, not the encoding. The encoding layer's obligation is to faithfully represent the caller's intent and to report truthfully when that intent cannot be fulfilled.

## 12. Implementation Notes

### 12.1. Parser State Machine

An implementation requires a two-state machine: *text mode* and *byte mode*. In text mode, the parser emits Tier 1 and Tier 2 output. Upon encountering ¤, it transitions to byte mode. In byte mode, each Braille Pattern character is mapped to a byte value by subtracting U+2800. Upon encountering a second ¤, the parser returns to text mode. No lookahead beyond one code point is required. No stack, backtracking, or nesting is involved.

### 12.2. Canonical Encoding Priority

When encoding, the tier priority order is: Tier 1 > Tier 2 > Tier 3. A byte eligible for a higher-priority tier must use that tier. This ensures that encode() produces a unique, canonical representation for every input.

### 12.3. Comparison and Identity

Two encoded strings are equal if and only if their decoded byte sequences are equal. Implementations should provide a comparison function that operates on decoded byte sequences to avoid false negatives from non-canonical encodings.

### 12.4. Display and Logging

When displaying encoded filenames in error messages, logs, or debug output, the encoded form is the display form. No further escaping is needed. The three-tier structure ensures that the encoded representation is always valid UTF-8 and safe for inclusion in any text-based output channel.
