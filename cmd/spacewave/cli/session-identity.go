//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"

	"github.com/pkg/errors"
	core_session "github.com/s4wave/spacewave/core/session"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// errSessionGone means the session list holds no entry for the wanted
// session. The session was logged out, deleted, or moved by a transfer, so
// nothing reaches it until it returns or the setup that named it runs again.
var errSessionGone = stderrors.New("session is not in the session list")

// sessionIdentity names a session by its provider account and session ID.
// Unlike the catalog index, it survives a move to another index, and a new
// session at a freed index never takes it over.
type sessionIdentity struct {
	// accountID is the provider account holding the session.
	accountID string
	// sessionID is the session's ID within its provider.
	sessionID string
}

// sessionIdentityOf returns the identity a session reference names.
func sessionIdentityOf(ref *core_session.SessionRef) sessionIdentity {
	res := ref.GetProviderResourceRef()
	return sessionIdentity{accountID: res.GetProviderAccountId(), sessionID: res.GetId()}
}

// findSessionEntry returns the entry of the session list naming id, or nil.
func findSessionEntry(entries []*core_session.SessionListEntry, id sessionIdentity) *core_session.SessionListEntry {
	for _, entry := range entries {
		if sessionIdentityOf(entry.GetSessionRef()) == id {
			return entry
		}
	}
	return nil
}

// findSession reads the session list and returns the entry naming id, or
// errSessionGone.
func (c *sdkClient) findSession(ctx context.Context, id sessionIdentity) (*core_session.SessionListEntry, error) {
	// Read the list and find the entry naming id.
	entries, err := c.listSessions(ctx, "list sessions")
	if err != nil {
		return nil, err
	}
	entry := findSessionEntry(entries, id)
	if entry == nil {
		return nil, errors.Wrapf(errSessionGone, "session %s in account %s", id.sessionID, id.accountID)
	}
	return entry, nil
}

// mountSessionAt mounts the session at idx, requiring it to be the session id
// names. A session that left idx between the read of the list and the mount
// returns errSessionGone instead of mounting whatever holds idx now.
func (c *sdkClient) mountSessionAt(ctx context.Context, idx uint32, id sessionIdentity) (*s4wave_session.Session, error) {
	// Mount the session the index holds now.
	resp, err := c.root.MountSessionByIdx(ctx, idx)
	if err != nil {
		return nil, errors.Wrap(err, "mount session")
	}
	if resp.GetNotFound() {
		return nil, errors.Wrapf(errSessionGone, "no session found at index %d", idx)
	}
	sessRef := c.resClient.CreateResourceReference(resp.GetResourceId())
	if sessionIdentityOf(resp.GetSessionRef()) != id {
		sessRef.Release()
		return nil, errors.Wrapf(errSessionGone, "session %s left index %d", id.sessionID, idx)
	}

	// Wrap the session resource.
	sess, err := s4wave_session.NewSession(c.resClient, sessRef)
	if err != nil {
		sessRef.Release()
		return nil, errors.Wrap(err, "session resource")
	}
	return sess, nil
}

// mountSessionByIdentity mounts the session id names wherever the session
// list holds it, or returns errSessionGone.
func (c *sdkClient) mountSessionByIdentity(ctx context.Context, id sessionIdentity) (*s4wave_session.Session, error) {
	entry, err := c.findSession(ctx, id)
	if err != nil {
		return nil, err
	}
	return c.mountSessionAt(ctx, entry.GetSessionIndex(), id)
}

// followSessions calls set with the session list, then again after each
// change, until ctx ends or the watch fails.
func (c *sdkClient) followSessions(ctx context.Context, set func([]*core_session.SessionListEntry)) error {
	stream, release, err := c.watchSessions(ctx)
	if err != nil {
		return errors.Wrap(err, "watch sessions")
	}
	defer func() { _ = release() }()
	for {
		resp, err := stream.Recv()
		if err != nil {
			return errors.Wrap(err, "watch sessions")
		}
		set(resp.GetSessions())
	}
}
