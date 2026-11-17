//go:build !tinygo

package block_store_s3

import (
	"context"
	"slices"

	"github.com/pkg/errors"
)

// maxCopyConflicts is the conflicting keys a version copy lists.
const maxCopyConflicts = 20

// VersionCopyOptions configures CopyVersions.
type VersionCopyOptions struct {
	// Client reaches both buckets. A server-side copy runs under the
	// destination request's credentials, so its key must read the source
	// bucket and write the destination.
	Client *Client
	// FromBucket is the source bucket.
	FromBucket string
	// ToBucket is the destination bucket.
	ToBucket string
	// Prefixes are the source key prefixes copied.
	Prefixes []string
	// ToPrefix starts every destination key, before the source key.
	ToPrefix string
	// DryRun lists and counts what the copy would write, writing nothing.
	DryRun bool
}

// CopyVersions copies every version and delete marker of every key under the
// prefixes to the destination, each key's oldest first, so the destination's
// version listing matches the source's: each version is a server-side copy of
// that version, and each delete marker a delete without a version id, which
// adds a marker. A key whose destination already lists the same history is
// skipped, so a stopped copy resumes; one whose destination holds another
// history is left alone and counted as a conflict. It holds one key's
// versions in memory at a time.
func CopyVersions(ctx context.Context, opts *VersionCopyOptions) (*VersionCopyResult, error) {
	res := &VersionCopyResult{}
	for _, prefix := range opts.Prefixes {
		// Gather each key's versions, newest first as listed, and copy them
		// when the listing moves to the next key.
		var key string
		var versions []ObjectVersion
		err := opts.Client.ListObjectVersions(ctx, opts.FromBucket, prefix, func(v ObjectVersion) error {
			if v.Key != key && len(versions) != 0 {
				if err := copyKey(ctx, opts, res, versions); err != nil {
					return err
				}
				versions = versions[:0]
			}
			key = v.Key
			versions = append(versions, v)
			return nil
		})
		if err == nil && len(versions) != 0 {
			err = copyKey(ctx, opts, res, versions)
		}
		if err != nil {
			return res, errors.Wrap(err, "copy the versions under "+prefix)
		}
	}
	return res, nil
}

// copyKey copies one key's versions, listed newest first, oldest first.
func copyKey(ctx context.Context, opts *VersionCopyOptions, res *VersionCopyResult, versions []ObjectVersion) error {
	// Compare the destination's history.
	src := versions[0].Key
	dst := opts.ToPrefix + src
	have, err := keyVersions(ctx, opts.Client, opts.ToBucket, dst)
	if err != nil {
		return err
	}
	switch {
	case sameHistory(have, versions):
		res.Skipped++
		return nil
	case len(have) != 0:
		res.Conflicts++
		if len(res.ConflictKeys) < maxCopyConflicts {
			res.ConflictKeys = append(res.ConflictKeys, src)
		}
		return nil
	}

	// Count the key, then write its versions oldest first.
	res.Keys++
	for _, v := range slices.Backward(versions) {
		if v.DeleteMarker {
			res.Markers++
		} else {
			res.Versions++
			res.Bytes += uint64(max(v.Size, 0))
		}
		if opts.DryRun {
			continue
		}
		if v.DeleteMarker {
			err = opts.Client.HideObject(ctx, opts.ToBucket, dst)
		} else {
			err = opts.Client.CopyObjectVersion(ctx, opts.FromBucket, src, v.VersionID, opts.ToBucket, dst)
		}
		if err != nil {
			return errors.Wrap(err, "copy "+src+"@"+v.VersionID)
		}
	}
	return nil
}

// keyVersions lists the versions of key alone, newest first.
func keyVersions(ctx context.Context, c *Client, bucket, key string) ([]ObjectVersion, error) {
	var out []ObjectVersion
	err := c.ListObjectVersions(ctx, bucket, key, func(v ObjectVersion) error {
		if v.Key == key {
			out = append(out, v)
		}
		return nil
	})
	return out, err
}

// sameHistory reports whether two histories, newest first, hold the same
// sequence of versions by size and markers.
func sameHistory(a, b []ObjectVersion) bool {
	return slices.EqualFunc(a, b, func(x, y ObjectVersion) bool {
		return x.DeleteMarker == y.DeleteMarker && x.Size == y.Size
	})
}
