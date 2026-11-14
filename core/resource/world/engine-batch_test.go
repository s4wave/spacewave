//go:build !js

package resource_world_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// TestExecuteWorldBatch checks that a batch commits every entry together, and
// that a refused entry leaves nothing written by the entries before it.
func TestExecuteWorldBatch(t *testing.T) {
	ctx := t.Context()
	tb, cleanup := setupWorldTestbed(ctx, t)
	defer cleanup()
	eng := &writeCountingEngine{Engine: tb.Engine}
	engineResource := resource_world.NewEngineResource(logrus.NewEntry(logrus.New()), tb.Bus, eng, nil, nil)
	defer engineResource.Close()
	ws := world.NewEngineWorldState(tb.Engine, true)

	create := func(key string) *s4wave_world.WorldMutation {
		return &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_CreateObject{
			CreateObject: &s4wave_world.CreateObjectRequest{ObjectKey: key, RootRef: &bucket.ObjectRef{}},
		}}
	}
	remove := func(key string) *s4wave_world.WorldMutation {
		return &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_DeleteObject{
			DeleteObject: &s4wave_world.DeleteObjectRequest{ObjectKey: key},
		}}
	}
	setQuad := func(subject string) *s4wave_world.WorldMutation {
		return &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_SetGraphQuad{
			SetGraphQuad: &s4wave_world.SetGraphQuadRequest{Quad: &quad.Quad{
				Subject: world.KeyToGraphValue(subject).String(), Predicate: "<type>", Obj: world.KeyToGraphValue(subject).String(),
			}},
		}}
	}
	exists := func(key string) bool {
		obj, found, err := ws.GetObject(ctx, key)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err.Error())
		}
		return found
	}

	// A batch refused at its last entry writes none of the earlier ones.
	resp, err := engineResource.ExecuteWorldBatch(ctx, &s4wave_world.ExecuteWorldBatchRequest{
		Mutations: []*s4wave_world.WorldMutation{create("refused"), setQuad("refused"), remove("missing")},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetFailedIndex() != 2 || resp.GetErrorCode() != s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_OBJECT_NOT_FOUND || resp.GetSeqno() != 0 {
		t.Fatalf("refused batch: %v", resp)
	}
	if exists("refused") {
		t.Fatal("refused batch created its object")
	}

	// An op the mount cannot resolve refuses the batch as unhandled before
	// any write transaction opens.
	opened := eng.writes.Load()
	if opened == 0 {
		t.Fatal("the refused batch opened no counted write transaction")
	}
	resp, err = engineResource.ExecuteWorldBatch(ctx, &s4wave_world.ExecuteWorldBatchRequest{
		Mutations: []*s4wave_world.WorldMutation{create("unhandled"), {Mutation: &s4wave_world.WorldMutation_Op{
			Op: &s4wave_world.ApplyWorldOpRequest{OpTypeId: "test/unknown"},
		}}},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetFailedIndex() != 1 || resp.GetErrorCode() != s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_UNHANDLED_OP {
		t.Fatalf("unhandled batch: %v", resp)
	}
	if exists("unhandled") {
		t.Fatal("unhandled batch created its object")
	}
	if n := eng.writes.Load() - opened; n != 0 {
		t.Fatalf("unhandled batch opened %d write transactions", n)
	}

	// An applied batch commits every entry, including a delete of an object an
	// earlier entry created.
	resp, err = engineResource.ExecuteWorldBatch(ctx, &s4wave_world.ExecuteWorldBatchRequest{
		Mutations: []*s4wave_world.WorldMutation{create("kept"), create("removed"), setQuad("kept"), remove("removed")},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetSeqno() == 0 || resp.GetErrorCode() != 0 {
		t.Fatalf("applied batch: %v", resp)
	}
	if !exists("kept") || exists("removed") {
		t.Fatal("applied batch did not commit its entries")
	}
	seqno, err := ws.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if seqno != resp.GetSeqno() {
		t.Fatalf("applied batch reported seqno %d, World is at %d", resp.GetSeqno(), seqno)
	}

	// Typing a new object creates the type's object, which a bare type quad
	// would need to exist already.
	resp, err = engineResource.ExecuteWorldBatch(ctx, &s4wave_world.ExecuteWorldBatchRequest{
		Mutations: []*s4wave_world.WorldMutation{create("typed"), {Mutation: &s4wave_world.WorldMutation_SetObjectType{
			SetObjectType: &s4wave_world.SetObjectTypeMutation{ObjectKey: "typed", TypeId: "test/batch-type"},
		}}},
	})
	if err != nil || resp.GetErrorCode() != 0 {
		t.Fatalf("typed batch: %v %v", resp, err)
	}
	typeID, err := world_types.GetObjectType(ctx, ws, "typed")
	if err != nil {
		t.Fatal(err.Error())
	}
	if typeID != "test/batch-type" || !exists(world_types.BuildTypeObjectKey("test/batch-type")) {
		t.Fatalf("typed batch set type %q", typeID)
	}
}

// writeCountingEngine counts the write transactions opened on its Engine.
type writeCountingEngine struct {
	world.Engine
	// writes counts NewTransaction calls with write set.
	writes atomic.Int32
}

// NewTransaction counts a write transaction and opens it on the Engine.
func (e *writeCountingEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	if write {
		e.writes.Add(1)
	}
	return e.Engine.NewTransaction(ctx, write)
}

// batchTestOpID is the type of batchTestOp.
const batchTestOpID = "test/batch-op"

// batchTestHooks observes and steers the batchTestOps one mount resolves.
type batchTestHooks struct {
	// mtx guards senders.
	mtx sync.Mutex
	// senders records the sender of each applied op, in order.
	senders []peer.ID
	// gate, when set, holds PrepareApply until it is closed, as a cold plugin
	// load would.
	gate chan struct{}
	// preparing receives once per PrepareApply that waits on gate.
	preparing chan struct{}
}

// batchTestOp is a World op whose payload names what it does: "reject"
// rejects, "timeout" fails as a plugin timeout, and "create:<key>" creates
// the object key.
type batchTestOp struct {
	// action is the decoded payload.
	action string
	// h receives the op's observations.
	h *batchTestHooks
}

// MarshalVT encodes the action.
func (o *batchTestOp) MarshalVT() ([]byte, error) { return []byte(o.action), nil }

// UnmarshalVT decodes the action.
func (o *batchTestOp) UnmarshalVT(data []byte) error {
	o.action = string(data)
	return nil
}

// TypeID returns batchTestOpID.
func (o *batchTestOp) TypeID() string { return batchTestOpID }

// PrepareApply waits on the hooks' gate when one is set.
func (o *batchTestOp) PrepareApply(ctx context.Context) (func(), error) {
	if o.h.gate == nil {
		return func() {}, nil
	}
	o.h.preparing <- struct{}{}
	select {
	case <-o.h.gate:
		return func() {}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ApplyWorldOp records the sender and performs the action.
func (o *batchTestOp) ApplyWorldOp(ctx context.Context, le *logrus.Entry, ws world.WorldState, sender peer.ID) (bool, error) {
	o.h.mtx.Lock()
	o.h.senders = append(o.h.senders, sender)
	o.h.mtx.Unlock()
	switch {
	case o.action == "reject":
		return false, &world.OperationRejection{Code: "test/refused", Message: "refused by the test op"}
	case o.action == "timeout":
		return false, &world.PluginOpTimeoutError{PluginID: "test", OperationTypeID: batchTestOpID, Deadline: time.Second}
	case strings.HasPrefix(o.action, "create:"):
		obj, err := ws.CreateObject(ctx, strings.TrimPrefix(o.action, "create:"), &bucket.ObjectRef{})
		world.ReleaseObjectState(obj)
		return false, err
	}
	return false, world.ErrUnhandledOp
}

// _ is a type assertion.
var _ world.PreparedOperation = (*batchTestOp)(nil)

// TestExecuteWorldBatchOps checks op entries: a rejection or a plugin timeout
// discards the batch and names its index and code, an op is applied as the
// sender ExecuteWorldOp binds, and a plugin load before the transaction holds
// no writer and ends with the caller's context error.
func TestExecuteWorldBatchOps(t *testing.T) {
	ctx := t.Context()
	tb, cleanup := setupWorldTestbed(ctx, t)
	defer cleanup()
	hooks := &batchTestHooks{}
	lookupOp := func(ctx context.Context, typeID string) (world.Operation, error) {
		if typeID != batchTestOpID {
			return nil, world.ErrUnhandledOp
		}
		return &batchTestOp{h: hooks}, nil
	}
	bound := peer.ID("bound-mount-peer")
	engineResource := resource_world.NewEngineResource(
		logrus.NewEntry(logrus.New()), tb.Bus, tb.Engine, lookupOp, nil,
		resource_world.WithSessionPeerID(bound),
	)
	defer engineResource.Close()
	ws := world.NewEngineWorldState(tb.Engine, true)

	opReq := func(action string) *s4wave_world.ApplyWorldOpRequest {
		return &s4wave_world.ApplyWorldOpRequest{OpTypeId: batchTestOpID, OpData: []byte(action), OpSender: "spoofed"}
	}
	op := func(action string) *s4wave_world.WorldMutation {
		return &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_Op{Op: opReq(action)}}
	}
	create := func(key string) *s4wave_world.WorldMutation {
		return &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_CreateObject{
			CreateObject: &s4wave_world.CreateObjectRequest{ObjectKey: key, RootRef: &bucket.ObjectRef{}},
		}}
	}
	exists := func(key string) bool {
		obj, found, err := ws.GetObject(ctx, key)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err.Error())
		}
		return found
	}
	batch := func(ctx context.Context, mutations ...*s4wave_world.WorldMutation) (*s4wave_world.ExecuteWorldBatchResponse, error) {
		return engineResource.ExecuteWorldBatch(ctx, &s4wave_world.ExecuteWorldBatchRequest{Mutations: mutations})
	}

	// A rejected op refuses the batch with its rejection and writes nothing.
	resp, err := batch(ctx, create("rejected"), op("reject"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetFailedIndex() != 1 || resp.GetRejectionCode() != "test/refused" ||
		resp.GetRejectionMessage() != "refused by the test op" || resp.GetErrorCode() != 0 || resp.GetSeqno() != 0 {
		t.Fatalf("rejected batch: %v", resp)
	}
	if exists("rejected") {
		t.Fatal("rejected batch created its object")
	}

	// A plugin timeout refuses the batch with code 4 and keeps nothing, not
	// even what an earlier op wrote.
	resp, err = batch(ctx, create("timed-out"), op("create:timed-out-op"), op("timeout"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetFailedIndex() != 2 || resp.GetErrorCode() != s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_PLUGIN_OP_TIMEOUT {
		t.Fatalf("timed out batch: %v", resp)
	}
	if exists("timed-out") || exists("timed-out-op") {
		t.Fatal("timed out batch kept a write")
	}

	// A batch op is applied as the sender ExecuteWorldOp binds: the mount's.
	hooks.senders = nil
	if _, err := engineResource.ExecuteWorldOp(ctx, opReq("create:single")); err != nil {
		t.Fatal(err.Error())
	}
	if resp, err := batch(ctx, op("create:batched")); err != nil || resp.GetErrorCode() != 0 {
		t.Fatalf("sender batch: %v %v", resp, err)
	}
	if len(hooks.senders) != 2 || hooks.senders[0] != bound || hooks.senders[1] != bound {
		t.Fatalf("senders: %v, want both %v", hooks.senders, bound)
	}

	// A cold plugin load runs before the batch takes the writer: another
	// writer commits while it waits.
	hooks.gate, hooks.preparing = make(chan struct{}), make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		resp, err := batch(ctx, create("cold"), op("create:cold-op"))
		if err == nil && resp.GetSeqno() == 0 {
			err = errors.New("cold batch did not commit")
		}
		done <- err
	}()
	<-hooks.preparing
	writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
	obj, err := ws.CreateObject(writeCtx, "concurrent", &bucket.ObjectRef{})
	writeCancel()
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatalf("writer waited behind a cold plugin load: %v", err)
	}
	close(hooks.gate)
	if err := <-done; err != nil {
		t.Fatal(err.Error())
	}
	if !exists("cold") || !exists("cold-op") || !exists("concurrent") {
		t.Fatal("cold batch or concurrent writer lost a write")
	}

	// A cold load the caller gives up on ends with the caller's context error,
	// not the timeout code, and writes nothing.
	hooks.gate, hooks.preparing = make(chan struct{}), make(chan struct{}, 1)
	callCtx, callCancel := context.WithCancel(ctx)
	go func() {
		_, err := batch(callCtx, create("abandoned"), op("create:abandoned-op"))
		if err == nil {
			err = errors.New("abandoned batch answered instead of failing")
		} else if !errors.Is(err, context.Canceled) {
			err = errors.New("abandoned batch: " + err.Error())
		} else {
			err = nil
		}
		done <- err
	}()
	<-hooks.preparing
	callCancel()
	if err := <-done; err != nil {
		t.Fatal(err.Error())
	}
	if exists("abandoned") {
		t.Fatal("abandoned batch created its object")
	}
}
