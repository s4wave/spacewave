package resource_world

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// TxResource wraps a Tx for resource access.
// It embeds WorldStateResource and adds Commit/Discard operations.
type TxResource struct {
	// WorldStateResource serves transaction-scoped World operations.
	*WorldStateResource
	// tx is the transaction released by this Resource.
	tx world.Tx
	// mux serves the mounted Resource RPCs.
	mux srpc.Mux
	// engine retains the granted transactional World capability.
	engine world.Engine

	// typedResource owns the typed handles created under this mount.
	typedResource *TypedObjectResource
	// released guards the effectless transition to released state.
	released atomic.Bool
	// terminalMtx serializes terminal transaction operations.
	terminalMtx sync.Mutex
	// terminal records acceptance of a commit or discard under terminalMtx.
	terminal bool
}

// newTxResource creates transaction RPCs with the granting mount policy.
//
// engine is optional. If provided, TypedObjectResourceService is registered
// on the mux.
func newTxResource(
	le *logrus.Entry,
	b bus.Bus,
	tx world.Tx,
	lookupOp world.LookupOp,
	engine world.Engine,
	mountEngine func(resource_server.ResourceClientContext, world.Engine, ...WorldStateResourceOption) (uint32, error),
	opts ...WorldStateResourceOption,
) *TxResource {
	// Construct the transaction Resource with the granting World options.
	access := newMountOptions(opts...)
	engine = world.GuardEngine(engine, access.writeGuard)
	mux := srpc.NewMux()
	wsResource := newWorldStateResource(le, b, tx, lookupOp, mux, engine, false, mountEngine, access)
	txResource := &TxResource{
		WorldStateResource: wsResource,
		tx:                 tx,
		mux:                mux,
		engine:             engine,
	}

	// Register transaction control on the World Resource mux.
	_ = s4wave_world.SRPCRegisterTxResourceService(mux, txResource)

	// Bind typed descendants to the transaction and granting engine options.
	if engine != nil {
		typedResource := NewTypedObjectResource(le, b, tx, engine, opts...)
		txResource.typedResource = typedResource
		_ = s4wave_world.SRPCRegisterTypedObjectResourceService(mux, typedResource)
	}
	return txResource
}

// CommitTx commits the transaction.
func (r *TxResource) CommitTx(ctx context.Context, req *s4wave_world.CommitRequest) (*s4wave_world.CommitResponse, error) {
	// Fence terminal transaction operations under the transaction lock.
	r.terminalMtx.Lock()
	if r.terminal {
		r.terminalMtx.Unlock()
		return nil, errors.New("transaction is closed")
	}
	r.terminal = true
	defer func() {
		r.terminalMtx.Unlock()
		r.Release()
	}()

	// Close typed descendants and commit after this terminal operation wins the lock.
	if r.typedResource != nil {
		r.typedResource.Close()
	}
	return s4wave_world.NewCommitResponse(req.GetKnownErrorCode(), r.tx.Commit(ctx))
}

// Discard discards the transaction without committing changes.
func (r *TxResource) Discard(ctx context.Context, req *s4wave_world.DiscardRequest) (*s4wave_world.DiscardResponse, error) {
	// Release typed descendants and discard the transaction exactly once.
	r.terminalMtx.Lock()
	if r.terminal {
		r.terminalMtx.Unlock()
		return &s4wave_world.DiscardResponse{}, nil
	}
	r.terminal = true
	r.terminalMtx.Unlock()
	r.Release()
	return &s4wave_world.DiscardResponse{}, nil
}

// Release discards the underlying transaction exactly once.
func (r *TxResource) Release() {
	if !r.released.CompareAndSwap(false, true) {
		return
	}
	if r.typedResource != nil {
		r.typedResource.Close()
	}
	r.tx.Discard()
}

// _ is a type assertion
var _ s4wave_world.SRPCTxResourceServiceServer = (*TxResource)(nil)
