package release

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

const (
	channelDirectoryRefBase  = 1000
	releaseMetadataAssetBase = 1000
	browserShellAssetRefBase = 1000
	browserAssetContentRef   = 1
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
	idx := int(id) - releaseMetadataAssetBase
	if idx < 0 || idx >= len(m.GetBrowserShell().GetAssets()) {
		return errors.Errorf("unknown release metadata ref id %d", id)
	}
	m.BrowserShell.Assets[idx].ContentRef = ref
	return nil
}

func (m *ReleaseMetadata) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	refs := make(map[uint32]*block.BlockRef, len(m.GetBrowserShell().GetAssets()))
	for i, asset := range m.GetBrowserShell().GetAssets() {
		if ref := asset.GetContentRef(); ref != nil {
			refs[uint32(releaseMetadataAssetBase+i)] = ref
		}
	}
	return refs, nil
}

func (m *ReleaseMetadata) GetBlockRefCtor(uint32) block.Ctor {
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
		if ref := asset.GetContentRef(); ref != nil {
			refs[uint32(browserShellAssetRefBase+i)] = ref
		}
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
	if m.GetContentRef() == nil {
		return nil, nil
	}
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

var (
	_ block.Block         = (*ChannelDirectory)(nil)
	_ block.BlockWithRefs = (*ChannelDirectory)(nil)
	_ block.Block         = (*ReleaseMetadata)(nil)
	_ block.BlockWithRefs = (*ReleaseMetadata)(nil)
	_ block.Block         = (*BrowserShellMetadata)(nil)
	_ block.BlockWithRefs = (*BrowserShellMetadata)(nil)
	_ block.Block         = (*BrowserAsset)(nil)
	_ block.BlockWithRefs = (*BrowserAsset)(nil)
	_ block.Block         = (*UpdateNotification)(nil)
)
