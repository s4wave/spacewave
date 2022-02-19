package cluster_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/util/keyed"
	task_controller "github.com/aperturerobotics/forge/task/controller"
)

// taskTracker tracks a Job Task managed by the Cluster.
type taskTracker struct {
	// t is the job tracker
	t *jobTracker
	// objKey is the task object key
	objKey string
}

// newTaskTracker constructs a new job task tracker routine.
func (t *jobTracker) newTaskTracker(key string) keyed.Routine {
	tr := &taskTracker{
		t:      t,
		objKey: key,
	}
	return tr.execute
}

// execute executes the task tracker.
func (t *taskTracker) execute(ctx context.Context) error {
	// execute the pass controller
	ctrlConf := t.t.c.conf
	taskConf := task_controller.NewConfig(
		ctrlConf.GetEngineId(),
		t.objKey,
		t.t.c.peerID,
	)
	t.t.c.le.Debugf("starting task tracker: %s", t.objKey)
	_, dirRef, err := task_controller.StartControllerWithConfig(ctx, t.t.c.bus, taskConf)
	if err != nil {
		return err
	}
	<-ctx.Done()
	dirRef.Release()
	return nil
}

// _ is a type assertion
var _ keyed.Constructor = ((*jobTracker)(nil)).newTaskTracker
