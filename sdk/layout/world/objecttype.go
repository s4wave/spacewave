package s4wave_layout_world

import (
	"context"
	"errors"
	"runtime/trace"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	resource_layout "github.com/s4wave/spacewave/core/resource/layout"
	"github.com/s4wave/spacewave/db/block"
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
	// Engine is required for write operations
	if engine == nil {
		return nil, nil, ErrEngineRequired
	}

	// WorldState is required for reading initial state
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}

	// Get the object from the world state
	objState, found, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, nil, world.ErrObjectNotFound
	}

	// Read the ObjectLayout from the object body
	var layout *ObjectLayout
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var err error
		layout, err = block.UnmarshalBlock[*ObjectLayout](ctx, bcs, NewObjectLayoutBlock)
		return err
	})
	if err != nil {
		return nil, nil, err
	}

	// Create a watchable container for the layout model
	stateCtr := ccontainer.NewCContainer(layout.GetLayoutModel())

	// setLayout updates the layout model in the world using a write transaction
	setLayout := func(ctx context.Context, model *s4wave_layout.LayoutModel) error {
		ctx, task := trace.NewTask(ctx, "alpha/layout/set-layout")
		defer task.End()

		// Create a write transaction
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

		// Get the object from the write transaction
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

		// Update via AccessObjectState
		{
			taskCtx, task := trace.NewTask(ctx, "alpha/layout/set-layout/mutate-object")
			_, _, err := world.AccessObjectState(taskCtx, writeState, true, func(bcs *block.Cursor) error {
				newLayout := layout.Clone()
				newLayout.LayoutModel = model.CloneVT()
				bcs.SetBlock(newLayout, true)
				return nil
			})
			task.End()
			if err != nil {
				wtx.Discard()
				return err
			}
		}

		// Commit the transaction
		{
			taskCtx, task := trace.NewTask(ctx, "alpha/layout/set-layout/commit")
			err := wtx.Commit(taskCtx)
			task.End()
			if err != nil {
				return err
			}
		}

		// Update the local state container
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
		logrus.Infof("navigateTab called: tabID=%s newPath=%s", tabID, newPath)
		if tabID == "" {
			return &s4wave_layout.NavigateTabResponse{}, nil
		}

		// Get current model
		currentModel := stateCtr.GetValue()
		if currentModel == nil {
			logrus.Info("navigateTab: no current model")
			return &s4wave_layout.NavigateTabResponse{}, nil
		}

		// Clone the model for modification
		updatedModel := currentModel.CloneVT()

		// Find the tab by ID and update its path
		var tabFound bool
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

				// Found the tab - unmarshal its data
				var tabData ObjectLayoutTab
				if len(tabDef.GetData()) > 0 {
					if err := tabData.UnmarshalVT(tabDef.GetData()); err != nil {
						logrus.Infof("navigateTab: unmarshal error: %v", err)
						return true
					}
				}

				// Update the path using CleanupPath to handle relative paths
				currentPath := tabData.GetPath()
				tabData.Path = resource_layout.CleanupPath(currentPath, newPath)
				logrus.Infof("navigateTab: tabID=%s currentPath=%s newPath=%s resolvedPath=%s", tabID, currentPath, newPath, tabData.Path)

				// Marshal back to data
				data, err := tabData.MarshalVT()
				if err != nil {
					logrus.Infof("navigateTab: marshal error: %v", err)
					return true
				}
				tabDef.Data = data
				tabFound = true
				return false
			})
			task.End()
		}

		if !tabFound {
			return &s4wave_layout.NavigateTabResponse{}, nil
		}

		// Use setLayout to persist the change
		if err := setLayout(ctx, updatedModel); err != nil {
			return nil, err
		}

		return &s4wave_layout.NavigateTabResponse{}, nil
	}

	// Create the LayoutResource
	layoutResource := resource_layout.NewLayoutResource(stateCtr, setLayout, navigateTab)

	return layoutResource.GetMux(), func() {}, nil
}
