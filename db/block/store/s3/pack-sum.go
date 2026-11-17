//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"

	"github.com/pkg/errors"
)

// indexSumPrefix keys the s/ row of a pack this store wrote: the SHA-256 of
// its bytes as uploaded. Removing the pack removes the row. Packs written
// before the row existed, or by another writer, have none.
const indexSumPrefix = "s/"

// objectHeaderPutter is an object store that uploads with headers, such as
// *Client.
type objectHeaderPutter interface {
	// PutObjectHeader uploads data to bucket/key with the given headers.
	PutObjectHeader(ctx context.Context, bucket, key string, data []byte, header http.Header) error
}

// putPackObject uploads a pack's bytes. With UploadChecksum and a client
// that takes headers, it sends their SHA-256 in the X-Amz-Checksum-Sha256
// header, so the service refuses a body that does not match it.
func (s *PackStore) putPackObject(ctx context.Context, key string, data, sum []byte) error {
	if putter, ok := s.client.(objectHeaderPutter); ok && s.uploadChecksum {
		return putter.PutObjectHeader(ctx, s.bucket, key, data, http.Header{
			"Content-Type":       {"application/octet-stream"},
			ChecksumSHA256Header: {base64.StdEncoding.EncodeToString(sum)},
		})
	}
	return s.client.PutObject(ctx, s.bucket, key, data, "application/octet-stream")
}

// putSum records the SHA-256 of a pack's uploaded bytes in its s/ row.
func (x *packIndex) putSum(ctx context.Context, id string, sum []byte) error {
	// A read-only index records nothing.
	if x.readOnly {
		return nil
	}

	// Write the row in its own transaction.
	tx, err := x.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := tx.Set(ctx, []byte(indexSumPrefix+id), sum); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// sum returns the recorded SHA-256 of a pack's bytes, or nil for none.
func (x *packIndex) sum(ctx context.Context, id string) ([]byte, error) {
	// Read the row; a malformed one is an error, not a missing sum.
	tx, err := x.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, []byte(indexSumPrefix+id))
	if err != nil || !found {
		return nil, err
	}
	if len(data) != 32 {
		return nil, errors.Errorf("pack sum of %d bytes", len(data))
	}
	return bytes.Clone(data), nil
}
