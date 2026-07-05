package order

import "github.com/s4wave/spacewave/db/block"

// AccessOrderManifestIdentity identifies the manifest a profile was recorded against.
type AccessOrderManifestIdentity struct {
	ManifestID      string
	PlatformID      string
	BuildType       string
	ManifestRootRef *block.BlockRef
	ManifestRev     uint64
}

// AccessOrderManifestIdentityFromRecord returns the manifest identity stored in record.
func AccessOrderManifestIdentityFromRecord(record *AccessOrderRecord) AccessOrderManifestIdentity {
	if record == nil {
		return AccessOrderManifestIdentity{}
	}
	return AccessOrderManifestIdentity{
		ManifestID:      record.GetManifestId(),
		PlatformID:      record.GetPlatformId(),
		BuildType:       record.GetBuildType(),
		ManifestRootRef: record.GetManifestRootRef(),
		ManifestRev:     record.GetManifestRev(),
	}
}

// MatchesRecord returns true when record was captured for the same manifest.
func (i AccessOrderManifestIdentity) MatchesRecord(record *AccessOrderRecord) bool {
	if record == nil {
		return false
	}
	if i.ManifestID != record.GetManifestId() {
		return false
	}
	if i.PlatformID != record.GetPlatformId() {
		return false
	}
	if i.BuildType != record.GetBuildType() {
		return false
	}
	if i.ManifestRev != record.GetManifestRev() {
		return false
	}
	return refKey(i.ManifestRootRef) == refKey(record.GetManifestRootRef())
}
