//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_task "github.com/s4wave/spacewave/forge/task"
	"github.com/sirupsen/logrus"
)

type forgeObserverKind string

const (
	forgeObserverJob       forgeObserverKind = "Job"
	forgeObserverTask      forgeObserverKind = "Task"
	forgeObserverPass      forgeObserverKind = "Pass"
	forgeObserverExecution forgeObserverKind = "Execution"
)

type forgeWorldObserver struct {
	ctx    context.Context
	cancel context.CancelFunc
	le     *logrus.Entry
	ws     world.WorldState

	mtx      sync.Mutex
	stopping bool
	watched  map[forgeObserverKind]map[string]struct{}
	wg       sync.WaitGroup
}

func observeForgeWorld(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	jobKey string,
) func() {
	observerCtx, cancel := context.WithCancel(ctx)
	o := &forgeWorldObserver{
		ctx:    observerCtx,
		cancel: cancel,
		le:     le,
		ws:     ws,
		watched: map[forgeObserverKind]map[string]struct{}{
			forgeObserverJob:       {},
			forgeObserverTask:      {},
			forgeObserverPass:      {},
			forgeObserverExecution: {},
		},
	}
	o.watchJob(jobKey)
	return o.stop
}

func (o *forgeWorldObserver) stop() {
	o.mtx.Lock()
	if o.stopping {
		o.mtx.Unlock()
		return
	}
	o.stopping = true
	o.cancel()
	o.mtx.Unlock()
	o.wg.Wait()
}

func (o *forgeWorldObserver) watch(
	kind forgeObserverKind,
	key string,
	handler world_control.WatchLoopHandler,
) {
	if key == "" {
		return
	}

	o.mtx.Lock()
	if o.stopping {
		o.mtx.Unlock()
		return
	}
	if _, ok := o.watched[kind][key]; ok {
		o.mtx.Unlock()
		return
	}
	o.watched[kind][key] = struct{}{}
	o.wg.Add(1)
	o.mtx.Unlock()

	go func() {
		defer o.wg.Done()
		err := world_control.NewWatchLoop(o.le, key, handler).Execute(o.ctx, o.ws)
		if err != nil && o.ctx.Err() == nil {
			o.logError(kind, key, err)
		}
	}()
}

func (o *forgeWorldObserver) watchJob(jobKey string) {
	var lastState forge_job.State
	var seen bool
	o.watch(forgeObserverJob, jobKey, world_control.NewWaitForStateHandler(
		func(ctx context.Context, ws world.WorldState, obj world.ObjectState, rootCs *block.Cursor, _ uint64) (bool, error) {
			if obj == nil {
				return true, nil
			}
			job, err := forge_job.UnmarshalJob(ctx, rootCs)
			if err != nil {
				o.logError(forgeObserverJob, jobKey, err)
				return true, nil
			}
			state := job.GetJobState()
			if !seen || state != lastState {
				o.logState(forgeObserverJob, jobKey, state.String())
				lastState = state
				seen = true
			}

			taskKeys, err := forge_job.ListJobTasks(ctx, ws, jobKey)
			if err != nil {
				o.logError(forgeObserverJob, jobKey, err)
				return true, nil
			}
			for _, taskKey := range taskKeys {
				o.watchTask(taskKey)
			}
			return true, nil
		},
	))
}

func (o *forgeWorldObserver) watchTask(taskKey string) {
	var lastState forge_task.State
	var seen bool
	o.watch(forgeObserverTask, taskKey, world_control.NewWaitForStateHandler(
		func(ctx context.Context, ws world.WorldState, obj world.ObjectState, rootCs *block.Cursor, _ uint64) (bool, error) {
			if obj == nil {
				return true, nil
			}
			task, err := forge_task.UnmarshalTask(ctx, rootCs)
			if err != nil {
				o.logError(forgeObserverTask, taskKey, err)
				return true, nil
			}
			state := task.GetTaskState()
			if !seen || state != lastState {
				o.logState(forgeObserverTask, taskKey, state.String())
				lastState = state
				seen = true
			}

			passKeys, err := forge_task.ListTaskPasses(ctx, ws, taskKey)
			if err != nil {
				o.logError(forgeObserverTask, taskKey, err)
				return true, nil
			}
			for _, passKey := range passKeys {
				o.watchPass(passKey)
			}
			return true, nil
		},
	))
}

func (o *forgeWorldObserver) watchPass(passKey string) {
	var lastState forge_pass.State
	var seen bool
	o.watch(forgeObserverPass, passKey, world_control.NewWaitForStateHandler(
		func(ctx context.Context, ws world.WorldState, obj world.ObjectState, rootCs *block.Cursor, _ uint64) (bool, error) {
			if obj == nil {
				return true, nil
			}
			pass, err := forge_pass.UnmarshalPass(ctx, rootCs)
			if err != nil {
				o.logError(forgeObserverPass, passKey, err)
				return true, nil
			}
			state := pass.GetPassState()
			if !seen || state != lastState {
				o.logState(forgeObserverPass, passKey, state.String())
				lastState = state
				seen = true
			}

			executionKeys, err := forge_pass.ListPassExecutions(ctx, ws, passKey)
			if err != nil {
				o.logError(forgeObserverPass, passKey, err)
				return true, nil
			}
			for _, executionKey := range executionKeys {
				o.watchExecution(executionKey)
			}
			return true, nil
		},
	))
}

func (o *forgeWorldObserver) watchExecution(executionKey string) {
	var lastState forge_execution.State
	var lastClaimID string
	var lastEpoch uint64
	var lastPeerID string
	var seen bool
	o.watch(forgeObserverExecution, executionKey, world_control.NewWaitForStateHandler(
		func(ctx context.Context, _ world.WorldState, obj world.ObjectState, rootCs *block.Cursor, _ uint64) (bool, error) {
			if obj == nil {
				return true, nil
			}
			execution, err := forge_execution.UnmarshalExecution(ctx, rootCs)
			if err != nil {
				o.logError(forgeObserverExecution, executionKey, err)
				return true, nil
			}

			claimID := ""
			epoch := uint64(0)
			if claim := execution.GetClaim(); claim != nil {
				claimID = claim.GetClaimId()
				epoch = claim.GetEpoch()
			}
			state := execution.GetExecutionState()
			peerID := execution.GetPeerId()
			if !seen || state != lastState || claimID != lastClaimID || epoch != lastEpoch || peerID != lastPeerID {
				o.logExecution(executionKey, state.String(), claimID, epoch, peerID)
				lastState = state
				lastClaimID = claimID
				lastEpoch = epoch
				lastPeerID = peerID
				seen = true
			}
			return true, nil
		},
	))
}

func (o *forgeWorldObserver) logState(kind forgeObserverKind, key, state string) {
	o.le.Debugf("[FORGE-OBS] kind=%s key=%q state=%s", kind, key, state)
}

func (o *forgeWorldObserver) logExecution(key, state, claimID string, epoch uint64, peerID string) {
	o.le.Debugf(
		"[FORGE-OBS] kind=%s key=%q state=%s claim_id=%q claim_epoch=%d peer_id=%q",
		forgeObserverExecution,
		key,
		state,
		claimID,
		epoch,
		peerID,
	)
}

func (o *forgeWorldObserver) logError(kind forgeObserverKind, key string, err error) {
	o.le.WithError(err).Errorf("[FORGE-OBS] kind=%s key=%q state=observer-error", kind, key)
}
