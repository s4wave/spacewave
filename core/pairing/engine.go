package pairing

import (
	"context"
	"io"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Status matches the Session resource's pairing status enum.
type Status int32

const (
	StatusIdle Status = iota
	StatusCodeGenerated
	StatusWaitingForPeer
	StatusPeerConnected
	StatusVerifyingEmoji
	StatusVerified
	StatusFailed
	StatusSignalingFailed
	StatusConnectionTimeout
	StatusWaitingForRemote
	StatusBothConfirmed
	StatusPairingRejected
	StatusConfirmationTimeout
	StatusEnrolling
	StatusSelectingAccount
)

// Snapshot describes one Session's current pairing operation.
type Snapshot struct {
	// Status is the operation's current phase.
	Status Status
	// Code is the pairing code this client registered, when it created one.
	Code string
	// RemotePeerID is the other client's Session peer once it is known.
	RemotePeerID peer.ID
	// Emoji is the SAS sequence both people compare before approving.
	Emoji []string
	// ErrMsg describes the failure when Status is a failure phase.
	ErrMsg string
	// AccountID is the account selected for enrollment.
	AccountID string
	// AccountName is the selected account's display name.
	AccountName string
	// ProviderID is the provider that holds the selected account.
	ProviderID string
	// RemoteLabel is the name the other client gave itself, shown on the
	// approval screen and recorded for the Session it enrolls.
	RemoteLabel string
	// Receiving indicates this client is adding the selected account.
	Receiving bool
	// Choice is the proposed account relationship once both offers are known.
	Choice *AccountChoice
}

// Engine owns approval and enrollment for one mounted Session. Account adapters
// own provider authorization and persistence; its transport remains owned by the
// Session's ordinary transport lifecycle.
type Engine struct {
	ctx       context.Context
	le        *logrus.Entry
	b         bus.Bus
	key       crypto.PrivKey
	session   session.Session
	peerID    peer.ID
	adapter   AccountAdapter
	transport func(context.Context, Relay) (*transport.SessionTransport, error)
	bcast     broadcast.Broadcast
	active    *attempt
}

// attempt is one pairing operation. The Engine's bcast guards snapshot,
// result, and release; the other fields are fixed when the attempt begins.
type attempt struct {
	// snapshot is the published state of the operation.
	snapshot Snapshot
	// offering indicates this client created the pairing code.
	offering bool
	// offerCurrent includes this Session's account in the exchange.
	offerCurrent bool
	// label names this client to the other client; empty uses the machine name.
	label string
	// choose delivers the code-entering client's account outcome.
	choose chan AccountOutcome
	// confirm delivers the local approval decision.
	confirm chan bool
	// cancel stops the operation's goroutines.
	cancel context.CancelFunc
	// result is the durable receiving Session after enrollment.
	result *session.SessionRef
	// release drops resources retained through enrollment.
	release func()
}

// NewEngine binds a Session's key, provider adapter, and transport owner.
func NewEngine(ctx context.Context, le *logrus.Entry, b bus.Bus, mounted session.Session, adapter AccountAdapter, getTransport func(context.Context, Relay) (*transport.SessionTransport, error)) (*Engine, error) {
	key := mounted.GetPrivKey()
	if ctx == nil || key == nil {
		return nil, errors.New("pairing requires an unlocked Session lifecycle")
	}
	peerID, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return nil, err
	}
	e := &Engine{ctx: ctx, le: le, b: b, key: key, session: mounted, peerID: peerID, adapter: adapter, transport: getTransport}
	context.AfterFunc(ctx, e.Clear)
	return e, nil
}

// Context returns the mounted Session lifetime for direct signaling operations.
func (e *Engine) Context() context.Context { return e.ctx }

// begin cancels the prior attempt before publishing another operation identity.
func (e *Engine) begin(offering, offerCurrent bool, label, code string, remote peer.ID, status Status) (context.Context, *attempt) {
	ctx, cancel := context.WithCancel(e.ctx)
	active := &attempt{
		offering:     offering,
		offerCurrent: offerCurrent,
		label:        label,
		cancel:       cancel,
		confirm:      make(chan bool, 1),
		choose:       make(chan AccountOutcome, 1),
		snapshot:     Snapshot{Status: status, Code: code, RemotePeerID: remote, Receiving: !offering},
	}
	var previous *attempt
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		previous, e.active = e.active, active
		broadcast()
	})
	releaseAttempt(previous)
	return ctx, active
}

// Clear releases the active exchange and its temporary transport demand.
func (e *Engine) Clear() {
	var active *attempt
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		active, e.active = e.active, nil
		broadcast()
	})
	releaseAttempt(active)
}

func releaseAttempt(active *attempt) {
	if active == nil {
		return
	}
	if active.cancel != nil {
		active.cancel()
	}
	if active.release != nil {
		active.release()
		active.release = nil
	}
}

func (e *Engine) retain(active *attempt, release func()) bool {
	var retained bool
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if e.active == active {
			previous := active.release
			active.release = func() {
				release()
				if previous != nil {
					previous()
				}
			}
			retained = true
		}
	})
	if !retained {
		release()
	}
	return retained
}

func (e *Engine) releaseResources(active *attempt) {
	var release func()
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if e.active == active {
			release, active.release = active.release, nil
		}
	})
	if release != nil {
		release()
	}
}

func (e *Engine) update(active *attempt, update func(*attempt)) {
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if e.active == active {
			update(active)
			broadcast()
		}
	})
}

func (e *Engine) fail(active *attempt, status Status, err error) {
	e.update(active, func(a *attempt) {
		if a.snapshot.Status == StatusPairingRejected {
			return
		}
		a.snapshot.Status = status
		a.snapshot.ErrMsg = err.Error()
	})
}

// SetFailed publishes a direct signaling failure, including before a link exists.
func (e *Engine) SetFailed(message string) {
	var previous *attempt
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		previous = e.active
		e.active = &attempt{snapshot: Snapshot{Status: StatusFailed, ErrMsg: message}}
		broadcast()
	})
	releaseAttempt(previous)
}

// ConfirmSAS submits the current screen's bilateral approval decision.
func (e *Engine) ConfirmSAS(confirmed bool) {
	var canceled *attempt
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if e.active == nil {
			return
		}
		if !confirmed && e.active.snapshot.Status != StatusVerifyingEmoji && e.active.snapshot.Status != StatusBothConfirmed {
			canceled = e.active
			canceled.snapshot.Status = StatusPairingRejected
			canceled.snapshot.ErrMsg = "Pairing canceled"
			broadcast()
			return
		}
		if e.active.snapshot.Status != StatusVerifyingEmoji {
			return
		}
		select {
		case e.active.confirm <- confirmed:
		default:
		}
	})
	if canceled != nil {
		canceled.cancel()
		e.releaseResources(canceled)
	}
}

// SelectAccount fixes the code-entering client's account proposal before approval.
// A different choice requires a new exchange once the proposal has been submitted.
func (e *Engine) SelectAccount(outcome AccountOutcome) error {
	var err error
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		active := e.active
		if active == nil || active.offering || active.snapshot.Status != StatusSelectingAccount {
			err = errors.New("this client is not choosing a pairing account")
			return
		}
		choice := active.snapshot.Choice.CloneVT()
		choice.Outcome = outcome
		if err = choice.Validate(); err != nil {
			return
		}
		select {
		case active.choose <- outcome:
			active.snapshot.Choice = choice
			active.snapshot.Status = StatusPeerConnected
			broadcast()
		default:
			err = errors.New("a pairing account was already selected")
		}
	})
	return err
}

// Watch observes coherent snapshots until cancellation or a callback error.
func (e *Engine) Watch(ctx context.Context, fn func(Snapshot) error) error {
	for {
		snapshot, wait := e.Snapshot()
		if err := fn(snapshot); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wait:
		}
	}
}

// Snapshot returns coherent state and a channel that closes on the next change.
func (e *Engine) Snapshot() (Snapshot, <-chan struct{}) {
	var snapshot Snapshot
	var wait <-chan struct{}
	e.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
		wait = getWait()
		if e.active != nil {
			snapshot = e.active.snapshot
			snapshot.Emoji = slices.Clone(snapshot.Emoji)
			snapshot.Choice = snapshot.Choice.CloneVT()
		}
	})
	return snapshot, wait
}

var (
	ErrExchangeMissing      = errors.New("pairing exchange is missing")
	ErrExchangeUnconfirmed  = errors.New("account enrollment is not complete")
	ErrExchangePeerMismatch = errors.New("pairing exchange peer does not match")
)

// Result returns the durable receiving attachment. The offering side returns
// nil. Repeated reads cannot grant more access or enroll another Session.
func (e *Engine) Result(remote peer.ID) (*session.SessionRef, error) {
	var result *session.SessionRef
	var err error
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		switch {
		case e.active == nil:
			err = ErrExchangeMissing
		case e.active.snapshot.RemotePeerID != remote:
			err = ErrExchangePeerMismatch
		case e.active.snapshot.Status != StatusBothConfirmed || (e.active.snapshot.Receiving && e.active.result == nil):
			err = ErrExchangeUnconfirmed
		default:
			if e.active.result != nil {
				result = e.active.result.CloneVT()
			}
		}
	})
	return result, err
}

// StartOnStream runs enrollment on an already authenticated duplex connection.
// The caller retains the stream's transport through completion. Label names
// this client to the other client; empty uses the machine name.
func (e *Engine) StartOnStream(ctx context.Context, stream io.ReadWriteCloser, remote peer.ID, offering, offerCurrent bool, label string) {
	attemptCtx, active := e.begin(offering, offerCurrent, label, "", remote, StatusPeerConnected)
	stop := context.AfterFunc(ctx, active.cancel)
	defer stop()
	e.runStream(attemptCtx, active, stream, remote, nil)
}

// StartDirect uses the same approval and enrollment on a manually signaled link.
func (e *Engine) StartDirect(lnk link.Link, offering, offerCurrent bool) {
	ctx, active := e.begin(offering, offerCurrent, "", "", lnk.GetRemotePeer(), StatusPeerConnected)
	go e.runDirect(ctx, active, lnk)
}
