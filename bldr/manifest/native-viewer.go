package bldr_manifest

import (
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pkg/errors"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	"github.com/s4wave/spacewave/db/block"
	native_viewer "github.com/s4wave/spacewave/sdk/viewer/native"
)

const nativeViewerProtocolVersion uint32 = native_viewer.NativeViewerProtocolVersion

func validateNativeViewerID(name, value string) error {
	if value == "" {
		return errors.Errorf("%s cannot be empty", name)
	}
	if !utf8.ValidString(value) || len(value) > 128 {
		return errors.Errorf("%s must be valid UTF-8 of at most 128 bytes", name)
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) || strings.ContainsRune("/\\", r) {
			return errors.Errorf("%s contains unsafe characters", name)
		}
	}
	return nil
}

func validateNativeViewerTypeID(value string) error {
	if value == "" || !utf8.ValidString(value) || len(value) > 128 {
		return errors.New("viewer_type_id must be valid UTF-8 of at most 128 bytes")
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return errors.New("viewer_type_id contains unsafe characters")
		}
	}
	return nil
}

func validateNativeEntrypoint(entrypoint string) error {
	if !utf8.ValidString(entrypoint) || strings.ContainsRune(entrypoint, '\x00') || strings.ContainsRune(entrypoint, '\\') {
		return errors.New("entrypoint is not safe")
	}
	if path.IsAbs(entrypoint) || path.Clean(entrypoint) != entrypoint || entrypoint == "." || strings.HasSuffix(entrypoint, "/") {
		return errors.New("entrypoint must be a safe relative regular file path")
	}
	for part := range strings.SplitSeq(entrypoint, "/") {
		if part == "" || part == "." || part == ".." {
			return errors.New("entrypoint must be a safe relative regular file path")
		}
	}
	return nil
}

func validateNativeObjectKey(value string) error {
	if value == "" {
		return errors.New("cannot be empty")
	}
	if !utf8.ValidString(value) || len(value) > 1000 {
		return errors.New("must be valid UTF-8 of at most 1000 bytes")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return errors.New("contains control characters")
		}
	}
	return nil
}

func validateNativeViewerMetadata(m *Manifest) error {
	meta := m.GetMeta()
	hasMetadata := meta.GetViewerId() != "" || meta.GetViewerTypeId() != "" || meta.GetViewerProfile() != "" || meta.GetViewerProtocolVersion() != 0
	if !hasMetadata {
		return nil
	}
	if err := validateNativeViewerID("viewer_id", meta.GetViewerId()); err != nil {
		return err
	}
	if err := validateNativeViewerTypeID(meta.GetViewerTypeId()); err != nil {
		return err
	}
	if err := validateNativeViewerID("viewer_profile", meta.GetViewerProfile()); err != nil {
		return err
	}
	if meta.GetViewerProtocolVersion() != nativeViewerProtocolVersion {
		return errors.Errorf("viewer_protocol_version must be %d", nativeViewerProtocolVersion)
	}
	platform, err := bldr_platform.ParsePlatform(meta.GetPlatformId())
	if err != nil || platform.GetBasePlatformID() != bldr_platform.PlatformID_DESKTOP || bldr_platform.IsWebPlatform(platform) {
		return errors.New("native viewer requires a desktop platform")
	}
	return validateNativeEntrypoint(m.GetEntrypoint())
}

// NativeViewerResolution is the frozen identity set used to start a native viewer.
type NativeViewerResolution struct {
	// pluginID identifies the plugin manifest.
	PluginID string
	// manifestObjectKey identifies the selected manifest object.
	ManifestObjectKey string
	// manifestDigest identifies the selected manifest root digest.
	ManifestDigest string
	// viewerID identifies the native viewer declared by the manifest.
	ViewerID string
	// viewerTypeID identifies the native viewer implementation type.
	ViewerTypeID string
	// viewerProfile selects the native viewer profile.
	ViewerProfile string
	// protocolVersion identifies the native viewer protocol.
	ProtocolVersion uint32
	// entrypoint is the safe relative viewer executable path.
	Entrypoint string
	// platformID identifies the canonical host platform.
	PlatformID string
}

// ResolveNativeViewer validates and freezes native viewer identities for a selected manifest reference.
func ResolveNativeViewer(
	manifest *Manifest,
	selected *ManifestRef,
	root *block.BlockRef,
	manifestObjectKey string,
	host bldr_platform.Platform,
) (*NativeViewerResolution, error) {
	if manifest == nil || selected == nil || root == nil || host == nil {
		return nil, errors.New("manifest, selected reference, and host platform are required")
	}
	if err := manifest.Validate(); err != nil {
		return nil, errors.Wrap(err, "manifest")
	}
	if err := validateNativeObjectKey(manifestObjectKey); err != nil {
		return nil, errors.Wrap(err, "manifest object key")
	}
	meta := manifest.GetMeta()
	if meta.GetViewerId() == "" {
		return nil, errors.New("manifest does not describe a native viewer")
	}
	manifestPlatform, err := bldr_platform.ParsePlatform(meta.GetPlatformId())
	if err != nil || manifestPlatform.GetPlatformID() != host.GetPlatformID() {
		return nil, errors.New("manifest platform does not match host platform")
	}
	if !selected.GetMeta().EqualVT(meta) {
		return nil, errors.New("selected manifest metadata does not match manifest")
	}
	if selected.GetManifestRef().GetRootRef().GetEmpty() || !selected.GetManifestRef().GetRootRef().EqualsRef(root) {
		return nil, errors.New("selected manifest root digest does not match")
	}
	// The selected root is the canonical manifest identity; preserve its exact bytes.
	manifestIdentity := root.GetHash().MarshalString()
	if manifestIdentity == "" {
		return nil, errors.New("selected manifest reference has no root digest")
	}
	return &NativeViewerResolution{
		PluginID:          meta.GetManifestId(),
		ManifestObjectKey: manifestObjectKey,
		ManifestDigest:    manifestIdentity,
		ViewerID:          meta.GetViewerId(),
		ViewerTypeID:      meta.GetViewerTypeId(),
		ViewerProfile:     meta.GetViewerProfile(),
		ProtocolVersion:   meta.GetViewerProtocolVersion(),
		Entrypoint:        manifest.GetEntrypoint(),
		PlatformID:        manifestPlatform.GetPlatformID(),
	}, nil
}
