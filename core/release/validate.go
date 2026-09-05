package release

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// Validate checks the channel directory for required channel entries.
func (d *ChannelDirectory) Validate() error {
	// A directory advertises at least one unambiguous channel selection.
	if d == nil {
		return errors.New("nil channel directory")
	}
	if len(d.GetChannels()) == 0 {
		return errors.New("no channels")
	}

	// Duplicate keys would make channel resolution depend on iteration order.
	seen := make(map[string]struct{}, len(d.GetChannels()))
	for i, entry := range d.GetChannels() {
		if err := entry.Validate(); err != nil {
			return errors.Wrapf(err, "validate channel entry %d", i)
		}
		key := entry.GetChannelKey()
		if _, ok := seen[key]; ok {
			return errors.Errorf("duplicate channel key %q", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// ValidateReleaseMetadataRefs checks that every channel points at available release metadata.
func (d *ChannelDirectory) ValidateReleaseMetadataRefs(hasRef func(*block.BlockRef) bool) error {
	// Structural validation precedes the caller's storage availability check.
	if err := d.Validate(); err != nil {
		return err
	}
	if hasRef == nil {
		return errors.New("nil metadata ref checker")
	}

	// Every advertised channel must resolve within the exported closure.
	for _, entry := range d.GetChannels() {
		if !hasRef(entry.GetReleaseMetadataRef()) {
			return errors.Errorf("missing release metadata for channel %q", entry.GetChannelKey())
		}
	}
	return nil
}

// Validate checks the channel entry for a channel key and release metadata ref.
func (e *ChannelEntry) Validate() error {
	// Both selection and content identity are required for an advertised channel.
	if e == nil {
		return errors.New("nil channel entry")
	}
	if strings.TrimSpace(e.GetChannelKey()) == "" {
		return errors.New("missing channel key")
	}
	if err := validateBlockRef(e.GetReleaseMetadataRef()); err != nil {
		return errors.Wrap(err, "invalid release metadata ref")
	}
	return nil
}

// Validate checks release identity and manifest references. Browser shell
// metadata is optional for native-only releases and validated when present.
func (m *ReleaseMetadata) Validate() error {
	// Identity fields bind manifests to one application release and channel.
	if m == nil {
		return errors.New("nil release metadata")
	}
	if strings.TrimSpace(m.GetProjectId()) == "" {
		return errors.New("missing project id")
	}
	if strings.TrimSpace(m.GetVersion()) == "" {
		return errors.New("missing version")
	}
	if strings.TrimSpace(m.GetChannelKey()) == "" {
		return errors.New("missing channel key")
	}

	// Every application distributes at least one validated manifest.
	if len(m.GetManifestRefs()) == 0 {
		return errors.New("no bldr manifest refs")
	}
	for i, ref := range m.GetManifestRefs() {
		if err := ref.Validate(); err != nil {
			return errors.Wrapf(err, "validate manifest ref %d", i)
		}
	}

	// Native-only releases omit browser metadata entirely.
	if shell := m.GetBrowserShell(); shell != nil {
		if err := shell.Validate(); err != nil {
			return errors.Wrap(err, "validate browser shell")
		}
	}
	return nil
}

// Validate checks the browser shell metadata for required paths and assets.
func (m *BrowserShellMetadata) Validate() error {
	// Browser startup requires the entrypoint and both worker paths.
	if m == nil {
		return errors.New("nil browser shell metadata")
	}
	if strings.TrimSpace(m.GetVersion()) == "" {
		return errors.New("missing version")
	}
	if strings.TrimSpace(m.GetGenerationId()) == "" {
		return errors.New("missing generation id")
	}
	if strings.TrimSpace(m.GetEntrypointPath()) == "" {
		return errors.New("missing entrypoint path")
	}
	if strings.TrimSpace(m.GetServiceWorkerPath()) == "" {
		return errors.New("missing service worker path")
	}
	if strings.TrimSpace(m.GetSharedWorkerPath()) == "" {
		return errors.New("missing shared worker path")
	}

	// Assets carry the content contract used by browser delivery.
	if len(m.GetAssets()) == 0 {
		return errors.New("no browser assets")
	}
	for i, asset := range m.GetAssets() {
		if err := asset.Validate(); err != nil {
			return errors.Wrapf(err, "validate asset %d", i)
		}
	}
	return nil
}

// Validate checks the browser asset for required content metadata.
func (a *BrowserAsset) Validate() error {
	// A content ref is optional for externally addressed browser assets.
	if a == nil {
		return errors.New("nil browser asset")
	}
	if strings.TrimSpace(a.GetPath()) == "" {
		return errors.New("missing path")
	}
	if ref := a.GetContentRef(); blockRefPresent(ref) {
		if err := validateBlockRef(ref); err != nil {
			return errors.Wrap(err, "invalid content ref")
		}
	}

	// Every asset still declares its size, digest, and response media type.
	if a.GetSize() == 0 {
		return errors.New("missing content size")
	}
	if len(a.GetSha256()) != 32 {
		return errors.New("invalid content sha256")
	}
	if strings.TrimSpace(a.GetContentType()) == "" {
		return errors.New("missing content type")
	}
	return nil
}

// Validate checks the update notification for required routing fields.
func (n *UpdateNotification) Validate() error {
	// Notifications name a concrete revision and its fetchable root pointer.
	if n == nil {
		return errors.New("nil update notification")
	}
	if strings.TrimSpace(n.GetChannelKey()) == "" {
		return errors.New("missing channel key")
	}
	if n.GetInnerSeqno() == 0 {
		return errors.New("missing inner seqno")
	}
	if strings.TrimSpace(n.GetRootPointerUrl()) == "" {
		return errors.New("missing root pointer url")
	}
	return nil
}

// validateBlockRef requires a nonempty content address for a release object.
func validateBlockRef(ref *block.BlockRef) error {
	if ref == nil {
		return errors.New("nil block ref")
	}
	if ref.GetEmpty() {
		return errors.New("empty block ref")
	}
	return ref.Validate(false)
}

// blockRefPresent distinguishes external assets from block-backed assets.
func blockRefPresent(ref *block.BlockRef) bool {
	return ref != nil && !ref.GetEmpty()
}
