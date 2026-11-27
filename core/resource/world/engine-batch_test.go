//go:build !js

package resource_world_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/core/sobject"
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
	// gate holds PrepareApply, if set, until gate is closed, as a cold
	// plugin load would.
	gate chan struct{}
	// preparing receives once per PrepareApply that waits on gate.
	preparing chan struct{}
}

// batchTestOp is a World op whose payload selects its action: "reject"
// rejects, "timeout" fails as a plugin timeout, "unavailable" fails to load
// its plugin, "missing" is rejected as a pinned executable that does not
// exist, and "create:<key>" creates the object key.
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

// errMissingExecutable is the error a pinned executable with no build for
// this device meets, whether its load runs in PrepareApply or in the apply.
var errMissingExecutable = &world.PluginExecutableMissingError{PluginID: "test", OperationTypeID: batchTestOpID, ManifestRoot: "root"}

// PrepareApply fails as an unavailable plugin for "unavailable", fails a
// missing executable for "missing", and otherwise waits on the hooks' gate
// when one is set.
func (o *batchTestOp) PrepareApply(ctx context.Context) (func(), error) {
	switch o.action {
	case "unavailable":
		return nil, &world.PluginUnavailableError{PluginID: "test", OperationTypeID: batchTestOpID, Err: context.DeadlineExceeded}
	case "missing":
		return nil, errMissingExecutable
	}
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
	case o.action == "missing":
		return false, errMissingExecutable
	case o.action == "timeout":
		return false, &world.PluginOpTimeoutError{PluginID: "test", OperationTypeID: batchTestOpID, Deadline: time.Second}
	case strings.HasPrefix(o.action, "create:"):
		obj, err := ws.CreateObject(ctx, strings.TrimPrefix(o.action, "create:"), &bucket.ObjectRef{})
		world.ReleaseObjectState(obj)
		return false, err
	}
	return false, world.ErrUnhandledOp
}

// ApplyWorldObjectOp rejects for "reject" and "missing" and otherwise
// increments the object's revision.
func (o *batchTestOp) ApplyWorldObjectOp(ctx context.Context, le *logrus.Entry, obj world.ObjectState, sender peer.ID) (bool, error) {
	switch o.action {
	case "reject":
		return false, &world.OperationRejection{Code: "test/refused", Message: "refused by the test op"}
	case "missing":
		return false, errMissingExecutable
	}
	_, err := obj.IncrementRev(ctx)
	return false, err
}

// _ is a type assertion.
var (
	_ world.PreparedOperation = (*batchTestOp)(nil)
	_ world.ObjectOperation   = (*batchTestOp)(nil)
)

// TestExecuteWorldBatchOps checks op entries: a rejection, a plugin timeout
// or an unavailable plugin discards the batch and reports its index and code,
// an op is applied as the sender ExecuteWorldOp binds, and a plugin load
// before the transaction holds no writer and ends with the caller's context
// error. It then checks that object entries report revisions at their
// indexes and refuse on an absent object, and that a missing pinned
// executable stays a rejection, not code 5, on every path.
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
		return &s4wave_world.ApplyWorldOpRequest{OpTypeId: batchTestOpID, OpData: []byte(action), OpSender: "spoofed", KnownErrorCode: s4wave_world.KnownErrorCode}
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

	// An unavailable plugin refuses the batch with code 5 before any op is
	// applied.
	hooks.senders = nil
	resp, err = batch(ctx, create("unreached"), op("create:unreached-op"), op("unavailable"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetFailedIndex() != 2 || resp.GetErrorCode() != s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_PLUGIN_UNAVAILABLE {
		t.Fatalf("unavailable batch: %v", resp)
	}
	if exists("unreached") || exists("unreached-op") || len(hooks.senders) != 0 {
		t.Fatal("unavailable batch applied an op")
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
	hooks.gate = nil

	// Object entries return the object's revision at their own index, and
	// zero at the other kinds' indexes.
	objOp := func(key, action string) *s4wave_world.WorldMutation {
		return &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_ApplyObjectOp{
			ApplyObjectOp: &s4wave_world.ApplyObjectOpMutation{ObjectKey: key, OpTypeId: batchTestOpID, OpData: []byte(action)},
		}}
	}
	setRoot := &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_SetRootRef{
		SetRootRef: &s4wave_world.SetRootRefMutation{ObjectKey: "revs", RootRef: &bucket.ObjectRef{}},
	}}
	incRev := func(key string) *s4wave_world.WorldMutation {
		return &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_IncrementRev{
			IncrementRev: &s4wave_world.IncrementRevMutation{ObjectKey: key},
		}}
	}
	dropGraph := &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_DeleteGraphObject{
		DeleteGraphObject: &s4wave_world.DeleteGraphObjectRequest{ObjectKey: "revs"},
	}}
	resp, err = batch(ctx, create("revs"), setRoot, objOp("revs", "bump"), dropGraph, incRev("revs"))
	if err != nil || resp.GetErrorCode() != 0 || resp.GetRejectionCode() != "" {
		t.Fatalf("object batch: %v %v", resp, err)
	}
	revs := make([]uint64, len(resp.GetResults()))
	for i, res := range resp.GetResults() {
		revs[i] = res.GetRev()
	}
	if len(revs) != 5 || revs[0] != 0 || revs[3] != 0 || revs[1] == 0 || revs[2] <= revs[1] || revs[4] <= revs[2] {
		t.Fatalf("object batch revs: %v", revs)
	}
	obj, found, err := ws.GetObject(ctx, "revs")
	if err != nil || !found {
		t.Fatalf("object batch lost its object: %v", err)
	}
	_, rev, err := obj.GetRootRef(ctx)
	world.ReleaseObjectState(obj)
	if err != nil || rev != revs[4] {
		t.Fatalf("object rev %d, last result %d: %v", rev, revs[4], err)
	}

	// An object entry on an absent object, a rejected object op, an object op
	// whose plugin is unavailable, an op or object op whose pinned executable
	// is missing, and a rename of an absent object each refuse the batch with
	// no results.
	rename := &s4wave_world.WorldMutation{Mutation: &s4wave_world.WorldMutation_RenameObject{
		RenameObject: &s4wave_world.RenameObjectRequest{OldObjectKey: "absent", NewObjectKey: "renamed"},
	}}
	for _, refused := range []struct {
		m    *s4wave_world.WorldMutation
		code s4wave_world.WorldErrorCode
		rej  string
	}{
		{incRev("absent"), s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_OBJECT_NOT_FOUND, ""},
		{objOp("revs", "reject"), 0, "test/refused"},
		{objOp("revs", "unavailable"), s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_PLUGIN_UNAVAILABLE, ""},
		{objOp("revs", "missing"), s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_PLUGIN_EXECUTABLE_MISSING, ""},
		{op("missing"), s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_PLUGIN_EXECUTABLE_MISSING, ""},
		{rename, s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_OBJECT_NOT_FOUND, ""},
	} {
		resp, err = batch(ctx, create("refused-obj"), refused.m)
		if err != nil {
			t.Fatal(err.Error())
		}
		if resp.GetFailedIndex() != 1 || resp.GetErrorCode() != refused.code ||
			resp.GetRejectionCode() != refused.rej || len(resp.GetResults()) != 0 {
			t.Fatalf("refused object batch: %v", resp)
		}
		if exists("refused-obj") {
			t.Fatal("refused object batch created its object")
		}
	}
	// A missing pinned executable is its own code, not a rejection, through
	// the single op and the object op as well as the batch above.
	opResp, err := engineResource.ExecuteWorldOp(ctx, opReq("missing"))
	if err != nil || opResp.GetErrorCode() != s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_PLUGIN_EXECUTABLE_MISSING || opResp.GetRejectionCode() != "" {
		t.Fatalf("missing executable op: %v %v", opResp, err)
	}
	obj, found, err = ws.GetObject(ctx, "revs")
	if err != nil || !found {
		t.Fatalf("missing executable object: %v", err)
	}
	defer world.ReleaseObjectState(obj)
	objResource := resource_world.NewObjectStateResource(logrus.NewEntry(logrus.New()), tb.Bus, obj, lookupOp)
	objResp, err := objResource.ApplyObjectOp(ctx, &s4wave_world.ApplyObjectOpRequest{OpTypeId: batchTestOpID, OpData: []byte("missing"), KnownErrorCode: s4wave_world.KnownErrorCode})
	if err != nil || objResp.GetErrorCode() != s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_PLUGIN_EXECUTABLE_MISSING || objResp.GetRejectionCode() != "" {
		t.Fatalf("missing executable object op: %v %v", objResp, err)
	}

	// A client built before the code, whose request declares no known code,
	// gets the refusal as a failed call, never as a response it reads as
	// success.
	oldReq := opReq("missing")
	oldReq.KnownErrorCode = 0
	if opResp, err := engineResource.ExecuteWorldOp(ctx, oldReq); err == nil || opResp != nil {
		t.Fatalf("missing executable op for an old client: %v %v, want a failed call", opResp, err)
	}
	objResp, err = objResource.ApplyObjectOp(ctx, &s4wave_world.ApplyObjectOpRequest{OpTypeId: batchTestOpID, OpData: []byte("missing")})
	if err == nil || objResp != nil {
		t.Fatalf("missing executable object op for an old client: %v %v, want a failed call", objResp, err)
	}
}

// replayWaitEngine refuses every write transaction with the replay wait, as a
// Space engine does while replay is not caught up.
type replayWaitEngine struct {
	world.Engine
}

// NewTransaction refuses a write with the replay wait.
func (e *replayWaitEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	if write {
		return nil, sobject.ErrReplayWaits
	}
	return e.Engine.NewTransaction(ctx, write)
}

// TestReplayWaitRefusal checks that a write the World refuses while replay
// waits reaches each RPC caller as the typed REPLAY_WAITS code when its
// request declares that code as known, and as a failed call otherwise:
// a caller built before the code never reads the refusal as success. The
// batch carries the code to every caller, as every batch client already
// fails on any code.
func TestReplayWaitRefusal(t *testing.T) {
	ctx := t.Context()
	tb, cleanup := setupWorldTestbed(ctx, t)
	defer cleanup()
	obj, err := world.NewEngineWorldState(tb.Engine, true).CreateObject(ctx, "waits", &bucket.ObjectRef{})
	if err != nil {
		t.Fatal(err.Error())
	}
	_, startRev, err := obj.GetRootRef(ctx)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	edge := world.NewGraphQuadWithKeys("waits", "<waits-rel>", "waits", "")
	if err := world.NewEngineWorldState(tb.Engine, true).SetGraphQuad(ctx, edge); err != nil {
		t.Fatal(err.Error())
	}
	eng := &replayWaitEngine{Engine: tb.Engine}
	lookupOp := func(ctx context.Context, typeID string) (world.Operation, error) {
		if typeID != batchTestOpID {
			return nil, world.ErrUnhandledOp
		}
		return &batchTestOp{h: &batchTestHooks{}}, nil
	}
	engineResource := resource_world.NewEngineResource(logrus.NewEntry(logrus.New()), tb.Bus, eng, lookupOp, nil)
	defer engineResource.Close()
	ctx = resource_server.WithResourceClientContext(ctx, &worldStateOperationResourceContext{ctx: ctx})
	waits := s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_REPLAY_WAITS
	known := s4wave_world.KnownErrorCode

	// Check NewTransaction.
	txResp, err := engineResource.NewTransaction(ctx, &s4wave_world.NewTransactionRequest{Write: true, KnownErrorCode: known})
	if err != nil || txResp.GetErrorCode() != waits || txResp.GetResourceId() != 0 {
		t.Fatalf("write transaction while replay waits: %v %v, want REPLAY_WAITS", txResp, err)
	}
	if txResp, err := engineResource.NewTransaction(ctx, &s4wave_world.NewTransactionRequest{Write: true}); !errors.Is(err, sobject.ErrReplayWaits) || txResp != nil {
		t.Fatalf("old client's write transaction while replay waits: %v %v, want a failed call", txResp, err)
	}

	// Check ApplyWorldOp through ExecuteWorldOp.
	opReq := &s4wave_world.ApplyWorldOpRequest{OpTypeId: batchTestOpID, OpData: []byte("create:op-waits"), KnownErrorCode: known}
	opResp, err := engineResource.ExecuteWorldOp(ctx, opReq)
	if err != nil || opResp.GetErrorCode() != waits {
		t.Fatalf("op while replay waits: %v %v, want REPLAY_WAITS", opResp, err)
	}
	opReq.KnownErrorCode = 0
	if opResp, err := engineResource.ExecuteWorldOp(ctx, opReq); !errors.Is(err, sobject.ErrReplayWaits) || opResp != nil {
		t.Fatalf("old client's op while replay waits: %v %v, want a failed call", opResp, err)
	}

	// Check the batch.
	batchResp, err := engineResource.ExecuteWorldBatch(ctx, &s4wave_world.ExecuteWorldBatchRequest{Mutations: []*s4wave_world.WorldMutation{{
		Mutation: &s4wave_world.WorldMutation_CreateObject{CreateObject: &s4wave_world.CreateObjectRequest{ObjectKey: "batch-waits", RootRef: &bucket.ObjectRef{}}},
	}}})
	if err != nil || batchResp.GetErrorCode() != waits || len(batchResp.GetResults()) != 0 {
		t.Fatalf("batch while replay waits: %v %v, want REPLAY_WAITS", batchResp, err)
	}

	// Check SetRootRef on an object of the waiting World.
	wobj, found, err := world.NewEngineWorldState(eng, true).GetObject(ctx, "waits")
	if err != nil || !found {
		t.Fatalf("object of the waiting World: %v %v", found, err)
	}
	defer world.ReleaseObjectState(wobj)
	objResource := resource_world.NewObjectStateResource(logrus.NewEntry(logrus.New()), tb.Bus, wobj, lookupOp)
	setResp, err := objResource.SetRootRef(ctx, &s4wave_world.SetRootRefRequest{RootRef: &bucket.ObjectRef{}, KnownErrorCode: known})
	if err != nil || setResp.GetErrorCode() != waits || setResp.GetRev() != 0 {
		t.Fatalf("root set while replay waits: %v %v, want REPLAY_WAITS", setResp, err)
	}
	if setResp, err := objResource.SetRootRef(ctx, &s4wave_world.SetRootRefRequest{RootRef: &bucket.ObjectRef{}}); !errors.Is(err, sobject.ErrReplayWaits) || setResp != nil {
		t.Fatalf("old client's root set while replay waits: %v %v, want a failed call", setResp, err)
	}

	// Check IncrementRev on the same object.
	revResp, err := objResource.IncrementRev(ctx, &s4wave_world.IncrementRevRequest{KnownErrorCode: known})
	if err != nil || revResp.GetErrorCode() != waits || revResp.GetRev() != 0 {
		t.Fatalf("rev increment while replay waits: %v %v, want REPLAY_WAITS", revResp, err)
	}
	if revResp, err := objResource.IncrementRev(ctx, &s4wave_world.IncrementRevRequest{}); !errors.Is(err, sobject.ErrReplayWaits) || revResp != nil {
		t.Fatalf("old client's rev increment while replay waits: %v %v, want a failed call", revResp, err)
	}

	// Check the direct World state writes, each with the code and as an old
	// client's failed call.
	stateResource := resource_world.NewWorldStateResource(logrus.NewEntry(logrus.New()), tb.Bus, world.NewEngineWorldState(eng, true), lookupOp)
	defer stateResource.Close()
	edgeQuad := world.GraphQuadToQuad(edge)
	newQuad := world.GraphQuadToQuad(world.NewGraphQuadWithKeys("waits", "<set-waits>", "waits", ""))
	writes := []struct {
		name  string
		write func(known s4wave_world.WorldErrorCode) (codeResponse, error)
	}{
		{"create", func(known s4wave_world.WorldErrorCode) (codeResponse, error) {
			req := &s4wave_world.CreateObjectRequest{ObjectKey: "create-waits", RootRef: &bucket.ObjectRef{}, KnownErrorCode: known}
			return nilIfErr(stateResource.CreateObject(ctx, req))
		}},
		{"rename", func(known s4wave_world.WorldErrorCode) (codeResponse, error) {
			req := &s4wave_world.RenameObjectRequest{OldObjectKey: "waits", NewObjectKey: "rename-waits", KnownErrorCode: known}
			return nilIfErr(stateResource.RenameObject(ctx, req))
		}},
		{"delete", func(known s4wave_world.WorldErrorCode) (codeResponse, error) {
			req := &s4wave_world.DeleteObjectRequest{ObjectKey: "waits", KnownErrorCode: known}
			return nilIfErr(stateResource.DeleteObject(ctx, req))
		}},
		{"set quad", func(known s4wave_world.WorldErrorCode) (codeResponse, error) {
			req := &s4wave_world.SetGraphQuadRequest{Quad: newQuad, KnownErrorCode: known}
			return nilIfErr(stateResource.SetGraphQuad(ctx, req))
		}},
		{"delete quad", func(known s4wave_world.WorldErrorCode) (codeResponse, error) {
			req := &s4wave_world.DeleteGraphQuadRequest{Quad: edgeQuad, KnownErrorCode: known}
			return nilIfErr(stateResource.DeleteGraphQuad(ctx, req))
		}},
		{"delete graph object", func(known s4wave_world.WorldErrorCode) (codeResponse, error) {
			req := &s4wave_world.DeleteGraphObjectRequest{ObjectKey: "waits", KnownErrorCode: known}
			return nilIfErr(stateResource.DeleteGraphObject(ctx, req))
		}},
		{"world op", func(known s4wave_world.WorldErrorCode) (codeResponse, error) {
			req := &s4wave_world.ApplyWorldOpRequest{OpTypeId: batchTestOpID, OpData: []byte("create:state-op-waits"), KnownErrorCode: known}
			return nilIfErr(stateResource.ApplyWorldOp(ctx, req))
		}},
		{"object op", func(known s4wave_world.WorldErrorCode) (codeResponse, error) {
			req := &s4wave_world.ApplyObjectOpRequest{OpTypeId: batchTestOpID, OpData: []byte("bump"), KnownErrorCode: known}
			return nilIfErr(objResource.ApplyObjectOp(ctx, req))
		}},
	}
	for _, w := range writes {
		resp, err := w.write(known)
		if err != nil || resp == nil || resp.GetErrorCode() != waits {
			t.Fatalf("%s while replay waits: %v %v, want REPLAY_WAITS", w.name, resp, err)
		}
		if resp, err := w.write(0); !errors.Is(err, sobject.ErrReplayWaits) || resp != nil {
			t.Fatalf("old client's %s while replay waits: %v %v, want a failed call", w.name, resp, err)
		}
	}

	// Nothing was written: no new object, the object kept its revision and
	// edge, and no new edge.
	ws := world.NewEngineWorldState(tb.Engine, false)
	for _, key := range []string{"op-waits", "batch-waits", "create-waits", "rename-waits", "state-op-waits"} {
		obj, found, err := ws.GetObject(ctx, key)
		world.ReleaseObjectState(obj)
		if err != nil || found {
			t.Fatalf("object %s after the refusals: found %v, %v", key, found, err)
		}
	}
	kept, found, err := ws.GetObject(ctx, "waits")
	if err != nil || !found {
		t.Fatalf("refused delete removed the object: %v %v", found, err)
	}
	_, rev, err := kept.GetRootRef(ctx)
	world.ReleaseObjectState(kept)
	if err != nil || rev != startRev {
		t.Fatalf("object revision after the refusals: %d %v, want %d", rev, err, startRev)
	}
	for pred, want := range map[string]int{"<waits-rel>": 1, "<set-waits>": 0} {
		quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("waits", pred, "waits", ""), 0)
		if err != nil || len(quads) != want {
			t.Fatalf("edges %s after the refusals: %d %v, want %d", pred, len(quads), err, want)
		}
	}
}

// codeResponse is a World RPC response that carries an error code.
type codeResponse interface {
	GetErrorCode() s4wave_world.WorldErrorCode
}

// nilIfErr returns a nil response with err, so a failed call compares as nil
// whatever the response type.
func nilIfErr[T interface {
	comparable
	codeResponse
}](resp T, err error) (codeResponse, error) {
	var zero T
	if err != nil || resp == zero {
		return nil, err
	}
	return resp, nil
}

// refuseKeyGuard refuses a write targeting key, and allows every other write.
type refuseKeyGuard struct{ key string }

// CheckWrite refuses w when it targets the guard's key.
func (g *refuseKeyGuard) CheckWrite(_ context.Context, _ world.WorldState, w *world.GuardedWrite) error {
	if slices.Contains(w.ObjectKeys, g.key) {
		return &world.OperationRejection{Code: "test/guard-refused", Message: "refused by the test guard"}
	}
	return nil
}

// TestGuardedEngineResourceExecutesOps checks an op applied on an engine mount
// guarded as a plugin's mount, through ExecuteWorldOp and through the World
// state the mount's typed objects write with: an allowed write commits, and
// the op fails and commits nothing when the mount's guard or a different
// guard stacked over the mount's state refuses a write.
func TestGuardedEngineResourceExecutesOps(t *testing.T) {
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
	mountGuard := &refuseKeyGuard{key: "test/mount-refused"}
	engineResource := resource_world.NewEngineResource(
		logrus.NewEntry(logrus.New()), tb.Bus, tb.Engine, lookupOp, nil,
		resource_world.WithWriteGuard(mountGuard),
	)
	defer engineResource.Close()
	ws := world.NewEngineWorldState(tb.Engine, true)
	exists := func(key string) bool {
		obj, found, err := ws.GetObject(ctx, key)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err.Error())
		}
		return found
	}

	// ExecuteWorldOp commits an allowed write and refuses the guard's key.
	for _, tc := range []struct {
		key       string
		rejection string
	}{
		{"test/mount-allowed", ""},
		{"test/mount-refused", "test/guard-refused"},
	} {
		req := &s4wave_world.ApplyWorldOpRequest{OpTypeId: batchTestOpID, OpData: []byte("create:" + tc.key)}
		resp, err := engineResource.ExecuteWorldOp(ctx, req)
		if err != nil || resp.GetErrorCode() != 0 || resp.GetRejectionCode() != tc.rejection {
			t.Fatalf("ExecuteWorldOp create %q: %v %v, want rejection %q", tc.key, resp, err, tc.rejection)
		}
		if exists(tc.key) != (tc.rejection == "") {
			t.Fatalf("ExecuteWorldOp create %q: object exists %v", tc.key, exists(tc.key))
		}
	}

	// The state the mount's typed objects write with is the mount's state
	// guarded again by the mount's guard. A different guard over that state
	// is consulted too.
	guarded := world.GuardEngine(tb.Engine, mountGuard)
	outerGuard := &refuseKeyGuard{key: "test/outer-refused"}
	for _, tc := range []struct {
		guard   world.WriteGuard
		key     string
		refused bool
	}{
		{mountGuard, "test/typed-allowed", false},
		{mountGuard, "test/mount-refused", true},
		{outerGuard, "test/outer-refused", true},
		{outerGuard, "test/mount-refused", true},
	} {
		typedWs := world.GuardWorldState(world.NewEngineWorldState(guarded, true), tc.guard)
		_, _, err := typedWs.ApplyWorldOp(ctx, &batchTestOp{h: hooks, action: "create:" + tc.key}, "")
		var rejection *world.OperationRejection
		if refused := errors.As(err, &rejection); refused != tc.refused || (!tc.refused && err != nil) {
			t.Fatalf("typed create %q: %v, want refused %v", tc.key, err, tc.refused)
		}
		if exists(tc.key) != !tc.refused {
			t.Fatalf("typed create %q: object exists %v", tc.key, exists(tc.key))
		}
	}
}
