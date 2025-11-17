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

Releases use semver (`vX.Y.Z`) and update `CHANGELOG.md`.

## License

MIT — contributions must be MIT-compatible.
