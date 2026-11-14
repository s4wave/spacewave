package provider_transfer

import (
	"context"

	"github.com/s4wave/spacewave/core/provider"
)

// TransferOptions carries the optional collaborators of a Transfer. The zero
// value is valid: no cleanup, no checkpointing, and no space-ID filter.
type TransferOptions struct {
	// Cleanup deletes each Space from the source once the target holds it,
	// in Merge and Migrate.
	Cleanup CleanupSource
	// DeleteSource deletes the source account, its sessions first and then its
	// volume, after Cleanup has deleted every moved Space. It calls keep
	// before the sessions go and again before the volume goes, and keeps the
	// volume when keep fails: keep fails while the source lists a Space the
	// run did not move. An error means the volume is whole, so the run fails;
	// failures once the volume is gone are its own to log.
	DeleteSource func(ctx context.Context, keep func(context.Context) error) error
	// Checkpoint persists resumable transfer progress.
	Checkpoint CheckpointStore
	// SourceSession names the session the run moves from. The checkpoint
	// records it so a resume checks its index still names it.
	SourceSession *provider.ProviderResourceRef
	// TargetSession names the session the run moves to, recorded likewise.
	TargetSession *provider.ProviderResourceRef
	// FilterSpaceIDs restricts the transfer to the listed Space IDs. With
	// Cleanup set, Merge and Migrate delete the whole source account, so they
	// refuse a filter that leaves out a Space.
	FilterSpaceIDs []string
	// CopyIdentity copies the source's peer key to the target. Migrate signs
	// the target Spaces with that key, so Execute runs it after every Space
	// reaches the target and before Cleanup deletes the source.
	CopyIdentity func(ctx context.Context) error
}
