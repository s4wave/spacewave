package release

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// Validate checks the channel directory for required channel entries.
func (d *ChannelDirectory) Validate() error {
	if d == nil {
		return errors.New("nil channel directory")
	}
	if len(d.GetChannels()) == 0 {
		return errors.New("no channels")
	}
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
	if err := d.Validate(); err != nil {
		return err
	}
	if hasRef == nil {
		return errors.New("nil metadata ref checker")
	}
	for _, entry := range d.GetChannels() {
		if !hasRef(entry.GetReleaseMetadataRef()) {
			return errors.Errorf("missing release metadata for channel %q", entry.GetChannelKey())
		}
	}
	return nil
}

// Validate checks the channel entry for a channel key and release metadata ref.
func (e *ChannelEntry) Validate() error {
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

// Validate checks the release metadata for required release-only fields.
func (m *ReleaseMetadata) Validate() error {
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
	if len(m.GetManifestRefs()) == 0 {
		return errors.New("no bldr manifest refs")
	}
	for i, ref := range m.GetManifestRefs() {
		if err := ref.Validate(); err != nil {
			return errors.Wrapf(err, "validate manifest ref %d", i)
		}
	}
	if err := m.GetBrowserShell().Validate(); err != nil {
		return errors.Wrap(err, "validate browser shell")
	}
	return nil
}

// Validate checks the browser shell metadata for required paths and assets.
func (m *BrowserShellMetadata) Validate() error {
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
	if strings.TrimSpace(m.GetWasmPath()) == "" {
		return errors.New("missing wasm path")
	}
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

func validateBlockRef(ref *block.BlockRef) error {
	if ref == nil {
		return errors.New("nil block ref")
	}
	if ref.GetEmpty() {
		return errors.New("empty block ref")
	}
	if err := ref.Validate(false); err != nil {
		return err
	}
	return nil
}

func blockRefPresent(ref *block.BlockRef) bool {
	return ref != nil && !ref.GetEmpty()
}

func isPlatformKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != ""
}
