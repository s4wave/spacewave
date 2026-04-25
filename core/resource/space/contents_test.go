package resource_space

import (
	"context"
	"crypto/rand"
	"slices"
	"testing"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	forge_dashboard "github.com/s4wave/spacewave/core/forge/dashboard"
	forge_job_ops "github.com/s4wave/spacewave/core/forge/job"
	forge_task_ops "github.com/s4wave/spacewave/core/forge/task"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	"github.com/s4wave/spacewave/db/world"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_forge_world "github.com/s4wave/spacewave/sdk/forge/world"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

type testWatchSpaceContentsStateStream struct {
	srpc.Stream
	ctx  context.Context
	msgs chan *s4wave_space.SpaceContentsState
}

func newTestWatchSpaceContentsStateStream(ctx context.Context) *testWatchSpaceContentsStateStream {
	return &testWatchSpaceContentsStateStream{
		ctx:  ctx,
		msgs: make(chan *s4wave_space.SpaceContentsState, 4),
	}
}

func (m *testWatchSpaceContentsStateStream) Context() context.Context {
	return m.ctx
}

func (m *testWatchSpaceContentsStateStream) Send(resp *s4wave_space.SpaceContentsState) error {
	select {
	case m.msgs <- resp:
		return nil
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
}

func (m *testWatchSpaceContentsStateStream) SendAndClose(resp *s4wave_space.SpaceContentsState) error {
	return m.Send(resp)
}

func (m *testWatchSpaceContentsStateStream) MsgRecv(_ srpc.Message) error {
	return nil
}

func (m *testWatchSpaceContentsStateStream) MsgSend(_ srpc.Message) error {
	return nil
}

func (m *testWatchSpaceContentsStateStream) CloseSend() error {
	return nil
}

func (m *testWatchSpaceContentsStateStream) Close() error {
	return nil
}

// TestSpaceContentsResource_GetPluginDescriptionsCache checks cache reuse and invalidation.
func TestSpaceContentsResource_GetPluginDescriptionsCache(t *testing.T) {
	ctx := t.Context()
	var calls int

	r := &SpaceContentsResource{
		buildDescriptions: func(_ context.Context, _ world.WorldState, pluginIDs []string) (map[string]string, error) {
			calls++
			return map[string]string{
				pluginIDs[0]: "desc-" + pluginIDs[0],
			}, nil
		},
	}

	descriptions, err := r.getPluginDescriptions(ctx, nil, []string{"alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 build, got %d", calls)
	}
	if descriptions["alpha"] != "desc-alpha" {
		t.Fatalf("unexpected description: %#v", descriptions)
	}

	descriptions["alpha"] = "mutated"
	cachedDescriptions, err := r.getPluginDescriptions(ctx, nil, []string{"alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected cache hit, got %d builds", calls)
	}
	if cachedDescriptions["alpha"] != "desc-alpha" {
		t.Fatalf("cache alias leaked mutation: %#v", cachedDescriptions)
	}

	_, err = r.getPluginDescriptions(ctx, nil, []string{"beta"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected rebuild after plugin set change, got %d builds", calls)
	}

	reorderedDescriptions, err := r.getPluginDescriptions(ctx, nil, []string{"beta", "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("expected rebuild for changed plugin set, got %d builds", calls)
	}
	if !slices.Equal(r.descriptionPluginIDs, []string{"beta", "alpha"}) {
		t.Fatalf("unexpected cached plugin ids: %v", r.descriptionPluginIDs)
	}
	if reorderedDescriptions["beta"] != "desc-beta" {
		t.Fatalf("unexpected rebuilt descriptions: %#v", reorderedDescriptions)
	}
}

func generateSpaceContentsTestPeerID(t *testing.T) peer.ID {
	t.Helper()

	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func applyWizardFinalizeOp(
	ctx context.Context,
	t testing.TB,
	engine world.Engine,
	wizardKey string,
	wizardTypeID string,
	targetTypeID string,
	targetKeyPrefix string,
	name string,
	op world.Operation,
	sender peer.ID,
) {
	t.Helper()

	wizardOp := s4wave_wizard.NewCreateWizardObjectOp(
		wizardKey,
		wizardTypeID,
		targetTypeID,
		targetKeyPrefix,
		name,
		time.Now(),
	)
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(%s wizard create): %v", wizardTypeID, err)
	}
	defer tx.Discard()
	_, _, err = tx.ApplyWorldOp(ctx, wizardOp, "")
	if err != nil {
		t.Fatalf("ApplyWorldOp(%s wizard): %v", wizardTypeID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit(%s wizard create): %v", wizardTypeID, err)
	}

	tx, err = engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(%s finalize): %v", wizardTypeID, err)
	}
	defer tx.Discard()
	_, _, err = tx.ApplyWorldOp(ctx, op, sender)
	if err != nil {
		t.Fatalf("ApplyWorldOp(%s finalize): %v", wizardTypeID, err)
	}
	_, err = tx.DeleteObject(ctx, wizardKey)
	if err != nil {
		t.Fatalf("DeleteObject(%s wizard): %v", wizardTypeID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit(%s finalize): %v", wizardTypeID, err)
	}
}

func waitForForgeExecutionState(
	ctx context.Context,
	t testing.TB,
	engine world.Engine,
	ws world.WorldState,
	jobKey string,
) (*forge_pass.Pass, *forge_execution.Execution) {
	t.Helper()

	seqno, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatalf("GetSeqno: %v", err)
	}

	for {
		taskKeys, err := forge_job.ListJobTasks(ctx, ws, jobKey)
		if err != nil {
			t.Fatalf("ListJobTasks: %v", err)
		}
		for _, taskKey := range taskKeys {
			passKeys, err := forge_task.ListTaskPasses(ctx, ws, taskKey)
			if err != nil {
				t.Fatalf("ListTaskPasses(%s): %v", taskKey, err)
			}
			for _, passKey := range passKeys {
				passState, _, err := forge_pass.LookupPass(ctx, ws, passKey)
				if err != nil {
					t.Fatalf("LookupPass(%s): %v", passKey, err)
				}
				execKeys, err := forge_pass.ListPassExecutions(ctx, ws, passKey)
				if err != nil {
					t.Fatalf("ListPassExecutions(%s): %v", passKey, err)
				}
				for _, execKey := range execKeys {
					execState, _, err := forge_execution.LookupExecution(ctx, ws, execKey)
					if err != nil {
						t.Fatalf("LookupExecution(%s): %v", execKey, err)
					}
					if passState.IsComplete() && execState.IsComplete() && len(execState.GetLogEntries()) != 0 {
						return passState, execState
					}
				}
			}
		}

		seqno, err = engine.WaitSeqno(ctx, seqno+1)
		if err != nil {
			t.Fatalf("WaitSeqno: %v", err)
		}
	}
}

func TestSpaceContentsResource_ForgeWizardChainStartsApprovedWorker(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tb.StaticResolver.AddFactory(plugin_space.NewFactory(tb.Bus))

	lookupFunc := func(ctx context.Context, typeID string) (objecttype.ObjectType, error) {
		if typeID == forge_worker.WorkerTypeID {
			return s4wave_forge_world.WorkerType, nil
		}
		return nil, nil
	}
	objectTypeCtrl := objecttype_controller.NewController(lookupFunc)
	objectTypeRef, err := tb.Bus.AddController(ctx, objectTypeCtrl, nil)
	if err != nil {
		t.Fatalf("AddController(objecttype): %v", err)
	}
	defer objectTypeRef()

	pid := generateSpaceContentsTestPeerID(t)
	sender := pid.String()
	ts := timestamppb.Now()
	clusterKey := "forge/cluster/wizard-test"
	jobKey := "forge/job/wizard-test"
	taskKey := "forge/task/wizard-extra"
	workerKey := "forge/worker/wizard-session"
	clusterWizardKey := "wizard/forge/cluster/wizard-test"
	jobWizardKey := "wizard/forge/job/wizard-test"
	taskWizardKey := "wizard/forge/task/wizard-test"

	clusterOp := &forge_cluster.ClusterCreateOp{
		ClusterKey: clusterKey,
		Name:       "wizard-cluster",
	}
	applyWizardFinalizeOp(
		ctx,
		t,
		tb.Engine,
		clusterWizardKey,
		"wizard/forge/cluster",
		forge_cluster.ClusterTypeID,
		"forge/cluster/",
		"wizard-cluster",
		clusterOp,
		pid,
	)

	jobOp := &forge_job_ops.ForgeJobCreateOp{
		JobKey:     jobKey,
		ClusterKey: clusterKey,
		TaskDefs: []*forge_job_ops.ForgeJobTaskDef{
			{Name: "build"},
		},
		Timestamp: ts,
	}
	applyWizardFinalizeOp(
		ctx,
		t,
		tb.Engine,
		jobWizardKey,
		"wizard/forge/job",
		forge_job.JobTypeID,
		"forge/job/",
		"wizard-job",
		jobOp,
		pid,
	)

	taskOp := &forge_task_ops.ForgeTaskCreateOp{
		TaskKey:   taskKey,
		Name:      "verify",
		JobKey:    jobKey,
		Timestamp: ts,
	}
	applyWizardFinalizeOp(
		ctx,
		t,
		tb.Engine,
		taskWizardKey,
		"wizard/forge/task",
		forge_task.TaskTypeID,
		"forge/task/",
		"wizard-task",
		taskOp,
		pid,
	)

	workerOp := forge_worker.NewWorkerCreateOp(workerKey, "session-worker", nil)
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, workerOp, pid); err != nil {
		t.Fatalf("ApplyWorldOp(worker create): %v", err)
	}
	if err := tb.WorldState.SetGraphQuad(ctx, identity_world.NewObjectToKeypairQuad(
		workerKey,
		identity_world.NewKeypairKey(sender),
	)); err != nil {
		t.Fatalf("SetGraphQuad(worker keypair): %v", err)
	}
	assignWorkerOp := forge_cluster.NewClusterAssignWorkerOp(clusterKey, workerKey)
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, assignWorkerOp, pid); err != nil {
		t.Fatalf("ApplyWorldOp(assign worker): %v", err)
	}

	taskKeys, err := forge_job.ListJobTasks(ctx, tb.WorldState, jobKey)
	if err != nil {
		t.Fatalf("ListJobTasks: %v", err)
	}
	if len(taskKeys) != 2 {
		t.Fatalf("expected 2 tasks linked to job, got %v", taskKeys)
	}
	for _, linkedTaskKey := range taskKeys {
		if err := forge_job.EnsureJobHasTask(ctx, tb.WorldState, jobKey, linkedTaskKey); err != nil {
			t.Fatalf("EnsureJobHasTask(%s): %v", linkedTaskKey, err)
		}
	}

	readTx, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction(verify wizard cleanup): %v", err)
	}
	defer readTx.Discard()
	for _, wizardKey := range []string{clusterWizardKey, jobWizardKey, taskWizardKey} {
		_, found, err := readTx.GetObject(ctx, wizardKey)
		if err != nil {
			t.Fatalf("GetObject(%s): %v", wizardKey, err)
		}
		if found {
			t.Fatalf("wizard object %s should be deleted", wizardKey)
		}
	}

	conf := &plugin_space.Config{
		SpaceId:       "space-test",
		VolumeId:      tb.EngineVolumeID,
		ObjectStoreId: "platform-account",
		EngineId:      tb.EngineID,
		SessionPeerId: sender,
	}
	ctrl, _, ctrlRef, err := plugin_space.StartControllerWithConfig(ctx, tb.Bus, conf, func() {})
	if err != nil {
		t.Fatalf("StartControllerWithConfig: %v", err)
	}

	resource := NewSpaceContentsResource(
		logrus.NewEntry(logrus.StandardLogger()),
		tb.Bus,
		tb.Engine,
		"space-test",
		tb.EngineID,
	)
	resource.ctrl = ctrl
	resource.ctrlRef = ctrlRef
	resource.volumeID = tb.EngineVolumeID
	resource.storeID = "platform-account"
	defer resource.Release()

	for _, linkedTaskKey := range taskKeys {
		passKeys, err := forge_task.ListTaskPasses(ctx, tb.WorldState, linkedTaskKey)
		if err != nil {
			t.Fatalf("ListTaskPasses(%s): %v", linkedTaskKey, err)
		}
		if len(passKeys) != 0 {
			t.Fatalf("expected no passes before approval for %s, got %v", linkedTaskKey, passKeys)
		}
	}

	_, err = resource.SetProcessBinding(ctx, &s4wave_space.SetProcessBindingRequest{
		ObjectKey: workerKey,
		TypeId:    forge_worker.WorkerTypeID,
		Approved:  true,
	})
	if err != nil {
		t.Fatalf("SetProcessBinding: %v", err)
	}

	jobState, err := forge_job.WaitJobComplete(
		ctx,
		logrus.NewEntry(logrus.StandardLogger()),
		tb.WorldState,
		jobKey,
	)
	if err != nil {
		t.Fatalf("WaitJobComplete: %v", err)
	}
	if !jobState.IsComplete() {
		t.Fatalf("expected complete job, got %s", jobState.GetJobState().String())
	}

	for _, linkedTaskKey := range taskKeys {
		passKeys, err := forge_task.ListTaskPasses(ctx, tb.WorldState, linkedTaskKey)
		if err != nil {
			t.Fatalf("ListTaskPasses(%s): %v", linkedTaskKey, err)
		}
		if len(passKeys) == 0 {
			t.Fatalf("expected passes after approval for %s", linkedTaskKey)
		}
		passState, _, err := forge_pass.LookupPass(ctx, tb.WorldState, passKeys[0])
		if err != nil {
			t.Fatalf("LookupPass(%s): %v", passKeys[0], err)
		}
		if !passState.IsComplete() {
			t.Fatalf("expected complete pass for %s, got %s", linkedTaskKey, passState.GetPassState().String())
		}
		execKeys, err := forge_pass.ListPassExecutions(ctx, tb.WorldState, passKeys[0])
		if err != nil {
			t.Fatalf("ListPassExecutions(%s): %v", passKeys[0], err)
		}
		if len(execKeys) == 0 {
			t.Fatalf("expected execution for pass %s", passKeys[0])
		}
		execState, _, err := forge_execution.LookupExecution(ctx, tb.WorldState, execKeys[0])
		if err != nil {
			t.Fatalf("LookupExecution(%s): %v", execKeys[0], err)
		}
		if !execState.IsComplete() {
			t.Fatalf("expected complete execution for %s, got %s", linkedTaskKey, execState.GetExecutionState().String())
		}
	}
}

func TestSpaceContentsResource_SetProcessBindingStartsForgeWorker(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tb.StaticResolver.AddFactory(plugin_space.NewFactory(tb.Bus))

	lookupFunc := func(ctx context.Context, typeID string) (objecttype.ObjectType, error) {
		if typeID == forge_worker.WorkerTypeID {
			return s4wave_forge_world.WorkerType, nil
		}
		return nil, nil
	}
	objectTypeCtrl := objecttype_controller.NewController(lookupFunc)
	objectTypeRef, err := tb.Bus.AddController(ctx, objectTypeCtrl, nil)
	if err != nil {
		t.Fatalf("AddController(objecttype): %v", err)
	}
	defer objectTypeRef()

	pid := generateSpaceContentsTestPeerID(t)
	op := &forge_dashboard.InitForgeQuickstartOp{
		LayoutKey:     "object-layout/forge",
		DashboardKey:  "forge/dashboard",
		ClusterKey:    "forge/cluster",
		ClusterName:   "default",
		WorkerKey:     "forge/worker/session",
		SessionPeerId: pid.String(),
		Timestamp:     timestamppb.Now(),
	}
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, op, pid); err != nil {
		t.Fatalf("ApplyWorldOp: %v", err)
	}

	conf := &plugin_space.Config{
		SpaceId:       "space-test",
		VolumeId:      tb.EngineVolumeID,
		ObjectStoreId: "platform-account",
		EngineId:      tb.EngineID,
		SessionPeerId: pid.String(),
	}
	ctrl, _, ctrlRef, err := plugin_space.StartControllerWithConfig(ctx, tb.Bus, conf, func() {})
	if err != nil {
		t.Fatalf("StartControllerWithConfig: %v", err)
	}

	resource := NewSpaceContentsResource(
		logrus.NewEntry(logrus.StandardLogger()),
		tb.Bus,
		tb.Engine,
		"space-test",
		tb.EngineID,
	)
	resource.ctrl = ctrl
	resource.ctrlRef = ctrlRef
	resource.volumeID = tb.EngineVolumeID
	resource.storeID = "platform-account"
	defer resource.Release()

	taskKeys, err := forge_job.ListJobTasks(ctx, tb.WorldState, "forge/cluster/job/sample")
	if err != nil {
		t.Fatalf("ListJobTasks: %v", err)
	}
	for _, taskKey := range taskKeys {
		passKeys, err := forge_task.ListTaskPasses(ctx, tb.WorldState, taskKey)
		if err != nil {
			t.Fatalf("ListTaskPasses(%s): %v", taskKey, err)
		}
		if len(passKeys) != 0 {
			t.Fatalf("expected no passes before approval, got %v", passKeys)
		}
	}

	_, err = resource.SetProcessBinding(ctx, &s4wave_space.SetProcessBindingRequest{
		ObjectKey: "forge/worker/session",
		TypeId:    "forge/worker",
		Approved:  true,
	})
	if err != nil {
		t.Fatalf("SetProcessBinding: %v", err)
	}

	watchCtx, watchCancel := context.WithCancel(ctx)
	stream := newTestWatchSpaceContentsStateStream(watchCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- resource.WatchState(&s4wave_space.WatchSpaceContentsStateRequest{}, stream)
	}()

	var resp *s4wave_space.SpaceContentsState
	select {
	case resp = <-stream.msgs:
	case <-time.After(time.Second):
		watchCancel()
		t.Fatal("timed out waiting for space contents state")
	}
	watchCancel()
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("WatchState: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for WatchState to stop")
	}
	if len(resp.GetProcessBindings()) != 1 || !resp.GetProcessBindings()[0].GetApproved() {
		t.Fatalf("expected one approved binding, got %+v", resp.GetProcessBindings())
	}

	passState, execState := waitForForgeExecutionState(
		ctx,
		t,
		tb.Engine,
		tb.WorldState,
		"forge/cluster/job/sample",
	)
	if !passState.IsComplete() {
		t.Fatalf("expected complete pass, got %s", passState.GetPassState().String())
	}
	if !execState.IsComplete() {
		t.Fatalf("expected complete execution, got %s", execState.GetExecutionState().String())
	}
	if !execState.GetResult().IsSuccessful() {
		t.Fatalf("expected successful execution, got %q", execState.GetResult().GetFailError())
	}
	if got := execState.GetLogEntries()[0].GetMessage(); got != "noop execution complete" {
		t.Fatalf("expected noop execution log, got %q", got)
	}
}
