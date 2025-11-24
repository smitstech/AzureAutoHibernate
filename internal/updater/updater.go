//go:build windows

package updater

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/smitstech/AzureAutoHibernate/internal/appinfo"
	"github.com/smitstech/AzureAutoHibernate/internal/version"
)

// UpdateInfo contains information about an available update
type UpdateInfo struct {
	CurrentVersion  string
	LatestVersion   string
	ReleaseNotes    string
	ReleaseURL      string
	DownloadURL     string
	UpdateAvailable bool
}

// CheckForUpdate checks GitHub for a newer version
func CheckForUpdate(ctx context.Context) (*UpdateInfo, error) {
	source, err := selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub source: %w", err)
	}

	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		Source:    source,
		Validator: nil, // TODO: Add signature validation
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create updater: %w", err)
	}

	// Get current version (strip 'v' prefix if present for comparison)
	currentVersion := strings.TrimPrefix(version.Version, "v")

	// Check for latest release
	slug := selfupdate.ParseSlug(appinfo.RepoOwner + "/" + appinfo.RepoName)
	latest, found, err := updater.DetectLatest(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to detect latest version: %w", err)
	}

	info := &UpdateInfo{
		CurrentVersion:  version.Version,
		UpdateAvailable: false,
	}

	if !found {
		return info, nil
	}

	info.LatestVersion = latest.Version()
	info.ReleaseNotes = latest.ReleaseNotes
	info.ReleaseURL = latest.URL
	info.DownloadURL = latest.AssetURL

	// Compare versions
	if latest.GreaterThan(currentVersion) {
		info.UpdateAvailable = true
	}

	return info, nil
}

// DownloadUpdate downloads the latest release zip and extracts it to a temporary location
// Returns the path to the directory containing the extracted files
func DownloadUpdate(ctx context.Context) (string, error) {
	// First, check for update to get the download URL
	info, err := CheckForUpdate(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to check for update: %w", err)
	}

	if !info.UpdateAvailable {
		return "", fmt.Errorf("no update available")
	}

	if info.DownloadURL == "" {
		return "", fmt.Errorf("no download URL available")
	}

	// Create temp directory for the update
	tempDir, err := os.MkdirTemp("", "azureautohib-update-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Download the zip file
	zipPath := filepath.Join(tempDir, "update.zip")
	if err := downloadFile(ctx, info.DownloadURL, zipPath); err != nil {
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to download update: %w", err)
	}

	// Extract the zip file
	extractDir := filepath.Join(tempDir, "extracted")
	if err := extractZip(zipPath, extractDir); err != nil {
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to extract update: %w", err)
	}

	// Remove the zip file to save space
	os.Remove(zipPath)

	return extractDir, nil
}

// downloadFile downloads a file from a URL to a local path
func downloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// extractZip extracts a zip file to a destination directory
func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	for _, f := range r.File {
		// Sanitize the file path to prevent zip slip attacks
		destPath := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(destPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(destPath, f.Mode())
			continue
		}

		// Create parent directories if needed
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		// Extract the file
		if err := extractZipFile(f, destPath); err != nil {
			return fmt.Errorf("failed to extract %s: %w", f.Name, err)
		}
	}

	return nil
}

// extractZipFile extracts a single file from a zip archive
func extractZipFile(f *zip.File, destPath string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}

// TriggerUpdate spawns the updater helper and signals the service to stop
func TriggerUpdate(tempDir string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	exeDir := filepath.Dir(exePath)
	updaterPath := filepath.Join(exeDir, appinfo.UpdaterExeName)

	// Check if updater exists
	if _, err := os.Stat(updaterPath); err != nil {
		return fmt.Errorf("updater executable not found at %s: %w", updaterPath, err)
	}

	// Spawn updater process with arguments
	cmd := exec.Command(updaterPath,
		"--service-name", appinfo.ServiceName,
		"--exe-path", exePath,
		"--update-dir", tempDir,
	)

	// Start but don't wait - the updater will run after we exit
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start updater: %w", err)
	}

	// Detach from the child process
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("failed to release updater process: %w", err)
	}

	return nil
}
