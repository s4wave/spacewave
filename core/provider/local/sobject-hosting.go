package provider_local

import (
	"bytes"
	"context"
	"slices"
	"sync/atomic"

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

		// A test may hold the reconcile back.
		if hook := soHostingReconcileHook.Load(); hook != nil {
			if err := (*hook)(ctx); err != nil {
				return err
			}
		}

		// Record the head only after its reconciliation succeeds.
		if err := a.reconcileSOHosting(ctx, soID, so); err != nil {
			return err
		}
		head, checked = next, true
	}
}

// soHostingReconcileHook, when set by a test, runs before each hosting
// reconcile and may hold it back. It is nil in production.
// Hosting routines read it concurrently, so it is set atomically.
var soHostingReconcileHook atomic.Pointer[func(ctx context.Context) error]

// reconcileSOHosting follows the owners named by the configuration of so.
// The account holding the successor of a departure commits it and promotes its
// storage identity in one change. An account whose storage identity owns so
// enrolls its running Session as an owner, so the configuration names the
// endpoint that serves so, and drops it from the trimming roster. Every
// account keeps a link to each other owner, so a participant reaches the owner
// account's current Session after the Session it joined through is replaced,
// and to each block peer soLinkPeers names, so a member reads a linked folder
// from the Device that links it while the owners are offline.
func (a *ProviderAccount) reconcileSOHosting(ctx context.Context, soID string, so *SharedObject) error {
	// The storage identity always signs; the session identity signs while its transport runs.
	st := a.GetSessionTransport()
	keys := map[string]crypto.PrivKey{so.localPid.String(): so.localPriv}
	if st != nil {
		keys[st.GetPeerID().String()] = st.GetPrivKey()
	}

	// A remaining local owner commits a departure carried at the head and
	// promotes the storage identity in the same change.
	for _, key := range keys {
		if _, err := sobject.CompleteSOOwnershipTransfer(ctx, so.soHost, key, so.localPid.String()); err != nil {
			return err
		}
	}

	// Promote a local successor of the latest departure.
	successor, err := sobject.ReadSOOwnershipSuccessor(ctx, so.soHost)
	if err != nil {
		return err
	}
	if key, local := keys[successor]; local {
		if err := sobject.PromoteSOOwner(ctx, so.soHost, key, so.localPid.String()); err != nil {
			return err
		}
	}

	// Enroll the running Session when the storage identity owns so. A pending
	// departure at the head waits for its successor, which commits it only
	// while it remains the head. A group's voters decide its owners.
	state, err := so.soHost.GetHostState(ctx)
	if err != nil {
		return err
	}
	config := state.GetConfig()
	own, err := sobject.ReadSOOwnership(ctx, config, so.soHost.ReadConfigEntry)
	if err != nil {
		return err
	}
	owners := own.SigningOwners()
	if st != nil && own.Settled() && !config.IsGroupControl() && own.Owns(so.localPid.String()) {
		if err := a.enrollSOSession(ctx, soID, so, st.GetPeerID(), owners); err != nil {
			return err
		}
	}

	// Keep a link to every other owner and block peer while the storage
	// identity serves so. A storage identity never connects and waits in
	// signaling.
	if !soLinksWanted(config, own, so.localPid.String()) {
		return nil
	}
	for _, id := range soLinkPeers(config, owners, keys) {
		remote, err := peer.IDB58Decode(id)
		if err != nil {
			return err
		}
		if err := a.RetainP2PPeer(ctx, remote); err != nil {
			a.le.WithError(err).WithField("so-id", soID).Debug("deferred retaining a shared object peer")
		}
	}
	return nil
}

// soLinkPeers returns the peers this account keeps links to for so, leaving
// out its own keys: the owners, and the participants it exchanges blocks with.
// A participant that may write so is a block peer of every other participant,
// and a reader is one of the writers only. The Device that publishes a linked
// folder writes the Space, so it may be the only holder of the folder's
// chunks: every member keeps a link to it and reads them from it while the
// owners are offline. Readers hold only what they fetched, so they keep no
// links to one another.
func soLinkPeers(config *sobject.SharedObjectConfig, owners []string, local map[string]crypto.PrivKey) []string {
	// A local key that may write makes this account a writer.
	var writes bool
	for id := range local {
		writes = writes || sobject.CanWriteOps(config.GetParticipantRole(id))
	}

	// Collect the owners and the block peers, each once.
	var peers []string
	add := func(id string) {
		if _, self := local[id]; !self && !slices.Contains(peers, id) {
			peers = append(peers, id)
		}
	}
	for _, owner := range owners {
		add(owner)
	}
	for _, p := range config.GetParticipants() {
		if role := p.GetRole(); sobject.CanWriteOps(role) || (writes && sobject.CanReadState(role)) {
			add(p.GetPeerId())
		}
	}
	return peers
}

// enrollSOSession adds the Session peer as an owner of so unless owners
// already name it, and keeps it off the trimming roster. The storage identity
// writes every operation of so, so the Session never builds on the
// checkpointer's edits and on the roster would hold the stable point at zero.
func (a *ProviderAccount) enrollSOSession(ctx context.Context, soID string, so *SharedObject, session peer.ID, owners []string) error {
	// The storage identity signs the Session's addition.
	if !slices.Contains(owners, session.String()) {
		pub, err := session.ExtractPublicKey()
		if err != nil {
			return err
		}
		role := sobject.SOParticipantRole_SOParticipantRole_OWNER
		_, _, _, err = sobject.AddSOParticipant(ctx, so.soHost, soID, so.localPriv, so.localPid.String(), session.String(), pub, role, "", "")
		if err != nil {
			return err
		}
	}

	// Drop the Session from the trimming roster.
	_, err := sobject.DropFromSORoster(ctx, so.soHost, []string{session.String()}, so.localPriv)
	return err
}

// soLinksWanted reports whether the storage identity self still serves so and
// keeps links to its peers. An owner that departs hands so to its successor,
// which commits the departure after the leaver has lost access, so the leaver
// reads its departure as pending forever. Its copy stays listed, but a link it
// demanded would be one-sided: the peer holds none, closes it after the link
// hold-open and the leaver redials at once, forever. The successor keeps the
// link while the hand-over is pending.
func soLinksWanted(config *sobject.SharedObjectConfig, own *sobject.SOOwnership, self string) bool {
	if own.Departing(self) {
		return false
	}
	_, ok := participantSet(config.GetParticipants())[self]
	return ok
}
