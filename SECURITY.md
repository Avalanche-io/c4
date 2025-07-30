# Security Policy

## Supported Versions

The C4 project is currently maintained on the master branch. We provide security updates for:

| Version | Supported          |
| ------- | ------------------ |
| master branch | :white_check_mark: |
| v0.8.1  | :white_check_mark: |
| v0.8.0  | :x:                |
| < v0.8  | :x:                |

**Note**: The `v0.8` tag always points to the latest v0.8.x release.

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
- Affected components (e.g., file walker, storage layer, etc.)
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

C4 is designed for content identification and may operate with elevated privileges when performing system-wide scans. Users should:

- Only run C4 with elevated privileges when necessary
- Be aware that C4 follows symbolic links by default, which could lead to scanning unintended locations or loops in untrusted directory structures
- Keep dependencies up to date

Note: C4 only reads and hashes file content using SHA-512. It does not execute or interpret file contents, making it safe to scan any file regardless of its content.

## Past Security Issues

Security fixes are documented in commit messages and GitHub security advisories.

### v0.8.1 Security Updates
- Updated golang.org/x/crypto to fix CVE-2024-45337, CVE-2022-27191, CVE-2021-43565, CVE-2025-22869, CVE-2023-48795
- Migrated from deprecated github.com/boltdb/bolt to maintained go.etcd.io/bbolt
- Updated bbolt to v1.4.2