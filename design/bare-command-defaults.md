# Bare c4 Command Defaults

## Summary

The bare `c4 <path>` and `echo | c4` forms always identify AND store by default. Add `-x` flag to skip storage.

## Current State

- `c4 <path>` → identify + store (implemented)
- `echo | c4` → identify only (needs store added)
- `-x` flag → not implemented

## Design

| Command | Identifies | Stores | Output |
|---|---|---|---|
| `c4 file.txt` | yes | yes | c4m entry |
| `c4 file.txt -x` | yes | no | c4m entry |
| `echo "data" \| c4` | yes | yes | bare ID |
| `echo "data" \| c4 -x` | yes | no | bare ID |
| `c4 id file.txt` | yes | no | c4m entry (unchanged) |
| `c4 id -s file.txt` | yes | yes | c4m entry (unchanged) |

The bare form is the ergonomic shortcut. `c4 id` keeps its current behavior for backward compatibility.

## Rationale

The most useful default is to store content. You almost always want the content available later. The rare case (just want the ID, don't store) gets the flag.

`-x` for "exclude from store" — short, memorable.

## Status

Pending implementation. Not blocking for announcement.
