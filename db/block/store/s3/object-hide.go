//go:build !tinygo

package block_store_s3

import (
	"context"
	"net/url"

	"github.com/pkg/errors"
)

// ObjectHider is an ObjectStore on a bucket that keeps old versions, which
// hides an object under a delete marker, B2's hide marker, and brings it back
// by deleting that marker, so a pack sweep sets a pack aside without copying
// it and without deleting any byte it holds.
//
// Invariants, each a contract of the methods below:
//
//   - A hide deletes no content version. Its only write is one delete
//     without a version id, which on a bucket whose versioning is Enabled
//     adds a marker and keeps every version under it. It refuses any other
//     bucket: with versioning off a plain delete removes the object, and
//     with versioning Suspended it replaces the key's null version.
//   - An unhide deletes only delete markers, each by the id the listing gave
//     it as a marker, and only those newer than the version it restores. It
//     refuses when a content version is newer than that version or when the
//     version is gone, and it checks after that the version is current.
//   - A read of a hidden version names its id, so it reads the bytes the hide
//     kept and never a later write to the key.
//   - Each method acts on a version listing only when the listing shows the
//     order it assumes, newest first: the first entry alone is IsLatest, and
//     each entry's LastModified is set and no later than the one before it.
//     The S3 API does not promise that order, so a listing that does not show
//     it is a refusal, ErrVersionOrder, before any write.
type ObjectHider interface {
	ObjectStore
	// HideObjectVersion hides key under a delete marker and returns the
	// content version the marker covers. A key already hidden returns the
	// version under its marker, so a repeated hide succeeds without a second
	// marker. A key with no content version returns ErrNotFound.
	HideObjectVersion(ctx context.Context, bucket, key string) (HiddenObject, error)
	// UnhideObjectVersion deletes the delete markers over version, making it
	// key's current version again. An empty version restores the newest
	// content version. A key whose version is already current succeeds.
	UnhideObjectVersion(ctx context.Context, bucket, key, version string) error
	// HiddenObjectVersion returns the content version under key's newest
	// delete marker, or "" when key is not hidden or holds no content.
	HiddenObjectVersion(ctx context.Context, bucket, key string) (string, error)
	// GetObjectVersionRange reads length bytes of one version of key from
	// offset off, as GetObjectRange reads the current one.
	GetObjectVersionRange(ctx context.Context, bucket, key, version string, off int64, length int) ([]byte, error)
}

// HiddenObject is an object a hide covered: the content version kept, and
// the delete marker over it.
type HiddenObject struct {
	// Version is the content version the marker covers.
	Version string
	// Marker is the delete marker's version id.
	Marker string
}

// ErrNotVersioned is returned by a hide on a bucket whose versioning is not
// Enabled, where a delete without a version id would lose the object.
var ErrNotVersioned = errors.New("the bucket does not keep old versions")

// HideObjectVersion hides key under a delete marker; see ObjectHider.
func (c *Client) HideObjectVersion(ctx context.Context, bucket, key string) (HiddenObject, error) {
	// Refuse a bucket where a plain delete loses the object.
	status, err := c.BucketVersioning(ctx, bucket)
	if err != nil {
		return HiddenObject{}, errors.Wrap(err, "read the bucket's versioning")
	}
	if status != "Enabled" {
		return HiddenObject{}, errors.Wrapf(ErrNotVersioned, "hide %s/%s: versioning is %q", bucket, key, status)
	}

	// Find the key hidden already, or its current version.
	versions, err := c.keyVersions(ctx, bucket, key)
	if err != nil {
		return HiddenObject{}, err
	}
	if hidden, ok := hiddenVersion(versions); ok {
		return hidden, nil
	}
	if len(versions) == 0 || versions[0].DeleteMarker {
		return HiddenObject{}, errors.Wrap(ErrNotFound, "hide "+bucket+"/"+key)
	}
	current := versions[0].VersionID

	// Add the marker, then check it covers the version the listing showed.
	if err := c.deleteObjectVersion(ctx, bucket, key, ""); err != nil {
		return HiddenObject{}, errors.Wrap(err, "hide "+bucket+"/"+key)
	}
	if versions, err = c.keyVersions(ctx, bucket, key); err != nil {
		return HiddenObject{}, err
	}
	hidden, ok := hiddenVersion(versions)
	if !ok || hidden.Version != current {
		return HiddenObject{}, errors.Errorf("hide %s/%s: version %s is not under the new marker", bucket, key, current)
	}
	return hidden, nil
}

// UnhideObjectVersion deletes the delete markers over version; see
// ObjectHider.
func (c *Client) UnhideObjectVersion(ctx context.Context, bucket, key, version string) error {
	// Find the markers newer than the version, refusing a newer content
	// version or a version that is gone.
	versions, err := c.keyVersions(ctx, bucket, key)
	if err != nil {
		return err
	}
	var markers []string
	found := false
	for _, v := range versions {
		if !v.DeleteMarker {
			if version != "" && v.VersionID != version {
				return errors.Errorf("unhide %s/%s: content version %s is newer than %s", bucket, key, v.VersionID, version)
			}
			version, found = v.VersionID, true
			break
		}
		markers = append(markers, v.VersionID)
	}
	if !found {
		return errors.Wrapf(ErrNotFound, "unhide %s/%s: no version %s", bucket, key, version)
	}

	// Delete each marker by its id, then check the version is current.
	for _, marker := range markers {
		if err := c.deleteObjectVersion(ctx, bucket, key, marker); err != nil {
			return errors.Wrapf(err, "unhide %s/%s: delete marker %s", bucket, key, marker)
		}
	}
	if versions, err = c.keyVersions(ctx, bucket, key); err != nil {
		return err
	}
	if len(versions) == 0 || versions[0].DeleteMarker || versions[0].VersionID != version {
		return errors.Errorf("unhide %s/%s: version %s is not current after the unhide", bucket, key, version)
	}
	return nil
}

// HiddenObjectVersion returns the version under key's newest marker; see
// ObjectHider.
func (c *Client) HiddenObjectVersion(ctx context.Context, bucket, key string) (string, error) {
	versions, err := c.keyVersions(ctx, bucket, key)
	if err != nil {
		return "", err
	}
	hidden, _ := hiddenVersion(versions)
	return hidden.Version, nil
}

// GetObjectVersionRange reads a range of one version; see ObjectHider.
func (c *Client) GetObjectVersionRange(ctx context.Context, bucket, key, version string, off int64, length int) ([]byte, error) {
	if version == "" {
		return nil, errors.Errorf("read %s/%s: no version named", bucket, key)
	}
	return c.getRange(ctx, bucket, key, url.Values{"versionId": {version}}, off, length)
}

// ErrVersionOrder is returned when a version listing does not show the
// newest-first order the hide and unhide act on.
var ErrVersionOrder = errors.New("the version listing is not newest first")

// keyVersions lists the versions and delete markers of key alone, newest
// first, refusing a listing that does not show that order.
func (c *Client) keyVersions(ctx context.Context, bucket, key string) ([]ObjectVersion, error) {
	// Keep the key's own entries; the prefix also lists longer keys.
	var out []ObjectVersion
	err := c.ListObjectVersions(ctx, bucket, key, func(v ObjectVersion) error {
		if v.Key == key {
			out = append(out, v)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "list the versions of "+bucket+"/"+key)
	}

	// Act only on a listing that shows its order.
	if err := checkVersionOrder(out); err != nil {
		return nil, errors.Wrapf(err, "list the versions of %s/%s", bucket, key)
	}
	return out, nil
}

// checkVersionOrder checks that versions, one key's listing, is newest first:
// only the first is IsLatest, and every LastModified is set and no later than
// the one before it. Equal times pass, since a hide in the same second as the
// write can list both with one time.
func checkVersionOrder(versions []ObjectVersion) error {
	for i, v := range versions {
		switch {
		case v.IsLatest != (i == 0):
			return errors.Wrapf(ErrVersionOrder, "entry %d (%s) has IsLatest %v", i, v.VersionID, v.IsLatest)
		case v.LastModified.IsZero():
			return errors.Wrapf(ErrVersionOrder, "entry %d (%s) has no LastModified", i, v.VersionID)
		case i > 0 && v.LastModified.After(versions[i-1].LastModified):
			return errors.Wrapf(ErrVersionOrder, "entry %d (%s) is newer than the entry before it", i, v.VersionID)
		}
	}
	return nil
}

// hiddenVersion returns the newest content version under the newest delete
// marker of a key's versions, newest first, and whether there is one.
func hiddenVersion(versions []ObjectVersion) (HiddenObject, bool) {
	if len(versions) == 0 || !versions[0].DeleteMarker {
		return HiddenObject{}, false
	}
	for _, v := range versions[1:] {
		if !v.DeleteMarker {
			return HiddenObject{Version: v.VersionID, Marker: versions[0].VersionID}, true
		}
	}
	return HiddenObject{}, false
}

// _ is a type assertion
var _ ObjectHider = (*Client)(nil)
