# C4M Metadata Extension Design

## Design Principles

1. **Backward Compatible**: Old parsers ignore unknown @ directives
2. **Optional**: Metadata is always optional, never required
3. **Minimal**: Stay true to C4M's minimalist philosophy
4. **Structured**: Easy to parse, validate, and extend
5. **Non-Breaking**: Adding metadata never changes manifest C4 ID calculation

## Metadata Directive Format

### Core Syntax
```
@directive-name [arguments...] ["quoted string"]
```

### Rules
1. Metadata lines start with `@` followed by directive name
2. Arguments are space-separated
3. Strings with spaces must be quoted
4. Directives appear after `@c4m` version header, before entries
5. Order matters for some directives (like `@creator` before `@author`)
6. Unknown directives are ignored (forward compatibility)

## Standard Metadata Directives

### 1. Creator Information

#### @creator
Records who/what/when/where the manifest was created.

**Format**:
```
@creator <timestamp> <hostname> <username> <tool> <version>
```

**Example**:
```
@creator 2025-09-20T01:49:47Z mymac.local john c4 1.0.0-beta
```

**Fields**:
- `timestamp`: ISO8601 UTC timestamp (same format as file timestamps)
- `hostname`: Machine hostname (no spaces)
- `username`: OS username (no spaces, or "unknown")
- `tool`: Tool name (no spaces, e.g., "c4", "ascmhl", "custom")
- `version`: Tool version (no spaces)

**Alternative verbose format** (for detailed info):
```
@creator timestamp="2025-09-20T01:49:47Z" hostname="mymac.local" user="john" tool="c4" version="1.0.0-beta"
```

#### @author
Records human author information (production context).

**Format**:
```
@author <name> <email> <phone> <role>
```

**Example**:
```
@author "John Doe" john.doe@example.com "+1-555-0100" DIT
```

**Fields**:
- `name`: Full name (quoted if spaces)
- `email`: Email address (or "-" if unknown)
- `phone`: Phone number (quoted, or "-" if unknown)
- `role`: Job role (DIT, Editor, DataManager, etc., or "-")

#### @location
Records physical location where manifest was created.

**Format**:
```
@location "location description"
```

**Example**:
```
@location "On Set - Stage 5, Pinewood Studios"
@location "Post House - Deluxe Toronto, Suite 3"
@location "Archive - Iron Mountain, Vault A-127"
```

#### @comment
Free-form comment about this manifest/generation.

**Format**:
```
@comment "free form text"
```

**Example**:
```
@comment "Initial ingest from CF card A002"
@comment "Verified after network transfer, 3 failures re-checked"
@comment "Archive snapshot before project delivery"
```

### 2. Process Information

#### @process
Describes what process created this manifest.

**Format**:
```
@process <type> [<parent-id>]
```

**Types**:
- `initial`: First generation, original capture
- `verify`: Verification without changes
- `copy`: Created during copy operation
- `transfer`: Created during network transfer
- `archive`: Created for archival purposes
- `restore`: Created during restore operation
- `inplace`: Created in-place (scanning existing files)
- `update`: Incremental update (@base reference required)

**Example**:
```
@process initial
@process verify c45previousManifest...
@process copy
@process update
```

#### @verify
Records verification results.

**Format**:
```
@verify <timestamp> <status> <files-checked> <files-passed> <files-failed>
```

**Example**:
```
@verify 2025-09-20T02:15:33Z complete 1024 1021 3
@verify 2025-09-20T01:49:47Z partial 512 512 0
```

**Status values**:
- `complete`: All files verified
- `partial`: Some files verified
- `failed`: Verification found errors
- `interrupted`: Verification stopped early

#### @ignore
Records ignore patterns used during scan.

**Format**:
```
@ignore <pattern>
```

**Example**:
```
@ignore .DS_Store
@ignore *.tmp
@ignore thumbs.db
@ignore .c4m_bundle/
```

Multiple `@ignore` directives can be specified.

### 3. Chain/Reference Metadata

#### @base
(Already exists in C4M) References parent manifest for delta updates.

**Format**:
```
@base <c4-id>
```

**Example**:
```
@base c45previousGeneration...
```

#### @previous (NEW)
Explicitly links to previous generation (for non-delta manifests).

**Format**:
```
@previous <c4-id> <filepath>
```

**Example**:
```
@previous c45gen001... /path/to/gen001.c4m
```

Useful for MHL-style full generations that reference previous state.

#### @chain
Records position in chain.

**Format**:
```
@chain <generation-number> <total-generations>
```

**Example**:
```
@chain 3 5
```

Indicates this is generation 3 of 5 total generations.

### 4. Data Integrity Metadata

#### @hashdate
Records when hashes were computed (per-entry granularity optional).

**Format** (global):
```
@hashdate 2025-09-20T01:49:47Z
```

**Per-entry format** (optional, for verification tracking):
```
-rw-r--r-- 2025-09-20T01:49:47Z 1024 file.txt c44aMtvPeo... @hashdate=2025-09-20T02:00:00Z
```

#### @roothash
Records directory hash for entire manifest root.

**Format**:
```
@roothash <c4-id> <type>
```

**Types**:
- `content`: Hash of all file content
- `structure`: Hash of directory structure
- `manifest`: Hash of manifest itself

**Example**:
```
@roothash c46contentHash... content
@roothash c47structureHash... structure
```

### 5. Multi-Hash Extension (Optional)

#### @hashformat
Declares additional hash formats (beyond C4).

**Format**:
```
@hashformat <format-name> <format-name> ...
```

**Example**:
```
@hashformat c4 xxh128 md5
```

When declared, entries can have multiple hashes:
```
-rw-r--r-- 2025-09-20T01:49:47Z 1024 file.txt \
  c4=c44aMtvPeo... \
  xxh128=8d02114c32e28cbe \
  md5=9e107d9d372bb682
```

**Note**: C4 is always the primary/canonical hash. Others are supplementary.

## Complete Example: Production Manifest

```
@c4m 1.0

# Creator information
@creator 2025-09-20T01:49:47Z mymac.local john c4 1.0.0-beta
@author "John Doe" john.doe@example.com "+1-555-0100" DIT
@location "On Set - Stage 5, Pinewood Studios"
@comment "Initial ingest from Camera A, CF card A002"

# Process information
@process initial
@hashdate 2025-09-20T01:49:47Z

# Ignore patterns (if any)
@ignore .DS_Store
@ignore *.tmp

# Root integrity
@roothash c46rootContent123... content
@roothash c47rootStructure456... structure

# Entries
drwxr-xr-x 2025-09-20T01:45:00Z 1024 Clips/
  -rw-r--r-- 2025-09-20T01:45:12Z 512000000 A002C006_141024_R2EC.mov c44aMtvPeo...
  -rw-r--r-- 2025-09-20T01:46:33Z 487000000 A002C007_141024_R2EC.mov c44bNuwQfp...
-rw-r--r-- 2025-09-20T01:44:00Z 58 Sidecar.txt c43xYzAbCd...
```

## Complete Example: Verification Update

```
@c4m 1.0
@base c45initialManifest...

# Creator information
@creator 2025-09-21T14:30:00Z fileserver.local backupadmin c4 1.0.0-beta
@author "Jane Smith" jane.smith@example.com "+1-555-0200" "Data Manager"
@location "Post House - Deluxe Toronto"
@comment "Verification after network transfer from set"

# Process information
@process verify c45initialManifest...
@verify 2025-09-21T14:30:00Z complete 3 3 0
@hashdate 2025-09-21T14:30:00Z

# Chain information
@previous c45initialManifest... /mnt/archive/ingests/initial.c4m
@chain 2 2

# Only changed/new files (delta mode with @base)
# In this case, no changes, so empty entry list
# (Verification passed, no files modified)
```

## Complete Example: Multi-Hash for Legacy Systems

```
@c4m 1.0
@hashformat c4 xxh128 md5

# Creator information
@creator 2025-09-20T01:49:47Z mymac.local john c4 1.0.0-beta
@comment "Multi-hash manifest for legacy system compatibility"

# Entries with multiple hashes
-rw-r--r-- 2025-09-20T01:49:47Z 1024 file1.txt \
  c4=c44aMtvPeo123... \
  xxh128=8d02114c32e28cbe \
  md5=9e107d9d372bb682

-rw-r--r-- 2025-09-20T01:50:12Z 2048 file2.txt \
  c4=c44bNuwQfp456... \
  xxh128=7c01003b21d17bad \
  md5=8f107e8e261a95c3
```

## Parsing Strategy

### Parser Compatibility Levels

**Level 0: Basic Parser** (current C4M)
- Reads `@c4m` version
- Skips all unknown `@` directives
- Parses entry lines
- Result: Works with metadata-enhanced manifests

**Level 1: Metadata-Aware Parser**
- Reads `@c4m` version
- Parses known metadata directives
- Stores metadata for access
- Parses entry lines
- Result: Can extract and use metadata

**Level 2: Metadata-Validating Parser**
- All Level 1 features
- Validates metadata format
- Checks required fields
- Reports metadata errors
- Result: Strict metadata compliance

### Implementation Example

```go
type ManifestMetadata struct {
    // Creator info
    CreatorTime     time.Time
    Hostname        string
    Username        string
    Tool            string
    ToolVersion     string

    // Author info (optional)
    AuthorName      string
    AuthorEmail     string
    AuthorPhone     string
    AuthorRole      string

    // Context
    Location        string
    Comment         string

    // Process
    ProcessType     string
    ProcessParentID *c4.ID

    // Verification (optional)
    VerifyTime      time.Time
    VerifyStatus    string
    VerifyChecked   int
    VerifyPassed    int
    VerifyFailed    int

    // Ignore patterns
    IgnorePatterns  []string

    // Chain info
    ChainGeneration int
    ChainTotal      int

    // Root hashes
    RootContentHash *c4.ID
    RootStructHash  *c4.ID
}

// Parser with metadata support
type MetadataParser struct {
    *Parser  // Embed existing parser
    Metadata ManifestMetadata
}

func (p *MetadataParser) ParseMetadataLine(line string) error {
    // Skip non-metadata lines
    if !strings.HasPrefix(line, "@") {
        return nil
    }

    parts := strings.Fields(line)
    if len(parts) == 0 {
        return nil
    }

    directive := strings.TrimPrefix(parts[0], "@")
    args := parts[1:]

    switch directive {
    case "creator":
        if len(args) >= 5 {
            p.Metadata.CreatorTime, _ = time.Parse(time.RFC3339, args[0])
            p.Metadata.Hostname = args[1]
            p.Metadata.Username = args[2]
            p.Metadata.Tool = args[3]
            p.Metadata.ToolVersion = args[4]
        }
    case "author":
        if len(args) >= 4 {
            p.Metadata.AuthorName = strings.Trim(args[0], `"`)
            p.Metadata.AuthorEmail = args[1]
            p.Metadata.AuthorPhone = strings.Trim(args[2], `"`)
            p.Metadata.AuthorRole = args[3]
        }
    case "location":
        p.Metadata.Location = strings.Trim(strings.Join(args, " "), `"`)
    case "comment":
        p.Metadata.Comment = strings.Trim(strings.Join(args, " "), `"`)
    case "process":
        if len(args) >= 1 {
            p.Metadata.ProcessType = args[0]
            if len(args) >= 2 {
                id, _ := c4.Parse(args[1])
                p.Metadata.ProcessParentID = &id
            }
        }
    case "ignore":
        if len(args) >= 1 {
            p.Metadata.IgnorePatterns = append(p.Metadata.IgnorePatterns, args[0])
        }
    // ... handle other directives
    default:
        // Unknown directive - ignore for forward compatibility
    }

    return nil
}
```

## Metadata Sidecar Option

Alternative approach: Keep metadata in separate file.

**Files**:
```
scan.c4m           # Pure C4M manifest
scan.c4m.meta      # Metadata sidecar
```

**scan.c4m** (unchanged):
```
@c4m 1.0
-rw-r--r-- 2025-09-20T01:49:47Z 1024 file.txt c44aMtvPeo...
```

**scan.c4m.meta**:
```
@c4m-metadata 1.0
@manifest c46scanManifest...
@creator 2025-09-20T01:49:47Z mymac.local john c4 1.0.0-beta
@author "John Doe" john.doe@example.com "+1-555-0100" DIT
@location "On Set"
@comment "Initial ingest"
```

**Advantages**:
- Manifest C4 ID never changes due to metadata
- Pure C4M remains minimal
- Metadata is truly optional (separate file)

**Disadvantages**:
- Two files to manage
- Can lose metadata file
- Less integrated

**Recommendation**: Inline metadata preferred, but sidecar option available.

## MHL Export Tool Design

### Tool: `c4-mhl`

Converts C4M manifests to MHL XML format.

### Command: `c4-mhl export`

```bash
# Export single manifest
c4-mhl export scan.c4m output.mhl

# Export bundle (all generations)
c4-mhl export bundle.c4m_bundle/ output_dir/

# Export with options
c4-mhl export --generation 1 \
              --hash-formats c4,xxh64,md5 \
              --creator-info \
              scan.c4m output.mhl
```

### Options

```
--generation <n>           Export specific generation (for bundles)
--all-generations          Export all generations (creates multiple .mhl files)
--hash-formats <list>      Include hash formats (c4, xxh64, md5, sha1, xxh128)
--creator-info             Include creator/author metadata (from @creator, @author)
--ignore-patterns          Include ignore patterns (from @ignore)
--root-hash                Include root directory hash
--chain-file               Generate ascmhl_chain.xml
--output-dir <path>        Output directory (for multiple generations)
```

### Export Logic

#### Single Manifest Export

**Input** (scan.c4m):
```
@c4m 1.0
@creator 2025-09-20T01:49:47Z mymac.local john c4 1.0.0-beta
@author "John Doe" john.doe@example.com "+1-555-0100" DIT
@location "On Set"
@comment "Initial ingest"
@ignore .DS_Store
@roothash c46rootContent... content

-rw-r--r-- 2025-09-20T01:49:47Z 1024 file.txt c44aMtvPeo...
drwxr-xr-x 2025-09-20T01:49:00Z 2048 mydir/
  -rw-r--r-- 2025-09-20T01:49:30Z 1024 subfile.txt c44bNuwQfp...
```

**Output** (scan.mhl):
```xml
<?xml version="1.0" encoding="UTF-8"?>
<hashlist version="2.0" xmlns="urn:ASC:MHL:v2.0">
  <creatorinfo>
    <creationdate>2025-09-20T01:49:47+00:00</creationdate>
    <hostname>mymac.local</hostname>
    <tool version="1.0.0-beta">c4</tool>
    <author>
      <name>John Doe</name>
      <email>john.doe@example.com</email>
      <phone>+1-555-0100</phone>
      <role>DIT</role>
    </author>
    <location>On Set</location>
    <comment>Initial ingest</comment>
  </creatorinfo>
  <processinfo>
    <process>initial</process>
    <roothash>
      <content>
        <c4 hashdate="2025-09-20T01:49:47+00:00">c46rootContent...</c4>
      </content>
    </roothash>
    <ignore>
      <pattern>.DS_Store</pattern>
    </ignore>
  </processinfo>
  <hashes>
    <hash>
      <path size="1024" lastmodificationdate="2025-09-20T01:49:47+00:00">file.txt</path>
      <c4 action="original" hashdate="2025-09-20T01:49:47+00:00">c44aMtvPeo...</c4>
    </hash>
    <directoryhash>
      <path lastmodificationdate="2025-09-20T01:49:00+00:00">mydir</path>
      <content>
        <c4 hashdate="2025-09-20T01:49:00+00:00">c46dirContent...</c4>
      </content>
    </directoryhash>
    <hash>
      <path size="1024" lastmodificationdate="2025-09-20T01:49:30+00:00">mydir/subfile.txt</path>
      <c4 action="original" hashdate="2025-09-20T01:49:47+00:00">c44bNuwQfp...</c4>
    </hash>
  </hashes>
</hashlist>
```

#### Bundle Export (Multiple Generations)

**Input**: `bundle.c4m_bundle/` with 3 generations

**Output Structure**:
```
output_dir/
├── ascmhl/
│   ├── 0001_bundle_2025-09-20_014947Z.mhl
│   ├── 0002_bundle_2025-09-21_143000Z.mhl
│   ├── 0003_bundle_2025-09-22_100000Z.mhl
│   └── ascmhl_chain.xml
```

**ascmhl_chain.xml**:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<ascmhldirectory xmlns="urn:ASC:MHL:DIRECTORY:v2.0">
  <hashlist sequencenr="1">
    <path>0001_bundle_2025-09-20_014947Z.mhl</path>
    <c4>c46generation1ManifestC4...</c4>
  </hashlist>
  <hashlist sequencenr="2">
    <path>0002_bundle_2025-09-21_143000Z.mhl</path>
    <c4>c47generation2ManifestC4...</c4>
  </hashlist>
  <hashlist sequencenr="3">
    <path>0003_bundle_2025-09-22_100000Z.mhl</path>
    <c4>c48generation3ManifestC4...</c4>
  </hashlist>
</ascmhldirectory>
```

### Multi-Hash Export

If C4M manifest has multi-hash entries:

**Input**:
```
@c4m 1.0
@hashformat c4 xxh64 md5

-rw-r--r-- 2025-09-20T01:49:47Z 1024 file.txt \
  c4=c44aMtvPeo... \
  xxh64=0ea03b369a463d9d \
  md5=9e107d9d372bb682
```

**Output**:
```xml
<hash>
  <path size="1024" lastmodificationdate="2025-09-20T01:49:47+00:00">file.txt</path>
  <c4 action="original" hashdate="2025-09-20T01:49:47+00:00">c44aMtvPeo...</c4>
  <xxh64 action="original" hashdate="2025-09-20T01:49:47+00:00">0ea03b369a463d9d</xxh64>
  <md5 action="original" hashdate="2025-09-20T01:49:47+00:00">9e107d9d372bb682</md5>
</hash>
```

### Implementation Outline

```go
package main

type MHLExporter struct {
    manifest    *c4m.Manifest
    metadata    *c4m.ManifestMetadata
    options     ExportOptions
}

type ExportOptions struct {
    HashFormats    []string  // c4, xxh64, md5, sha1, xxh128
    IncludeCreator bool
    IncludeIgnore  bool
    IncludeRootHash bool
    Generation     int       // Which generation (for bundles)
    AllGenerations bool      // Export all (for bundles)
}

func (e *MHLExporter) Export(outputPath string) error {
    // Create XML structure
    mhl := &MHLHashList{
        Version: "2.0",
        Xmlns: "urn:ASC:MHL:v2.0",
    }

    // Add creator info from metadata
    if e.options.IncludeCreator && e.metadata != nil {
        mhl.CreatorInfo = e.buildCreatorInfo()
    }

    // Add process info
    mhl.ProcessInfo = e.buildProcessInfo()

    // Convert entries
    for _, entry := range e.manifest.Entries {
        if entry.IsDir() {
            // Directory hash
            dirHash := e.buildDirectoryHash(entry)
            mhl.Hashes = append(mhl.Hashes, dirHash)
        } else {
            // File hash
            fileHash := e.buildFileHash(entry)
            mhl.Hashes = append(mhl.Hashes, fileHash)
        }
    }

    // Write XML
    return e.writeXML(mhl, outputPath)
}

func (e *MHLExporter) buildCreatorInfo() *MHLCreatorInfo {
    ci := &MHLCreatorInfo{
        CreationDate: e.metadata.CreatorTime.Format(time.RFC3339),
        Hostname: e.metadata.Hostname,
        Tool: MHLTool{
            Name: e.metadata.Tool,
            Version: e.metadata.ToolVersion,
        },
    }

    if e.metadata.AuthorName != "" {
        ci.Author = &MHLAuthor{
            Name: e.metadata.AuthorName,
            Email: e.metadata.AuthorEmail,
            Phone: e.metadata.AuthorPhone,
            Role: e.metadata.AuthorRole,
        }
    }

    if e.metadata.Location != "" {
        ci.Location = e.metadata.Location
    }

    if e.metadata.Comment != "" {
        ci.Comment = e.metadata.Comment
    }

    return ci
}

func (e *MHLExporter) buildFileHash(entry *c4m.Entry) *MHLHash {
    hash := &MHLHash{
        Path: MHLPath{
            Value: entry.Name,
            Size: entry.Size,
            LastModificationDate: entry.Timestamp.Format(time.RFC3339),
        },
    }

    // Add C4 hash (always present)
    hash.C4 = &MHLHashValue{
        Value: entry.C4ID.String(),
        Action: "original",
        HashDate: entry.Timestamp.Format(time.RFC3339),
    }

    // Add additional hash formats if present
    // (from multi-hash extension)

    return hash
}

// Command-line tool
func main() {
    app := &cli.App{
        Name: "c4-mhl",
        Usage: "Convert C4M manifests to MHL format",
        Commands: []*cli.Command{
            {
                Name: "export",
                Usage: "Export C4M to MHL",
                Flags: []cli.Flag{
                    &cli.StringSliceFlag{
                        Name: "hash-formats",
                        Value: cli.NewStringSlice("c4"),
                        Usage: "Hash formats to include",
                    },
                    &cli.BoolFlag{
                        Name: "creator-info",
                        Value: true,
                        Usage: "Include creator information",
                    },
                    &cli.IntFlag{
                        Name: "generation",
                        Value: -1,
                        Usage: "Specific generation (for bundles)",
                    },
                    &cli.BoolFlag{
                        Name: "all-generations",
                        Value: false,
                        Usage: "Export all generations",
                    },
                    &cli.BoolFlag{
                        Name: "chain-file",
                        Value: true,
                        Usage: "Generate chain file",
                    },
                },
                Action: exportCommand,
            },
        },
    }

    app.Run(os.Args)
}
```

## Reverse Direction: MHL Import

### Command: `c4-mhl import`

```bash
# Import single MHL file
c4-mhl import scan.mhl output.c4m

# Import MHL directory (all generations)
c4-mhl import ascmhl/ output.c4m_bundle/

# Import with metadata preservation
c4-mhl import --preserve-metadata scan.mhl output.c4m
```

### Import Logic

**Input** (scan.mhl):
```xml
<hashlist version="2.0">
  <creatorinfo>
    <creationdate>2025-09-20T01:49:47+00:00</creationdate>
    <hostname>mymac.local</hostname>
    <tool version="0.3">ascmhl.py</tool>
  </creatorinfo>
  <hashes>
    <hash>
      <path size="1024">file.txt</path>
      <c4>c44aMtvPeo...</c4>
    </hash>
  </hashes>
</hashlist>
```

**Output** (scan.c4m):
```
@c4m 1.0
@creator 2025-09-20T01:49:47Z mymac.local unknown ascmhl.py 0.3
@comment "Imported from MHL"

-rw-r--r-- 2025-09-20T01:49:47Z 1024 file.txt c44aMtvPeo...
```

## Practical Usage Scenarios

### Scenario 1: On-Set Ingest with Metadata

```bash
# Camera operator creates manifest with metadata
c4 -mr /Volumes/CFCard \
    --creator "John Doe" john@example.com "+1-555-0100" DIT \
    --location "On Set - Stage 5" \
    --comment "Camera A, CF card A002" \
    > ingest_001.c4m

# Later, data manager wants MHL for post house
c4-mhl export ingest_001.c4m posthou se/ascmhl/0001_ingest_2025-09-20.mhl
```

### Scenario 2: Post House Receives C4M, Needs MHL

```bash
# Post house receives: production.c4m_bundle/
# They need MHL for their workflow

c4-mhl export --all-generations \
              --chain-file \
              production.c4m_bundle/ \
              /mnt/project/ascmhl/

# Result: /mnt/project/ascmhl/ now has full MHL history
# Their existing MHL tools work perfectly
```

### Scenario 3: Archive with Both Formats

```bash
# Create C4M bundle (efficient, primary)
c4 --bundle /mnt/archive/project/media

# Also generate MHL (compatibility)
c4-mhl export /mnt/archive/project/media/*.c4m_bundle/ \
              /mnt/archive/project/ascmhl/

# Now have both:
# - C4M bundle for deduplication, boolean ops, efficiency
# - MHL for legacy tool compatibility
```

## Integration with Main C4 Tool

### Add Metadata Flags to `c4` command

```bash
c4 -mr /path \
   --creator-name "John Doe" \
   --creator-email john@example.com \
   --creator-phone "+1-555-0100" \
   --creator-role DIT \
   --location "On Set" \
   --comment "Initial ingest" \
   > output.c4m
```

Generates:
```
@c4m 1.0
@creator 2025-09-20T01:49:47Z $(hostname) $(whoami) c4 1.0.0-beta
@author "John Doe" john@example.com "+1-555-0100" DIT
@location "On Set"
@comment "Initial ingest"
...
```

## Conclusion

**Metadata Extension**:
- ✅ Backward compatible (old parsers ignore)
- ✅ Optional (never required)
- ✅ Minimal (stay true to C4M philosophy)
- ✅ Structured (easy to parse and extend)
- ✅ Practical (covers MHL use cases)

**MHL Bridge**:
- ✅ Export C4M → MHL (full feature parity)
- ✅ Import MHL → C4M (preserve metadata)
- ✅ Bundle support (multiple generations)
- ✅ Chain file generation
- ✅ Multi-hash support

**Strategic Value**:
- C4M remains minimal and efficient
- Optional metadata for production workflows
- Full MHL interoperability
- C4M as primary, MHL as export format
- Best of both worlds
