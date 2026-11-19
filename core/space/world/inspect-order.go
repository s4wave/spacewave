package space_world

import (
	"bytes"
	"context"
	"maps"
	"slices"

	"github.com/aperturerobotics/protobuf-go-lite/encoding/protowire"
	"github.com/s4wave/spacewave/core/sobject"
)

// InspectOrder walks the agreed order of so's current state from its
// checkpoint to the head and calls emit with what this build reads of each
// operation, in order. It returns the counts of the whole order.
//
// It reads the state and the saved replay cursor and writes nothing: it runs
// no operation and mounts no World. It keeps only the counts; each operation
// is passed to emit and dropped. The order's hashes are the state's own,
// which the SharedObject already holds in memory.
func InspectOrder(ctx context.Context, so sobject.SharedObject, emit func(*OrderOperation) error) (*OrderInspection, error) {
	// Read the agreed order and the saved replay cursor.
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		return nil, err
	}
	ev, err := sobject.ReadOrder(ctx, snap)
	if err != nil {
		return nil, err
	}
	cursor, err := readSavedCursor(ctx, so)
	if err != nil {
		return nil, err
	}
	out := &OrderInspection{
		CheckpointHeight: ev.Checkpoint.GetHeight(),
		Operations:       uint32(len(ev.Order)), //nolint:gosec // an order in memory.
		Cursor:           summarizeCursor(cursor),
	}

	// A saved outcome describes a position only when the cursor replayed
	// from the same checkpoint.
	var saved []*ReplayCursorOutcome
	if cursor != nil && cursor.GetHeight() == out.CheckpointHeight {
		saved = cursor.GetOutcomes()
	}

	// Read each operation as replay prepares it.
	kinds := make(map[WorldOpKind]uint32)
	for i, h := range ev.Order {
		op, err := ev.Operation(ctx, i)
		if err != nil {
			return nil, err
		}
		entry := readOrderOperation(op)
		entry.Position = uint32(i) //nolint:gosec // an index into the order in memory.
		entry.PeerId, entry.Nonce = op.Inner.GetPeerId(), op.Inner.GetNonce()
		if i < len(saved) && bytes.Equal(saved[i].GetHash(), h) {
			entry.SavedReason = saved[i].GetReason()
		}

		// Count it.
		switch read := entry.GetRead().(type) {
		case *OrderOperation_Body:
			kinds[read.Body.GetKind()]++
		case *OrderOperation_Unreadable:
			out.Unreadable++
		case *OrderOperation_Empty:
			out.Empty++
		case *OrderOperation_Invalid:
			out.Invalid++
		case *OrderOperation_Refused:
			out.Refused++
		case *OrderOperation_Acknowledgment:
			out.Acknowledgments++
		}
		if err := emit(entry); err != nil {
			return nil, err
		}
	}

	// List the kinds in kind order.
	for _, kind := range slices.Sorted(maps.Keys(kinds)) {
		out.Kinds = append(out.Kinds, &OrderKindCount{Kind: kind, Count: kinds[kind]})
	}
	return out, nil
}

// Undecodable reports whether this build cannot decode op's body: it is
// unreadable, empty or invalid data. A refused operation or an acknowledgment
// has no body to decode.
func (op *OrderOperation) Undecodable() bool {
	switch op.GetRead().(type) {
	case *OrderOperation_Unreadable, *OrderOperation_Empty, *OrderOperation_Invalid:
		return true
	default:
		return false
	}
}

// SavedUnknownType reports whether the saved replay cursor refused op as an
// unknown operation type.
func (op *OrderOperation) SavedUnknownType() bool {
	return op.GetSavedReason() == reasonUnknownOperationType
}

// readOrderOperation returns what this build reads of a prepared operation,
// with its position fields unset.
func readOrderOperation(op *sobject.OrderedOperation) *OrderOperation {
	// An operation refused before its body, or one with no data, has no body.
	switch {
	case op.Refused != "":
		return &OrderOperation{Read: &OrderOperation_Refused{Refused: &OrderOperationRefused{Reason: op.Refused}}}
	case op.Data == nil:
		return &OrderOperation{Read: &OrderOperation_Acknowledgment{Acknowledgment: &OrderOperationAcknowledgment{}}}
	}

	// Decode the WorldOp as replay does, deciding it by the fields this build
	// does not declare before its body.
	wop := &WorldOp{}
	if err := wop.UnmarshalVT(op.Data); err != nil {
		return &OrderOperation{Read: &OrderOperation_Invalid{Invalid: &OrderOperationInvalid{Error: err.Error()}}}
	}
	if reason, err := readUnknownFields(wop); err != nil {
		return &OrderOperation{Read: &OrderOperation_Unreadable{Unreadable: &OrderOperationUnreadable{Fields: fieldNumbers(wop.unknownFields)}}}
	} else if reason != "" {
		return &OrderOperation{Read: &OrderOperation_Refused{Refused: &OrderOperationRefused{Reason: reason}}}
	}
	if kind := worldOpKind(wop); kind != WorldOpKind_WORLD_OP_KIND_UNKNOWN {
		return &OrderOperation{Read: &OrderOperation_Body{Body: &OrderOperationBody{Kind: kind}}}
	}
	return &OrderOperation{Read: &OrderOperation_Empty{Empty: &OrderOperationEmpty{}}}
}

// worldOpKind names the body of wop, or returns WORLD_OP_KIND_UNKNOWN when it
// has none this build declares.
func worldOpKind(wop *WorldOp) WorldOpKind {
	switch wop.GetBody().(type) {
	case *WorldOp_InitWorld:
		return WorldOpKind_WORLD_OP_KIND_INIT_WORLD
	case *WorldOp_ApplyTxOp:
		return WorldOpKind_WORLD_OP_KIND_APPLY_TX
	case *WorldOp_SetRetainedRoot:
		return WorldOpKind_WORLD_OP_KIND_SET_RETAINED_ROOT
	case *WorldOp_SetTransformConf:
		return WorldOpKind_WORLD_OP_KIND_SET_TRANSFORM_CONF
	case *WorldOp_GraphSettingsStep:
		return WorldOpKind_WORLD_OP_KIND_GRAPH_SETTINGS_STEP
	case *WorldOp_Fence:
		return WorldOpKind_WORLD_OP_KIND_FENCE
	case *WorldOp_RootSet:
		return WorldOpKind_WORLD_OP_KIND_ROOT_SET
	case *WorldOp_RegenerateWorld:
		return WorldOpKind_WORLD_OP_KIND_REGENERATE_WORLD
	case *WorldOp_ReleaseWorldRegen:
		return WorldOpKind_WORLD_OP_KIND_RELEASE_WORLD_REGEN
	default:
		return WorldOpKind_WORLD_OP_KIND_UNKNOWN
	}
}

// fieldNumbers returns the distinct field numbers of the encoded fields in
// data, in the order they first appear, stopping at the first malformed one.
func fieldNumbers(data []byte) []uint32 {
	var out []uint32
	seen := make(map[uint32]struct{})
	for len(data) != 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			break
		}
		m := protowire.ConsumeFieldValue(num, typ, data[n:])
		if m < 0 {
			break
		}
		field := uint32(num) //nolint:gosec // a field number fits in 29 bits.
		if _, ok := seen[field]; !ok {
			seen[field] = struct{}{}
			out = append(out, field)
		}
		data = data[n+m:]
	}
	return out
}

// summarizeCursor counts the saved replay cursor's outcomes, or returns nil
// when none is saved.
func summarizeCursor(cursor *ReplayCursor) *OrderCursor {
	if cursor == nil {
		return nil
	}
	out := &OrderCursor{
		Height:   cursor.GetHeight(),
		Outcomes: uint32(len(cursor.GetOutcomes())), //nolint:gosec // a cursor in memory.
	}
	for _, outcome := range cursor.GetOutcomes() {
		if outcome.GetReason() == reasonUnknownOperationType {
			out.UnknownOperationType++
		}
	}
	return out
}
