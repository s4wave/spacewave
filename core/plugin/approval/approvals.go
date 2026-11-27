package plugin_approval

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/volume"
	s4wave_plugin_approval "github.com/s4wave/spacewave/sdk/plugin/approval"
)

// Approvals reads and writes the device's plugin decisions and reports each
// committed write. It is the only writer of the decisions, so its broadcast
// covers every change.
type Approvals struct {
	// b resolves the ObjectStore.
	b bus.Bus
	// volumeID is the device-local volume holding the store.
	volumeID string
	// storeID is the ObjectStore holding the decisions.
	storeID string
	// bcast fires after each committed write.
	bcast broadcast.Broadcast
}

// NewApprovals constructs Approvals over the storeID ObjectStore of volumeID.
func NewApprovals(b bus.Bus, volumeID, storeID string) *Approvals {
	return &Approvals{b: b, volumeID: volumeID, storeID: storeID}
}

// WaitCh returns a channel closed after the next committed write.
func (a *Approvals) WaitCh() <-chan struct{} {
	var ch <-chan struct{}
	a.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
	})
	return ch
}

// Set records peerID's decision about one root and wakes the watchers. The
// decision must identify its plugin and root and carry a known non-zero state.
func (a *Approvals) Set(ctx context.Context, peerID string, approval *s4wave_plugin_approval.PluginApproval) error {
	// Refuse a decision without a peer, plugin, or root.
	if peerID == "" {
		return errors.New("peer id is required")
	}
	if approval.GetPluginId() == "" || approval.GetManifestRoot() == "" {
		return errors.New("plugin id and manifest root are required")
	}
	if _, ok := s4wave_plugin_approval.PluginApprovalState_name[int32(approval.GetState())]; !ok ||
		approval.GetState() == s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_UNKNOWN {
		return errors.Errorf("unknown plugin approval state %d", approval.GetState())
	}

	// Stamp and persist the decision, then wake the watchers.
	approval = approval.CloneVT()
	if approval.GetDecidedAt() == nil {
		approval.DecidedAt = timestamppb.Now()
	}
	err := a.withStore(ctx, func(store volume.ObjectStoreHandle) error {
		return PutApproval(ctx, store.GetObjectStore(), peerID, approval)
	})
	if err != nil {
		return err
	}
	a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})
	return nil
}

// Get reads peerID's decision about one root, or nil when there is none.
func (a *Approvals) Get(ctx context.Context, peerID, pluginID, manifestRoot string) (*s4wave_plugin_approval.PluginApproval, error) {
	var out *s4wave_plugin_approval.PluginApproval
	err := a.withStore(ctx, func(store volume.ObjectStoreHandle) error {
		var err error
		out, err = GetApproval(ctx, store.GetObjectStore(), peerID, pluginID, manifestRoot)
		return err
	})
	return out, err
}

// Scan calls cb with each of peerID's decisions.
func (a *Approvals) Scan(ctx context.Context, peerID string, cb func(*s4wave_plugin_approval.PluginApproval) error) error {
	return a.withStore(ctx, func(store volume.ObjectStoreHandle) error {
		return ScanApprovals(ctx, store.GetObjectStore(), peerID, cb)
	})
}

// Admits reports whether peerID allowed the root. A missing record, a record
// that does not decode, and any state but ALLOWED all refuse access. The error
// says why a record could not be read.
func (a *Approvals) Admits(ctx context.Context, peerID, pluginID, manifestRoot string) (bool, error) {
	if peerID == "" {
		return false, nil
	}
	approval, err := a.Get(ctx, peerID, pluginID, manifestRoot)
	if err != nil {
		return false, err
	}
	return approvalAdmits(approval), nil
}

// approvalAdmits reports whether a decoded record allows its root. Only
// ALLOWED admits the root. A nil record and every other value, known or not,
// refuse access.
func approvalAdmits(approval *s4wave_plugin_approval.PluginApproval) bool {
	return approval.GetState() == s4wave_plugin_approval.PluginApprovalState_PluginApprovalState_ALLOWED
}

// withStore runs cb with the device-local ObjectStore.
func (a *Approvals) withStore(ctx context.Context, cb func(volume.ObjectStoreHandle) error) error {
	// Build the device-local store, refusing when it is unavailable.
	handle, _, ref, err := volume.ExBuildObjectStoreAPI(ctx, a.b, false, a.storeID, a.volumeID, nil)
	if err != nil {
		return err
	}
	if handle == nil || ref == nil {
		return errors.New("plugin approval object store unavailable")
	}
	defer ref.Release()

	// Run cb while the store reference is held.
	return cb(handle)
}
