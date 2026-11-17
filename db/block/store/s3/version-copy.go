//go:build !tinygo

package block_store_s3

import (
	"context"
	"slices"
	"strings"

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
// adds a marker. It first refuses a destination that keeps no versions, and
// one in the source bucket that the source prefixes cover, which the listing
// would reach and copy again. A key whose destination already lists the same
// history is skipped, and one whose destination lists the start of it, left
// by a stopped copy, is finished; one whose destination holds another history
// is left alone and counted as a conflict. After writing a key it lists the
// key again and fails unless the destination holds the source's history. It
// holds one key's versions in memory at a time.
func CopyVersions(ctx context.Context, opts *VersionCopyOptions) (*VersionCopyResult, error) {
	// Refuse a wrong destination before any write.
	if err := checkCopyTarget(ctx, opts); err != nil {
		return nil, err
	}

	// Copy each prefix's keys.
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

// checkCopyTarget refuses a destination in the source bucket whose keys a
// source prefix covers, or covering a source prefix, and a destination bucket
// whose versioning is not enabled, where each copy would replace the last.
func checkCopyTarget(ctx context.Context, opts *VersionCopyOptions) error {
	// Two key ranges given by prefixes overlap when one prefix starts the
	// other.
	if opts.FromBucket == opts.ToBucket {
		for _, from := range opts.Prefixes {
			to := opts.ToPrefix + from
			for _, src := range opts.Prefixes {
				if strings.HasPrefix(to, src) || strings.HasPrefix(src, to) {
					return errors.Errorf("destination %q overlaps source prefix %q in bucket %s", to, src, opts.ToBucket)
				}
			}
		}
	}

	// Require a destination that keeps every version.
	status, err := opts.Client.BucketVersioning(ctx, opts.ToBucket)
	if err != nil {
		return errors.Wrap(err, "read the destination's versioning")
	}
	if status != "Enabled" {
		return errors.Errorf("destination bucket %s does not keep versions (versioning %q)", opts.ToBucket, status)
	}
	return nil
}

// copyKey copies one key's versions, listed newest first, oldest first,
// after any the destination already holds.
func copyKey(ctx context.Context, opts *VersionCopyOptions, res *VersionCopyResult, versions []ObjectVersion) error {
	// Order the source history oldest first, and note its largest version.
	src := versions[0].Key
	dst := opts.ToPrefix + src
	want := slices.Clone(versions)
	slices.Reverse(want)
	for _, v := range want {
		res.Largest = max(res.Largest, uint64(max(v.Size, 0)))
	}

	// Compare the destination's history: the same, its start, or another.
	have, err := keyHistory(ctx, opts.Client, opts.ToBucket, dst)
	if err != nil {
		return err
	}
	switch {
	case slices.EqualFunc(have, want, sameVersion):
		res.Skipped++
		return nil
	case len(have) < len(want) && slices.EqualFunc(have, want[:len(have)], sameVersion):
		if len(have) != 0 {
			res.Resumed++
		}
	default:
		res.Conflicts++
		if len(res.ConflictKeys) < maxCopyConflicts {
			res.ConflictKeys = append(res.ConflictKeys, src)
		}
		return nil
	}

	// Count the key, then write the versions it lacks, oldest first.
	res.Keys++
	for _, v := range want[len(have):] {
		if err := copyVersion(ctx, opts, res, src, dst, v); err != nil {
			return err
		}
	}
	if opts.DryRun {
		return nil
	}

	// Check that the destination now holds the source's history.
	have, err = keyHistory(ctx, opts.Client, opts.ToBucket, dst)
	if err != nil {
		return err
	}
	if !slices.EqualFunc(have, want, sameVersion) {
		return errors.Errorf("%s/%s lists %d versions after the copy, not the source's %d", opts.ToBucket, dst, len(have), len(want))
	}
	return nil
}

// copyVersion counts one version and, unless the copy is a dry run, writes
// it to dst: a server-side copy of a version, or a delete marker.
func copyVersion(ctx context.Context, opts *VersionCopyOptions, res *VersionCopyResult, src, dst string, v ObjectVersion) error {
	// Count it.
	if v.DeleteMarker {
		res.Markers++
	} else {
		res.Versions++
		res.Bytes += uint64(max(v.Size, 0))
	}
	if opts.DryRun {
		return nil
	}

	// Write it.
	var err error
	if v.DeleteMarker {
		err = opts.Client.HideObject(ctx, opts.ToBucket, dst)
	} else {
		err = opts.Client.CopyObjectVersion(ctx, opts.FromBucket, src, v.VersionID, opts.ToBucket, dst)
	}
	return errors.Wrap(err, "copy "+src+"@"+v.VersionID)
}

// keyHistory lists the versions of key alone, oldest first.
func keyHistory(ctx context.Context, c *Client, bucket, key string) ([]ObjectVersion, error) {
	var out []ObjectVersion
	err := c.ListObjectVersions(ctx, bucket, key, func(v ObjectVersion) error {
		if v.Key == key {
			out = append(out, v)
		}
		return nil
	})
	slices.Reverse(out)
	return out, err
}

// sameVersion reports whether two listed versions match: both delete
// markers, or versions of one size whose entity tags agree. A single-request
// copy keeps a single-part object's MD5 tag, so two such tags must be equal;
// a multipart tag, or a missing one, is compared by size alone.
func sameVersion(x, y ObjectVersion) bool {
	if x.DeleteMarker != y.DeleteMarker || x.Size != y.Size {
		return false
	}
	if x.DeleteMarker || !md5ETag(x.ETag) || !md5ETag(y.ETag) {
		return true
	}
	return x.ETag == y.ETag
}

// md5ETag reports whether an entity tag is a single-part object's MD5.
func md5ETag(tag string) bool {
	return tag != "" && !strings.Contains(tag, "-")
}
