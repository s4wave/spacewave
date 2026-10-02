package provider_local

import (
	"bytes"
	"context"
	"slices"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// watchSOHosting reconciles hosting each time the configuration head of so changes.
func (a *ProviderAccount) watchSOHosting(ctx context.Context, soID string, so *SharedObject) error {
	// Watch the accepted state of the object.
	states, release, err := so.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		return err
	}
	defer release()

	// Reconcile once at start, then on each new configuration head.
	var state *sobject.SOState
	var head []byte
	checked := false
	for {
		state, err = states.WaitValueChange(ctx, state, nil)
		if err != nil {
			return err
		}
		next := state.GetConfig().GetConfigChainHash()
		if checked && bytes.Equal(next, head) {
			continue
		}

		// Record the head only after its reconciliation succeeds.
		if err := a.reconcileSOHosting(ctx, soID, so); err != nil {
			return err
		}
		head, checked = next, true
	}
}

// reconcileSOHosting moves hosting of so to follow its latest ownership
// transfer. The account holding the successor commits the carried departure
// and promotes its storage identity in one change, so it signs as host once the
// departure settles, and stops naming a remote endpoint. Every other account
// routes to the successor. An object without a departure transfer keeps the
// endpoint its enrollment recorded.
func (a *ProviderAccount) reconcileSOHosting(ctx context.Context, soID string, so *SharedObject) error {
	// The storage identity always signs; the session identity signs while its transport runs.
	keys := map[string]crypto.PrivKey{so.localPid.String(): so.localPriv}
	if st := a.GetSessionTransport(); st != nil {
		keys[st.GetPeerID().String()] = st.GetPrivKey()
	}

	// A remaining local owner commits a departure carried at the head and
	// promotes the storage identity in the same change.
	for _, key := range keys {
		if _, err := sobject.CompleteSOOwnershipTransfer(ctx, so.soHost, key, so.localPid.String()); err != nil {
			return err
		}
	}

	// Route to the latest successor, or host when it is a local identity.
	successor, err := sobject.ReadSOOwnershipSuccessor(ctx, so.soHost)
	if err != nil || successor == "" {
		return err
	}
	key, local := keys[successor]
	if !local {
		return a.setSOEndpoint(ctx, soID, successor)
	}
	if err := sobject.PromoteSOOwner(ctx, so.soHost, key, so.localPid.String()); err != nil {
		return err
	}
	return a.setSOEndpoint(ctx, soID, "")
}

// setSOEndpoint persists the transport peer that hosts soID, empty when this
// account hosts it, and retains a link to a remote host.
func (a *ProviderAccount) setSOEndpoint(ctx context.Context, soID, endpoint string) error {
	// Record the endpoint; an unchanged entry needs no new link.
	changed, err := a.writeSOEndpoint(ctx, soID, endpoint)
	if err != nil || !changed || endpoint == "" {
		return err
	}

	// Keep a link to the new host. A stopped sync retains it when it starts.
	remote, err := peer.IDB58Decode(endpoint)
	if err != nil {
		return err
	}
	if err := a.RetainP2PPeer(ctx, remote); err != nil {
		a.le.WithError(err).WithField("so-id", soID).Debug("deferred retaining the shared object host")
	}
	return nil
}

// writeSOEndpoint replaces the endpoint of soID in the stored list under the
// account mutation lock. It reports false when the entry is absent or unchanged.
func (a *ProviderAccount) writeSOEndpoint(ctx context.Context, soID, endpoint string) (bool, error) {
	// Hold the account mutation lock across the read and write.
	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		return false, err
	}
	defer relMtx()

	// Find the entry to update.
	list := a.soListCtr.GetValue().CloneVT()
	index := slices.IndexFunc(list.GetSharedObjects(), func(entry *sobject.SharedObjectListEntry) bool {
		return entry.GetRef().GetProviderResourceRef().GetId() == soID
	})
	if index == -1 || list.GetSharedObjects()[index].GetTransportPeerId() == endpoint {
		return false, nil
	}

	// Persist the new endpoint before publishing it.
	list.SharedObjects[index].TransportPeerId = endpoint
	if err := a.writeSharedObjectList(ctx, list); err != nil {
		return false, err
	}
	a.soListCtr.SetValue(list)
	return true, nil
}

// waitSOValidator waits until the local storage identity can validate so.
func waitSOValidator(ctx context.Context, so *SharedObject) error {
	// Watch the accepted state of the object.
	states, release, err := so.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		return err
	}
	defer release()

	// Return once the storage identity holds a validating role.
	_, err = states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
		return slices.ContainsFunc(state.GetConfig().GetParticipants(), func(p *sobject.SOParticipantConfig) bool {
			return p.GetPeerId() == so.localPid.String() && sobject.IsValidatorOrOwner(p.GetRole())
		}), nil
	}, nil)
	return err
}
