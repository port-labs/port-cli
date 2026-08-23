# Port CLI Release Checklist

This document outlines the process for creating a new release of Port CLI.

## Pre-Release Checklist

- [ ] All tests pass (`make -f Makefile.go test`)
- [ ] Code is formatted (`make -f Makefile.go format`)
- [ ] Linter passes (`make -f Makefile.go lint`)
- [ ] Binary builds successfully (`make -f Makefile.go build-release`)
- [ ] Binary is tested (`scripts/test-binary.sh bin/port`)
- [ ] CHANGELOG.md is updated with release notes
- [ ] Version number is updated in relevant files
- [ ] Documentation is up to date

## Release Process

### 1. Prepare Release

```bash
# Ensure you're on main branch and up to date
git checkout main
git pull origin main

# Run all quality checks
make -f Makefile.go quality

# Build release binary locally to test
make -f Makefile.go build-release
scripts/test-binary.sh bin/port
```

### 2. Create Tag

```bash
# Create and push tag (replace X.X.X with version)
git tag -a vX.X.X -m "Release vX.X.X"
git push origin vX.X.X
```

### 3. GitHub Actions Release

The release workflow will automatically:

1. Trigger on tag push (`v*`)
2. Run tests
3. Build binaries for all platforms using Goreleaser
4. Create GitHub release with:
   - Release notes from CHANGELOG.md
   - Binaries for all platforms
   - Checksums file

### 4. Verify Release

- [ ] GitHub release is created successfully
- [ ] All platform binaries are present
- [ ] Checksums file is included
- [ ] Release notes are correct
- [ ] Binary downloads work

### 5. Test Installation

Test installation on different platforms:

```bash
# macOS
curl -fsSL https://raw.githubusercontent.com/port-labs/port-cli/main/scripts/install.sh | bash

# Linux
curl -fsSL https://raw.githubusercontent.com/port-labs/port-cli/main/scripts/install.sh | bash
```

### 6. Post-Release

- [ ] Announce release (if applicable)
- [ ] Update any external documentation
- [ ] Monitor for issues

## Manual Release (Alternative)

If you need to create a release manually:

```bash
# Install Goreleaser
go install github.com/goreleaser/goreleaser@latest

# Create release (dry-run first)
goreleaser release --snapshot --skip-publish

# Create actual release
GITHUB_TOKEN=your_token goreleaser release --clean
```

## Version Numbering

Follow [Semantic Versioning](https://semver.org/):

- **MAJOR**: Breaking changes
- **MINOR**: New features (backward compatible)
- **PATCH**: Bug fixes (backward compatible)

Examples:
- `v1.0.0` - Initial release
- `v1.1.0` - New features added
- `v1.1.1` - Bug fixes
- `v2.0.0` - Breaking changes

# Port CLI Release Checklist

This document outlines the process for creating a new release of Port CLI.

## Pre-Release Checklist

- [ ] All tests pass (`make test`)
- [ ] Code is formatted (`make format`)
- [ ] Linter passes (`make lint`)
- [ ] Binary builds successfully (`make build-release`)
- [ ] Binary is tested (`scripts/test-binary.sh bin/port`)
- [ ] CHANGELOG.md is updated with release notes
- [ ] Version number is updated in relevant files
- [ ] Documentation is up to date (README.md, INSTALL.md, CONTRIBUTING.md)
- [ ] New features are documented
- [ ] Breaking changes are clearly documented

## Release Process

### 1. Prepare Release

```bash
# Ensure you're on main branch and up to date
git checkout main
git pull origin main

# Run all quality checks
make quality

# Build release binary locally to test
make build-release
scripts/test-binary.sh bin/port

# Test key functionality
./bin/port version
./bin/port --help
./bin/port export --help
./bin/port import --help
./bin/port migrate --help
```

### 2. Create Tag

```bash
# Create and push tag (replace X.X.X with version)
git tag -a vX.X.X -m "Release vX.X.X"
git push origin vX.X.X
```

### 3. GitHub Actions Release

The release workflow will automatically:

1. Trigger on tag push (`v*`)
2. Run tests
3. Build binaries for all platforms using Goreleaser:
   - Linux (amd64, arm64)
   - macOS (amd64, arm64)
   - Windows (amd64)
4. Create GitHub release with:
   - Release notes from CHANGELOG.md
   - Binaries for all platforms
   - Checksums file (`checksums.txt`)

### 4. Verify Release

- [ ] GitHub release is created successfully
- [ ] All platform binaries are present:
  - [ ] `port-linux-amd64`
  - [ ] `port-linux-arm64`
  - [ ] `port-darwin-amd64`
  - [ ] `port-darwin-arm64`
  - [ ] `port-windows-amd64.exe`
- [ ] Checksums file is included
- [ ] Release notes are correct
- [ ] Binary downloads work

### 5. Test Installation

Test installation on different platforms:

```bash
# macOS
curl -fsSL https://raw.githubusercontent.com/port-labs/port-cli/main/scripts/install.sh | bash

# Linux
curl -fsSL https://raw.githubusercontent.com/port-labs/port-cli/main/scripts/install.sh | bash
```

### 6. Post-Release

- [ ] Announce release (if applicable)
- [ ] Update any external documentation
- [ ] Monitor for issues
- [ ] Update release notes if needed

## Manual Release (Alternative)

If you need to create a release manually:

```bash
# Install Goreleaser
go install github.com/goreleaser/goreleaser@latest

# Create release (dry-run first)
goreleaser release --snapshot --skip-publish

# Create actual release
GITHUB_TOKEN=your_token goreleaser release --clean
```

## Version Numbering

Follow [Semantic Versioning](https://semver.org/):

- **MAJOR**: Breaking changes (e.g., removed flags, changed command behavior)
- **MINOR**: New features (backward compatible, e.g., new flags, new resources)
- **PATCH**: Bug fixes (backward compatible, e.g., bug fixes, performance improvements)

Examples:
- `v1.0.0` - Initial release
- `v1.1.0` - New features added (e.g., dual credentials support)
- `v1.1.1` - Bug fixes
- `v2.0.0` - Breaking changes (e.g., removed deprecated flags)

## Testing Before Release

### Smoke Tests

```bash
# Build release binary
make build-release

# Test basic functionality
./bin/port version
./bin/port --help
./bin/port config --init
./bin/port config --show

# Test export (dry run)
./bin/port export --output test.tar.gz --dry-run || echo "Expected: need credentials"

# Test import (dry run)
./bin/port import --input test.tar.gz --dry-run || echo "Expected: need credentials"

# Test migrate (dry run)
./bin/port migrate --source-org test --target-org test --dry-run || echo "Expected: need credentials"
```

### Feature Tests

- [ ] Dual credentials work correctly
- [ ] Diff validation works correctly
- [ ] All resource types are handled
- [ ] Options (--skip-entities, --include) work correctly
- [ ] Dry run provides accurate predictions

## Troubleshooting

### Release fails in GitHub Actions

1. Check Actions logs for errors
2. Verify GITHUB_TOKEN has correct permissions (releases scope)
3. Ensure tag format is correct (`vX.X.X`)
4. Verify `.goreleaser.yml` is valid

### Binary not building

1. Ensure Go version is 1.21+
2. Check for CGO dependencies (should be disabled: `CGO_ENABLED=0`)
3. Verify all imports are available
4. Check for missing dependencies in `go.mod`

### Checksums missing

1. Check `.goreleaser.yml` configuration
2. Verify checksum generation is enabled in the `archives` section
3. Ensure checksums target is configured

### Build size too large

1. Verify `-s -w` flags are in ldflags (strip symbols)
2. Ensure `CGO_ENABLED=0` is set
3. Check for unnecessary dependencies

## Release Notes Template

When updating CHANGELOG.md, use this format:

```markdown
## [X.X.X] - YYYY-MM-DD

### Added
- New feature 1
- New feature 2
- Support for dual organization credentials
- Diff validation for import/migrate commands

### Changed
- Improved performance (X% faster)
- Updated dependencies
- Enhanced error messages

### Fixed
- Bug fix 1
- Bug fix 2

### Breaking Changes
- Breaking change description (if any)
  - Migration guide/instructions
```

### Feature Categorization

When documenting features:

- **Added**: New commands, flags, resource types, or capabilities
- **Changed**: Modified behavior (backward compatible improvements)
- **Fixed**: Bug fixes and error corrections
- **Deprecated**: Features that will be removed in future versions
- **Removed**: Removed features (breaking changes)
- **Security**: Security-related fixes

## Changelog Guidelines

- Group related changes together
- Use present tense ("Add feature" not "Added feature")
- Be specific about what changed
- Mention affected commands/flags
- Include examples for complex features
- Link to relevant issues/PRs

## Release Notes Template

When updating CHANGELOG.md, use this format:

```markdown
## [X.X.X] - YYYY-MM-DD

### Added
- New feature 1
- New feature 2

### Changed
- Improved performance
- Updated dependencies

### Fixed
- Bug fix 1
- Bug fix 2

### Breaking Changes
- Breaking change description
```

