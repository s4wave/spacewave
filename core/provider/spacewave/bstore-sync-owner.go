package provider_spacewave

import (
	"context"

	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type bstoreSyncOwner struct {
	le      *logrus.Entry
	sc      *syncController
	execute func(context.Context) error
	jobs    *routine.RoutineContainer
}

func newBstoreSyncOwner(
	le *logrus.Entry,
	sc *syncController,
) *bstoreSyncOwner {
	return newBstoreSyncOwnerWithRoutine(
		le,
		sc,
		sc.Execute,
		routine.WithRetry(providerBackoff),
	)
}

func newBstoreSyncOwnerWithRoutine(
	le *logrus.Entry,
	sc *syncController,
	execute func(context.Context) error,
	opts ...routine.Option,
) *bstoreSyncOwner {
	o := &bstoreSyncOwner{
		le:      le,
		sc:      sc,
		execute: execute,
	}
	opts = append(opts, routine.WithExitCb(o.recordExit))
	o.jobs = routine.NewRoutineContainerWithLogger(
		le.WithField("routine", "bstore-sync"),
		opts...,
	)
	o.jobs.SetRoutine(o.run)
	return o
}

func (o *bstoreSyncOwner) Start(ctx context.Context) {
	o.jobs.SetContext(ctx, true)
}

func (o *bstoreSyncOwner) Stop() {
	waitCh, _ := o.jobs.SetRoutine(nil)
	o.jobs.ClearContext()
	if waitCh != nil {
		<-waitCh
	}
}

func (o *bstoreSyncOwner) run(ctx context.Context) error {
	return o.execute(ctx)
}

func (o *bstoreSyncOwner) recordExit(err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	o.le.WithError(err).Warn("sync controller exited with error; will retry")
	o.sc.recordSyncOwnerError(err)
}
