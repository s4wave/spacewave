package release

import (
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

const (
	channelDirectoryRefBase   = 1000
	releaseManifestBrowserRef = 1
	releaseManifestEntryBase  = 1000
	releaseManifestPluginBase = 2000
	entrypointManifestArchive = 1
	pluginManifestManifestRef = 1
	pluginManifestArtifactRef = 2
	browserShellAssetRefBase  = 1000
	browserAssetContentRef    = 1
	manifestRefRef            = 1
)

func (m *ChannelDirectory) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

func (m *ChannelDirectory) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

func (m *ChannelDirectory) ApplyBlockRef(id uint32, ref *block.BlockRef) error {
	idx := int(id) - channelDirectoryRefBase
	if idx < 0 || idx >= len(m.GetChannels()) {
		return errors.Errorf("unknown channel directory ref id %d", id)
	}
	m.Channels[idx].ReleaseManifestRef = ref
	return nil
}

func (m *ChannelDirectory) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	refs := make(map[uint32]*block.BlockRef, len(m.GetChannels()))
	for i, entry := range m.GetChannels() {
		refs[uint32(channelDirectoryRefBase+i)] = entry.GetReleaseManifestRef()
	}
	return refs, nil
}

func (m *ChannelDirectory) GetBlockRefCtor(uint32) block.Ctor {
	return nil
}

func (m *ReleaseManifest) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

func (m *ReleaseManifest) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

func (m *ReleaseManifest) ApplyBlockRef(id uint32, ref *block.BlockRef) error {
	switch {
	case id == releaseManifestBrowserRef:
		if m.BrowserShell == nil {
			m.BrowserShell = &ManifestRef{}
		}
		m.BrowserShell.Ref = ref
		return nil
	case id >= releaseManifestEntryBase && id < releaseManifestPluginBase:
		keys := sortedManifestRefKeys(m.GetEntrypoints())
		idx := int(id) - releaseManifestEntryBase
		if idx < 0 || idx >= len(keys) {
			return errors.Errorf("unknown entrypoint ref id %d", id)
		}
		m.Entrypoints[keys[idx]].Ref = ref
		return nil
	case id >= releaseManifestPluginBase:
		keys := sortedManifestRefKeys(m.GetPlugins())
		idx := int(id) - releaseManifestPluginBase
		if idx < 0 || idx >= len(keys) {
			return errors.Errorf("unknown plugin ref id %d", id)
		}
		m.Plugins[keys[idx]].Ref = ref
		return nil
	default:
		return errors.Errorf("unknown release manifest ref id %d", id)
	}
}

func (m *ReleaseManifest) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	refs := make(map[uint32]*block.BlockRef, len(m.GetEntrypoints())+len(m.GetPlugins())+1)
	refs[releaseManifestBrowserRef] = m.GetBrowserShell().GetRef()
	for i, key := range sortedManifestRefKeys(m.GetEntrypoints()) {
		refs[uint32(releaseManifestEntryBase+i)] = m.GetEntrypoints()[key].GetRef()
	}
	for i, key := range sortedManifestRefKeys(m.GetPlugins()) {
		refs[uint32(releaseManifestPluginBase+i)] = m.GetPlugins()[key].GetRef()
	}
	return refs, nil
}

func (m *ReleaseManifest) GetBlockRefCtor(uint32) block.Ctor {
	return nil
}

func (m *ManifestRef) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

func (m *ManifestRef) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

func (m *ManifestRef) ApplyBlockRef(id uint32, ref *block.BlockRef) error {
	if id != manifestRefRef {
		return errors.Errorf("unknown manifest ref id %d", id)
	}
	m.Ref = ref
	return nil
}

func (m *ManifestRef) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return map[uint32]*block.BlockRef{manifestRefRef: m.GetRef()}, nil
}

func (m *ManifestRef) GetBlockRefCtor(uint32) block.Ctor {
	return nil
}

func (m *EntrypointManifest) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

func (m *EntrypointManifest) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

func (m *EntrypointManifest) ApplyBlockRef(id uint32, ref *block.BlockRef) error {
	if id != entrypointManifestArchive {
		return errors.Errorf("unknown entrypoint manifest ref id %d", id)
	}
	m.ArchiveRef = ref
	return nil
}

func (m *EntrypointManifest) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return map[uint32]*block.BlockRef{entrypointManifestArchive: m.GetArchiveRef()}, nil
}

func (m *EntrypointManifest) GetBlockRefCtor(uint32) block.Ctor {
	return nil
}

func (m *PluginManifest) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

func (m *PluginManifest) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

func (m *PluginManifest) ApplyBlockRef(id uint32, ref *block.BlockRef) error {
	switch id {
	case pluginManifestManifestRef:
		m.ManifestRef = ref
	case pluginManifestArtifactRef:
		m.ArtifactRef = ref
	default:
		return errors.Errorf("unknown plugin manifest ref id %d", id)
	}
	return nil
}

func (m *PluginManifest) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return map[uint32]*block.BlockRef{
		pluginManifestManifestRef: m.GetManifestRef(),
		pluginManifestArtifactRef: m.GetArtifactRef(),
	}, nil
}

func (m *PluginManifest) GetBlockRefCtor(uint32) block.Ctor {
	return nil
}

func (m *BrowserShellManifest) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

func (m *BrowserShellManifest) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

func (m *BrowserShellManifest) ApplyBlockRef(id uint32, ref *block.BlockRef) error {
	idx := int(id) - browserShellAssetRefBase
	if idx < 0 || idx >= len(m.GetAssets()) {
		return errors.Errorf("unknown browser shell asset ref id %d", id)
	}
	m.Assets[idx].ContentRef = ref
	return nil
}

func (m *BrowserShellManifest) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	refs := make(map[uint32]*block.BlockRef, len(m.GetAssets()))
	for i, asset := range m.GetAssets() {
		refs[uint32(browserShellAssetRefBase+i)] = asset.GetContentRef()
	}
	return refs, nil
}

func (m *BrowserShellManifest) GetBlockRefCtor(uint32) block.Ctor {
	return nil
}

func (m *BrowserAsset) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

func (m *BrowserAsset) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

func (m *BrowserAsset) ApplyBlockRef(id uint32, ref *block.BlockRef) error {
	if id != browserAssetContentRef {
		return errors.Errorf("unknown browser asset ref id %d", id)
	}
	m.ContentRef = ref
	return nil
}

func (m *BrowserAsset) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return map[uint32]*block.BlockRef{browserAssetContentRef: m.GetContentRef()}, nil
}

func (m *BrowserAsset) GetBlockRefCtor(uint32) block.Ctor {
	return nil
}

func (m *UpdateNotification) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

func (m *UpdateNotification) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

func sortedManifestRefKeys(refs map[string]*ManifestRef) []string {
	keys := make([]string, 0, len(refs))
	for key := range refs {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

var (
	_ block.Block         = (*ChannelDirectory)(nil)
	_ block.BlockWithRefs = (*ChannelDirectory)(nil)
	_ block.Block         = (*ReleaseManifest)(nil)
	_ block.BlockWithRefs = (*ReleaseManifest)(nil)
	_ block.Block         = (*ManifestRef)(nil)
	_ block.BlockWithRefs = (*ManifestRef)(nil)
	_ block.Block         = (*EntrypointManifest)(nil)
	_ block.BlockWithRefs = (*EntrypointManifest)(nil)
	_ block.Block         = (*PluginManifest)(nil)
	_ block.BlockWithRefs = (*PluginManifest)(nil)
	_ block.Block         = (*BrowserShellManifest)(nil)
	_ block.BlockWithRefs = (*BrowserShellManifest)(nil)
	_ block.Block         = (*BrowserAsset)(nil)
	_ block.BlockWithRefs = (*BrowserAsset)(nil)
	_ block.Block         = (*UpdateNotification)(nil)
)
