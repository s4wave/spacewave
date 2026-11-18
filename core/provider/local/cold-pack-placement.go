package provider_local

import (
	"context"
	"errors"
	"path"
	"strings"
	"sync/atomic"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/sirupsen/logrus"
)

// ColdPackPlacementConfig opts one placed block store into reading cold packs
// in its existing S3 backend's bucket. It is host-local, not account state.
// Do not move or delete a placement with live blocks held only by cold packs:
// storage moves copy from the hot backend and do not carry this configuration.
type ColdPackPlacementConfig struct {
	// BlockStoreLocalID identifies the provider, account and block store.
	BlockStoreLocalID string
	// BackendID pins the configuration to the intended placement's S3 backend.
	BackendID string
	// ColdPackOptions names the separate cold prefix and local files and budgets.
	block_store_s3.ColdPackOptions
}

// coldPackPlacement ties the host configuration to its current mount.
type coldPackPlacement struct {
	// config is immutable while registered.
	config ColdPackPlacementConfig
	// store is the mounted cold reader, or nil while no matching store is mounted.
	store atomic.Pointer[block_store_s3.ColdPackStore]
}

// configuredColdPacks holds the optional cold-pack placement for this process.
var configuredColdPacks atomic.Pointer[coldPackPlacement]

// ConfigureColdPacks registers one placed store before the local provider
// starts. Release the registration after the provider has stopped. The mount
// owns the cold reader and closes it on unmount, including placement changes.
func ConfigureColdPacks(config ColdPackPlacementConfig) (func(), error) {
	// Require a single exact placement and a local manifest.
	if config.BlockStoreLocalID == "" || config.BackendID == "" || config.ManifestFile == "" {
		return nil, errors.New("cold packs require a block store ID, S3 backend ID and manifest file")
	}
	if config.PromoteCap < 0 || config.TailCacheBytes < 0 || config.NegativeCacheKeys < 0 {
		return nil, errors.New("cold pack budgets must not be negative")
	}

	// Require a relative directory prefix with no aliases or escapes.
	prefix := strings.TrimSuffix(config.Prefix, "/")
	if prefix == "" || prefix == "." || prefix == ".." || strings.HasPrefix(prefix, "../") ||
		strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, "\\\x00") || path.Clean(prefix) != prefix {
		return nil, errors.New("cold packs require a nonempty, clean, relative bucket prefix")
	}
	config.Prefix = prefix + "/"

	// Publish the immutable configuration before any matching mount can open.
	placement := &coldPackPlacement{config: config}
	if !configuredColdPacks.CompareAndSwap(nil, placement) {
		return nil, errors.New("cold packs already configured for this process")
	}
	return func() { configuredColdPacks.CompareAndSwap(placement, nil) }, nil
}

// GetColdPackStats snapshots the active mount's counters. The second result
// is false when the configured store has no mounted cold reader.
func GetColdPackStats() (block_store_s3.ColdPackStats, bool) {
	// Report counters from the current registration and mount only.
	if p := configuredColdPacks.Load(); p != nil {
		if store := p.store.Load(); store != nil {
			return store.Stats(), true
		}
	}
	return block_store_s3.ColdPackStats{}, false
}

// validateColdPackPrefix excludes the entire hot prefix: deleting an S3
// placement lists the whole prefix, not only its packs and entries.
func validateColdPackPrefix(coldPrefix, hotPrefix string) error {
	// Match both the literal object key and its slash-clean form.
	cleanHot := strings.TrimSuffix(path.Clean(hotPrefix), "/") + "/"
	if strings.HasPrefix(coldPrefix, hotPrefix) || strings.HasPrefix(hotPrefix, coldPrefix) ||
		strings.HasPrefix(coldPrefix, cleanHot) || strings.HasPrefix(cleanHot, coldPrefix) {
		return errors.New("cold pack prefix overlaps the placed S3 store's prefix (including packs and entries)")
	}
	return nil
}

// openColdPacks opens the cold reader for the mount's actual S3 placement, or
// returns nil if the store is not on the configured backend. The ordinary S3
// and SQL read paths stay available when this optional tier cannot open.
func (t *bstoreTracker) openColdPacks(
	ctx context.Context,
	le *logrus.Entry,
	p *coldPackPlacement,
	backend *account_settings.StorageBackend,
	serve bool,
) (*block_store_s3.ColdPackStore, error) {
	// Bind the manifest to its selected backend, even after a storage move.
	location := backend.GetS3()
	if backend.GetId() != p.config.BackendID || location == nil {
		return nil, nil
	}

	// Use the actual backend's prefix and bucket, not a second configuration
	// that could disagree and put cold objects under the hot delete prefix.
	opts := p.config.ColdPackOptions
	if err := validateColdPackPrefix(opts.Prefix, location.BlockStorePrefix(t.id)); err != nil {
		return nil, err
	}

	// Reuse the hot signer after it opens; manifest loading and the mount do
	// not have to wait for the backend's credential to become available.
	if serve {
		opts.PromotedLog = ""
	}
	reader := &coldPackRangeReader{tracker: t, backend: backend}
	return block_store_s3.NewColdPackStore(ctx, le, reader, location.GetBucket(), opts)
}
