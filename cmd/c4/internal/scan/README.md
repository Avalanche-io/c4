# Scan Package

Internal package for recursive filesystem scanning. Produces c4m manifests
from directory trees with optional C4 ID computation and sequence detection.

Exclusion is supported via glob patterns (`--exclude`), explicit exclude
files (`--exclude-file`), and an env-named exclude file (`C4_EXCLUDE_FILE`).

Used by the `c4 id` command for directory scanning.
