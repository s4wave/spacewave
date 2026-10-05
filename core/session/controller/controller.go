package session_controller

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/scrub"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/volume"
)

// ControllerID is the controller id.
const ControllerID = "session"

// Version is the component version.
var Version = controller.MustParseVersion("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "session list controller"

// Controller is the session controller.
type Controller struct {
	// BusController provides the configured controller and bus lifetime.
	*bus.BusController[*Config]

	// mtx serializes Session storage and guards lifetimes and the cached store.
	mtx sync.Mutex
	// bcast announces changes to the registered Sessions.
	bcast broadcast.Broadcast
	// volumeID identifies the Volume containing the Session registry.
	volumeID string
	// objectStoreID identifies the Session registry ObjectStore.
	objectStoreID string
	// objStore is the cached registry store, guarded by mtx.
	objStore object.ObjectStore
	// objStoreRel releases the cached store directive, guarded by mtx.
	objStoreRel func()
	// lifetimes holds mounted Session stop functions, guarded by mtx.
	lifetimes map[*sessionLifetime]struct{}
}

// sessionLifetime binds a mounted Session to its provider's shutdown operation.
type sessionLifetime struct {
	// ref identifies the mounted Session independently of caller mutations.
	ref *session.SessionRef
	// stop cancels the mounted lifetime and waits for all its resources to release.
	stop func(context.Context) error
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
			// Resolve the Volume that stores this controller's sessions.
			volumeID := base.GetConfig().GetVolumeId()
			if volumeID == "" {
				volumeID = bldr_plugin.PluginVolumeID
			}

			// Resolve the object-store key for the session list.
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

// Close releases controller-owned resources.
func (c *Controller) Close() error {
	// Detach the cached object store before releasing its directive reference.
	c.mtx.Lock()
	objStoreRel := c.objStoreRel
	c.objStore = nil
	c.objStoreRel = nil
	c.mtx.Unlock()

	// Release the object-store directive after dropping the controller lock.
	if objStoreRel != nil {
		objStoreRel()
	}
	return c.BusController.Close()
}

// GetSessionBroadcast returns the broadcast that fires when sessions change.
func (c *Controller) GetSessionBroadcast() *broadcast.Broadcast {
	return &c.bcast
}

// GetSessionByIdx looks up the given session index.
// Returns nil, nil if not found.
func (c *Controller) GetSessionByIdx(ctx context.Context, idx uint32) (*session.SessionListEntry, error) {
	// Serialize access to the session list.
	c.mtx.Lock()
	defer c.mtx.Unlock()

	// Open the sessions object store.
	objStore, err := c.buildObjectStoreLocked(ctx)
	if err != nil {
		return nil, err
	}

	// Read the session entry at the index.
	var val *session.SessionListEntry
	err = kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			// Read the stored session entry at the requested index.
			data, found, err := tx.Get(ctx, sessionListEntryKey(idx))
			if err != nil {
				return err
			}
			if !found {
				val = nil
				return nil
			}

			// Decode the stored session entry into its typed record.
			next := &session.SessionListEntry{}
			if err := next.UnmarshalVT(data); err != nil {
				return err
			}
			val = next
			return nil
		},
	)
	return val, err
}

// ListSessions lists the sessions in storage.
func (c *Controller) ListSessions(ctx context.Context) ([]*session.SessionListEntry, error) {
	// Trace the storage lookup and serialize access to the session list.
	ctx, task := trace.NewTask(ctx, "hydra/session/list-sessions")
	defer task.End()
	c.mtx.Lock()
	defer c.mtx.Unlock()

	// Open the sessions object store before starting its scan trace.
	_, waitStoreTask := trace.NewTask(ctx, "hydra/session/list-sessions/wait-object-store")
	objStore, err := c.buildObjectStoreLocked(ctx)
	waitStoreTask.End()
	if err != nil {
		return nil, err
	}

	// Collect valid session entries and errors from malformed records.
	var elems []*session.SessionListEntry
	var invalidEntryErrs []error
	_, scanTask := trace.NewTask(ctx, "hydra/session/list-sessions/scan")
	err = kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, otx kvtx.Tx) error {
			// Reset scan results before reading the current transaction snapshot.
			elems = nil
			invalidEntryErrs = nil
			size, err := otx.Size(ctx)
			if err != nil {
				return err
			}
			if size == 0 {
				return nil
			}

			return otx.ScanPrefix(ctx, sessionListPrefix, func(_ []byte, value []byte) error {
				entry := &session.SessionListEntry{}
				if err := entry.UnmarshalVT(value); err != nil {
					invalidEntryErrs = append(invalidEntryErrs, err)
					return nil
				}
				elems = append(elems, entry)
				return nil
			})
		},
	)
	scanTask.End()
	if err != nil {
		return nil, err
	}
	for _, invalidEntryErr := range invalidEntryErrs {
		c.GetLogger().WithError(invalidEntryErr).Warn("ignoring invalid session list entry")
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
	// Serialize access to the session list.
	c.mtx.Lock()
	defer c.mtx.Unlock()

	// Open the sessions object store.
	objStore, err := c.buildObjectStoreLocked(ctx)
	if err != nil {
		return nil, err
	}

	// Freeze caller metadata before a retry can replay the session census.
	if metadata != nil && metadata.GetCreatedAt() == 0 {
		metadata.CreatedAt = time.Now().UnixMilli()
	}
	var metadataData []byte
	if metadata != nil {
		metadataData, err = metadata.MarshalVT()
		if err != nil {
			return nil, err
		}
		defer scrub.Scrub(metadataData)
	}

	// Replay the complete census and write in one transaction.
	var result *session.SessionListEntry
	var created bool
	var resultData []byte
	defer func() { scrub.Scrub(resultData) }()
	var invalidEntryErrs []error
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, otx kvtx.Tx) error {
			// Reset retry outputs before replaying the session census.
			scrub.Scrub(resultData)
			resultData = nil
			result = nil
			created = false
			invalidEntryErrs = nil
			var maxSessionIndex uint32

			// Read transaction size to skip scanning an empty session list.
			size, err := otx.Size(ctx)
			if err != nil {
				return err
			}
			if size != 0 {
				err = otx.ScanPrefix(ctx, sessionListPrefix, func(_ []byte, value []byte) error {
					// Decode each stored session before comparing its reference.
					entry := &session.SessionListEntry{}
					if err := entry.UnmarshalVT(value); err != nil {
						invalidEntryErrs = append(invalidEntryErrs, err)
						return nil
					}
					if result == nil && entry.GetSessionRef().EqualVT(ref) {
						result = entry
					}
					maxSessionIndex = max(maxSessionIndex, entry.GetSessionIndex())
					return nil
				})
				if err != nil {
					return err
				}
			}

			// Allocate and persist an entry when the census found no match.
			if result == nil {
				created = true
				result = &session.SessionListEntry{
					SessionIndex: maxSessionIndex + 1,
					SessionRef:   ref,
				}
				resultData, err = result.MarshalVT()
				if err != nil {
					return err
				}
				if err := otx.Set(ctx, sessionListEntryKey(result.GetSessionIndex()), resultData); err != nil {
					return err
				}
			}

			// Persist metadata alongside the resolved entry when supplied.
			if metadataData != nil {
				if err := otx.Set(ctx, sessionMetaKey(result.GetSessionIndex()), metadataData); err != nil {
					return err
				}
			}
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	for _, invalidEntryErr := range invalidEntryErrs {
		c.GetLogger().WithError(invalidEntryErr).Warn("ignoring invalid session list entry")
	}

	// Wake session list watchers.
	if created {
		c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			broadcast()
		})
	}
	return result, nil
}

// GetSessionMetadata returns the metadata for a session by index.
// Returns nil, nil if not found.
func (c *Controller) GetSessionMetadata(ctx context.Context, idx uint32) (*session.SessionMetadata, error) {
	// Serialize access to the session list.
	c.mtx.Lock()
	defer c.mtx.Unlock()

	// Open the sessions object store.
	objStore, err := c.buildObjectStoreLocked(ctx)
	if err != nil {
		return nil, err
	}

	// Read the session metadata at the index.
	var val *session.SessionMetadata
	err = kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			// Read metadata for the requested session index.
			data, found, err := tx.Get(ctx, sessionMetaKey(idx))
			if err != nil {
				return err
			}
			if !found {
				val = nil
				return nil
			}

			// Decode the stored metadata into its typed record.
			next := &session.SessionMetadata{}
			if err := next.UnmarshalVT(data); err != nil {
				return err
			}
			val = next
			return nil
		},
	)
	return val, err
}

// UpdateSessionMetadata updates the metadata for a session by ref.
// Does nothing if no session entry matches the ref.
func (c *Controller) UpdateSessionMetadata(ctx context.Context, ref *session.SessionRef, metadata *session.SessionMetadata) error {
	// Serialize access to the session list.
	c.mtx.Lock()
	defer c.mtx.Unlock()

	// Open the sessions object store.
	objStore, err := c.buildObjectStoreLocked(ctx)
	if err != nil {
		return err
	}

	// Encode the metadata once before the transaction can replay.
	metaData, err := metadata.MarshalVT()
	if err != nil {
		return err
	}
	defer scrub.Scrub(metaData)

	// Replay the ref lookup and write in one transaction.
	var updated bool
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, otx kvtx.Tx) error {
			// Reset the update result before searching this transaction attempt.
			updated = false

			// Find the session index by scanning for the matching ref.
			var idx uint32
			var found bool
			scanErr := otx.ScanPrefix(ctx, sessionListPrefix, func(_ []byte, value []byte) error {
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
			if scanErr != nil && !errors.Is(scanErr, io.EOF) {
				return scanErr
			}
			if !found {
				return nil
			}
			updated = true
			return otx.Set(ctx, sessionMetaKey(idx), metaData)
		},
	)
	if err != nil {
		return err
	}

	// Wake session list watchers.
	if updated {
		c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			broadcast()
		})
	}
	return nil
}

// TrackSession retains a stop function for a mounted Session until released.
func (c *Controller) TrackSession(ref *session.SessionRef, stop func(context.Context) error) func() {
	// Retain an independent reference and its cleanup under the registry lock.
	lifetime := &sessionLifetime{ref: ref.CloneVT(), stop: stop}
	c.mtx.Lock()
	if c.lifetimes == nil {
		c.lifetimes = make(map[*sessionLifetime]struct{})
	}
	c.lifetimes[lifetime] = struct{}{}
	c.mtx.Unlock()

	// Remove the completed lifetime without stopping other mounts of the Session.
	return func() {
		c.mtx.Lock()
		delete(c.lifetimes, lifetime)
		c.mtx.Unlock()
	}
}

// DeleteSession removes the Session registration and waits for its mounted resources to stop.
// Returns nil if neither a registration nor a mounted lifetime exists.
func (c *Controller) DeleteSession(ctx context.Context, ref *session.SessionRef) error {
	// Commit the registry deletion before stopping the matching mounted lifetimes.
	c.mtx.Lock()
	err := c.deleteSessionLocked(ctx, ref)
	lifetimes := make([]*sessionLifetime, 0, len(c.lifetimes))
	if err == nil {
		for lifetime := range c.lifetimes {
			if lifetime.ref.EqualVT(ref) {
				lifetimes = append(lifetimes, lifetime)
			}
		}
	}
	c.mtx.Unlock()
	if err != nil {
		return err
	}

	// Stop outside the registry lock because trackers release registry references.
	var stopErrs []error
	for _, lifetime := range lifetimes {
		if err := lifetime.stop(ctx); err != nil {
			stopErrs = append(stopErrs, err)
		}
	}
	return errors.Join(stopErrs...)
}

// deleteSessionLocked deletes Session records with mtx held.
func (c *Controller) deleteSessionLocked(ctx context.Context, ref *session.SessionRef) error {
	// Open the sessions object store.
	objStore, err := c.buildObjectStoreLocked(ctx)
	if err != nil {
		return err
	}

	// Replay the scan and delete in one transaction.
	var deleted bool
	var invalidEntryErrs []error
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, otx kvtx.Tx) error {
			// Reset deletion results before scanning this transaction attempt.
			deleted = false
			invalidEntryErrs = nil

			// Locate the matching session key and index in the list.
			var matchedKey []byte
			var matchedIdx uint32
			scanErr := otx.ScanPrefix(ctx, sessionListPrefix, func(key []byte, value []byte) error {
				// Decode each entry and stop when its reference matches.
				entry := &session.SessionListEntry{}
				if err := entry.UnmarshalVT(value); err != nil {
					invalidEntryErrs = append(invalidEntryErrs, err)
					return nil
				}
				if entry.GetSessionRef().EqualVT(ref) {
					matchedKey = slices.Clone(key)
					matchedIdx = entry.GetSessionIndex()
					return io.EOF
				}
				return nil
			})
			if len(matchedKey) == 0 && scanErr != nil && !errors.Is(scanErr, io.EOF) {
				return scanErr
			}
			if len(matchedKey) == 0 {
				return nil
			}
			if err := otx.Delete(ctx, matchedKey); err != nil {
				return err
			}

			// Also delete stale session metadata for this index.
			if err := otx.Delete(ctx, sessionMetaKey(matchedIdx)); err != nil {
				return err
			}
			deleted = true
			return nil
		},
	)
	if err != nil {
		return err
	}
	for _, invalidEntryErr := range invalidEntryErrs {
		c.GetLogger().WithError(invalidEntryErr).Warn("ignoring invalid session list entry")
	}

	// Wake session list watchers.
	if deleted {
		c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			broadcast()
		})
	}
	return nil
}

// buildObjectStoreLocked builds or returns the cached sessions object store.
// c.mtx must be held by the caller.
func (c *Controller) buildObjectStoreLocked(ctx context.Context) (object.ObjectStore, error) {
	// Reuse the cached sessions store when it is already attached.
	if c.objStore != nil {
		return c.objStore, nil
	}

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
		return nil, err
	}

	// Cache the store and retain its directive reference for controller lifetime.
	c.objStore = objStoreHandle.GetObjectStore()
	c.objStoreRel = diRef.Release
	return c.objStore, nil
}

// _ is a type assertion
var (
	_ controller.Controller     = (*Controller)(nil)
	_ session.SessionController = (*Controller)(nil)
)
