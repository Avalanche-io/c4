# Contributing to C4

Thank you for your interest in contributing to the C4 reference implementation! This document provides guidelines and information for contributors.

## Project Structure

The C4 project consists of:
- **Core library**: The main C4 ID implementation
- **C4M package**: Manifest format for filesystem tracking
- **Command-line tool**: The `c4` executable

## Development Workflow

### Branches

- `master` - Current stable release
- `dev` - Development branch for upcoming releases
- Feature branches - Created from `dev` for new features
- Bug branches - Created from `dev` for bug fixes

### Creating a Branch

Feature and bug branches should follow the GitHub integrated naming convention:

```bash
# For new features
git checkout dev
git checkout -b new/#99_description_of_feature

# For bug fixes
git checkout dev
git checkout -b bug/#88_description_of_bug
```

If a branch for an issue already exists, check it out and work from it.

### Pull Requests

1. Create your branch from `dev`
2. Make your changes with clear, concise commits
3. Add tests for new functionality
4. Ensure all tests pass: `go test ./...`
5. Update documentation as needed
6. Submit PR against the `dev` branch

## Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Keep functions focused and well-documented
- Add comments for exported functions and types
- Avoid adding comments to code unless specifically requested

## Testing

- Write tests for new functionality
- Maintain or improve code coverage
- Run tests before submitting PRs: `go test ./...`
- Check coverage: `go test -cover ./...`

## Documentation

- Update README.md for user-facing changes
- Document new features in the c4m package
- Keep examples current and working
- Use clear, concise language

## C4 Tools Ecosystem

### Creating C4 Extension Tools

The C4 ecosystem supports git-style extensibility. Any executable named `c4-*` in your PATH becomes available as a c4 subcommand.

#### How It Works

```bash
# An executable named: c4-verify
# Can be called as:    c4 verify [args]
```

#### Creating Your Own C4 Tool

1. **Name your tool**: Use the pattern `c4-<command>`
2. **Any language**: Write in Go, Python, Shell, Rust, etc.
3. **Install to PATH**: Make it executable and accessible
4. **Integration**: Automatically available as `c4 <command>`

#### Example Tool Structure

```bash
#!/usr/bin/env python3
# c4-backup - Backup tool with C4 verification

import sys
import subprocess

def main():
    # Tool implementation
    pass

if __name__ == "__main__":
    main()
```

#### Tool Guidelines

- **Unix philosophy**: Do one thing well
- **Composability**: Work with pipes and standard tools
- **C4M format**: Use C4M for manifest input/output when appropriate
- **Exit codes**: Use standard exit codes (0 for success)
- **Help text**: Provide `-h/--help` documentation

#### Proposed Tools

The core `c4` CLI now includes `cp`, `mv`, `rm`, `diff`, and `patch` as built-in subcommands. Extension tools can add complementary functionality:
- `c4-verify` - Verify against c4m files
- `c4-backup` - Backup workflows
- `c4-watch` - Monitor filesystem changes

### Contributing Tools

You can contribute C4 tools in several ways:

1. **Separate repository**: Create your own repo for your tool
2. **c4-tools repository**: Contribute to a future community tools collection
3. **Share and announce**: Let the community know about your tool

### Tool Development Tips

- Start with a simple prototype
- Get community feedback early
- Consider existing Unix tool conventions
- Test with real-world use cases
- Document usage examples

## Community

### Getting Help

- Open an issue for bugs or questions
- Check existing issues before creating new ones
- Provide clear reproduction steps for bugs
- Include version information and environment details

### Feature Requests

- Open an issue describing the use case
- Explain why existing functionality doesn't meet the need
- Be open to alternative approaches
- Consider implementing it as an external tool first

### Code of Conduct

Please read our [Code of Conduct](CODE_OF_CONDUCT.md) before participating.

## Release Process

1. Features are developed in feature branches
2. Merged to `dev` after review
3. Periodically `dev` is merged to `master` for release
4. Releases are tagged with semantic versioning

## License

By contributing to C4, you agree that your contributions will be licensed under the MIT License. See [LICENSE](LICENSE) for details.

## Questions?

If you have questions about contributing, please open an issue and we'll be happy to help!