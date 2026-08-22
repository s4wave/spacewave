package s4wave_layout_world

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	resource_layout "github.com/s4wave/spacewave/core/resource/layout"
	"github.com/s4wave/spacewave/db/block"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/world"
	s4wave_layout "github.com/s4wave/spacewave/sdk/layout"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// ErrEngineRequired is returned when the layout factory requires an Engine but it is nil.
var ErrEngineRequired = errors.New("engine is required for layout object type")

// ObjectLayoutType is the ObjectType for ObjectLayout objects.
var ObjectLayoutType = objecttype.NewObjectType(ObjectLayoutTypeID, ObjectLayoutFactory)

// ObjectLayoutFactory creates a LayoutResource from an ObjectLayout world object.
//
// objectKey is the key of the layout object.
// ws is the WorldState for reading initial state (required).
// engine is the Engine for creating write transactions (required for setLayout).
func ObjectLayoutFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	if engine == nil {
		return nil, nil, ErrEngineRequired
	}
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}

	objState, found, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, nil, world.ErrObjectNotFound
	}

	var layout *ObjectLayout
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var err error
		layout, err = block.UnmarshalBlock[*ObjectLayout](ctx, bcs, NewObjectLayoutBlock)
		return err
	})
	if err != nil {
		return nil, nil, err
	}

	// stateCtr holds the observed layout model; the watch below republishes
	// it after each World revision so remote edits are visible locally.
	stateCtr := ccontainer.NewCContainerVT(layout.GetLayoutModel())

	watch := routine.NewRoutineContainer()
	watch.SetRoutine(func(ctx context.Context) error {
		for {
			if err := ctx.Err(); err != nil {
				return err
			}

			seqno, err := ws.GetSeqno(ctx)
			if err != nil {
				return err
			}

			objState, found, err := ws.GetObject(ctx, objectKey)
			if err != nil {
				return err
			}
			if !found {
				return world.ErrObjectNotFound
			}
			err = func() error {
				// Release the object-state handle before the WaitSeqno block
				// below so the read scope does not span the wait.
				defer world.ReleaseObjectState(objState)
				var cur *ObjectLayout
				_, _, aerr := world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
					var uerr error
					cur, uerr = block.UnmarshalBlock[*ObjectLayout](ctx, bcs, NewObjectLayoutBlock)
					return uerr
				})
				if aerr != nil {
					return aerr
				}
				stateCtr.SetValue(cur.GetLayoutModel())
				return nil
			}()
			if err != nil {
				return err
			}

			if _, err := ws.WaitSeqno(ctx, seqno+1); err != nil {
				return err
			}
		}
	})
	watch.SetContext(context.Background(), false)

	// setLayout updates the layout model in the world using a write transaction
	setLayout := func(ctx context.Context, model *s4wave_layout.LayoutModel) error {
		ctx, task := trace.NewTask(ctx, "alpha/layout/set-layout")
		defer task.End()

		var wtx world.Tx
		{
			taskCtx, task := trace.NewTask(ctx, "alpha/layout/set-layout/new-transaction")
			var err error
			wtx, err = engine.NewTransaction(taskCtx, true)
			task.End()
			if err != nil {
				return err
			}
		}

		var writeState world.ObjectState
		var found bool
		{
			taskCtx, task := trace.NewTask(ctx, "alpha/layout/set-layout/get-object")
			var err error
			writeState, found, err = wtx.GetObject(taskCtx, objectKey)
			task.End()
			if err != nil {
				wtx.Discard()
				return err
			}
		}
		if !found {
			wtx.Discard()
			return world.ErrObjectNotFound
		}

		{
			taskCtx, task := trace.NewTask(ctx, "alpha/layout/set-layout/mutate-object")
			_, _, err := world.AccessObjectState(taskCtx, writeState, true, func(bcs *block.Cursor) error {
				cur, uerr := block.UnmarshalBlock[*ObjectLayout](ctx, bcs, NewObjectLayoutBlock)
				if uerr != nil {
					return uerr
				}
				if cur == nil {
					cur = &ObjectLayout{}
				}
				cur.LayoutModel = model.CloneVT()
				bcs.SetBlock(cur, true)
				return nil
			})
			task.End()
			if err != nil {
				wtx.Discard()
				return err
			}
		}

		{
			taskCtx, task := trace.NewTask(ctx, "alpha/layout/set-layout/commit")
			err := wtx.Commit(taskCtx)
			task.End()
			if err != nil {
				return err
			}
		}

		{
			_, task := trace.NewTask(ctx, "alpha/layout/set-layout/publish-local-state")
			stateCtr.SetValue(model.CloneVT())
			task.End()
		}
		return nil
	}

	// navigateTab updates the path field of a tab in the layout
	navigateTab := func(ctx context.Context, req *s4wave_layout.NavigateTabRequest) (*s4wave_layout.NavigateTabResponse, error) {
		ctx, task := trace.NewTask(ctx, "alpha/layout/navigate-tab")
		defer task.End()

		tabID := req.GetTabId()
		newPath := req.GetPath()
		if tabID == "" {
			return &s4wave_layout.NavigateTabResponse{}, nil
		}

		currentModel := stateCtr.GetValue()
		if currentModel == nil {
			return &s4wave_layout.NavigateTabResponse{}, nil
		}

		updatedModel := currentModel.CloneVT()

		var tabFound bool
		var walkErr error
		{
			_, task := trace.NewTask(ctx, "alpha/layout/navigate-tab/update-model")
			resource_layout.WalkLayoutModel(updatedModel, func(node any) bool {
				tabDef, ok := node.(*s4wave_layout.TabDef)
				if !ok {
					return true
				}
				if tabDef.GetId() != tabID {
					return true
				}

				var tabData ObjectLayoutTab
				if len(tabDef.GetData()) > 0 {
					if err := tabData.UnmarshalVT(tabDef.GetData()); err != nil {
						walkErr = err
						return false
					}
				}

				// CleanupPath resolves newPath relative to the tab's current path.
				currentPath := tabData.GetPath()
				tabData.Path = resource_layout.CleanupPath(currentPath, newPath)

				data, err := tabData.MarshalVT()
				if err != nil {
					walkErr = err
					return false
				}
				tabDef.Data = data
				tabFound = true
				return false
			})
			task.End()
		}
		if walkErr != nil {
			return nil, walkErr
		}

		if !tabFound {
			return &s4wave_layout.NavigateTabResponse{}, nil
		}

		if err := setLayout(ctx, updatedModel); err != nil {
			return nil, err
		}

		return &s4wave_layout.NavigateTabResponse{}, nil
	}

	// replaceTab updates the durable payload fields of one tab in the layout.
	replaceTab := func(ctx context.Context, req *s4wave_layout.ReplaceTabRequest) (*s4wave_layout.ReplaceTabResponse, error) {
		ctx, task := trace.NewTask(ctx, "alpha/layout/replace-tab")
		defer task.End()

		tabID := req.GetTabId()
		replacement := req.GetTab()
		if tabID == "" || replacement == nil {
			return &s4wave_layout.ReplaceTabResponse{}, nil
		}

		currentModel := stateCtr.GetValue()
		if currentModel == nil {
			return &s4wave_layout.ReplaceTabResponse{}, nil
		}

		updatedModel := currentModel.CloneVT()
		var replaced bool
		{
			_, task := trace.NewTask(ctx, "alpha/layout/replace-tab/update-model")
			replaced = resource_layout.ReplaceLayoutModelTab(updatedModel, tabID, replacement)
			task.End()
		}
		if !replaced {
			return &s4wave_layout.ReplaceTabResponse{}, nil
		}

		if err := setLayout(ctx, updatedModel); err != nil {
			return nil, err
		}

		return &s4wave_layout.ReplaceTabResponse{}, nil
	}

	layoutResource := resource_layout.NewLayoutResource(stateCtr, setLayout, navigateTab)
	layoutResource.SetReplaceTabFunc(replaceTab)

	return layoutResource.GetMux(), func() {
		watch.ClearContext()
	}, nil
}
