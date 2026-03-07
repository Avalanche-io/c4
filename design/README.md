# Design Documents

## Active Specs

These are the authoritative design documents for the current system.

| Document | Scope |
|----------|-------|
| [cli_v1.md](cli_v1.md) | CLI command vocabulary, flags, pathspec grammar, all subcommands |
| [unified_cli.md](unified_cli.md) | c4/c4d architecture split, colon syntax, locations, mesh model |
| [c4d_api_v1.md](c4d_api_v1.md) | c4d HTTP API, namespace routing, content store interface |
| [retention.md](retention.md) | Content retention model, GC, tombstones, safety mechanisms |

## Format and Identity

| Document | Scope |
|----------|-------|
| [c4m_range_format.md](c4m_range_format.md) | Sequence/range notation, `@data` blocks |
| [expand_range_folding.md](expand_range_folding.md) | `@expand` directive, folding control, identity impact |
| [by_note_identity.md](by_note_identity.md) | `@by`/`@note` do not affect C4 IDs |
| [c4m_metadata_extension.md](c4m_metadata_extension.md) | Why production metadata stays external to c4m |
| [c4_package_format.md](c4_package_format.md) | Encrypted delivery analysis (conclusion: trust relays) |

## Architecture and Philosophy

| Document | Scope |
|----------|-------|
| [push_intent_pull_content.md](push_intent_pull_content.md) | Core transfer model: push intent, pull data |
| [reframing_file_transfer.md](reframing_file_transfer.md) | Formal argument: manifests as preprocessing |
| [mhl_vs_c4m_comparison.md](mhl_vs_c4m_comparison.md) | Feature comparison with ASC MHL |

## Future Work

| Document | Scope |
|----------|-------|
| [c4m_language_server.md](c4m_language_server.md) | LSP design for c4m files |

## Explored Ideas

Short notes on concepts investigated but not yet implemented.
See [explored/](explored/) for details.

- [ID Caching](explored/id_caching.md) — skip re-hashing unchanged files
- [Delta Sync](explored/delta_sync.md) — FastCDC chunking for efficient transfer
- [Incremental Scanning](explored/incremental_scanning.md) — priority-driven progressive scanning
- [OpenAssetIO Integration](explored/openassetio_integration.md) — M&E pipeline integration

## Archived

Superseded designs preserved for reference: [archived/](archived/)
