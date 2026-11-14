package resource_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// errBatchRefused ends a batch attempt so its transaction is discarded.
var errBatchRefused = errors.New("world batch refused")

// ExecuteWorldBatch applies the mutations in order in one write transaction
// and commits it. The first refused mutation discards the transaction and
// names its index; a stale base replays the whole list in a fresh transaction.
func (r *EngineResource) ExecuteWorldBatch(ctx context.Context, req *s4wave_world.ExecuteWorldBatchRequest) (*s4wave_world.ExecuteWorldBatchResponse, error) {
	var resp *s4wave_world.ExecuteWorldBatchResponse
	err := world.ExecTransaction(ctx, r.engine, true, func(ctx context.Context, wtx world.WorldState) error {
		// Ops go through the World state Resource so they bind the sender and
		// report rejections as ExecuteWorldOp does.
		state := NewWorldStateResource(r.le, r.b, wtx, r.lookupOp, r.worldStateOptions...)
		for i, m := range req.GetMutations() {
			refused, err := applyWorldMutation(ctx, state, m)
			if err != nil {
				return errors.Wrapf(err, "mutations[%d]", i)
			}
			if refused != nil {
				refused.FailedIndex = uint32(i)
				resp = refused
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

// applyWorldMutation applies one mutation to the World state behind state. It
// returns the refusal for a mutation the World refused, or nil once applied.
func applyWorldMutation(
	ctx context.Context,
	state *WorldStateResource,
	m *s4wave_world.WorldMutation,
) (*s4wave_world.ExecuteWorldBatchResponse, error) {
	ws := state.ws
	switch m := m.GetMutation().(type) {
	case *s4wave_world.WorldMutation_Op:
		resp, err := state.ApplyWorldOp(ctx, m.Op)
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
