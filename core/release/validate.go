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

// ValidateReleaseManifestRefs checks that every channel points at an available release manifest.
func (d *ChannelDirectory) ValidateReleaseManifestRefs(hasRef func(*block.BlockRef) bool) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if hasRef == nil {
		return errors.New("nil manifest ref checker")
	}
	for _, entry := range d.GetChannels() {
		if !hasRef(entry.GetReleaseManifestRef()) {
			return errors.Errorf("missing release manifest for channel %q", entry.GetChannelKey())
		}
	}
	return nil
}

// Validate checks the channel entry for a channel key and release manifest ref.
func (e *ChannelEntry) Validate() error {
	if e == nil {
		return errors.New("nil channel entry")
	}
	if strings.TrimSpace(e.GetChannelKey()) == "" {
		return errors.New("missing channel key")
	}
	if err := validateBlockRef(e.GetReleaseManifestRef()); err != nil {
		return errors.Wrap(err, "invalid release manifest ref")
	}
	return nil
}

// Validate checks the release manifest for required release graph refs.
func (m *ReleaseManifest) Validate() error {
	if m == nil {
		return errors.New("nil release manifest")
	}
	if strings.TrimSpace(m.GetProjectId()) == "" {
		return errors.New("missing project id")
	}
	if strings.TrimSpace(m.GetVersion()) == "" {
		return errors.New("missing version")
	}
	if len(m.GetEntrypoints()) == 0 {
		return errors.New("no entrypoints")
	}
	for platform, ref := range m.GetEntrypoints() {
		if !isPlatformKey(platform) {
			return errors.Errorf("unknown platform key %q", platform)
		}
		if err := ref.Validate(); err != nil {
			return errors.Wrapf(err, "validate entrypoint %q", platform)
		}
	}
	for plugin, ref := range m.GetPlugins() {
		if strings.TrimSpace(plugin) == "" {
			return errors.New("missing plugin id")
		}
		if err := ref.Validate(); err != nil {
			return errors.Wrapf(err, "validate plugin %q", plugin)
		}
	}
	if err := m.GetBrowserShell().Validate(); err != nil {
		return errors.Wrap(err, "validate browser shell")
	}
	return nil
}

// Validate checks the manifest ref for a block ref.
func (r *ManifestRef) Validate() error {
	if r == nil {
		return errors.New("nil manifest ref")
	}
	if err := validateBlockRef(r.GetRef()); err != nil {
		return errors.Wrap(err, "invalid block ref")
	}
	return nil
}

// Validate checks the entrypoint manifest for required artifact metadata.
func (m *EntrypointManifest) Validate() error {
	if m == nil {
		return errors.New("nil entrypoint manifest")
	}
	if !isPlatformKey(m.GetPlatform()) {
		return errors.Errorf("unknown platform key %q", m.GetPlatform())
	}
	if strings.TrimSpace(m.GetVersion()) == "" {
		return errors.New("missing version")
	}
	if err := validateBlockRef(m.GetArchiveRef()); err != nil {
		return errors.Wrap(err, "invalid archive ref")
	}
	if m.GetSize() == 0 {
		return errors.New("missing archive size")
	}
	if len(m.GetSha256()) != 32 {
		return errors.New("invalid archive sha256")
	}
	if strings.TrimSpace(m.GetArchiveName()) == "" {
		return errors.New("missing archive name")
	}
	return nil
}

// Validate checks the plugin manifest for required refs.
func (m *PluginManifest) Validate() error {
	if m == nil {
		return errors.New("nil plugin manifest")
	}
	if strings.TrimSpace(m.GetPluginId()) == "" {
		return errors.New("missing plugin id")
	}
	if strings.TrimSpace(m.GetVersion()) == "" {
		return errors.New("missing version")
	}
	if err := validateBlockRef(m.GetManifestRef()); err != nil {
		return errors.Wrap(err, "invalid manifest ref")
	}
	if m.GetArtifactRef() != nil {
		if err := validateBlockRef(m.GetArtifactRef()); err != nil {
			return errors.Wrap(err, "invalid artifact ref")
		}
	}
	return nil
}

// Validate checks the browser shell manifest for required paths and assets.
func (m *BrowserShellManifest) Validate() error {
	if m == nil {
		return errors.New("nil browser shell manifest")
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
	if err := validateBlockRef(a.GetContentRef()); err != nil {
		return errors.Wrap(err, "invalid content ref")
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
	h := ref.GetHash()
	if h == nil {
		return errors.New("nil hash")
	}
	if h.GetHashType() == 0 {
		return errors.New("missing hash type")
	}
	if len(h.GetHash()) == 0 {
		return errors.New("missing hash")
	}
	return nil
}

func isPlatformKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != ""
}
