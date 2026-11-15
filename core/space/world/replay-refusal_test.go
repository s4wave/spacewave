package space_world

import (
	"context"
	"errors"
	"testing"

	pkg_errors "github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
)

// TestReplayRefuses checks the refusal classifier: each member of the
// refusal set refuses, through a wrap, and each wait class, an untyped
// error and a recovered panic wait, even one wrapping a refusal. A
// malformed transaction refuses through its Validate, and a transaction
// type this build does not know waits, alone or in a batch.
func TestReplayRefuses(t *testing.T) {
	refusals := []error{
		&world.OperationRejection{Code: "refused", Message: "refused"},
		world.NewOpRefusedError(errors.New("refused")),
		world.NewOpRefusedError(&block.DecodeError{Err: errors.New("bad wire type")}),
	}
	badRoot := &bucket.ObjectRef{TransformConf: &block_transform.Config{
		Steps: []*block_transform.StepConfig{{}},
	}}
	refusals = append(refusals,
		(&world_block_tx.TxCreateObject{ObjectKey: "a", RootRef: badRoot}).Validate(),
		(&world_block_tx.TxObjectSet{ObjectKey: "a", RootRef: badRoot}).Validate(),
		(&world_block_tx.TxDeleteObject{}).Validate(),
		(&world_block_tx.TxApplyWorldOp{OperationTypeId: "test/op", OpSender: "bad"}).Validate(),
		(&world_block_tx.TxApplyObjectOp{OperationTypeId: "test/op", ObjectKey: "a", OpSender: "bad"}).Validate(),
	)
	refusals = append(refusals, replayRefusals...)
	for _, err := range refusals {
		if !replayRefuses(err) {
			t.Errorf("replayRefuses(%v) = false, want a refusal", err)
		}
		if wrapped := pkg_errors.Wrap(err, "apply"); !replayRefuses(wrapped) {
			t.Errorf("replayRefuses(wrapped %v) = false, want a refusal", err)
		}
	}

	unknownTx := &world_block_tx.Tx{TxType: world_block_tx.TxType(99)}
	_, locateErr := unknownTx.LocateTx()
	batch := &world_block_tx.TxBatch{Txs: []*world_block_tx.Tx{unknownTx}}
	unknown := []error{locateErr, unknownTx.Validate(), batch.Validate()}
	for _, err := range unknown {
		if !errors.Is(err, world.ErrUnhandledOp) {
			t.Errorf("unknown transaction type failed with %v, want ErrUnhandledOp", err)
		}
	}
	waits := append(unknown,
		nil,
		errors.New("untyped"),
		block.ErrNotFound,
		block.ErrStageReleased,
		&unresolvedOpError{opTypeID: "test/op"},
		&world.PluginUnavailableError{PluginID: "p", OperationTypeID: "test/op", Err: errors.New("down")},
		&world.PluginExecutableMissingError{},
		context.Canceled,
		context.DeadlineExceeded,
		tx.ErrDiscarded,
		world.ErrSuperseded,
		world.ErrEngineRetired,
		world.ErrDurabilityUnknown,
		world.ErrHistoryUnavailable,
		world.ErrWorldStorageUnavailable,
		world.ErrStateNotFound,
		tx.ErrNotWrite,
		world.ErrUnhandledOp,
		world.ErrEngineUnguardedOp,
		world.NewOpSystemError(nil),
		world.NewOpSystemError(world.ErrObjectNotFound),
		&world.OpPanicError{Value: "boom"},
		&world.OpPanicError{Value: world.ErrObjectExists},
		pkg_errors.Wrap(context.Canceled, "apply"),
		world.NewOpRefusedError(context.Canceled),
		&world.PluginUnavailableError{PluginID: "p", Err: world.ErrObjectExists},
		&block.CorruptError{},
		block.ErrBlockCorrupt,
		world.NewOpRefusedError(block.ErrBlockCorrupt),
		&block.DecodeError{Err: errors.New("bad wire type")},
		world.NewOpRefusedError(&block.CorruptError{}),
	)
	for _, err := range waits {
		if replayRefuses(err) {
			t.Errorf("replayRefuses(%v) = true, want a wait", err)
		}
		if err != nil && replayRefuses(pkg_errors.Wrap(err, "apply")) {
			t.Errorf("replayRefuses(wrapped %v) = true, want a wait", err)
		}
	}
}
