# Contributing to ethpandaops-mcp

Thank you for your interest in contributing to the ethpandaops MCP server! This document provides guidelines and instructions for contributing.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Pull Request Process](#pull-request-process)
- [Coding Standards](#coding-standards)
- [Testing](#testing)
- [Documentation](#documentation)
- [Commit Messages](#commit-messages)
- [Release Process](#release-process)

## Code of Conduct

This project and everyone participating in it is governed by our commitment to:

- Be respectful and inclusive
- Welcome newcomers and help them learn
- Focus on constructive feedback
- Accept responsibility for mistakes

## Getting Started

### Prerequisites

- Go 1.24 or later
- Docker (for sandbox testing)
- Git

### Setting Up Your Development Environment

1. **Fork the repository** on GitHub

2. **Clone your fork:**
   ```bash
   git clone https://github.com/YOUR_USERNAME/mcp.git
   cd mcp
   ```

3. **Install dependencies:**
   ```bash
   go mod download
   ```

4. **Install development tools:**
   ```bash
   go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
   ```

5. **Copy configuration:**
   ```bash
   cp config.example.yaml config.yaml
   # Edit config.yaml with your datasource credentials
   ```

6. **Build and run:**
   ```bash
   make build
   make run-sse
   ```

## Development Workflow

### Branch Naming

Use descriptive branch names:

- `feature/add-postgres-plugin` - New features
- `bugfix/fix-timeout-handling` - Bug fixes
- `docs/improve-readme` - Documentation
- `refactor/cleanup-sandbox` - Refactoring

### Making Changes

1. **Create a branch:**
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make your changes** following our coding standards

3. **Run tests and linting:**
   ```bash
   make lint
   make test
   ```

4. **Commit your changes** (see [Commit Messages](#commit-messages))

5. **Push to your fork:**
   ```bash
   git push origin feature/your-feature-name
   ```

## Pull Request Process

1. **Ensure your PR includes:**
   - Clear description of the changes
   - Link to related issues (if any)
   - Updated tests (if applicable)
   - Updated documentation (if applicable)

2. **Before submitting:**
   - Run `make lint` - all checks must pass
   - Run `make test` - all tests must pass
   - Rebase on the latest `main` branch
   - Ensure commits are clean and meaningful

3. **PR title format:**
   - `feat: add support for X` - New features
   - `fix: resolve issue with Y` - Bug fixes
   - `docs: update Z documentation` - Documentation
   - `refactor: improve W implementation` - Refactoring
   - `test: add tests for V` - Tests

4. **PR description template:**
   ```markdown
   ## Description
   Brief description of the changes

   ## Related Issues
   Fixes #123

   ## Changes
   - Change 1
   - Change 2

   ## Testing
   - How was this tested?
   - Test commands used

   ## Checklist
   - [ ] Tests pass
   - [ ] Linting passes
   - [ ] Documentation updated
   ```

5. **Review process:**
   - Maintainers will review your PR
   - Address review comments
   - Once approved, a maintainer will merge

## Coding Standards

### Go Code Style

We follow standard Go conventions:

- **Formatting:** Use `gofmt` (run `make fmt`)
- **Linting:** Use `golangci-lint` (run `make lint`)
- **Imports:** Group imports: stdlib, third-party, local
- **Naming:** Use camelCase for variables, PascalCase for exported functions
- **Comments:** Document all exported functions, types, and packages

Example:
```go
package mypackage

import (
    "context"
    "fmt"
    
    "github.com/sirupsen/logrus"
    
    "github.com/ethpandaops/mcp/pkg/types"
)

// MyFunction does something important.
// It returns an error if the operation fails.
func MyFunction(ctx context.Context, input string) error {
    // implementation
}
```

### Error Handling

- Always check errors
- Wrap errors with context using `fmt.Errorf("...: %w", err)`
- Return early on errors to reduce nesting
- Use sentinel errors for common cases (see `pkg/plugin/plugin.go`)

### Logging

- Use `github.com/sirupsen/logrus`
- Use structured logging with fields
- Appropriate log levels:
  - `Debug` - Detailed debugging info
  - `Info` - General operational info
  - `Warn` - Warnings that don't prevent operation
  - `Error` - Errors that prevent operation

Example:
```go
log.WithFields(logrus.Fields{
    "plugin": pluginName,
    "duration": duration,
}).Info("Plugin initialized")
```

### Configuration

- Use struct tags for YAML parsing
- Provide sensible defaults in `ApplyDefaults()`
- Validate configuration in `Validate()`
- Support environment variable substitution

### Plugin Development

See [docs/PLUGIN_DEVELOPMENT.md](docs/PLUGIN_DEVELOPMENT.md) for detailed plugin development guidelines.

Key principles:
- Plugins are self-contained in `plugins/{name}/`
- Never pass credentials to the sandbox
- Provide clear examples in `examples.yaml`
- Document the Python API

## Testing

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run specific test
go test -v ./pkg/plugin/...

# Run with race detector
make test
```

### Writing Tests

- Use `github.com/stretchr/testify` for assertions
- Name tests clearly: `TestFunctionName_Scenario`
- Table-driven tests for multiple cases
- Mock external dependencies

Example:
```go
func TestPlugin_Validate(t *testing.T) {
    tests := []struct {
        name    string
        config  Config
        wantErr bool
    }{
        {
            name: "valid config",
            config: Config{Timeout: 30},
            wantErr: false,
        },
        {
            name: "zero timeout",
            config: Config{Timeout: 0},
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            p := &Plugin{cfg: tt.config}
            err := p.Validate()
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### Integration Tests

Integration tests require Docker:

```bash
# Build sandbox image
make docker-sandbox

# Run integration tests
make test-sandbox
```

## Documentation

### Code Documentation

- All exported functions, types, and packages must have doc comments
- Comments should explain "why" not just "what"
- Use complete sentences

### README Updates

- Keep README.md up to date with new features
- Update the table of contents for new sections
- Ensure code examples work

### Architecture Documentation

For significant architectural changes, update:
- `docs/ARCHITECTURE.md` - System architecture
- `docs/PLUGIN_DEVELOPMENT.md` - Plugin development guide
- `docs/deployments.md` - Deployment configurations

## Commit Messages

We follow [Conventional Commits](https://www.conventionalcommits.org/):

### Format

```
<type>(<scope>): <subject>

<body>

<footer>
```

### Types

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, no logic change)
- `refactor`: Code refactoring
- `test`: Test changes
- `chore`: Build process, dependencies, etc.

### Examples

```
feat(clickhouse): add schema discovery for materialized views

Implement automatic discovery of materialized views in ClickHouse
to provide better query suggestions.

Fixes #123
```

```
fix(sandbox): resolve timeout handling for long queries

Previously, queries longer than 60s would fail silently. Now we
properly propagate timeout errors to the client.
```

```
docs(readme): add badges and improve quickstart

- Add CI status and license badges
- Reorganize quickstart section
- Add table of contents
```

### Scope

Common scopes:
- `clickhouse`, `prometheus`, `loki`, `dora` - Plugin-specific
- `sandbox` - Sandbox execution
- `proxy` - Credential proxy
- `server` - MCP server core
- `plugin` - Plugin system
- `docs` - Documentation

## Release Process

Releases are managed by maintainers:

1. Version bump in relevant files
2. Update CHANGELOG.md
3. Create git tag: `git tag -a v1.2.3 -m "Release v1.2.3"`
4. Push tag: `git push origin v1.2.3`
5. GitHub Actions builds and publishes release

## Questions?

- Open an issue for bug reports or feature requests
- Start a discussion for questions or ideas
- Join our community chat (if available)

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

---

Thank you for contributing to ethpandaops-mcp! 🎉
