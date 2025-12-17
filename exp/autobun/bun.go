package autobun

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/aperturerobotics/bldr/util/http"
	"github.com/sirupsen/logrus"
)

// DefaultBunVersion is the default version of bun to download.
const DefaultBunVersion = "1.3.4"

// BunDownloadURLTemplate is the URL template for downloading bun releases.
// Format: https://github.com/oven-sh/bun/releases/download/bun-v{version}/bun-{platform}-{arch}.zip
const BunDownloadURLTemplate = "https://github.com/oven-sh/bun/releases/download/bun-v%s/bun-%s-%s.zip"

// GetBunPlatform returns the bun platform name for the current OS.
func GetBunPlatform() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return "darwin", nil
	case "linux":
		return "linux", nil
	case "windows":
		return "windows", nil
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// GetBunArch returns the bun architecture name for the current architecture.
func GetBunArch() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return "x64", nil
	case "arm64":
		if runtime.GOOS == "darwin" {
			return "aarch64", nil
		}
		return "aarch64", nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}
}

// GetBunDownloadURL returns the download URL for the specified bun version.
func GetBunDownloadURL(version string) (string, error) {
	platform, err := GetBunPlatform()
	if err != nil {
		return "", err
	}
	arch, err := GetBunArch()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(BunDownloadURLTemplate, version, platform, arch), nil
}

// GetBunBinaryName returns the bun binary name for the current platform.
func GetBunBinaryName() string {
	if runtime.GOOS == "windows" {
		return "bun.exe"
	}
	return "bun"
}

// GetLocalBunDir returns the local directory where bun is downloaded.
func GetLocalBunDir(stateDir string) string {
	return filepath.Join(stateDir, "bun")
}

// GetLocalBunPath returns the path to the local bun binary.
func GetLocalBunPath(stateDir, version string) string {
	return filepath.Join(GetLocalBunDir(stateDir), version, GetBunBinaryName())
}

// FindBunPath finds the path to the bun binary.
// First checks if bun is in the system PATH, then checks the local installation.
func FindBunPath(stateDir, version string) (string, error) {
	// Check system PATH first
	if path, err := exec.LookPath("bun"); err == nil {
		return path, nil
	}

	// Check local installation
	localPath := GetLocalBunPath(stateDir, version)
	if _, err := os.Stat(localPath); err == nil {
		return localPath, nil
	}

	return "", fmt.Errorf("bun not found in PATH or local installation")
}

// DownloadBun downloads the specified version of bun to the state directory.
func DownloadBun(ctx context.Context, le *logrus.Entry, stateDir, version string) (string, error) {
	bunPath := GetLocalBunPath(stateDir, version)

	// Check if already downloaded
	if _, err := os.Stat(bunPath); err == nil {
		le.WithField("path", bunPath).Debug("bun already downloaded")
		return bunPath, nil
	}

	downloadURL, err := GetBunDownloadURL(version)
	if err != nil {
		return "", err
	}

	le.WithFields(logrus.Fields{
		"url":     downloadURL,
		"version": version,
	}).Info("downloading bun")

	// Create the directory
	bunDir := filepath.Dir(bunPath)
	if err := os.MkdirAll(bunDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create bun directory: %w", err)
	}

	// Download the zip file
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download bun: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to download bun: %s", resp.Status)
	}

	// Create a temporary file to store the zip
	tmpFile, err := os.CreateTemp("", "bun-*.zip")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Copy the response body to the temp file
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write zip file: %w", err)
	}
	tmpFile.Close()

	// Extract the bun binary from the zip
	if err := extractBunFromZip(tmpPath, bunDir); err != nil {
		return "", fmt.Errorf("failed to extract bun: %w", err)
	}

	// Make the binary executable
	if err := os.Chmod(bunPath, 0o755); err != nil {
		return "", fmt.Errorf("failed to make bun executable: %w", err)
	}

	le.WithField("path", bunPath).Info("bun downloaded successfully")
	return bunPath, nil
}

// extractBunFromZip extracts the bun binary from a zip file to the destination directory.
func extractBunFromZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	bunName := GetBunBinaryName()

	for _, f := range r.File {
		// The zip contains a directory like "bun-darwin-aarch64/bun"
		// We want to extract just the bun binary
		name := filepath.Base(f.Name)
		if name != bunName {
			continue
		}

		// Skip directories
		if f.FileInfo().IsDir() {
			continue
		}

		destPath := filepath.Join(destDir, bunName)

		// Open the file in the zip
		rc, err := f.Open()
		if err != nil {
			return err
		}

		// Create the destination file
		outFile, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			return err
		}

		// Copy the contents
		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}

		return nil
	}

	return fmt.Errorf("bun binary not found in zip")
}

// EnsureBun ensures bun is available, downloading it if necessary.
// Returns the path to the bun binary.
func EnsureBun(ctx context.Context, le *logrus.Entry, stateDir, version string) (string, error) {
	// Try to find existing bun
	path, err := FindBunPath(stateDir, version)
	if err == nil {
		return path, nil
	}

	// Download bun
	return DownloadBun(ctx, le, stateDir, version)
}

// RunBun runs bun with the given arguments.
func RunBun(ctx context.Context, le *logrus.Entry, stateDir, version string, args []string) error {
	bunPath, err := EnsureBun(ctx, le, stateDir, version)
	if err != nil {
		return err
	}

	le.WithFields(logrus.Fields{
		"bun":  bunPath,
		"args": strings.Join(args, " "),
	}).Debug("running bun")

	cmd := exec.CommandContext(ctx, bunPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	return cmd.Run()
}
