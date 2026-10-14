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
go test ./...          # unit tests
make test-integration  # against the docker compose services
```

### Frontend

The checks that gate a PR are the linter, the unit tests, the type-check and a
production build — the same CI runs.

```bash
cd web
pnpm lint
pnpm test    # vitest
pnpm build   # tsc -b && vite build
```

### End to end

`make test-e2e` builds the frontend and the binary, starts two servers against
the compose services — every module, and users and groups only — and checks in
a browser, for the admin and the read-only seed accounts, that the UI offers
exactly what the API lets that session do: the sections in the nav and the
command palette, the create, import and selection controls, read-only detail
pages, and a redirect for any page the session cannot use. The expectations are
asked of the API rather than written in the test, so a permission change in
`internal/rbac` needs no edit there. It only reads; the write probes send a body
the server rejects before acting on it.

```bash
cd web && pnpm exec playwright install chromium   # once
make test-e2e
```

## Cutting a Release

Versions are date-based: `YYYY.MM.DD.HHMMSS`, UTC. There is no semantic version to
bump — the tag *is* the version.

Tag by hand rather than using the workflow button. Both work, but only the tag
route lets you know the version before the release runs, which is what makes it
possible to write it into the changelog:

```bash
# 1. Everything green on main
go test ./... && make test-integration
(cd web && pnpm lint && pnpm build)

# 2. Name the version and record it in the changelog
V=$(date -u +'%Y.%m.%d.%H%M%S')
# rename "## Unreleased" to "## $V — $(date -u +%Y-%m-%d)" in CHANGELOG.md
git commit -am "docs: release $V"

# 3. Tag it and push — this is what triggers the release workflow
git tag "$V"
git push origin main "$V"
```

The workflow builds the frontend and the Linux binaries, publishes a GitHub
release and pushes a multi-arch image to `ghcr.io/ldapwarden/ldapwarden`.

**The changelog is the release body.** The workflow extracts the topmost `## `
section of `CHANGELOG.md` and uses it as the release notes, with the generated
commit list appended underneath. So anything an operator must *do* to upgrade —
a migration to run, a volume that changes name, an API response that changes shape
— belongs in an **Upgrading** block at the top of that section. A commit list
cannot say "run this before starting the new version".

Start the next section as `## Unreleased` right after tagging, so changes have
somewhere to land.

Running the workflow from the Actions tab still works and is fine for a
throwaway build: it makes up a timestamp, tags it, and titles the release body
with that version even though the file still says `## Unreleased`.

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
