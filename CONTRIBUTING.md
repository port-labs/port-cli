# Contributing to Port CLI

Thank you for your interest in contributing to Port CLI! This document provides guidelines and instructions for contributing.

## Development Setup

### Prerequisites

- **Go 1.25.9+**: Required for the Go CLI
- **Make**: For build automation

### Getting Started

1. **Clone the repository**

```bash
git clone https://github.com/port-labs/port-cli.git
cd port-cli
```

2. **Set up the Go CLI**

```bash
go mod download
make build
./bin/port --help
```

3. **Run tests**

```bash
make test
```

## Project Structure

```
port-cli/
├── cmd/port/              # Go CLI entry point
│   └── main.go           # Main application
├── internal/              # Go implementation
│   ├── api/              # Port API client
│   │   ├── client.go     # HTTP client with auth & retry
│   │   └── requests.go   # API endpoint methods
│   ├── commands/         # CLI commands
│   │   ├── export.go     # Export command
│   │   ├── import.go     # Import command
│   │   ├── migrate.go    # Migrate command
│   │   ├── api.go        # API command
│   │   ├── config.go     # Config command
│   │   ├── version.go    # Version command
│   │   └── context.go    # Global flags context
│   ├── config/           # Configuration management
│   │   ├── config.go     # Config structures
│   │   └── loader.go     # Config loading with precedence
│   └── modules/          # Business logic modules
│       ├── export/       # Export module
│       │   ├── export.go # Export orchestration
│       │   └── collector.go # Data collection
│       ├── import_module/ # Import module
│       │   ├── import.go  # Import orchestration
│       │   ├── loader.go  # Data loading
│       │   ├── importer.go # Data importing
│       │   └── diff.go    # Diff validation
│       └── migrate/       # Migration module
│           └── migrate.go # Migration logic
├── go.mod                # Go dependencies
└── Makefile              # Go build
```

## Development Workflow

### Go CLI Development

1. **Make changes to Go code**
2. **Format code**: `make format`
3. **Run linter**: `make lint`
4. **Run tests**: `make test`
5. **Build**: `make build`
6. **Test CLI**: `./bin/port --help`

## Code Style

### Go

- Follow [Effective Go](https://go.dev/doc/effective_go) guidelines
- Use `gofmt` for formatting (run `make format`)
- Use `golangci-lint` for linting (run `make lint`)
- Keep functions focused and testable
- Use meaningful variable and function names
- Add comments for exported functions and types
- Handle errors explicitly (don't ignore errors)

## Testing

### Go CLI Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-cov

# Run specific package tests
go test ./internal/modules/export/...
```

## Key Features and Architecture

### Dual Credentials Support

The CLI supports working with multiple organizations simultaneously:

- **Base org**: Source organization (for export)
- **Target org**: Destination organization (for import/migrate)

**Flags:**
- `--client-id`, `--client-secret`: Base org credentials
- `--target-client-id`, `--target-client-secret`: Target org credentials

**Environment Variables:**
- `PORT_CLIENT_ID`, `PORT_CLIENT_SECRET`: Base org
- `PORT_TARGET_CLIENT_ID`, `PORT_TARGET_CLIENT_SECRET`: Target org

### Diff Validation

The import and migrate commands use diff validation to:
- Compare import data with current organization state
- Only create/update resources that differ
- Skip identical resources
- Provide accurate dry-run predictions

### Resource Types

The CLI handles these resource types:
- Blueprints
- Entities
- Scorecards
- Actions (blueprint-level and organization-wide automations)
- Teams
- Users
- Pages
- Integrations

### Options

- `--skip-entities`: Skip entity data (schema only)
- `--include`: Selectively include specific resource types
- `--dry-run`: Validate without applying changes

## Adding New Features

### Adding a New Resource Type

1. Add resource type to `internal/api/requests.go` (type definition and API methods)
2. Update `internal/modules/export/collector.go` (collection logic)
3. Update `internal/modules/import_module/importer.go` (import logic)
4. Update `internal/modules/import_module/diff.go` (diff comparison)
5. Update `internal/modules/migrate/migrate.go` (migration logic)
6. Update command files (`export.go`, `import.go`, `migrate.go`) to handle the new resource
7. Add tests
8. Update documentation

### Adding a New Command

1. Create command file in `internal/commands/` (e.g., `newcommand.go`)
2. Implement `RegisterNewCommand` function
3. Register in `cmd/port/main.go`
4. Add help text and flags
5. Add tests
6. Update README.md

### Adding a New Export Format

1. Update `internal/modules/export/export.go` (`writeTar`, `writeJSON` functions)
2. Add format-specific logic
3. Update `Options` struct if needed
4. Add tests
5. Update documentation

## Pull Request Process

1. **Fork** the repository
2. **Create a branch** for your feature: `git checkout -b feature/my-feature`
3. **Make your changes** following the code style guidelines
4. **Write tests** for your changes
5. **Run all tests** to ensure nothing breaks: `make test`
6. **Format and lint** your code: `make format lint`
7. **Commit your changes** with clear commit messages
8. **Push to your fork**: `git push origin feature/my-feature`
9. **Create a Pull Request** with a clear description

### Commit Message Format

Use conventional commits format:

```
type(scope): subject

body (optional)

footer (optional)
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

Examples:
```
feat(export): add support for selective blueprint export
fix(api): handle token expiration correctly
docs(readme): update installation instructions
feat(migrate): add diff validation support
```

## Code Review Guidelines

- **Tests**: All new features should include tests
- **Documentation**: Update README.md for user-facing changes
- **Backward Compatibility**: Consider impact on existing users
- **Performance**: Consider performance implications for large datasets
- **Error Handling**: Provide clear error messages
- **Logging**: Use appropriate log levels

## Documentation

- Update README.md for user-facing changes
- Add inline comments for complex logic
- Update API documentation for new endpoints
- Add examples for new features
- Update CONTRIBUTING.md if workflow changes

## Getting Help

- **Issues**: Open an issue for bugs or feature requests
- **Discussions**: Use GitHub Discussions for questions
- **Slack**: Join Port's community Slack

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
