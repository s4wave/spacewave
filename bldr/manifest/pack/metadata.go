package bldr_manifest_pack

import (
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/identity"
	"github.com/s4wave/spacewave/db/bucket"
)

const (
	// MetadataFormatVersion is the current manifest-pack metadata schema.
	MetadataFormatVersion uint32 = 1
	// PackSHA256Length is the byte length of a SHA-256 pack digest.
	PackSHA256Length = 32
)

// Validate validates the manifest-pack metadata.
func (m *ManifestPackMetadata) Validate() error {
	if m.GetFormatVersion() != MetadataFormatVersion {
		return errors.Errorf("format_version must be %d", MetadataFormatVersion)
	}
	if m.GetGitSha() == "" {
		return errors.New("git_sha is empty")
	}
	if m.GetBuildType() == "" {
		return errors.New("build_type is empty")
	}
	if m.GetProducerTarget() == "" {
		return errors.New("producer_target is empty")
	}
	if m.GetCacheSchema() == "" {
		return errors.New("cache_schema is empty")
	}
	if len(m.GetManifests()) == 0 {
		return errors.New("manifests is empty")
	}
	for i, tuple := range m.GetManifests() {
		if err := tuple.Validate(); err != nil {
			return errors.Wrapf(err, "manifests[%d]", i)
		}
	}
	if err := ValidateCleanObjectRef("manifest_bundle_ref", m.GetManifestBundleRef()); err != nil {
		return err
	}
	if err := validatePackEntry(m.GetPack()); err != nil {
		return errors.Wrap(err, "pack")
	}
	if len(m.GetPackSha256()) != PackSHA256Length {
		return errors.New("pack_sha256 must be 32 bytes")
	}
	return nil
}

// Validate validates the manifest tuple.
func (m *ManifestTuple) Validate() error {
	return m.validate(true)
}

// ValidateRequest validates a manifest tuple before FetchManifest resolves rev.
func (m *ManifestTuple) ValidateRequest() error {
	return m.validate(false)
}

func (m *ManifestTuple) validate(requireRev bool) error {
	if err := bldr_manifest.ValidateManifestID(m.GetManifestId(), false); err != nil {
		return errors.Wrap(err, "manifest_id")
	}
	if m.GetPlatformId() == "" {
		return errors.New("platform_id is empty")
	}
	if requireRev && m.GetRev() == 0 {
		return errors.New("rev is empty")
	}
	if m.GetObjectKey() == "" {
		return errors.New("object_key is empty")
	}
	for i, key := range m.GetLinkObjectKeys() {
		if key == "" {
			return errors.Errorf("link_object_keys[%d] is empty", i)
		}
	}
	return nil
}

// ValidateCleanObjectRef validates a manifest-pack object ref and rejects
// transform configuration pointers that must not cross the CI artifact boundary.
func ValidateCleanObjectRef(name string, ref *bucket.ObjectRef) error {
	if ref.GetRootRef().GetEmpty() {
		return errors.Errorf("%s root_ref is empty", name)
	}
	if err := ref.Validate(); err != nil {
		return errors.Wrap(err, name)
	}
	if !ref.GetTransformConf().GetEmpty() {
		return errors.Errorf("%s contains inline transform config", name)
	}
	if !ref.GetTransformConfRef().GetEmpty() {
		return errors.Errorf("%s contains transform config ref", name)
	}
	return nil
}

func validatePackEntry(entry *packfile.PackfileEntry) error {
	if entry == nil {
		return errors.New("is nil")
	}
	if err := identity.ValidatePackID(entry.GetId()); err != nil {
		return errors.Wrap(err, "id")
	}
	if len(entry.GetBloomFilter()) == 0 {
		return errors.New("bloom_filter is empty")
	}
	if entry.GetBloomFormatVersion() != packfile.BloomFormatVersionV1 {
		return errors.New("bloom_format_version is invalid")
	}
	if entry.GetBlockCount() == 0 {
		return errors.New("block_count is empty")
	}
	if entry.GetSizeBytes() == 0 {
		return errors.New("size_bytes is empty")
	}
	if err := entry.GetCreatedAt().Validate(false); err != nil {
		return errors.Wrap(err, "created_at")
	}
	if entry.GetSupersededBy() != "" || entry.GetSupersededAt() != nil {
		return errors.New("pack entry is superseded")
	}
	return nil
}
