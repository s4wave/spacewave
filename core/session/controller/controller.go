package session_controller

import (
	"bytes"
	"context"
	"io"
	"slices"
	"strconv"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/scrub"
	"github.com/blang/semver/v4"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/volume"
)

// ControllerID is the controller id.
const ControllerID = "session"

// Version is the component version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "session list controller"

// Controller is the session controller.
type Controller struct {
	*bus.BusController[*Config]

	mtx           csync.Mutex
	bcast         broadcast.Broadcast
	volumeID      string
	objectStoreID string
}

// sessionListPrefix is the key prefix for items in the session list.
var sessionListPrefix = []byte("s/")

// sessionListEntryKey returns the key for a session list entry.
func sessionListEntryKey(idx uint32) []byte {
	idStr := strconv.FormatUint(uint64(idx), 10)
	return bytes.Join([][]byte{sessionListPrefix, []byte(idStr)}, nil)
}

// NewFactory constructs the component factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		func() *Config {
			return &Config{}
		},
		func(base *bus.BusController[*Config]) (*Controller, error) {
			volumeID := base.GetConfig().GetVolumeId()
			if volumeID == "" {
				volumeID = bldr_plugin.PluginVolumeID
			}

			objectStoreID := base.GetConfig().GetObjectStoreId()
			if objectStoreID == "" {
				objectStoreID = "sessions/list"
			}

			return &Controller{
				BusController: base,
				objectStoreID: objectStoreID,
				volumeID:      volumeID,
			}, nil
		},
	)
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case session.LookupSessionController:
		if d.LookupSessionControllerID() == "" || d.LookupSessionControllerID() == c.GetConfig().GetSessionControllerId() {
			return directive.R(directive.NewValueResolver([]session.LookupSessionControllerValue{c}), nil)
		}
	}

	return nil, nil
}

// GetSessionBroadcast returns the broadcast that fires when sessions change.
func (c *Controller) GetSessionBroadcast() *broadcast.Broadcast {
	return &c.bcast
}

// GetSessionByIdx looks up the given session index.
// Returns nil, nil if not found.
func (c *Controller) GetSessionByIdx(ctx context.Context, idx uint32) (*session.SessionListEntry, error) {
	objStore, objStoreRel, err := c.buildObjectStore(ctx)
	if err != nil {
		return nil, err
	}
	defer objStoreRel()

	rel, err := c.mtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer rel()

	otx, err := objStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer otx.Discard()

	data, found, err := otx.Get(ctx, sessionListEntryKey(idx))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	val := &session.SessionListEntry{}
	if err := val.UnmarshalVT(data); err != nil {
		return nil, err
	}

	return val, nil
}

// ListSessions lists the sessions in storage.
func (c *Controller) ListSessions(ctx context.Context) ([]*session.SessionListEntry, error) {
	objStore, objStoreRel, err := c.buildObjectStore(ctx)
	if err != nil {
		return nil, err
	}
	defer objStoreRel()

	rel, err := c.mtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer rel()

	otx, err := objStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer otx.Discard()

	size, err := otx.Size(ctx)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, nil
	}

	var elems []*session.SessionListEntry
	err = otx.ScanPrefix(ctx, sessionListPrefix, func(key, value []byte) error {
		entry := &session.SessionListEntry{}
		if err := entry.UnmarshalVT(value); err != nil {
			c.GetLogger().WithError(err).Warn("ignoring invalid session list entry")
			return nil
		}

		elems = append(elems, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return elems, nil
}

// sessionMetaPrefix is the key prefix for session metadata entries.
var sessionMetaPrefix = []byte("sessions/meta/")

// sessionMetaKey returns the key for session metadata by session index.
func sessionMetaKey(idx uint32) []byte {
	idStr := strconv.FormatUint(uint64(idx), 10)
	return bytes.Join([][]byte{sessionMetaPrefix, []byte(idStr)}, nil)
}

// RegisterSession registers a session ref in storage or returns the existing matching entry.
// If metadata is non-nil, it is written to the session controller ObjectStore.
func (c *Controller) RegisterSession(ctx context.Context, ref *session.SessionRef, metadata *session.SessionMetadata) (*session.SessionListEntry, error) {
	objStore, objStoreRel, err := c.buildObjectStore(ctx)
	if err != nil {
		return nil, err
	}
	defer objStoreRel()

	rel, err := c.mtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer rel()

	otx, err := objStore.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer otx.Discard()

	// Inject created_at if not set by the caller.
	if metadata != nil && metadata.GetCreatedAt() == 0 {
		metadata.CreatedAt = time.Now().UnixMilli()
	}

	var matchedEntry *session.SessionListEntry
	var maxSessionIndex uint32
	size, err := otx.Size(ctx)
	if err != nil {
		return nil, err
	}
	if size != 0 {
		err = otx.ScanPrefix(ctx, sessionListPrefix, func(key, value []byte) error {
			entry := &session.SessionListEntry{}
			if err := entry.UnmarshalVT(value); err != nil {
				c.GetLogger().WithError(err).Warn("ignoring invalid session list entry")
				return nil
			}

			if matchedEntry == nil && entry.GetSessionRef().EqualVT(ref) {
				matchedEntry = entry
			}

			maxSessionIndex = max(maxSessionIndex, entry.GetSessionIndex())
			return nil
		})
	}
	if matchedEntry != nil || err != nil {
		// Write metadata even for existing entries (may have changed).
		if matchedEntry != nil && metadata != nil {
			metaData, err := metadata.MarshalVT()
			if err != nil {
				return nil, err
			}
			defer scrub.Scrub(metaData)
			if err := otx.Set(ctx, sessionMetaKey(matchedEntry.GetSessionIndex()), metaData); err != nil {
				return nil, err
			}
			if err := otx.Commit(ctx); err != nil {
				return nil, err
			}
		}
		return matchedEntry, err
	}

	// No matching entry. Add an entry.
	writeEntry := &session.SessionListEntry{
		SessionIndex: maxSessionIndex + 1,
		SessionRef:   ref,
	}
	data, err := writeEntry.MarshalVT()
	if err != nil {
		return nil, err
	}
	defer scrub.Scrub(data)

	key := sessionListEntryKey(writeEntry.SessionIndex)
	if err := otx.Set(ctx, key, data); err != nil {
		return nil, err
	}

	// Write metadata if provided.
	if metadata != nil {
		metaData, err := metadata.MarshalVT()
		if err != nil {
			return nil, err
		}
		defer scrub.Scrub(metaData)
		if err := otx.Set(ctx, sessionMetaKey(writeEntry.GetSessionIndex()), metaData); err != nil {
			return nil, err
		}
	}

	if err := otx.Commit(ctx); err != nil {
		return nil, err
	}

	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})

	return writeEntry, nil
}

// GetSessionMetadata returns the metadata for a session by index.
// Returns nil, nil if not found.
func (c *Controller) GetSessionMetadata(ctx context.Context, idx uint32) (*session.SessionMetadata, error) {
	objStore, objStoreRel, err := c.buildObjectStore(ctx)
	if err != nil {
		return nil, err
	}
	defer objStoreRel()

	rel, err := c.mtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer rel()

	otx, err := objStore.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer otx.Discard()

	data, found, err := otx.Get(ctx, sessionMetaKey(idx))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	val := &session.SessionMetadata{}
	if err := val.UnmarshalVT(data); err != nil {
		return nil, err
	}

	return val, nil
}

// UpdateSessionMetadata updates the metadata for a session by ref.
// Creates the metadata entry if it does not exist.
func (c *Controller) UpdateSessionMetadata(ctx context.Context, ref *session.SessionRef, metadata *session.SessionMetadata) error {
	objStore, objStoreRel, err := c.buildObjectStore(ctx)
	if err != nil {
		return err
	}
	defer objStoreRel()

	rel, err := c.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer rel()

	otx, err := objStore.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer otx.Discard()

	// Find the session index by scanning for the matching ref.
	var idx uint32
	var found bool
	err = otx.ScanPrefix(ctx, sessionListPrefix, func(key, value []byte) error {
		entry := &session.SessionListEntry{}
		if err := entry.UnmarshalVT(value); err != nil {
			return nil
		}
		if entry.GetSessionRef().EqualVT(ref) {
			idx = entry.GetSessionIndex()
			found = true
			return io.EOF
		}
		return nil
	})
	if !found {
		return nil
	}

	metaData, err := metadata.MarshalVT()
	if err != nil {
		return err
	}
	defer scrub.Scrub(metaData)

	if err := otx.Set(ctx, sessionMetaKey(idx), metaData); err != nil {
		return err
	}
	if err := otx.Commit(ctx); err != nil {
		return err
	}

	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})
	return nil
}

// DeleteSession removes the matching session ref from the list.
// Returns nil if not found.
func (c *Controller) DeleteSession(ctx context.Context, ref *session.SessionRef) error {
	objStore, objStoreRel, err := c.buildObjectStore(ctx)
	if err != nil {
		return err
	}
	defer objStoreRel()

	rel, err := c.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer rel()

	otx, err := objStore.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer otx.Discard()

	var matchedKey []byte
	var matchedIdx uint32
	err = otx.ScanPrefix(ctx, sessionListPrefix, func(key, value []byte) error {
		entry := &session.SessionListEntry{}
		if err := entry.UnmarshalVT(value); err != nil {
			c.GetLogger().WithError(err).Warn("ignoring invalid session list entry")
			return nil
		}

		if entry.GetSessionRef().EqualVT(ref) {
			matchedKey = slices.Clone(key)
			matchedIdx = entry.GetSessionIndex()
			return io.EOF
		}
		return nil
	})
	if len(matchedKey) != 0 {
		// Note: we ignore the value of err= for ScanPrefix in this case (intentionally).
		err = otx.Delete(ctx, matchedKey)
		if err == nil {
			// Also delete stale session metadata for this index.
			_ = otx.Delete(ctx, sessionMetaKey(matchedIdx))
		}
		if err == nil {
			err = otx.Commit(ctx)
		}
		if err == nil {
			c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				broadcast()
			})
		}
	}
	return err
}

// buildObjectStore builds the object store for the sessions list.
func (c *Controller) buildObjectStore(ctx context.Context) (object.ObjectStore, func(), error) {
	// Open the object store for the session list.
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(
		ctx,
		c.GetBus(),
		false,
		c.objectStoreID,
		c.volumeID,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}

	return objStoreHandle.GetObjectStore(), diRef.Release, nil
}

// _ is a type assertion
var (
	_ controller.Controller     = ((*Controller)(nil))
	_ session.SessionController = ((*Controller)(nil))
)
