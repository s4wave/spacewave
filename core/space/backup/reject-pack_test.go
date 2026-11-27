package space_backup_test

import (
	"context"
	"strings"

	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// rejectPack drops a pack before Drive can receive its ciphertext bytes.
type rejectPack struct {
	// ObjectStore handles every non-pack operation normally.
	block_store_s3.ObjectStore
	// err is the injected upload-limit or missing-upload failure.
	err error
}

// PutObject rejects pack uploads without creating their entries or indexes.
func (s *rejectPack) PutObject(ctx context.Context, bucket, key string, data []byte, contentType string) error {
	if strings.Contains(key, "/packs/") {
		return s.err
	}
	return s.ObjectStore.PutObject(ctx, bucket, key, data, contentType)
}

// _ is a type assertion.
var _ block_store_s3.ObjectStore = (*rejectPack)(nil)
