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
	// TrackSession registers a mounted lifetime's stop function until the returned release is called.
	// DeleteSession calls stop and waits for it to release the Session's resources.
	// stop must tolerate concurrent calls from repeated deletions.
	// A lifetime may be tracked before registration while a provider completes login.
	TrackSession(ref *SessionRef, stop func(context.Context) error) func()
	// DeleteSession removes the registration and waits for tracked lifetimes to stop.
	// Returns nil if neither a registration nor a tracked lifetime exists.
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
	// TransitionSession replaces a registered attachment with its accepted destination.
	TransitionSession(context.Context, *SessionRef, *SessionRef) error
}
