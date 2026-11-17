# Scan Package

Internal package for recursive filesystem scanning. Produces c4m manifests
from directory trees with optional C4 ID computation and sequence detection.

Exclusion is supported via glob patterns (`--exclude`), explicit exclude
files (`--exclude-file`), and an env-named exclude file (`C4_EXCLUDE_FILE`).

Not imported by any package on this line; `c4 id` scans with the top-level `scan` package.
