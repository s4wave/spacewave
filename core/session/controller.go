package session

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
)

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

// SessionTransitionController atomically rebinds a registered client after
// provider migration has durably installed its independent credential.
type SessionTransitionController interface {
	TransitionSession(context.Context, *SessionRef, *SessionRef) error
}
