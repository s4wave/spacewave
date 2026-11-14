package resource_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// errBatchRefused ends a batch attempt so its transaction is discarded.
var errBatchRefused = errors.New("world batch refused")

// batchOp is an op entry of a batch, resolved, decoded and prepared before
// the batch's transaction opens.
type batchOp struct {
	// op is the decoded operation.
	op world.Operation
	// sender is the peer the op is applied as.
	sender peer.ID
}

// ExecuteWorldBatch applies the mutations in order in one write transaction
// and commits it. The first refused mutation discards the transaction and
// names its index; a stale base replays the whole list in a fresh transaction.
//
// Every op entry is resolved and prepared before the transaction takes the
// engine's writer, so a cold plugin load costs this caller time while other
// writers proceed, and ends with the caller's context error. Because a refused
// batch discards its transaction, a plugin that times out inside it is
// reported as WORLD_ERROR_CODE_PLUGIN_OP_TIMEOUT: nothing was written.
func (r *EngineResource) ExecuteWorldBatch(ctx context.Context, req *s4wave_world.ExecuteWorldBatchRequest) (*s4wave_world.ExecuteWorldBatchResponse, error) {
	mutations := req.GetMutations()
	ops, refused, release, err := r.prepareBatchOps(ctx, mutations)
	defer release()
	if err != nil {
		return nil, err
	}

	var resp *s4wave_world.ExecuteWorldBatchResponse
	err = world.ExecTransaction(ctx, r.engine, true, func(ctx context.Context, wtx world.WorldState) error {
		for i, m := range mutations {
			if refused != nil && int(refused.GetFailedIndex()) == i {
				resp = refused
				return errBatchRefused
			}
			refusal, err := applyWorldMutation(ctx, wtx, m, ops[i])
			if err != nil {
				return errors.Wrapf(err, "mutations[%d]", i)
			}
			if refusal != nil {
				refusal.FailedIndex = uint32(i)
				resp = refusal
				return errBatchRefused
			}
		}

		// Report the sequence number the commit publishes.
		seqno, err := wtx.GetSeqno(ctx)
		if err != nil {
			return err
		}
		resp = &s4wave_world.ExecuteWorldBatchResponse{Seqno: seqno}
		return nil
	})
	if err != nil && !errors.Is(err, errBatchRefused) {
		return nil, err
	}
	return resp, nil
}

// prepareBatchOps resolves, decodes and prepares the op entries of mutations
// in order, outside any transaction. ops is indexed like mutations and holds
// nil for entries that are not ops. The first op the mount cannot resolve
// stops the walk and is returned as the refusal at its index. release frees
// every prepared op and must be called once the transaction has ended.
func (r *EngineResource) prepareBatchOps(
	ctx context.Context,
	mutations []*s4wave_world.WorldMutation,
) (ops []*batchOp, refused *s4wave_world.ExecuteWorldBatchResponse, release func(), err error) {
	access := &WorldStateResource{lookupOp: r.lookupOp}
	applyWorldStateResourceOptions(access, r.worldStateOptions...)

	var releases []func()
	release = func() {
		for _, rel := range releases {
			rel()
		}
	}
	ops = make([]*batchOp, len(mutations))
	for i, m := range mutations {
		opReq := m.GetOp()
		if opReq == nil {
			continue
		}
		op, sender, opRefused, err := access.resolveWorldOp(ctx, opReq)
		if err != nil {
			return nil, nil, release, errors.Wrapf(err, "mutations[%d]", i)
		}
		if opRefused != nil {
			return ops, &s4wave_world.ExecuteWorldBatchResponse{
				FailedIndex: uint32(i),
				ErrorCode:   opRefused.GetErrorCode(),
			}, release, nil
		}
		rel, err := world.PrepareOp(ctx, op)
		if err != nil {
			return nil, nil, release, errors.Wrapf(err, "mutations[%d]", i)
		}
		releases = append(releases, rel)
		ops[i] = &batchOp{op: op, sender: sender}
	}
	return ops, nil, release, nil
}

// applyWorldMutation applies one mutation to ws, with op holding the prepared
// operation of an op entry. It returns the refusal for a mutation the World
// refused, or nil once applied.
func applyWorldMutation(
	ctx context.Context,
	ws world.WorldState,
	m *s4wave_world.WorldMutation,
	op *batchOp,
) (*s4wave_world.ExecuteWorldBatchResponse, error) {
	switch m := m.GetMutation().(type) {
	case *s4wave_world.WorldMutation_Op:
		// The batch discards its transaction on any refusal, so a plugin
		// timeout here kept nothing and is reported as a code.
		resp, err := applyResolvedWorldOp(ctx, ws, op.op, op.sender, true)
		if err != nil {
			return nil, err
		}
		if resp.GetErrorCode() == 0 && resp.GetRejectionCode() == "" {
			return nil, nil
		}
		return &s4wave_world.ExecuteWorldBatchResponse{
			ErrorCode:        resp.GetErrorCode(),
			RejectionCode:    resp.GetRejectionCode(),
			RejectionMessage: resp.GetRejectionMessage(),
		}, nil
	case *s4wave_world.WorldMutation_CreateObject:
		obj, err := ws.CreateObject(ctx, m.CreateObject.GetObjectKey(), m.CreateObject.GetRootRef())
		world.ReleaseObjectState(obj)
		return nil, err
	case *s4wave_world.WorldMutation_SetGraphQuad:
		q := m.SetGraphQuad.GetQuad()
		if q == nil {
			return nil, errors.New("no graph quad")
		}
		return nil, ws.SetGraphQuad(ctx, world.NewGraphQuad(q.GetSubject(), q.GetPredicate(), q.GetObj(), q.GetLabel()))
	case *s4wave_world.WorldMutation_SetObjectType:
		return nil, world_types.SetObjectType(ctx, ws, m.SetObjectType.GetObjectKey(), m.SetObjectType.GetTypeId())
	case *s4wave_world.WorldMutation_DeleteObject:
		deleted, err := ws.DeleteObject(ctx, m.DeleteObject.GetObjectKey())
		if err != nil || deleted {
			return nil, err
		}
		return &s4wave_world.ExecuteWorldBatchResponse{
			ErrorCode: s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_OBJECT_NOT_FOUND,
		}, nil
	default:
		return nil, errors.New("empty mutation")
	}
}
