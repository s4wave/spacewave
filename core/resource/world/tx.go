package resource_world

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
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
	// terminalLocker serializes terminal transaction operations.
	terminalLocker sync.Locker
	// terminal records acceptance of a commit or discard under terminalLocker.
	terminal bool
}

// NewTxResource creates a new TxResource.
//
// engine is optional - if provided, TypedObjectResourceService is registered on the mux.
func NewTxResource(
	le *logrus.Entry,
	b bus.Bus,
	tx world.Tx,
	lookupOp world.LookupOp,
	engine world.Engine,
	opts ...WorldStateResourceOption,
) *TxResource {
	// Construct the transaction Resource with the granting World options.
	wsResource := NewWorldStateResource(le, b, tx, lookupOp, opts...)
	if engine != nil {
		wsResource.storage = engine
	}
	mux := wsResource.mux.(srpc.Mux)
	txResource := &TxResource{
		WorldStateResource: wsResource,
		tx:                 tx,
		mux:                mux,
		engine:             engine,
		terminalLocker:     &sync.Mutex{},
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

// CommitMutations applies ordered mutations and commits the transaction.
func (r *TxResource) CommitMutations(
	ctx context.Context,
	req *s4wave_world.CommitMutationsRequest,
) (_ *s4wave_world.CommitMutationsResponse, retErr error) {
	// Keep mutations and their commit together with every terminal transaction operation.
	r.terminalLocker.Lock()
	if r.terminal {
		r.terminalLocker.Unlock()
		return nil, errors.New("transaction is closed")
	}
	r.terminal = true
	release := false
	defer func() {
		r.terminalLocker.Unlock()
		if retErr != nil || release {
			r.Release()
		}
	}()

	// Apply every requested mutation to the transaction before committing it.
	results := make([]*s4wave_world.TransactionMutationResult, 0, len(req.GetMutations()))
	for i, mutation := range req.GetMutations() {
		// Stop before the next mutation when the caller cancels the request.
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		switch mutation := mutation.GetMutation().(type) {
		case *s4wave_world.TransactionMutation_CreateObject:
			obj, err := r.tx.CreateObject(ctx, mutation.CreateObject.GetObjectKey(), mutation.CreateObject.GetRootRef())
			if err != nil {
				world.ReleaseObjectState(obj)
				return nil, err
			}
			key := obj.GetKey()
			_, rev, err := obj.GetRootRef(ctx)
			world.ReleaseObjectState(obj)
			if err != nil {
				return nil, err
			}
			results = append(results, &s4wave_world.TransactionMutationResult{
				Result: &s4wave_world.TransactionMutationResult_CreateObject{
					CreateObject: &s4wave_world.CreateObjectMutationResult{ObjectKey: key, Rev: rev},
				},
			})
		case *s4wave_world.TransactionMutation_SetObjectRoot:
			obj, found, err := r.tx.GetObject(ctx, mutation.SetObjectRoot.GetObjectKey())
			if err != nil {
				world.ReleaseObjectState(obj)
				return nil, err
			}
			if !found {
				return nil, errors.Errorf("object not found: %s", mutation.SetObjectRoot.GetObjectKey())
			}
			rev, err := obj.SetRootRef(ctx, mutation.SetObjectRoot.GetRootRef())
			world.ReleaseObjectState(obj)
			if err != nil {
				return nil, err
			}
			results = append(results, &s4wave_world.TransactionMutationResult{
				Result: &s4wave_world.TransactionMutationResult_SetObjectRoot{
					SetObjectRoot: &s4wave_world.SetObjectRootMutationResult{Rev: rev},
				},
			})
		case *s4wave_world.TransactionMutation_SetGraphQuad:
			q := mutation.SetGraphQuad.GetQuad()
			if q == nil {
				return nil, errors.Errorf("mutation %d has no graph quad", i)
			}
			if err := r.tx.SetGraphQuad(ctx, world.NewGraphQuad(q.GetSubject(), q.GetPredicate(), q.GetObj(), q.GetLabel())); err != nil {
				return nil, err
			}
			results = append(results, &s4wave_world.TransactionMutationResult{
				Result: &s4wave_world.TransactionMutationResult_SetGraphQuad{
					SetGraphQuad: &s4wave_world.SetGraphQuadResponse{},
				},
			})
		default:
			return nil, errors.Errorf("mutation %d is unset", i)
		}
	}

	// Fence terminal transaction operations under the transaction lock.
	if err := r.tx.Commit(ctx); err != nil {
		return nil, err
	}
	release = true
	return &s4wave_world.CommitMutationsResponse{Results: results}, nil
}

// Commit commits the transaction.
func (r *TxResource) Commit(ctx context.Context, req *s4wave_world.CommitRequest) (*s4wave_world.CommitResponse, error) {
	// Fence terminal transaction operations under the transaction lock.
	r.terminalLocker.Lock()
	if r.terminal {
		r.terminalLocker.Unlock()
		return nil, errors.New("transaction is closed")
	}
	r.terminal = true
	defer func() {
		r.terminalLocker.Unlock()
		r.Release()
	}()

	// Close typed descendants and commit after this terminal operation wins the lock.
	if r.typedResource != nil {
		r.typedResource.Close()
	}
	err := r.tx.Commit(ctx)
	if err != nil {
		return nil, err
	}
	return &s4wave_world.CommitResponse{}, nil
}

// Discard discards the transaction without committing changes.
func (r *TxResource) Discard(ctx context.Context, req *s4wave_world.DiscardRequest) (*s4wave_world.DiscardResponse, error) {
	// Release typed descendants and discard the transaction exactly once.
	r.terminalLocker.Lock()
	if r.terminal {
		r.terminalLocker.Unlock()
		return &s4wave_world.DiscardResponse{}, nil
	}
	r.terminal = true
	r.terminalLocker.Unlock()
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
