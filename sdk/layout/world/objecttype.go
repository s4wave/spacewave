package s4wave_layout_world

import (
	"context"
	"errors"
	"sync"

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
	// Require both the write engine and the observed World state.
	if engine == nil {
		return nil, nil, ErrEngineRequired
	}
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}

	// Read the layout and the revision it was read at.
	seqno, err := ws.GetSeqno(ctx)
	if err != nil {
		return nil, nil, err
	}
	layout, err := readLayout(ctx, ws, objectKey)
	if err != nil {
		return nil, nil, err
	}

	// stateCtr holds the observed layout model. The watch below republishes
	// it after each later World revision so remote edits are visible, and
	// local writes publish their committed model at once. pubMtx orders the
	// two publishers: writes counts local publications, and the watch drops
	// a read that a local write overtook, so the model never moves back to an
	// older revision.
	stateCtr := ccontainer.NewCContainerVT(layout.GetLayoutModel())
	var pubMtx sync.Mutex
	var writes uint64

	// Follow later World revisions until the resource is released.
	watch := routine.NewRoutineContainer()
	watch.SetRoutine(func(ctx context.Context) error {
		seqno := seqno
		for {
			// Wait for the next World revision.
			if _, err := ws.WaitSeqno(ctx, seqno+1); err != nil {
				return err
			}

			// Read the layout at the latest revision.
			pubMtx.Lock()
			readWrites := writes
			pubMtx.Unlock()
			var err error
			seqno, err = ws.GetSeqno(ctx)
			if err != nil {
				return err
			}
			cur, err := readLayout(ctx, ws, objectKey)
			if err != nil {
				return err
			}

			// Publish unless a local write published a newer model meanwhile.
			pubMtx.Lock()
			if writes == readWrites {
				stateCtr.SetValue(cur.GetLayoutModel())
			}
			pubMtx.Unlock()
		}
	})
	watch.SetContext(context.Background(), false)

	// updateLayout applies update to the stored layout in one write
	// transaction, so each change starts from the committed model. update
	// reports whether it changed the layout; an unchanged layout discards the
	// transaction.
	updateLayout := func(ctx context.Context, update func(layout *ObjectLayout) (bool, error)) error {
		// Open the write transaction and the stored layout object.
		wtx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		defer wtx.Discard()
		writeState, found, err := wtx.GetObject(ctx, objectKey)
		defer world.ReleaseObjectState(writeState)
		if err != nil {
			return err
		}
		if !found {
			return world.ErrObjectNotFound
		}

		// Apply the update to the stored layout and keep its new model.
		var changed bool
		var model *s4wave_layout.LayoutModel
		_, _, err = world.AccessObjectState(ctx, writeState, true, func(bcs *block.Cursor) error {
			// Decode the stored layout, starting from an empty one.
			cur, err := block.UnmarshalBlock[*ObjectLayout](ctx, bcs, NewObjectLayoutBlock)
			if err != nil {
				return err
			}
			if cur == nil {
				cur = &ObjectLayout{}
			}

			// Write the changed layout back to the object.
			changed, err = update(cur)
			if err != nil || !changed {
				return err
			}
			model = cur.GetLayoutModel().CloneVT()
			bcs.SetBlock(cur, true)
			return nil
		})
		if err != nil || !changed {
			return err
		}

		// Commit and publish under pubMtx so concurrent writes publish in
		// commit order.
		pubMtx.Lock()
		defer pubMtx.Unlock()
		if err := wtx.Commit(ctx); err != nil {
			return err
		}
		writes++
		stateCtr.SetValue(model)
		return nil
	}

	// setLayout replaces the stored layout model.
	setLayout := func(ctx context.Context, model *s4wave_layout.LayoutModel) error {
		// Trace the replacement and store the new model.
		ctx, task := trace.NewTask(ctx, "alpha/layout/set-layout")
		defer task.End()
		return updateLayout(ctx, func(layout *ObjectLayout) (bool, error) {
			layout.LayoutModel = model.CloneVT()
			return true, nil
		})
	}

	// navigateTab resolves a path against one tab's current path.
	navigateTab := func(ctx context.Context, req *s4wave_layout.NavigateTabRequest) (*s4wave_layout.NavigateTabResponse, error) {
		// Trace the navigation and ignore a request without a tab.
		ctx, task := trace.NewTask(ctx, "alpha/layout/navigate-tab")
		defer task.End()
		tabID := req.GetTabId()
		if tabID == "" {
			return &s4wave_layout.NavigateTabResponse{}, nil
		}

		// Resolve the path against the committed tab.
		err := updateLayout(ctx, func(layout *ObjectLayout) (bool, error) {
			return navigateLayoutTab(layout.GetLayoutModel(), tabID, req.GetPath())
		})
		if err != nil {
			return nil, err
		}
		return &s4wave_layout.NavigateTabResponse{}, nil
	}

	// replaceTab updates the durable payload fields of one tab in the layout.
	replaceTab := func(ctx context.Context, req *s4wave_layout.ReplaceTabRequest) (*s4wave_layout.ReplaceTabResponse, error) {
		// Trace the replacement and ignore an incomplete request.
		ctx, task := trace.NewTask(ctx, "alpha/layout/replace-tab")
		defer task.End()
		tabID := req.GetTabId()
		replacement := req.GetTab()
		if tabID == "" || replacement == nil {
			return &s4wave_layout.ReplaceTabResponse{}, nil
		}

		// Replace the tab in the committed layout.
		err := updateLayout(ctx, func(layout *ObjectLayout) (bool, error) {
			return resource_layout.ReplaceLayoutModelTab(layout.GetLayoutModel(), tabID, replacement), nil
		})
		if err != nil {
			return nil, err
		}
		return &s4wave_layout.ReplaceTabResponse{}, nil
	}

	// Serve the layout and stop the watch on release.
	layoutResource := resource_layout.NewLayoutResource(stateCtr, setLayout, navigateTab)
	layoutResource.SetReplaceTabFunc(replaceTab)

	return layoutResource.GetMux(), func() {
		watch.ClearContext()
	}, nil
}

// navigateLayoutTab resolves newPath against the current path of the tab with
// tabID in model and stores the result in the tab. It reports whether the tab
// was found.
func navigateLayoutTab(model *s4wave_layout.LayoutModel, tabID, newPath string) (bool, error) {
	// Find the tab and rewrite its path.
	var found bool
	var walkErr error
	resource_layout.WalkLayoutModel(model, func(node any) bool {
		// Skip every node except the requested tab.
		tabDef, ok := node.(*s4wave_layout.TabDef)
		if !ok || tabDef.GetId() != tabID {
			return true
		}

		// Decode the tab's current path.
		var tabData ObjectLayoutTab
		if err := tabData.UnmarshalVT(tabDef.GetData()); err != nil {
			walkErr = err
			return false
		}

		// Store the resolved path back into the tab.
		tabData.Path = resource_layout.CleanupPath(tabData.GetPath(), newPath)
		data, err := tabData.MarshalVT()
		if err != nil {
			walkErr = err
			return false
		}
		tabDef.Data = data
		found = true
		return false
	})

	// Report a decode or encode failure before the found result.
	if walkErr != nil {
		return false, walkErr
	}
	return found, nil
}

// readLayout reads the layout object at objectKey from ws.
func readLayout(ctx context.Context, ws world.WorldState, objectKey string) (*ObjectLayout, error) {
	// Look up the layout object.
	objState, found, err := ws.GetObject(ctx, objectKey)
	defer world.ReleaseObjectState(objState)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, world.ErrObjectNotFound
	}

	// Decode its stored layout.
	var layout *ObjectLayout
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var err error
		layout, err = block.UnmarshalBlock[*ObjectLayout](ctx, bcs, NewObjectLayoutBlock)
		return err
	})
	return layout, err
}
