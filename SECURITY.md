# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| v1.0.x  | :white_check_mark: |
| < v1.0  | :x:                |

## Reporting a Vulnerability

We take security vulnerabilities seriously. If you discover a security issue in C4, please report it responsibly.

### How to Report

1. **DO NOT** create a public GitHub issue for security vulnerabilities
2. Instead, please report security issues via GitHub's private vulnerability reporting:
   - Go to https://github.com/Avalanche-io/c4/security/advisories
   - Click "Report a vulnerability"
   - Provide a detailed description of the vulnerability
   - Include steps to reproduce if possible

### What to Include

- Type of vulnerability (e.g., path traversal, denial of service, etc.)
- Affected components (e.g., scanner, store, reconciler, etc.)
- Potential impact
- Steps to reproduce
- Suggested fixes (if any)

### Response Timeline

- **Initial Response**: Within 48 hours
- **Status Update**: Within 7 days
- **Resolution Target**: 30 days for critical issues, 90 days for lower severity

### After Reporting

- We will acknowledge receipt of your report
- We will investigate and validate the issue
- We will work on a fix and coordinate disclosure
- Credit will be given to reporters (unless you prefer to remain anonymous)

## Security Considerations

C4 scans filesystem trees and may operate on untrusted directory structures. Users should:

- Only run C4 with elevated privileges when necessary
- Be aware that C4 does **not** follow symbolic links by default — symlink targets are recorded but not traversed
- Understand that `c4 patch` modifies the filesystem — use `--dry-run` to preview changes

C4 only reads and hashes file content using SHA-512. It does not execute or interpret file contents, making it safe to scan any file regardless of its content.

## Past Security Issues

### v1.0.2

- Fixed 12 code quality issues including unchecked error returns, TOCTOU race in content sourcing, and path construction for Windows compatibility
- Fixed timestamp precision mismatch at c4m/filesystem boundary

### v1.0.0

- Zero external dependencies — eliminates supply chain risk
- Removed deprecated packages (id/, db/, util/) that had outdated transitive dependencies
