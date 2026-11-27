package bldr_manifest

import (
	b58 "github.com/mr-tron/base58/base58"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	"github.com/sirupsen/logrus"
)

// NewManifestMeta constructs a new ManifestMeta.
func NewManifestMeta(manifestID string, buildType BuildType, platformID string, rev uint64) *ManifestMeta {
	return &ManifestMeta{
		ManifestId: manifestID,
		BuildType:  string(buildType),
		PlatformId: platformID,
		Rev:        rev,
	}
}

// Resolve resolves the platform ID to a fully qualified ID.
//
// returns a copy of the meta object w/ the platform id set.
func (m *ManifestMeta) Resolve() (*ManifestMeta, bldr_platform.Platform, error) {
	// Copy the manifest metadata before qualifying its platform ID.
	meta := m.CloneVT()

	// parse platform id
	buildPlatform, err := bldr_platform.ParsePlatform(meta.GetPlatformId())
	if err != nil {
		return nil, nil, err
	}
	meta.PlatformId = buildPlatform.GetPlatformID()
	return meta, buildPlatform, nil
}

// Validate validates the ManifestMeta.
func (m *ManifestMeta) Validate(allowEmpty bool) error {
	if err := ValidateManifestID(m.GetManifestId(), allowEmpty); err != nil {
		return err
	}
	return nil
}

// MarshalB58 marshals the meta to a b58 string.
func (m *ManifestMeta) MarshalB58() string {
	dat, _ := m.MarshalVT()
	return b58.Encode(dat)
}

// Logger adds logging fields for the meta.
func (m *ManifestMeta) Logger(le *logrus.Entry) *logrus.Entry {
	// Collect the populated manifest metadata fields for the logger.
	fields := logrus.Fields{}
	if manifestID := m.GetManifestId(); manifestID != "" {
		fields["manifest-id"] = manifestID
	}
	if buildType := m.GetBuildType(); buildType != "" {
		fields["build-type"] = buildType
	}
	if platformID := m.GetPlatformId(); platformID != "" {
		fields["platform-id"] = platformID
	}
	if rev := m.GetRev(); rev != 0 {
		fields["manifest-rev"] = rev
	}
	if len(fields) == 0 {
		return le
	}
	return le.WithFields(fields)
}
