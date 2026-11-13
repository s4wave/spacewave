package provider_local

import (
	"context"
	"sync"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// LocalSOHost publishes the local participant's view of a shared object and
// adds the local participant's operations to it.
type LocalSOHost struct {
	// le is the logger.
	le *logrus.Entry
	// privKey signs local operations.
	privKey crypto.PrivKey
	// peerID identifies the local participant.
	peerID peer.ID
	// sharedObjectID is the shared object id.
	sharedObjectID string
	// sfs builds the block transforms of key epochs.
	sfs *block_transform.StepFactorySet
	// soHost owns the shared object state.
	soHost *sobject.SOHost
	// snapMtx orders end with each snapshot Execute publishes.
	snapMtx sync.Mutex
	// ended is set by end; no snapshot is published after it. Guarded by
	// snapMtx.
	ended bool
	// stateSnapCtr holds the snapshot of the latest committed host state,
	// and nil once the host has ended.
	stateSnapCtr *ccontainer.CContainer[sobject.SharedObjectStateSnapshot]
	// publishedConfigCtr holds the config of the latest published snapshot.
	publishedConfigCtr *ccontainer.CContainer[*sobject.SharedObjectConfig]
	// lifetime ends when the mount that runs Execute stops. A commit that
	// started ends with it.
	lifetime context.Context
}

// NewLocalSOHost constructs a new LocalSOHost. lifetime is the context its
// owner runs Execute under.
func NewLocalSOHost(
	lifetime context.Context,
	le *logrus.Entry,
	privKey crypto.PrivKey,
	soHost *sobject.SOHost,
	sharedObjectID string,
	sfs *block_transform.StepFactorySet,
) (*LocalSOHost, error) {
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, err
	}
	return &LocalSOHost{
		le:                 le,
		privKey:            privKey,
		peerID:             peerID,
		sharedObjectID:     sharedObjectID,
		sfs:                sfs,
		soHost:             soHost,
		stateSnapCtr:       ccontainer.NewCContainer[sobject.SharedObjectStateSnapshot](nil),
		publishedConfigCtr: ccontainer.NewCContainer[*sobject.SharedObjectConfig](nil),
		lifetime:           lifetime,
	}, nil
}

// Execute publishes a snapshot of every committed host state, and its config,
// until ctx is canceled.
func (l *LocalSOHost) Execute(ctx context.Context) error {
	// Watch the committed host state.
	stateCtr, relStateCtr, err := l.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		return err
	}
	defer relStateCtr()
	defer l.end()

	// Publish a snapshot and the config of each.
	var state *sobject.SOState
	for {
		state, err = stateCtr.WaitValueChange(ctx, state, nil)
		if err != nil {
			return err
		}
		l.publishSnapshot(l.buildSnapshot(state))
		l.publishedConfigCtr.SetValue(state.GetConfig().CloneVT())
	}
}

// publishSnapshot sets the state snapshot unless the host has ended.
func (l *LocalSOHost) publishSnapshot(snap sobject.SharedObjectStateSnapshot) {
	l.snapMtx.Lock()
	defer l.snapMtx.Unlock()
	if !l.ended {
		l.stateSnapCtr.SetValue(snap)
	}
}

// end clears the state snapshot for good, so a holder of the state watchable
// stops seeing the state once the mount has ended. The tracker calls it when
// it retires the mount's run, before a delete returns, and Execute calls it
// as it exits.
func (l *LocalSOHost) end() {
	l.snapMtx.Lock()
	defer l.snapMtx.Unlock()
	l.ended = true
	l.stateSnapCtr.SetValue(nil)
}

// buildSnapshot returns the local participant's snapshot of state.
func (l *LocalSOHost) buildSnapshot(state *sobject.SOState) *sobject.SOStateParticipantHandle {
	return sobject.NewSOStateParticipantHandle(
		l.le,
		l.sfs,
		l.sharedObjectID,
		state,
		l.privKey,
		l.peerID,
	).WithConfigHistory(l.soHost.ReadConfigEntry).WithOperationVerifier(l.soHost.GetOperationVerifier()).WithGrantKeyring(l.soHost.GetGrantKeyring())
}

// waitPublishedConfig waits until body readers can observe target or a verified descendant.
func (l *LocalSOHost) waitPublishedConfig(ctx context.Context, target *sobject.SharedObjectConfig) error {
	if target == nil {
		return errors.New("published SharedObject configuration is unavailable")
	}

	// Accept the target configuration or a descendant with verified history.
	_, err := l.publishedConfigCtr.WaitValueWithValidator(ctx, func(current *sobject.SharedObjectConfig) (bool, error) {
		// Wait for the target's sequence number, then accept the target itself.
		if current == nil || current.GetConfigChainSeqno() < target.GetConfigChainSeqno() {
			return false, nil
		}
		if current.EqualVT(target) {
			return true, nil
		}
		changes, err := l.soHost.ReadConfigHistory(ctx, target.GetConfigChainHash(), current.GetConfigChainHash())
		if err != nil {
			return false, err
		}
		if err := sobject.VerifyConfigChainSuffix(l.sharedObjectID, target, current, changes); err != nil {
			return false, err
		}
		return true, nil
	}, nil)
	return err
}

// AccessSharedObjectState adds a reference to the durable state and returns
// the state container. released is called once the host's lifetime ends; the
// container then holds nil.
func (l *LocalSOHost) AccessSharedObjectState(ctx context.Context, released func()) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	if released == nil {
		return l.stateSnapCtr, func() {}, nil
	}
	stop := context.AfterFunc(l.lifetime, released)
	return l.stateSnapCtr, func() { stop() }, nil
}

// detach returns ctx's values without its cancellation, ended when the host's
// lifetime ends. The caller must call cancel.
func (l *LocalSOHost) detach(ctx context.Context) (context.Context, context.CancelFunc) {
	detached, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stop := context.AfterFunc(l.lifetime, cancel)
	return detached, func() {
		stop()
		cancel()
	}
}

// QueueOperation signs op as the local participant and adds it to the
// operation set. It returns the local operation id once the committed snapshot
// holds the operation, or a later checkpoint covers it.
func (l *LocalSOHost) QueueOperation(ctx context.Context, op []byte) (string, error) {
	// Sign the operation and add it to the host state.
	ctx, task := trace.NewTask(ctx, "alpha/local-so/queue-operation")
	defer task.End()
	localID, nonce, err := l.soHost.AddLocalOperation(ctx, l.le, l.sfs, l.privKey, op)
	if err != nil {
		return "", err
	}

	// Wait for the snapshot that holds the operation. The state write
	// committed it, so the wait ignores ctx's cancellation and ends with the
	// host's lifetime.
	ctx, cancel := l.detach(ctx)
	defer cancel()
	peerID := l.peerID.String()
	_, err = l.stateSnapCtr.WaitValueWithValidator(ctx, func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		if snap == nil {
			return false, nil
		}
		set, err := snap.GetOperationSet(ctx)
		if err != nil {
			return false, err
		}
		return set.Find(peerID, localID) != nil || set.Covers(peerID, nonce), nil
	}, nil)
	if err != nil {
		return "", err
	}
	return localID, nil
}
