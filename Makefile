.PHONY: build clean test version help

# Get version from git tags
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build flags
LDFLAGS := -X github.com/smitstech/AzureAutoHibernate/internal/version.Version=$(VERSION) \
           -X github.com/smitstech/AzureAutoHibernate/internal/version.GitCommit=$(COMMIT) \
           -X github.com/smitstech/AzureAutoHibernate/internal/version.BuildDate=$(BUILD_DATE)

# Target OS and architecture
GOOS := windows
GOARCH := amd64

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build service, notifier, and updater executables
	@echo "Building AzureAutoHibernate..."
	@echo "  Version: $(VERSION)"
	@echo "  Commit:  $(COMMIT)"
	@echo "  Date:    $(BUILD_DATE)"
	@echo ""
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="$(LDFLAGS)" -o AzureAutoHibernate.exe ./cmd/autohibernate
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="-H=windowsgui $(LDFLAGS)" -o AzureAutoHibernate.Notifier.exe ./cmd/notifier
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="$(LDFLAGS)" -o AzureAutoHibernate.Updater.exe ./cmd/updater
	@echo ""
	@echo "Build complete!"

version: ## Show version information
	@echo "Version: $(VERSION)"
	@echo "Commit:  $(COMMIT)"
	@echo "Date:    $(BUILD_DATE)"

test: ## Run tests
	go test ./... -v

clean: ## Remove build artifacts
	rm -f AzureAutoHibernate.exe AzureAutoHibernate.Notifier.exe AzureAutoHibernate.Updater.exe
	rm -rf dist/

dist: build ## Create distribution package
	mkdir -p dist
	cp AzureAutoHibernate.exe dist/
	cp AzureAutoHibernate.Notifier.exe dist/
	cp AzureAutoHibernate.Updater.exe dist/
	cp config.json dist/
	cd dist && zip -r ../AzureAutoHibernate-$(VERSION)-windows-amd64.zip .
	@echo "Package created: AzureAutoHibernate-$(VERSION)-windows-amd64.zip"
