#!/usr/bin/env pwsh
# Build script for AzureAutoHibernate with version injection

param(
    [switch]$Release,
    [string]$Version = ""
)

# Determine version
if ($Version -eq "") {
    # Try to get version from git
    $gitVersion = git describe --tags --always --dirty 2>$null
    if ($LASTEXITCODE -eq 0 -and $gitVersion) {
        $Version = $gitVersion
    } else {
        $Version = "dev"
    }
}

# Get git commit
$gitCommit = git rev-parse HEAD 2>$null
if ($LASTEXITCODE -ne 0 -or -not $gitCommit) {
    $gitCommit = "unknown"
}

# Get build date
$buildDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

# Build ldflags
$ldflags = "-X github.com/smitstech/AzureAutoHibernate/internal/version.Version=$Version " +
           "-X github.com/smitstech/AzureAutoHibernate/internal/version.GitCommit=$gitCommit " +
           "-X github.com/smitstech/AzureAutoHibernate/internal/version.BuildDate=$buildDate"

Write-Host "Building AzureAutoHibernate..." -ForegroundColor Cyan
Write-Host "  Version: $Version"
Write-Host "  Commit:  $gitCommit"
Write-Host "  Date:    $buildDate"
Write-Host ""

# Build main service
Write-Host "Building service executable..." -ForegroundColor Yellow
go build -ldflags="$ldflags" -o AzureAutoHibernate.exe ./cmd/autohibernate
if ($LASTEXITCODE -ne 0) {
    Write-Host "Failed to build service" -ForegroundColor Red
    exit 1
}

# Build notifier
Write-Host "Building notifier executable..." -ForegroundColor Yellow
$notifierLdflags = "-H=windowsgui $ldflags"
go build -ldflags="$notifierLdflags" -o AzureAutoHibernate.Notifier.exe ./cmd/notifier
if ($LASTEXITCODE -ne 0) {
    Write-Host "Failed to build notifier" -ForegroundColor Red
    exit 1
}

# Build updater
Write-Host "Building updater executable..." -ForegroundColor Yellow
go build -ldflags="$ldflags" -o AzureAutoHibernate.Updater.exe ./cmd/updater
if ($LASTEXITCODE -ne 0) {
    Write-Host "Failed to build updater" -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "Build complete!" -ForegroundColor Green
Write-Host "  AzureAutoHibernate.exe"
Write-Host "  AzureAutoHibernate.Notifier.exe"
Write-Host "  AzureAutoHibernate.Updater.exe"

# Test version flag
Write-Host ""
Write-Host "Testing version:" -ForegroundColor Cyan
& .\AzureAutoHibernate.exe -version
