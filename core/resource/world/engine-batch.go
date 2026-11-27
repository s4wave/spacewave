package resource_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/quad"
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

// ExecuteWorldBatch applies the mutations in order in one write transaction and
// commits it. The first refused mutation discards the transaction and reports
// its index. A stale base replays the whole list in a fresh transaction.
//
// Every op entry is resolved and prepared before the transaction takes the
// engine's writer, so a cold plugin load costs this caller time while other
// writers proceed, and ends with the caller's context error. An op the mount
// cannot resolve, or whose plugin load fails or passes its deadline, refuses
// the batch before any transaction opens, whatever entries precede it. Because
// a refused batch discards its transaction, a plugin that times out inside it
// is reported as WORLD_ERROR_CODE_PLUGIN_OP_TIMEOUT: nothing was written. So is
// a batch whose replays spent the write budget. That refusal identifies no
// entry, and its failed index is zero.
func (r *EngineResource) ExecuteWorldBatch(ctx context.Context, req *s4wave_world.ExecuteWorldBatchRequest) (*s4wave_world.ExecuteWorldBatchResponse, error) {
	// Resolve and prepare every op before the transaction takes the writer.
	mutations := req.GetMutations()
	access := &WorldStateResource{lookupOp: r.lookupOp}
	applyWorldStateResourceOptions(access, r.worldStateOptions...)
	ops, refused, release, err := prepareBatchOps(ctx, access, mutations)
	defer release()
	if err != nil || refused != nil {
		return refused, err
	}

	// Apply the mutations in order, stopping at the first refusal.
	var resp *s4wave_world.ExecuteWorldBatchResponse
	err = world.ExecTransaction(ctx, r.engine, true, func(ctx context.Context, wtx world.WorldState) error {
		// Record the failed index on a refusal and discard the
		// transaction.
		ws := world.GuardWorldState(wtx, access.writeGuard)
		results := make([]*s4wave_world.WorldMutationResult, len(mutations))
		for i, m := range mutations {
			rev, refusal, err := applyWorldMutation(ctx, ws, m, ops[i])
			if err != nil {
				refusal, err = batchOpRefusal(err)
			}
			if err != nil {
				return errors.Wrapf(err, "mutations[%d]", i)
			}
			if refusal != nil {
				refusal.FailedIndex = uint32(i)
				resp = refusal
				return errBatchRefused
			}
			results[i] = &s4wave_world.WorldMutationResult{Rev: rev}
		}

		// Report the sequence number the commit publishes.
		seqno, err := wtx.GetSeqno(ctx)
		if err != nil {
			return err
		}
		resp = &s4wave_world.ExecuteWorldBatchResponse{Seqno: seqno, Results: results}
		return nil
	})
	if err != nil && !errors.Is(err, errBatchRefused) {
		resp, err = batchOpRefusal(err)
		if err != nil {
			return nil, err
		}
	}

	// Return an unknown outcome to a client that cannot decode its code as
	// the bare durability unknown error.
	unknown := s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_DURABILITY_UNKNOWN
	if resp.GetErrorCode() == unknown && !s4wave_world.Decodes(req.GetKnownErrorCode(), unknown) {
		return nil, world.ErrDurabilityUnknown
	}
	return resp, nil
}

// prepareBatchOps resolves, decodes and prepares the World and object op
// entries of mutations in order, outside any transaction, with access holding
// the mount options ExecuteWorldOp resolves ops with. ops is indexed like
// mutations and holds nil for entries that are not ops. The first op the
// mount cannot resolve, or whose preparation is refused, such as by a
// rejection or an unavailable plugin, stops the walk and is returned as the
// refusal at its index, and the batch then opens no transaction. release
// frees every prepared op and must be called once the transaction has ended.
func prepareBatchOps(
	ctx context.Context,
	access *WorldStateResource,
	mutations []*s4wave_world.WorldMutation,
) (ops []*batchOp, refused *s4wave_world.ExecuteWorldBatchResponse, release func(), err error) {
	// Prepare each op in order, collecting the releases.
	var releases []func()
	release = func() {
		for _, rel := range releases {
			rel()
		}
	}
	ops = make([]*batchOp, len(mutations))
	for i, m := range mutations {
		opReq := m.GetOp()
		if objOp := m.GetApplyObjectOp(); objOp != nil {
			opReq = &s4wave_world.ApplyWorldOpRequest{
				OpTypeId: objOp.GetOpTypeId(), OpData: objOp.GetOpData(), OpSender: objOp.GetOpSender(),
			}
		}
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
			prepRefused, prepErr := batchOpRefusal(err)
			if prepErr != nil {
				return nil, nil, release, errors.Wrapf(prepErr, "mutations[%d]", i)
			}
			prepRefused.FailedIndex = uint32(i)
			return ops, prepRefused, release, nil
		}
		releases = append(releases, rel)
		ops[i] = &batchOp{op: op, sender: sender}
	}
	return ops, nil, release, nil
}

// applyWorldMutation applies one mutation to ws, with op holding the prepared
// operation of an op entry. It returns the object revision an object mutation
// leaves, and the refusal for an entry whose object is absent. The caller
// classifies a returned error with batchOpRefusal.
func applyWorldMutation(
	ctx context.Context,
	ws world.WorldState,
	m *s4wave_world.WorldMutation,
	op *batchOp,
) (uint64, *s4wave_world.ExecuteWorldBatchResponse, error) {
	switch m := m.GetMutation().(type) {
	case *s4wave_world.WorldMutation_Op:
		_, _, err := ws.ApplyWorldOp(ctx, op.op, op.sender)
		return 0, nil, err
	case *s4wave_world.WorldMutation_CreateObject:
		obj, err := ws.CreateObject(ctx, m.CreateObject.GetObjectKey(), m.CreateObject.GetRootRef())
		world.ReleaseObjectState(obj)
		return 0, nil, err
	case *s4wave_world.WorldMutation_RenameObject:
		req := m.RenameObject
		obj, err := ws.RenameObject(ctx, req.GetOldObjectKey(), req.GetNewObjectKey(), req.GetDescendants())
		world.ReleaseObjectState(obj)
		if errors.Is(err, world.ErrObjectNotFound) {
			return 0, objectNotFound(), nil
		}
		return 0, nil, err
	case *s4wave_world.WorldMutation_SetGraphQuad:
		q, err := batchGraphQuad(m.SetGraphQuad.GetQuad())
		if err != nil {
			return 0, nil, err
		}
		return 0, nil, ws.SetGraphQuad(ctx, q)
	case *s4wave_world.WorldMutation_DeleteGraphQuad:
		q, err := batchGraphQuad(m.DeleteGraphQuad.GetQuad())
		if err != nil {
			return 0, nil, err
		}
		return 0, nil, ws.DeleteGraphQuad(ctx, q)
	case *s4wave_world.WorldMutation_DeleteGraphObject:
		return 0, nil, ws.DeleteGraphObject(ctx, m.DeleteGraphObject.GetObjectKey())
	case *s4wave_world.WorldMutation_SetObjectType:
		return 0, nil, world_types.SetObjectType(ctx, ws, m.SetObjectType.GetObjectKey(), m.SetObjectType.GetTypeId())
	case *s4wave_world.WorldMutation_DeleteObject:
		deleted, err := ws.DeleteObject(ctx, m.DeleteObject.GetObjectKey())
		if err != nil || deleted {
			return 0, nil, err
		}
		return 0, objectNotFound(), nil
	case *s4wave_world.WorldMutation_SetRootRef:
		return applyObjectMutation(ctx, ws, m.SetRootRef.GetObjectKey(), func(obj world.ObjectState) (uint64, error) {
			return obj.SetRootRef(ctx, m.SetRootRef.GetRootRef())
		})
	case *s4wave_world.WorldMutation_ApplyObjectOp:
		return applyObjectMutation(ctx, ws, m.ApplyObjectOp.GetObjectKey(), func(obj world.ObjectState) (uint64, error) {
			rev, _, err := obj.ApplyObjectOp(ctx, op.op, op.sender)
			return rev, err
		})
	case *s4wave_world.WorldMutation_IncrementRev:
		return applyObjectMutation(ctx, ws, m.IncrementRev.GetObjectKey(), func(obj world.ObjectState) (uint64, error) {
			return obj.IncrementRev(ctx)
		})
	default:
		return 0, nil, errors.New("empty mutation")
	}
}

// applyObjectMutation calls apply with the object at key and returns the
// revision it leaves. An absent object refuses the batch.
func applyObjectMutation(
	ctx context.Context,
	ws world.WorldState,
	key string,
	apply func(obj world.ObjectState) (uint64, error),
) (uint64, *s4wave_world.ExecuteWorldBatchResponse, error) {
	// Acquire the object, refusing the batch when it is absent.
	obj, found, err := ws.GetObject(ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return 0, nil, err
	}
	if !found {
		return 0, objectNotFound(), nil
	}
	rev, err := apply(obj)
	return rev, nil, err
}

// batchOpRefusal returns the refusal an entry's error carries, such as a
// rejection by the op or the mount's write guard, or the error when it carries
// none. The batch discards its transaction on any refusal, so a plugin timeout
// is a refusal: nothing was kept.
func batchOpRefusal(err error) (*s4wave_world.ExecuteWorldBatchResponse, error) {
	// Classify the error, passing on one that carries no refusal.
	if err == nil {
		return nil, nil
	}
	code, rejection, ok := opErrorRefusal(err, true)
	if !ok {
		return nil, err
	}
	refusal := &s4wave_world.ExecuteWorldBatchResponse{ErrorCode: code}
	if rejection != nil {
		refusal.RejectionCode, refusal.RejectionMessage = rejection.Code, rejection.Message
	}
	return refusal, nil
}

// batchGraphQuad converts the quad of a graph quad entry.
func batchGraphQuad(q *quad.Quad) (world.GraphQuad, error) {
	if q == nil {
		return nil, errors.New("no graph quad")
	}
	return world.NewGraphQuad(q.GetSubject(), q.GetPredicate(), q.GetObj(), q.GetLabel()), nil
}

// objectNotFound is the refusal of an entry whose object is absent.
func objectNotFound() *s4wave_world.ExecuteWorldBatchResponse {
	return &s4wave_world.ExecuteWorldBatchResponse{
		ErrorCode: s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_OBJECT_NOT_FOUND,
	}
}
