package release

import (
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

const (
	channelDirectoryRefBase       = 1000
	releaseMetadataArchiveRefBase = 1000
	releaseMetadataAssetRefBase   = 10000
	desktopArchiveRef            = 1
	browserShellAssetRefBase      = 1000
	browserAssetContentRef        = 1
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
	m.Channels[idx].ReleaseMetadataRef = ref
	return nil
}

func (m *ChannelDirectory) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	refs := make(map[uint32]*block.BlockRef, len(m.GetChannels()))
	for i, entry := range m.GetChannels() {
		refs[uint32(channelDirectoryRefBase+i)] = entry.GetReleaseMetadataRef()
	}
	return refs, nil
}

func (m *ChannelDirectory) GetBlockRefCtor(uint32) block.Ctor {
	return nil
}

func (m *ReleaseMetadata) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

func (m *ReleaseMetadata) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

func (m *ReleaseMetadata) ApplyBlockRef(id uint32, ref *block.BlockRef) error {
	switch {
	case id >= releaseMetadataArchiveRefBase && id < releaseMetadataAssetRefBase:
		keys := sortedDesktopArchiveKeys(m.GetDesktopArchives())
		idx := int(id) - releaseMetadataArchiveRefBase
		if idx < 0 || idx >= len(keys) {
			return errors.Errorf("unknown desktop archive ref id %d", id)
		}
		m.DesktopArchives[keys[idx]].ArchiveRef = ref
		return nil
	case id >= releaseMetadataAssetRefBase:
		idx := int(id) - releaseMetadataAssetRefBase
		if idx < 0 || idx >= len(m.GetBrowserShell().GetAssets()) {
			return errors.Errorf("unknown browser asset ref id %d", id)
		}
		m.BrowserShell.Assets[idx].ContentRef = ref
		return nil
	default:
		return errors.Errorf("unknown release metadata ref id %d", id)
	}
}

func (m *ReleaseMetadata) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	refs := make(map[uint32]*block.BlockRef, len(m.GetDesktopArchives())+len(m.GetBrowserShell().GetAssets()))
	for i, key := range sortedDesktopArchiveKeys(m.GetDesktopArchives()) {
		refs[uint32(releaseMetadataArchiveRefBase+i)] = m.GetDesktopArchives()[key].GetArchiveRef()
	}
	for i, asset := range m.GetBrowserShell().GetAssets() {
		refs[uint32(releaseMetadataAssetRefBase+i)] = asset.GetContentRef()
	}
	return refs, nil
}

func (m *ReleaseMetadata) GetBlockRefCtor(uint32) block.Ctor {
	return nil
}

func (m *DesktopArchive) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

func (m *DesktopArchive) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

func (m *DesktopArchive) ApplyBlockRef(id uint32, ref *block.BlockRef) error {
	if id != desktopArchiveRef {
		return errors.Errorf("unknown desktop archive ref id %d", id)
	}
	m.ArchiveRef = ref
	return nil
}

func (m *DesktopArchive) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return map[uint32]*block.BlockRef{desktopArchiveRef: m.GetArchiveRef()}, nil
}

func (m *DesktopArchive) GetBlockRefCtor(uint32) block.Ctor {
	return nil
}

func (m *BrowserShellMetadata) MarshalBlock() ([]byte, error) {
	return m.MarshalVT()
}

func (m *BrowserShellMetadata) UnmarshalBlock(data []byte) error {
	return m.UnmarshalVT(data)
}

func (m *BrowserShellMetadata) ApplyBlockRef(id uint32, ref *block.BlockRef) error {
	idx := int(id) - browserShellAssetRefBase
	if idx < 0 || idx >= len(m.GetAssets()) {
		return errors.Errorf("unknown browser shell asset ref id %d", id)
	}
	m.Assets[idx].ContentRef = ref
	return nil
}

func (m *BrowserShellMetadata) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	refs := make(map[uint32]*block.BlockRef, len(m.GetAssets()))
	for i, asset := range m.GetAssets() {
		refs[uint32(browserShellAssetRefBase+i)] = asset.GetContentRef()
	}
	return refs, nil
}

func (m *BrowserShellMetadata) GetBlockRefCtor(uint32) block.Ctor {
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

func sortedDesktopArchiveKeys(refs map[string]*DesktopArchive) []string {
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
	_ block.Block         = (*ReleaseMetadata)(nil)
	_ block.BlockWithRefs = (*ReleaseMetadata)(nil)
	_ block.Block         = (*DesktopArchive)(nil)
	_ block.BlockWithRefs = (*DesktopArchive)(nil)
	_ block.Block         = (*BrowserShellMetadata)(nil)
	_ block.BlockWithRefs = (*BrowserShellMetadata)(nil)
	_ block.Block         = (*BrowserAsset)(nil)
	_ block.BlockWithRefs = (*BrowserAsset)(nil)
	_ block.Block         = (*UpdateNotification)(nil)
)
