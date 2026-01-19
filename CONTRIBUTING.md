# Contributing to LDAP Warden

Thank you for your interest in contributing to LDAP Warden! This document provides guidelines and instructions for contributing.

## Getting Started

1. Fork the repository
2. Clone your fork locally
3. Set up the development environment (see [README.md](README.md#development))

## Development Workflow

### Branch Naming

- `feature/` — New features (e.g., `feature/ldaps-support`)
- `fix/` — Bug fixes (e.g., `fix/session-timeout`)
- `docs/` — Documentation changes
- `refactor/` — Code refactoring without feature changes

### Commit Messages

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add LDAPS support
fix: resolve session expiration issue
docs: update configuration guide
refactor: simplify LDAP client connection handling
test: add user creation tests
```

### Pull Request Process

1. Create your feature branch from `main`:
   ```bash
   git checkout -b feature/amazing-feature
   ```

2. Make your changes and commit them:
   ```bash
   git commit -m 'feat: add some amazing feature'
   ```

3. Push to your fork:
   ```bash
   git push origin feature/amazing-feature
   ```

4. Open a Pull Request against `main`

5. Ensure your PR:
   - Passes all CI checks
   - Has a clear description of changes
   - Includes tests for new functionality
   - Updates documentation if needed

## Code Style

### Go

- Follow standard Go formatting (`go fmt`)
- Use meaningful variable and function names
- Add comments for exported functions
- Handle errors explicitly

### TypeScript/React

- Use TypeScript strict mode
- Follow the existing component patterns
- Use Zod schemas for API responses
- Prefer functional components with hooks

## Testing

### Backend

```bash
go test ./...
```

### Frontend

```bash
cd web && pnpm test
```

## Reporting Issues

When reporting issues, please include:

- A clear, descriptive title
- Steps to reproduce the issue
- Expected vs actual behavior
- Environment details (OS, browser, versions)
- Relevant logs or screenshots

## Feature Requests

Feature requests are welcome! Please:

- Check existing issues to avoid duplicates
- Clearly describe the use case
- Explain why this feature would be useful

## Code of Conduct

- Be respectful and inclusive
- Provide constructive feedback
- Focus on the issue, not the person
- Help others learn and grow

## Questions?

Feel free to open an issue for questions or reach out to the maintainers.

---

Thank you for contributing!
