package plugin_host_scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/sirupsen/logrus"
)

// candidateActivation exposes the ready candidate's admission boundary.
type candidateActivation struct {
	activated chan struct{}
}

// Check declares support for staged replacement.
func (a *candidateActivation) Check(context.Context, *plugin.CheckActivationRequest) (*plugin.CheckActivationResponse, error) {
	return &plugin.CheckActivationResponse{}, nil
}

// Activate records admission after the worker reports startup complete.
func (a *candidateActivation) Activate(context.Context, *plugin.ActivatePluginRequest) (*plugin.ActivatePluginResponse, error) {
	close(a.activated)
	return &plugin.ActivatePluginResponse{}, nil
}

// TestSelectedPluginRetainsAdmittedWorker exercises the actual selection owner
// while worker startup is blocked, fails, succeeds, or is superseded.
func TestSelectedPluginRetainsAdmittedWorker(t *testing.T) {
	// Retain worker lifetime independently from each candidate-selection request.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	c := &Controller{
		le: le, conf: &Config{},
		pluginStatusCtr: ccontainer.NewCContainer(&PluginStatusSnapshot{}),
		pluginStatus:    make(map[string]*plugin.PluginStatus),
	}
	_, instance := c.newPluginInstance(pluginReference{pluginID: "colors"})
	type execution struct {
		worker     *pluginInstance
		start      chan bool
		closed     chan struct{}
		activation *candidateActivation
	}
	started := make(chan *execution, 1)
	instance.executions = keyed.NewKeyedRefCount(func(key executionReference) (keyed.Routine, *pluginInstance) {
		_, worker := instance.newExecution(key)
		run := &execution{worker: worker, start: make(chan bool, 1), closed: make(chan struct{}), activation: &candidateActivation{activated: make(chan struct{})}}
		return func(ctx context.Context) error {
			defer close(run.closed)
			defer worker.updateRpcClient(nil)
			started <- run
			select {
			case <-ctx.Done():
				return ctx.Err()
			case success := <-run.start:
				if !success {
					return errors.New("startup failed")
				}
			}
			mux := srpc.NewMux()
			if err := plugin.SRPCRegisterActivation(mux, run.activation); err != nil {
				return err
			}
			worker.updateRpcClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
			worker.finishInitialCapabilityRegistration(true)
			<-ctx.Done()
			return ctx.Err()
		}, worker
	})
	instance.executions.SetContext(ctx, true)
	defer instance.executions.ClearContext()
	defer instance.clearExecution(nil)
	start := func(rev uint64) (*execution, context.CancelFunc, <-chan error) {
		t.Helper()
		attempt, stop := context.WithCancel(ctx)
		args := &executePluginArgs{pluginHost: &testPluginHost{id: "test"}, manifestSnapshot: &manifest.ManifestSnapshot{
			ManifestRef: newTestManifestRef("colors", "js", rev, "candidate").GetManifestRef(),
			Manifest:    &manifest.Manifest{Meta: manifest.NewManifestMeta("colors", manifest.BuildType_DEV, "js", rev)},
		}}
		done := make(chan error, 1)
		go func() { done <- instance.execSelectedPlugin(attempt, args) }()
		select {
		case run := <-started:
			return run, stop, done
		case <-ctx.Done():
			t.Fatal(ctx.Err())
			return nil, nil, nil
		}
	}
	wait := func(closed <-chan struct{}) {
		t.Helper()
		select {
		case <-closed:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}

	// The first worker becomes ready and survives cancellation of its selector.
	old, stopOld, doneOld := start(1)
	old.start <- true
	initial, err := instance.runningPluginCtr.WaitValue(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	stopOld()
	<-doneOld
	select {
	case <-old.closed:
		t.Fatal("canceling selection stopped the admitted worker")
	default:
	}

	// A broken candidate cannot replace the family RPC alias or stop old work.
	failed, stopFailed, doneFailed := start(2)
	defer stopFailed()
	if !failed.worker.prepared || instance.runningPluginCtr.GetValue() != initial {
		t.Fatal("preparing candidate replaced the admitted worker")
	}
	failed.start <- false
	if err := <-doneFailed; err == nil {
		t.Fatal("failed startup succeeded")
	}
	wait(failed.closed)
	if instance.runningPluginCtr.GetValue() != initial {
		t.Fatal("failed candidate removed the admitted worker")
	}

	// Superseding an unfinished candidate releases it without disturbing old work.
	canceled, stopCanceled, doneCanceled := start(3)
	stopCanceled()
	if err := <-doneCanceled; err == nil {
		t.Fatal("canceled candidate succeeded")
	}
	wait(canceled.closed)
	if instance.runningPluginCtr.GetValue() != initial {
		t.Fatal("canceling candidate removed the admitted worker")
	}

	// A ready candidate is activated once; the retired worker's teardown cannot
	// clear the new RPC alias. Releasing the binding ends the remaining worker.
	next, stopNext, doneNext := start(4)
	next.start <- true
	wait(next.activation.activated)
	if _, err := instance.runningPluginCtr.WaitValueChange(ctx, initial, nil); err != nil {
		t.Fatal(err)
	}
	wait(old.closed)
	if instance.runningPluginCtr.GetValue() == nil {
		t.Fatal("retired teardown cleared the replacement")
	}
	stopNext()
	<-doneNext
	instance.closeExecutions()
	wait(next.closed)
	if len(instance.executions.GetKeys()) != 0 {
		t.Fatal("released binding retained worker references")
	}
	if instance.admitExecution(next.worker, func() {}, nil) {
		t.Fatal("closed binding admitted a late candidate")
	}
}
