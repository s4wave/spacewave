package v86_wazero

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

const (
	// DefaultCdnBaseURL serves the harness's published image fixtures.
	DefaultCdnBaseURL = "https://cdn-staging.spacewave.app"
	// DefaultCdnSpaceID identifies the fixture Space on the CDN.
	DefaultCdnSpaceID = "01kpn3x0y79yr94ps1yae206vp"
	// DefaultV86ImageKey selects the default boot image in the fixture Space.
	DefaultV86ImageKey = "v86image-01kszf4rsev1s7zkq2ms2y5r0w"
	// DefaultAssetCacheSubdir stores disposable boot files below the repository.
	DefaultAssetCacheSubdir = ".tmp/v86-wazero"
)

// AssetOptions configures where the harness finds the real v86 image.
type AssetOptions struct {
	// Le records hydration activity; nil selects the harness's standard logger.
	Le *logrus.Entry
	// CacheDir stores downloaded boot files.
	CacheDir string
	// AssetDir takes precedence when it already contains a complete image.
	AssetDir string
	// V86Dir supplies emulator and BIOS files from a local checkout.
	V86Dir string
	// V86FSDir supplies the guest kernel and filesystem beside V86Dir.
	V86FSDir string
	// CdnBaseURL serves image metadata and content blocks.
	CdnBaseURL string
	// CdnSpaceID identifies the published fixture Space.
	CdnSpaceID string
	// ImageKey selects a preferred V86Image object within the Space.
	ImageKey string
	// Refresh bypasses cached downloads while retaining explicit local overrides.
	Refresh bool
}

// OptionsFromEnv reads the V86_WAZERO_* and V86_DIR/V86FS_DIR environment variables.
func OptionsFromEnv() AssetOptions {
	return AssetOptions{
		CacheDir:   strings.TrimSpace(os.Getenv("V86_WAZERO_CACHE_DIR")),
		AssetDir:   strings.TrimSpace(os.Getenv("V86_WAZERO_ASSET_DIR")),
		V86Dir:     strings.TrimSpace(os.Getenv("V86_DIR")),
		V86FSDir:   strings.TrimSpace(os.Getenv("V86FS_DIR")),
		CdnBaseURL: strings.TrimSpace(os.Getenv("V86_WAZERO_CDN_BASE_URL")),
		CdnSpaceID: strings.TrimSpace(os.Getenv("V86_WAZERO_CDN_SPACE_ID")),
		ImageKey:   strings.TrimSpace(os.Getenv("V86_WAZERO_IMAGE_KEY")),
		Refresh:    strings.EqualFold(strings.TrimSpace(os.Getenv("V86_WAZERO_REFRESH")), "true"),
	}
}

// withDefaults fills unset options with the standard fixture and cache locations.
func (o AssetOptions) withDefaults() AssetOptions {
	// Supply the v86 asset logger and published CDN fixture defaults.
	if o.Le == nil {
		o.Le = logrus.NewEntry(logrus.StandardLogger())
	}
	if o.CdnBaseURL == "" {
		o.CdnBaseURL = DefaultCdnBaseURL
	}
	if o.CdnSpaceID == "" {
		o.CdnSpaceID = DefaultCdnSpaceID
	}
	if o.ImageKey == "" {
		o.ImageKey = DefaultV86ImageKey
	}

	// Place the default v86 asset cache under the repository root.
	if o.CacheDir == "" {
		o.CacheDir = filepath.Join(repoRootOrCwd(), DefaultAssetCacheSubdir)
	}

	return o
}
