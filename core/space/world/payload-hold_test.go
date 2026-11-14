package space_world_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
)

// TestPayloadHoldCoversUnappliedOps gives a Space more than a cursor save's
// worth of another device's operations whose payloads only a stage holds, as
// a migration's import does at a target with no peer to fetch them from.
// Replay stops at an operation whose plugin has not registered, after a
// cursor save, with the operations after it unapplied. The stage releases,
// and a collection that frees runs: every payload must still read, and the
// replay must finish once the plugin registers.
func TestPayloadHoldCoversUnappliedOps(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	f := newAdapterFixtureOn(t, fixtureOpts{frees: true, opTypes: &space_world.OpTypes{
		Declared: func(_ context.Context, _ world.WorldState, opTypeID string) ([]string, error) {
			if opTypeID == world_mock.MockWorldOpTypeID {
				return []string{testOpPlugin}, nil
			}
			return nil, nil
		},
	}})
	eng, engRef := f.start(t)
	defer engRef.Release()
	so := f.space(t)
	store := so.GetBlockStore()
	dev := addDevice(t, so)

	// Store the payloads under a stage root, as an import copies them.
	const n = 70
	var payloads []*block.BlockRef
	var entries []*block.PutBatchEntry
	for i := range 2 * n {
		data := []byte("payload " + strconv.Itoa(i))
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		payloads = append(payloads, ref)
		entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: data, RefsKnown: true})
	}
	stageData := []byte("stage")
	for _, ref := range payloads {
		stageData = append(stageData, ref.MarshalString()...)
	}
	stageRef, err := block.BuildBlockRef(stageData, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries = append(entries, &block.PutBatchEntry{Ref: stageRef, Data: stageData, Refs: payloads, RefsKnown: true})
	if err := store.SetRoots(ctx, entries, []block.NamedRoot{{Name: "test-stage", Ref: stageRef}}); err != nil {
		t.Fatal(err)
	}
	cursor, err := store.GetRoot(ctx, space_world.RootReplayCursor)
	if err != nil {
		t.Fatal(err)
	}

	// Add the device's operations in one state change: n creates naming a
	// payload each, an operation only the plugin resolves, and n more.
	keys := []string{"plugin-object"}
	ops := [][]byte{createOp(t, "plugin-object")}
	for i := range 2 * n {
		if i == n {
			ops = append(ops, txOp(t, &world_block_tx.Tx{
				TxType: world_block_tx.TxType_TxType_APPLY_WORLD_OP,
				TxApplyWorldOp: &world_block_tx.TxApplyWorldOp{
					OperationTypeId: world_mock.MockWorldOpTypeID,
					OperationBody:   marshalOp(t, world_mock.NewMockWorldOp("plugin-object", "hello")),
				},
			}))
		}
		key := "payload-" + strconv.Itoa(i)
		keys = append(keys, key)
		ops = append(ops, txOp(t, &world_block_tx.Tx{
			TxType:         world_block_tx.TxType_TxType_CREATE_OBJECT,
			TxCreateObject: &world_block_tx.TxCreateObject{ObjectKey: key, RootRef: &bucket.ObjectRef{RootRef: payloads[i]}},
		}))
	}
	loads := make(chan []string, 1)
	start := make(chan struct{})
	releaseLoader, err := f.tb.Bus.AddController(ctx, &gatedOpPluginLoader{b: f.tb.Bus, engineID: f.engineID, loads: loads, start: start}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseLoader()
	if err := so.(sobject.InviteHost).GetSOHost().UpdateSOState(ctx, func(state *sobject.SOState) error {
		for _, op := range ops {
			if err := dev.add(t, so, state, op, false); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Wait for replay to stop at the plugin's operation, after a cursor save.
	select {
	case <-loads:
	case <-ctx.Done():
		t.Fatal("replay never stopped at the plugin's operation")
	}
	for {
		saved, err := store.GetRoot(ctx, space_world.RootReplayCursor)
		if err != nil {
			t.Fatal(err)
		}
		if !saved.EqualsRef(cursor) {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("replay stopped without saving the cursor")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Release the stage and collect: every payload still reads.
	if err := store.SetRoots(ctx, nil, []block.NamedRoot{{Name: "test-stage", Previous: stageRef}}); err != nil {
		t.Fatal(err)
	}
	f.collect(t)
	requirePayloads := func(step string) {
		t.Helper()
		for i, ref := range payloads {
			if _, found, err := store.GetBlock(ctx, ref); err != nil || !found {
				t.Fatalf("%s: payload %d freed: %v, %v", step, i, found, err)
			}
		}
	}
	requirePayloads("after the stage released")

	// Register the plugin: replay applies every operation.
	close(start)
	waitObjects(ctx, t, eng, keys...)
	f.collect(t)
	requirePayloads("after replay")
}

// txOp encodes the World operation that applies tx.
func txOp(t *testing.T, tx *world_block_tx.Tx) []byte {
	t.Helper()
	op, err := (&space_world.WorldOp{Body: &space_world.WorldOp_ApplyTxOp{ApplyTxOp: &space_world.ApplyTxOp{Tx: tx}}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return op
}

// marshalOp encodes a World operation's body.
func marshalOp(t *testing.T, op *world_mock.MockWorldOp) []byte {
	t.Helper()
	data, err := op.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// gatedOpPluginLoader stands in for a Space resource's operation plugin
// loader, as testOpPluginLoader does, but starts the plugin only once start
// closes.
type gatedOpPluginLoader struct {
	// b is the bus the plugin registers on.
	b bus.Bus
	// engineID is the Space World engine.
	engineID string
	// loads receives the plugins of the first request.
	loads chan []string
	// start closes to start the plugin.
	start chan struct{}
}

// HandleDirective reports the engine's first request, and starts the test
// plugin once start closes.
func (l *gatedOpPluginLoader) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(space_world.LoadOpPlugins)
	if !ok || dir.LoadOpPluginsEngineID() != l.engineID {
		return nil, nil
	}
	select {
	case l.loads <- dir.LoadOpPluginsPluginIDs():
		go func() {
			<-l.start
			_, _ = l.b.AddController(context.WithoutCancel(ctx), world.NewLookupOpController(testOpPlugin, "", world_mock.LookupMockOp), nil)
		}()
	default:
	}
	return nil, nil
}

// GetControllerInfo returns information about the controller.
func (l *gatedOpPluginLoader) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/gated-op-plugin-loader", controller.MustParseVersion("0.0.1"), "gated test operation plugin loader")
}

// Execute returns immediately.
func (l *gatedOpPluginLoader) Execute(context.Context) error {
	return nil
}

// Close releases nothing.
func (l *gatedOpPluginLoader) Close() error {
	return nil
}
