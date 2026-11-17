//go:build !tinygo

package block_store_s3_overlay

import "context"

// overlayVersion names the one version a rehearsal's purge reports per key.
const overlayVersion = "overlay"

// PurgeObject hides the object, as a rehearsal's purge of every version, and
// reports one version when the key read before. Nothing is deleted from the
// base.
func (s *Store) PurgeObject(ctx context.Context, bucket, key string, deleted func(version string)) error {
	// Find whether the key reads.
	name, err := s.path(bucket, key)
	if err != nil {
		return err
	}
	v, err := s.lookup(name, key)
	if err != nil {
		return err
	}
	exists := v.file != nil
	if exists {
		_ = v.file.Close()
	} else if !v.hidden {
		if exists, err = s.baseHas(ctx, v.target); err != nil {
			return err
		}
	}
	if !exists {
		return nil
	}

	// Hide it and report it.
	if err := writeRecord(name, kindHidden, nil); err != nil {
		return err
	}
	if deleted != nil {
		deleted(overlayVersion)
	}
	return nil
}
