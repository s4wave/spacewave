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
	*WorldStateResource
	tx     world.Tx
	mux    srpc.Mux
	engine world.Engine

	typedResource  *TypedObjectResource
	released       atomic.Bool
	terminalLocker sync.Locker
	terminal       bool
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
	wsResource := NewWorldStateResource(le, b, tx, lookupOp, opts...)
	mux := wsResource.mux.(srpc.Mux)
	txResource := &TxResource{
		WorldStateResource: wsResource,
		tx:                 tx,
		mux:                mux,
		engine:             engine,
		terminalLocker:     &sync.Mutex{},
	}
	// Register TxResourceService on the same mux
	_ = s4wave_world.SRPCRegisterTxResourceService(mux, txResource)
	// Register TypedObjectResourceService if engine is available
	if engine != nil {
		typedResource := newTypedObjectResourceWithSessionPeerID(
			le,
			b,
			tx,
			engine,
			wsResource.sessionPeerID,
			wsResource.sessionPeerIDBound,
		)
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
				return nil, err
			}
			_, rev, err := obj.GetRootRef(ctx)
			if err != nil {
				return nil, err
			}
			results = append(results, &s4wave_world.TransactionMutationResult{
				Result: &s4wave_world.TransactionMutationResult_CreateObject{
					CreateObject: &s4wave_world.CreateObjectMutationResult{ObjectKey: obj.GetKey(), Rev: rev},
				},
			})
		case *s4wave_world.TransactionMutation_SetObjectRoot:
			obj, found, err := r.tx.GetObject(ctx, mutation.SetObjectRoot.GetObjectKey())
			if err != nil {
				return nil, err
			}
			if !found {
				return nil, errors.Errorf("object not found: %s", mutation.SetObjectRoot.GetObjectKey())
			}
			rev, err := obj.SetRootRef(ctx, mutation.SetObjectRoot.GetRootRef())
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

	if err := r.tx.Commit(ctx); err != nil {
		return nil, err
	}
	release = true
	return &s4wave_world.CommitMutationsResponse{Results: results}, nil
}

// Commit commits the transaction.
func (r *TxResource) Commit(ctx context.Context, req *s4wave_world.CommitRequest) (*s4wave_world.CommitResponse, error) {
	r.terminalLocker.Lock()
	if r.terminal {
		r.terminalLocker.Unlock()
		return nil, errors.New("transaction is closed")
	}
	r.terminal = true
	defer r.terminalLocker.Unlock()

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
