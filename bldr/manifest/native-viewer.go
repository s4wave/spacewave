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

// validateNativeViewerID requires a non-empty bounded UTF-8 ID without whitespace, controls, or path separators.
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

// validateNativeViewerTypeID requires a non-empty bounded UTF-8 ID without whitespace or controls.
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

// validateNativeEntrypoint requires a clean relative path without NULs, backslashes, or traversal components.
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

// validateNativeObjectKey requires a non-empty bounded UTF-8 object key without control characters.
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

// validateNativeViewerMetadata requires complete native viewer metadata for the current protocol and a desktop platform.
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
	pluginID string
	// manifestObjectKey identifies the selected manifest object.
	manifestObjectKey string
	// manifestDigest identifies the selected manifest root digest.
	manifestDigest string
	// viewerID identifies the native viewer declared by the manifest.
	viewerID string
	// viewerTypeID identifies the native viewer implementation type.
	viewerTypeID string
	// viewerProfile selects the native viewer profile.
	viewerProfile string
	// protocolVersion identifies the native viewer protocol.
	protocolVersion uint32
	// entrypoint is the safe relative viewer executable path.
	entrypoint string
	// platformID identifies the canonical host platform.
	platformID string
}

// PluginID returns the frozen plugin ID.
func (r *NativeViewerResolution) PluginID() string {
	if r == nil {
		return ""
	}
	return r.pluginID
}

// ManifestObjectKey returns the frozen manifest object key.
func (r *NativeViewerResolution) ManifestObjectKey() string {
	if r == nil {
		return ""
	}
	return r.manifestObjectKey
}

// ManifestDigest returns the frozen manifest digest.
func (r *NativeViewerResolution) ManifestDigest() string {
	if r == nil {
		return ""
	}
	return r.manifestDigest
}

// ViewerID returns the frozen viewer ID.
func (r *NativeViewerResolution) ViewerID() string {
	if r == nil {
		return ""
	}
	return r.viewerID
}

// ViewerTypeID returns the frozen viewer type ID.
func (r *NativeViewerResolution) ViewerTypeID() string {
	if r == nil {
		return ""
	}
	return r.viewerTypeID
}

// ViewerProfile returns the frozen viewer profile.
func (r *NativeViewerResolution) ViewerProfile() string {
	if r == nil {
		return ""
	}
	return r.viewerProfile
}

// ProtocolVersion returns the frozen protocol version.
func (r *NativeViewerResolution) ProtocolVersion() uint32 {
	if r == nil {
		return 0
	}
	return r.protocolVersion
}

// Entrypoint returns the frozen entrypoint.
func (r *NativeViewerResolution) Entrypoint() string {
	if r == nil {
		return ""
	}
	return r.entrypoint
}

// PlatformID returns the frozen platform ID.
func (r *NativeViewerResolution) PlatformID() string {
	if r == nil {
		return ""
	}
	return r.platformID
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
		pluginID:          meta.GetManifestId(),
		manifestObjectKey: manifestObjectKey,
		manifestDigest:    manifestIdentity,
		viewerID:          meta.GetViewerId(),
		viewerTypeID:      meta.GetViewerTypeId(),
		viewerProfile:     meta.GetViewerProfile(),
		protocolVersion:   meta.GetViewerProtocolVersion(),
		entrypoint:        manifest.GetEntrypoint(),
		platformID:        manifestPlatform.GetPlatformID(),
	}, nil
}
