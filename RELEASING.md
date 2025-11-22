# Releasing

This document describes how to create a new release of AzureAutoHibernate.

## Prerequisites

- You must have push access to the repository
- All changes for the release must be merged to `main`

## Release Process

### 1. Create a Release Preparation PR

Create a branch to update the changelog:

```bash
git checkout main
git pull origin main
git checkout -b release-vX.Y.Z
```

Update `CHANGELOG.md` to move items from `[Unreleased]` to a new version section:

```markdown
## [Unreleased]

- (nothing yet)

---

## [X.Y.Z] - YYYY-MM-DD

### Added
- (move items from Unreleased here)

### Changed
- (move items from Unreleased here)
```

Commit and push:

```bash
git add CHANGELOG.md
git commit -m "Prepare release vX.Y.Z"
git push origin release-vX.Y.Z
```

### 2. Merge the PR

Open a PR from `release-vX.Y.Z` to `main` and merge it after review.

### 3. Create and Push the Tag

After the PR is merged:

```bash
git checkout main
git pull origin main
git tag vX.Y.Z
git push origin vX.Y.Z
```

### 4. Automated Release

Pushing the tag triggers the GitHub Actions release workflow which:

1. Runs tests
2. Builds both executables with version info embedded
3. Packages them into a zip file
4. Extracts release notes from the `[X.Y.Z]` section in CHANGELOG.md
5. Creates a GitHub Release with the zip and release notes

## Version Numbering

Follow [Semantic Versioning](https://semver.org/):

- **Major (X)**: Breaking changes
- **Minor (Y)**: New features, backward compatible
- **Patch (Z)**: Bug fixes, backward compatible

## Common Issues

### "Could not find version X.Y.Z in CHANGELOG.md"

This warning appears when the release workflow can't find a matching version section. Make sure you:

1. Updated CHANGELOG.md **before** pushing the tag
2. Used the correct format: `## [X.Y.Z]` (without the `v` prefix)
3. The version in the tag matches the version in the changelog

### Fixing a Failed Release

If you need to redo a release:

```bash
# Delete the remote tag
git push origin --delete vX.Y.Z

# Delete the local tag
git tag -d vX.Y.Z

# Make your fixes via PR, then re-tag after merge
git checkout main
git pull origin main
git tag vX.Y.Z
git push origin vX.Y.Z
```

Note: You may also need to delete the GitHub Release manually if one was created.
