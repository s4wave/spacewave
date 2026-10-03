package provider_local

import (
	"context"

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
	// stateSnapCtr holds the snapshot of the latest host state.
	stateSnapCtr *ccontainer.CContainer[sobject.SharedObjectStateSnapshot]
	// publishedConfigCtr holds the config of the latest published snapshot.
	publishedConfigCtr *ccontainer.CContainer[*sobject.SharedObjectConfig]
}

// NewLocalSOHost constructs a new LocalSOHost.
func NewLocalSOHost(
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
	}, nil
}

// Execute publishes a snapshot of every host state until ctx is canceled.
func (l *LocalSOHost) Execute(ctx context.Context) error {
	// Publish a snapshot and the config of every host state.
	stateCtr, relStateCtr, err := l.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		return err
	}
	defer relStateCtr()
	var state *sobject.SOState
	for {
		state, err = stateCtr.WaitValueChange(ctx, state, nil)
		if err != nil {
			return err
		}
		l.stateSnapCtr.SetValue(l.buildSnapshot(state))
		l.publishedConfigCtr.SetValue(state.GetConfig().CloneVT())
	}
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
	).WithConfigHistory(l.soHost.ReadConfigEntry)
}

// waitPublishedConfig waits until body readers can observe target or a verified descendant.
func (l *LocalSOHost) waitPublishedConfig(ctx context.Context, target *sobject.SharedObjectConfig) error {
	if target == nil {
		return errors.New("published SharedObject configuration is unavailable")
	}

	// Accept the target configuration or a descendant with verified history.
	_, err := l.publishedConfigCtr.WaitValueWithValidator(ctx, func(current *sobject.SharedObjectConfig) (bool, error) {
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

// AccessSharedObjectState adds a reference to the state and returns the state container.
func (l *LocalSOHost) AccessSharedObjectState(ctx context.Context, released func()) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	return l.stateSnapCtr, func() {}, nil
}

// QueueOperation signs op as the local participant and adds it to the
// operation set. It returns the local operation id once the published snapshot
// holds the operation. When ctx carries sobject.WithOrderedOperation, the state
// write is ordered: applied on return and durable at the store's next
// durability point.
func (l *LocalSOHost) QueueOperation(ctx context.Context, op []byte) (string, error) {
	// Sign the operation and add it to the host state.
	ctx, task := trace.NewTask(ctx, "alpha/local-so/queue-operation")
	defer task.End()
	localID, nonce, err := l.soHost.AddLocalOperation(ctx, l.le, l.sfs, l.privKey, op)
	if err != nil {
		return "", err
	}

	// Wait for the snapshot that holds the operation, or a later checkpoint.
	_, err = l.stateSnapCtr.WaitValueWithValidator(ctx, func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		// A snapshot holds the operation, or its checkpoint covers it.
		if snap == nil {
			return false, nil
		}
		set, err := snap.GetOperationSet(ctx)
		if err != nil {
			return false, err
		}
		peerID := l.peerID.String()
		return set.Find(peerID, localID) != nil || set.Covers(peerID, nonce), nil
	}, nil)
	if err != nil {
		return "", err
	}
	return localID, nil
}
