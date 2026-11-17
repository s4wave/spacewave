//go:build !tinygo

package block_store_s3

import (
	"context"

	"github.com/pkg/errors"
)

// PurgeObject deletes every version of key by its version id, delete markers
// included, calling deleted with each id after the bucket confirmed it. It
// never falls back to a plain delete, which on a bucket that keeps old versions
// only hides the object and leaves its bytes billed: where the versions cannot
// be listed it fails.
func (c *Client) PurgeObject(ctx context.Context, bucket, key string, deleted func(version string)) error {
	// List the versions, refusing a service or key that cannot.
	versions, err := c.ObjectVersions(ctx, bucket, key)
	if err != nil {
		return errors.Wrap(err, "list the versions of "+bucket+"/"+key)
	}

	// Delete each by its id.
	for _, version := range versions {
		if version == "" {
			return errors.Errorf("purge %s/%s: a version has no id", bucket, key)
		}
		if err := c.deleteObjectVersion(ctx, bucket, key, version); err != nil {
			return err
		}
		if deleted != nil {
			deleted(version)
		}
	}
	return nil
}
