package provider_local

import (
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// UploadStatus is a block store's storage backend and upload status.
type UploadStatus struct {
	// Backend is the storage backend holding the store, nil for the account's
	// own storage.
	Backend *account_settings.StorageBackend
	// Status is the upload status of the queued writes.
	block_store_writeback.Status
	// Fallback counts the reads and fills of the store's read fallback.
	Fallback FallbackStatus
	// Reads counts the store's block reads by where they were served.
	Reads ReadStatus
	// Packs counts the finalized packs of the store's packahead log, or is
	// nil when the store has no log.
	Packs *packahead.DrainStatus
}

// GetUploadStatus returns the storage backend and upload status, and a channel
// closed when either changes. Packs is a snapshot: GetPackStatus returns the
// channel that follows it.
func (b *BlockStore) GetUploadStatus() (UploadStatus, <-chan struct{}) {
	return b.placement.uploadStatus()
}

// GetPackStatus returns the status of the finalized packs of the store's
// packahead log, and a channel that closes when it next changes, or nils when
// the store has no log.
func (b *BlockStore) GetPackStatus() (*packahead.DrainStatus, <-chan struct{}) {
	return b.placement.packStatus()
}

// uploadStatus returns the placement's backend, the status of its upload
// queue, fallback and reads, and a snapshot of its packs' status, with a
// channel closed when the backend or the upload queue changes.
func (p *placementState) uploadStatus() (UploadStatus, <-chan struct{}) {
	status, changed := p.wb.GetStatus()
	out := UploadStatus{Backend: p.backend.Load(), Status: status, Fallback: p.fallbackFills.status(), Reads: p.readStatus()}
	out.Packs, _ = p.packStatus()
	return out, changed
}

// packStatus returns the status of the finalized packs of the placement's
// packahead log, and a channel that closes when it next changes, or nils when
// the store has no log.
func (p *placementState) packStatus() (*packahead.DrainStatus, <-chan struct{}) {
	if p.drain == nil {
		return nil, nil
	}
	status, changed := p.drain.Status()
	return &status, changed
}
