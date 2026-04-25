package session

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
)

// FindSessionMetadata looks up a session's index and metadata by ref.
// Returns 0, nil if the session is not found.
func FindSessionMetadata(ctx context.Context, ctrl SessionController, ref *SessionRef) (uint32, *SessionMetadata) {
	sessions, err := ctrl.ListSessions(ctx)
	if err != nil {
		return 0, nil
	}
	for _, entry := range sessions {
		if entry.GetSessionRef().EqualVT(ref) {
			idx := entry.GetSessionIndex()
			meta, _ := ctrl.GetSessionMetadata(ctx, idx)
			return idx, meta
		}
	}
	return 0, nil
}

// SessionController is the session list controller.
type SessionController interface {
	// GetSessionByIdx looks up the given session index.
	// Returns nil, nil if not found.
	GetSessionByIdx(ctx context.Context, idx uint32) (*SessionListEntry, error)
	// ListSessions lists the sessions in storage.
	ListSessions(ctx context.Context) ([]*SessionListEntry, error)
	// RegisterSession registers a session ref in storage or returns the existing matching entry.
	// If metadata is non-nil, it is written to the session controller ObjectStore.
	RegisterSession(ctx context.Context, ref *SessionRef, metadata *SessionMetadata) (*SessionListEntry, error)
	// DeleteSession removes the matching session ref from the list.
	// Returns nil if not found.
	DeleteSession(ctx context.Context, ref *SessionRef) error
	// GetSessionMetadata returns the metadata for a session by index.
	// Returns nil, nil if not found.
	GetSessionMetadata(ctx context.Context, idx uint32) (*SessionMetadata, error)
	// UpdateSessionMetadata updates the metadata for a session by ref.
	// Creates the metadata entry if it does not exist.
	UpdateSessionMetadata(ctx context.Context, ref *SessionRef, metadata *SessionMetadata) error
	// GetSessionBroadcast returns the broadcast that fires when sessions change.
	GetSessionBroadcast() *broadcast.Broadcast
}
