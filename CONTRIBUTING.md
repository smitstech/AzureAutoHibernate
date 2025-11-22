# Contributing to AzureAutoHibernate

Thanks for your interest in contributing!

AzureAutoHibernate is a Windows service and notifier that helps automatically hibernate Azure VMs when they are idle, using IMDS and Managed Identity. Contributions are very welcome.

## Getting started

### Prerequisites

- Go (version from `go.mod`)
- Windows (for building/testing)
- Git

### Setup

```bash
git clone https://github.com/smitstech/AzureAutoHibernate.git
cd AzureAutoHibernate
go test ./...
```

### Building

Use the build scripts for automatic version injection:

```powershell
# Windows
.\build.ps1

# Linux/Mac
make build
```

Or build manually:

```bash
go build -o AzureAutoHibernate.exe ./cmd/autohibernate
go build -ldflags="-H=windowsgui" -o AzureAutoHibernate.Notifier.exe ./cmd/notifier
```

## Contributing workflow

1. Fork the repo
2. Create a branch from `main`
3. Make changes
4. Run:
   - `go test ./...`
   - `go vet ./...`
5. Submit a PR

## Code style

- Use `gofmt`
- Prefer clarity over cleverness

## Release process

This project uses [Semantic Versioning](https://semver.org/) and follows the [Keep a Changelog](https://keepachangelog.com/) format.

### How to release

1. **Create a release preparation PR**
   ```bash
   git checkout -b release-vX.Y.Z
   # Edit CHANGELOG.md:
   #  - Move items from [Unreleased] to new [X.Y.Z] section
   #  - Use format: ## [X.Y.Z] - YYYY-MM-DD
   #  - Reset [Unreleased] to "- (nothing yet)"
   git add CHANGELOG.md
   git commit -m "Prepare release vX.Y.Z"
   git push origin release-vX.Y.Z
   ```

2. **Merge the PR** (with review/approvals as needed)

3. **Tag the release from main**
   ```bash
   git checkout main
   git pull origin main
   git tag vX.Y.Z
   git push origin vX.Y.Z  # Push tag only
   ```

4. **Automated workflow**
   - GitHub Actions automatically builds both executables with version info
   - Extracts release notes from the corresponding version section in CHANGELOG.md
   - Creates GitHub release with binaries and release notes

**Alternative:** Create the release directly from GitHub UI (Releases → Draft a new release → Create new tag)

### Version number guidelines

- **Major (X)**: Breaking changes
- **Minor (Y)**: New features, backward compatible
- **Patch (Z)**: Bug fixes, backward compatible

## License

MIT — contributions must be MIT-compatible.
