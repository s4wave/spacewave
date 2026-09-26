package session_controller

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/db/kvtx"
)

// TransitionSession replaces an attachment at its existing index after the
// destination has durably accepted its credential. Source credentials remain
// in their provider volume for recovery. Replaying the same transition is safe.
func (c *Controller) TransitionSession(ctx context.Context, source, destination *session.SessionRef) error {
	if err := source.Validate(); err != nil {
		return err
	}
	if err := destination.Validate(); err != nil {
		return err
	}
	if source.EqualVT(destination) {
		return nil
	}
	c.mtx.Lock()
	defer c.mtx.Unlock()
	store, err := c.buildObjectStoreLocked(ctx)
	if err != nil {
		return err
	}
	err = kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		var old, next *session.SessionListEntry
		if err := tx.ScanPrefix(ctx, sessionListPrefix, func(_ []byte, data []byte) error {
			entry := &session.SessionListEntry{}
			if err := entry.UnmarshalVT(data); err != nil {
				return err
			}
			if entry.GetSessionRef().EqualVT(source) {
				old = entry
			}
			if entry.GetSessionRef().EqualVT(destination) {
				next = entry
			}
			return nil
		}); err != nil {
			return err
		}
		if old == nil {
			if next != nil {
				return nil
			}
			return errors.New("the source Session is no longer registered")
		}
		old.SessionRef = destination.CloneVT()
		data, err := old.MarshalVT()
		if err != nil {
			return err
		}
		if err := tx.Set(ctx, sessionListEntryKey(old.GetSessionIndex()), data); err != nil {
			return err
		}
		data, found, err := tx.Get(ctx, sessionMetaKey(old.GetSessionIndex()))
		if err != nil {
			return err
		}
		metadata := &session.SessionMetadata{}
		if found {
			if err := metadata.UnmarshalVT(data); err != nil {
				return err
			}
		}
		ref := destination.GetProviderResourceRef()
		metadata.ProviderId = ref.GetProviderId()
		metadata.ProviderAccountId = ref.GetProviderAccountId()
		displayName, err := transitionProviderDisplayName(ctx, tx, next, ref.GetProviderId())
		if err != nil {
			return err
		}
		metadata.ProviderDisplayName = displayName
		data, err = metadata.MarshalVT()
		if err != nil {
			return err
		}
		if err := tx.Set(ctx, sessionMetaKey(old.GetSessionIndex()), data); err != nil {
			return err
		}
		if next != nil && next.GetSessionIndex() != old.GetSessionIndex() {
			if err := tx.Delete(ctx, sessionListEntryKey(next.GetSessionIndex())); err != nil {
				return err
			}
			return tx.Delete(ctx, sessionMetaKey(next.GetSessionIndex()))
		}
		return nil
	})
	if err == nil {
		c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) { broadcast() })
	}
	return err
}

// transitionProviderDisplayName returns the destination provider's label,
// preferring the label its provider registered for the destination Session.
func transitionProviderDisplayName(ctx context.Context, tx kvtx.Tx, next *session.SessionListEntry, providerID string) (string, error) {
	if next != nil {
		data, found, err := tx.Get(ctx, sessionMetaKey(next.GetSessionIndex()))
		if err != nil {
			return "", err
		}
		if found {
			metadata := &session.SessionMetadata{}
			if err := metadata.UnmarshalVT(data); err != nil {
				return "", err
			}
			if name := metadata.GetProviderDisplayName(); name != "" {
				return name, nil
			}
		}
	}
	if providerID == "local" {
		return "Local", nil
	}
	return "Cloud", nil
}
