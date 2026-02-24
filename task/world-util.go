package forge_task

import (
	"context"

	"github.com/aperturerobotics/cayley"
	forge_pass "github.com/aperturerobotics/forge/pass"
	forge_target "github.com/aperturerobotics/forge/target"
	"github.com/aperturerobotics/hydra/world"
	world_parent "github.com/aperturerobotics/hydra/world/parent"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/pkg/errors"
)

// CheckTaskType checks the type graph quad for a Task.
func CheckTaskType(ctx context.Context, ws world.WorldState, objKey string) error {
	return world_types.CheckObjectType(ctx, ws, objKey, TaskTypeID)
}

// LookupTask looks up a task in the world.
func LookupTask(ctx context.Context, ws world.WorldState, objKey string) (*Task, world.ObjectState, error) {
	return world.LookupObject[*Task](ctx, ws, objKey, NewTaskBlock)
}

// ListTaskPasses lists all Pass object keys that are linked to by the Task.
func ListTaskPasses(ctx context.Context, w world.WorldState, taskKeys ...string) ([]string, error) {
	return world.CollectPathWithKeys(
		ctx,
		w,
		taskKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			return p.Out(PredTaskToPass), nil
		},
	)
}

// CollectTaskPasses collects all active Pass linked to by the Task.
// If any of the linked states are invalid, returns an error.
func CollectTaskPasses(
	ctx context.Context,
	ws world.WorldState,
	taskKeys ...string,
) ([]*forge_pass.Pass, []*forge_target.Target, []string, error) {
	kpObjectKeys, err := ListTaskPasses(ctx, ws, taskKeys...)
	if err != nil {
		return nil, nil, nil, err
	}

	states := make([]*forge_pass.Pass, len(kpObjectKeys))
	tgts := make([]*forge_target.Target, len(kpObjectKeys))
	for i, objKey := range kpObjectKeys {
		states[i], tgts[i], err = forge_pass.LookupPass(ctx, ws, objKey)
		if err == nil {
			err = states[i].Validate(false)
		}
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "passes[%s]", objKey)
		}
	}

	return states, tgts, kpObjectKeys, nil
}

// LookupTaskPass looks up the task pass with the given nonce.
// Queries via the <value> field, which must be set correctly.
// If not found, returns nil, "", nil
// If nonce = 0, looks up any pass associated with the task.
func LookupTaskPass(
	ctx context.Context,
	ws world.WorldState,
	taskKey string,
	nonce uint64,
) (*forge_pass.Pass, *forge_target.Target, string, error) {
	gqs, err := ws.LookupGraphQuads(ctx, NewTaskToPassQuad(taskKey, "", nonce), 1)
	if err != nil {
		return nil, nil, "", err
	}

	if len(gqs) == 0 {
		return nil, nil, "", nil
	}

	gq := gqs[0]
	passKey, err := world.GraphValueToKey(gq.GetObj())
	if err != nil {
		return nil, nil, "", err
	}

	pass, tgt, err := forge_pass.LookupPass(ctx, ws, passKey)
	if err != nil {
		return nil, nil, passKey, err
	}
	return pass, tgt, passKey, nil
}

// CheckTaskHasPass checks if the Task is linked to a Pass.
func CheckTaskHasPass(ctx context.Context, w world.WorldState, taskKey, passKey string) (bool, error) {
	gq, err := w.LookupGraphQuads(ctx, world.NewGraphQuad(
		world.KeyToGraphValue(taskKey).String(),
		PredTaskToPass.String(),
		world.KeyToGraphValue(passKey).String(),
		"",
	), 1)
	if err != nil {
		return false, err
	}
	return len(gq) != 0, nil
}

// EnsureTaskHasPass checks if the Task has the Pass and returns an error otherwise.
func EnsureTaskHasPass(ctx context.Context, w world.WorldState, taskKey, passKey string) error {
	hasPass, err := CheckTaskHasPass(ctx, w, taskKey, passKey)
	if err == nil && !hasPass {
		err = errors.Errorf("task %s does not have pass %s", taskKey, passKey)
	}
	return err
}

// ListTaskTargets lists all Target object keys that are linked to by the Tasks.
// note: we only expect 1 target to be linked to each at any given time.
func ListTaskTargets(ctx context.Context, w world.WorldState, taskKeys ...string) ([]string, error) {
	return world.CollectPathWithKeys(
		ctx,
		w,
		taskKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			return p.Out(PredTaskToTarget), nil
		},
	)
}

// CollectTaskTargets collects all active Target linked to by the Tasks.
// If any of the linked states are invalid, returns an error.
func CollectTaskTargets(
	ctx context.Context,
	ws world.WorldState,
	taskKeys ...string,
) ([]*forge_target.Target, []string, error) {
	kpObjectKeys, err := ListTaskTargets(ctx, ws, taskKeys...)
	if err != nil {
		return nil, nil, err
	}

	states := make([]*forge_target.Target, len(kpObjectKeys))
	for i, objKey := range kpObjectKeys {
		states[i], err = forge_target.LookupTarget(ctx, ws, objKey)
		if err == nil {
			err = states[i].Validate()
		}
		if err != nil {
			return nil, nil, errors.Wrapf(err, "targets[%s]", objKey)
		}
	}

	return states, kpObjectKeys, nil
}

// LookupTaskTarget looks up a single Target for a given Task.
// Returns nil, nil if no Target is resolved.
// Returns an error if more than one Target is resolved.
func LookupTaskTarget(
	ctx context.Context,
	ws world.WorldState,
	taskKey string,
) (*forge_target.Target, string, error) {
	tgts, tgtKeys, err := CollectTaskTargets(ctx, ws, taskKey)
	if err != nil || len(tgts) == 0 {
		return nil, "", err
	}
	if len(tgtKeys) != 1 {
		return tgts[0], tgtKeys[0], errors.Errorf(
			"task[%s]: expected single target but found %d",
			taskKey, len(tgtKeys),
		)
	}
	return tgts[0], tgtKeys[0], nil
}

// FindPassWithNonce searches for the Pass with the given nonce in a set.
// returns nil, -1 if not found
func FindPassWithNonce(passNonce uint64, passes []*forge_pass.Pass) (*forge_pass.Pass, int) {
	for i, pass := range passes {
		if pass.GetPassNonce() == passNonce {
			return pass, i
		}
	}
	return nil, -1
}

// ListTaskSubtasks lists all subtask object keys for a parent Task.
func ListTaskSubtasks(ctx context.Context, w world.WorldState, taskKeys ...string) ([]string, error) {
	return world.CollectPathWithKeys(
		ctx,
		w,
		taskKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			return p.Out(PredTaskToSubtask), nil
		},
	)
}

// CollectTaskSubtasks collects all subtask objects for a parent Task.
// If any of the linked tasks are invalid, returns an error.
func CollectTaskSubtasks(
	ctx context.Context,
	ws world.WorldState,
	taskKeys ...string,
) ([]*Task, []string, error) {
	objKeys, err := ListTaskSubtasks(ctx, ws, taskKeys...)
	if err != nil {
		return nil, nil, err
	}

	tasks := make([]*Task, len(objKeys))
	for i, objKey := range objKeys {
		tasks[i], _, err = LookupTask(ctx, ws, objKey)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "subtasks[%s]", objKey)
		}
	}

	return tasks, objKeys, nil
}

// LinkTaskSubtask creates the graph links between parent and child Tasks.
// Sets both forge/task-subtask (parent->child) and hydra/world/parent (child->parent).
func LinkTaskSubtask(ctx context.Context, ws world.WorldState, parentTaskKey, childTaskKey string) error {
	err := world_parent.SetObjectParent(ctx, ws, childTaskKey, parentTaskKey, false)
	if err != nil {
		return err
	}
	return ws.SetGraphQuad(ctx, NewTaskToSubtaskQuad(parentTaskKey, childTaskKey))
}

// LinkTaskCached creates a graph link from a Task to a previous Task whose
// result is inherited.
func LinkTaskCached(ctx context.Context, ws world.WorldState, taskKey, cachedTaskKey string) error {
	return ws.SetGraphQuad(ctx, NewTaskToCachedQuad(taskKey, cachedTaskKey))
}

// LookupTaskCached looks up the cached task linked to a given task.
// Returns "", nil if no cached task is linked.
func LookupTaskCached(ctx context.Context, ws world.WorldState, taskKey string) (string, error) {
	gqs, err := ws.LookupGraphQuads(ctx, world.NewGraphQuad(
		world.KeyToGraphValue(taskKey).String(),
		PredTaskToCached.String(),
		"",
		"",
	), 1)
	if err != nil || len(gqs) == 0 {
		return "", err
	}
	return world.GraphValueToKey(gqs[0].GetObj())
}
