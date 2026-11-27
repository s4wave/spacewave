package provider_local

import (
	"context"
	"time"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/sirupsen/logrus"
)

// CheckS3Location writes, reads back, and deletes a probe object in the
// bucket, and classifies the first failure.
func CheckS3Location(
	ctx context.Context,
	location *account_settings.S3Location,
	creds *block_store_s3.Credentials,
) *block_store_s3.CheckResult {
	client, err := buildS3Client(location, creds)
	if err != nil {
		return &block_store_s3.CheckResult{
			Outcome: block_store_s3.CheckOutcome_CHECK_OUTCOME_FAILED,
			Detail:  err.Error(),
		}
	}
	return block_store_s3.CheckBucket(ctx, client, location.GetBucket(), location.GetObjectPrefix())
}

// buildS3BlockStore opens the block store's packfile store on the location's
// bucket.
func buildS3BlockStore(
	le *logrus.Entry,
	location *account_settings.S3Location,
	blockStoreID string,
	creds *block_store_s3.Credentials,
	index kvtx.Store,
) (*block_store_s3.PackStore, *block_store_s3.Client, error) {
	// Build the signer shared by the hot store and its optional cold reader.
	client, err := buildS3Client(location, creds)
	if err != nil {
		return nil, nil, err
	}

	// Bind the hot store to its own object prefix and pack index.
	return block_store_s3.NewPackStore(le, client, location.GetBucket(), location.BlockStorePrefix(blockStoreID), &block_store_s3.PackStoreOptions{
		Pricing:   location.GetPricing(),
		KeepPacks: location.GetKeepPacks(),
		Index:     index,
		IndexOnly: location.GetIndexOnly(),
		// A value past int64 wraps negative, which the store reads as the
		// default pack size.
		MaxPackBytes:  int64(location.GetMaxPackBytes()), //nolint:gosec // see above.
		Linger:        time.Duration(location.GetLingerSeconds()) * time.Second,
		ReadOnlyIndex: serveOnly.Load(),
	}), client, nil
}

// deleteS3BlockStore deletes the block store's objects from the location's
// bucket.
func deleteS3BlockStore(
	ctx context.Context,
	location *account_settings.S3Location,
	blockStoreID string,
	creds *block_store_s3.Credentials,
) error {
	client, err := buildS3Client(location, creds)
	if err != nil {
		return err
	}
	return block_store_s3.DeletePackStore(ctx, client, location.GetBucket(), location.BlockStorePrefix(blockStoreID))
}

// buildS3Client builds a signing client for the location's endpoint.
func buildS3Client(
	location *account_settings.S3Location,
	creds *block_store_s3.Credentials,
) (*block_store_s3.Client, error) {
	return block_store_s3.BuildClient(&block_store_s3.ClientConfig{
		Endpoint:    location.GetEndpoint(),
		Credentials: creds,
		DisableSsl:  location.GetDisableSsl(),
		Region:      location.GetRegion(),
	})
}

// _ asserts that the pack store confirms evicted blocks from its local index,
// so eviction never falls back to probing the bucket by a signature drift.
var _ block_store_writeback.DurableConfirmer = (*block_store_s3.PackStore)(nil)
