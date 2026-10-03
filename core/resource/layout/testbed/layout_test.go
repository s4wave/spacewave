//go:build !js

package layout_testbed_test

import (
	"context"
	"testing"

	layout_testbed "github.com/s4wave/spacewave/core/resource/layout/testbed"
	s4wave_layout "github.com/s4wave/spacewave/sdk/layout"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	s4wave_web_object "github.com/s4wave/spacewave/web/object"
)

type layoutTabState struct {
	tabID       string
	name        string
	enableClose bool
	objectKey   string
	objectType  string
	componentID string
	path        string
}

func getMainFilesTabState(t *testing.T, model *s4wave_layout.LayoutModel) layoutTabState {
	// Attribute layout state helper failures to the calling test.
	t.Helper()

	// Require the seeded Files tab in the main layout tabset.
	if model == nil {
		t.Fatal("expected layout model")
	}
	firstChild := model.GetLayout().GetChildren()[0]
	tabSet := firstChild.GetTabSet()
	if tabSet == nil {
		t.Fatal("expected main tabset")
	}
	tab := tabSet.GetChildren()[0]
	if tab == nil {
		t.Fatal("expected files tab")
	}

	// Decode the Files tab target and navigation data.
	var tabData s4wave_layout_world.ObjectLayoutTab
	if err := tabData.UnmarshalVT(tab.GetData()); err != nil {
		t.Fatalf("unmarshal tab data failed: %v", err)
	}

	// Require a stable Files tab ID before returning its state.
	tabID := tab.GetId()
	if tabID == "" {
		t.Fatal("expected files tab id")
	}
	return layoutTabState{
		tabID:       tabID,
		name:        tab.GetName(),
		enableClose: tab.GetEnableClose(),
		objectKey:   tabData.GetObjectInfo().GetWorldObjectInfo().GetObjectKey(),
		objectType:  tabData.GetObjectInfo().GetWorldObjectInfo().GetObjectType(),
		componentID: tabData.GetComponentId(),
		path:        tabData.GetPath(),
	}
}

func openLayoutClient(t *testing.T, tb *layout_testbed.Testbed, resourceID uint32) s4wave_layout.SRPCLayoutHostClient {
	// Attribute layout client helper failures to the calling test.
	t.Helper()

	// Retain the layout resource for the calling test lifetime.
	layoutRef := tb.ResClient.CreateResourceReference(resourceID)
	t.Cleanup(layoutRef.Release)

	// Open the retained layout resource RPC client.
	layoutSrpcClient, err := layoutRef.GetClient()
	if err != nil {
		t.Fatalf("GetClient failed: %v", err)
	}
	return s4wave_layout.NewSRPCLayoutHostClient(layoutSrpcClient)
}

func objectLayoutTabData(t *testing.T, objectKey, objectType, path, componentID string) []byte {
	// Attribute tab encoding helper failures to the calling test.
	t.Helper()

	// Encode the object target, path, and component for a layout tab.
	tabData := &s4wave_layout_world.ObjectLayoutTab{
		ComponentId: componentID,
		ObjectInfo: &s4wave_web_object.ObjectInfo{
			Info: &s4wave_web_object.ObjectInfo_WorldObjectInfo{
				WorldObjectInfo: &s4wave_web_object.WorldObjectInfo{
					ObjectKey:  objectKey,
					ObjectType: objectType,
				},
			},
		},
		Path: path,
	}
	data, err := tabData.MarshalVT()
	if err != nil {
		t.Fatalf("marshal tab data failed: %v", err)
	}
	return data
}

// TestLayoutResource tests the LayoutResource functionality.
func TestLayoutResource(t *testing.T) {
	// Share a context across the layout resource scenarios.
	ctx := context.Background()

	// Construct a layout testbed for the resource RPC scenarios.
	tb, err := layout_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Verify the initial model supplied by the layout watch.
	t.Run("WatchLayoutModel", func(t *testing.T) {
		// Create a retained layout engine with its seeded Files tab.
		objectKey := "object-layout/test-watch-" + t.Name()
		setup, err := tb.SetupLayoutEngine(ctx, objectKey)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer setup.Release()

		// Create a reference to the layout resource
		layoutRef := tb.ResClient.CreateResourceReference(setup.LayoutResourceID)
		defer layoutRef.Release()

		// Get the SRPC client for the layout resource
		layoutSrpcClient, err := layoutRef.GetClient()
		if err != nil {
			t.Fatalf("GetClient failed: %v", err)
		}

		// Create a client for the layout host service
		layoutClient := s4wave_layout.NewSRPCLayoutHostClient(layoutSrpcClient)

		// Start watching the layout model
		strm, err := layoutClient.WatchLayoutModel(ctx)
		if err != nil {
			t.Fatalf("WatchLayoutModel failed: %v", err)
		}

		// Receive the initial layout model
		model, err := strm.Recv()
		if err != nil {
			t.Fatalf("Recv failed: %v", err)
		}

		// Verify the model structure matches the demo layout
		if model.GetLayout() == nil {
			t.Fatal("expected layout to be non-nil")
		}
		if model.GetLayout().GetId() != "root" {
			t.Fatalf("expected root row id 'root', got %q", model.GetLayout().GetId())
		}

		// Require the seeded layout to contain one main tabset.
		children := model.GetLayout().GetChildren()
		if len(children) != 1 {
			t.Fatalf("expected 1 child (main tabset), got %d", len(children))
		}

		// Verify main tabset
		mainTabSet := children[0].GetTabSet()
		if mainTabSet == nil {
			t.Fatal("expected main tabset")
		}
		if mainTabSet.GetId() != "main-tabset" {
			t.Fatalf("expected main-tabset id, got %q", mainTabSet.GetId())
		}
		if len(mainTabSet.GetChildren()) != 1 {
			t.Fatalf("expected 1 tab in main tabset, got %d", len(mainTabSet.GetChildren()))
		}
		if mainTabSet.GetChildren()[0].GetName() != "Files" {
			t.Fatalf("expected 'Files' tab name, got %q", mainTabSet.GetChildren()[0].GetName())
		}

		// Report the tab count observed in the initial layout model.
		t.Logf("Successfully received initial layout model with main tabset containing %d tabs", len(mainTabSet.GetChildren()))
	})

	// Verify typed object access through the engine returns the seeded layout.
	t.Run("EngineAccessTypedObjectSeedModel", func(t *testing.T) {
		// Create a retained layout engine for typed object access.
		objectKey := "object-layout/test-engine-access-" + t.Name()
		setup, err := tb.SetupLayoutEngine(ctx, objectKey)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer setup.Release()

		// Access the layout resource through the engine typed object service.
		engineClient, err := setup.Engine.GetResourceRef().GetClient()
		if err != nil {
			t.Fatalf("GetClient(engine) failed: %v", err)
		}
		typedSvcClient := s4wave_world.NewSRPCTypedObjectResourceServiceClient(engineClient)
		resp, err := typedSvcClient.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{
			ObjectKey: objectKey,
		})
		if err != nil {
			t.Fatalf("AccessTypedObject via engine failed: %v", err)
		}

		// Require the typed object response to identify a retained layout resource.
		if resp.GetTypeId() != s4wave_layout_world.ObjectLayoutTypeID {
			t.Fatalf("expected type %q, got %q", s4wave_layout_world.ObjectLayoutTypeID, resp.GetTypeId())
		}
		if resp.GetResourceId() == 0 {
			t.Fatal("expected non-zero layout resource ID")
		}

		// Watch the layout resource returned by engine typed object access.
		layoutClient := openLayoutClient(t, tb, resp.GetResourceId())
		strm, err := layoutClient.WatchLayoutModel(ctx)
		if err != nil {
			t.Fatalf("WatchLayoutModel via engine typed object failed: %v", err)
		}
		model, err := strm.Recv()
		if err != nil {
			t.Fatalf("Recv engine typed object model failed: %v", err)
		}

		// Require the engine layout to preserve the seeded Files tab target.
		filesTab := getMainFilesTabState(t, model)
		if filesTab.tabID != "files" {
			t.Fatalf("expected seeded files tab id, got %q", filesTab.tabID)
		}
		if filesTab.name != "Files" {
			t.Fatalf("expected seeded files tab name, got %q", filesTab.name)
		}
		if filesTab.objectKey != "files" {
			t.Fatalf("expected seeded tab object key files, got %q", filesTab.objectKey)
		}
	})

	// Verify a replacement layout model reaches the watch stream.
	t.Run("SetModel", func(t *testing.T) {
		// Create a retained layout engine for model replacement.
		objectKey := "object-layout/test-setmodel-" + t.Name()
		setup, err := tb.SetupLayoutEngine(ctx, objectKey)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer setup.Release()

		// Create a reference to the layout resource
		layoutRef := tb.ResClient.CreateResourceReference(setup.LayoutResourceID)
		defer layoutRef.Release()

		// Get the SRPC client for the layout resource
		layoutSrpcClient, err := layoutRef.GetClient()
		if err != nil {
			t.Fatalf("GetClient failed: %v", err)
		}

		// Create a client for the layout host service
		layoutClient := s4wave_layout.NewSRPCLayoutHostClient(layoutSrpcClient)

		// Start watching the layout model
		strm, err := layoutClient.WatchLayoutModel(ctx)
		if err != nil {
			t.Fatalf("WatchLayoutModel failed: %v", err)
		}

		// Receive the initial layout model
		initialModel, err := strm.Recv()
		if err != nil {
			t.Fatalf("Recv initial model failed: %v", err)
		}
		t.Logf("Received initial model with root id: %s", initialModel.GetLayout().GetId())

		// Create an updated model with a new tab in main tabset
		updatedModel := initialModel.CloneVT()
		mainTabSet := updatedModel.GetLayout().GetChildren()[0].GetTabSet()
		mainTabSet.Children = append(mainTabSet.Children, &s4wave_layout.TabDef{
			Id:   "new-tab",
			Name: "New Tab",
		})

		// Send the updated model
		err = strm.Send(&s4wave_layout.WatchLayoutModelRequest{
			Body: &s4wave_layout.WatchLayoutModelRequest_SetModel{
				SetModel: updatedModel,
			},
		})
		if err != nil {
			t.Fatalf("Send SetModel failed: %v", err)
		}

		// Receive the updated model from the stream
		receivedModel, err := strm.Recv()
		if err != nil {
			t.Fatalf("Recv updated model failed: %v", err)
		}

		// Verify the update was applied
		updatedMainTabSet := receivedModel.GetLayout().GetChildren()[0].GetTabSet()
		if len(updatedMainTabSet.GetChildren()) != 2 {
			t.Fatalf("expected 2 tabs in main tabset after update, got %d", len(updatedMainTabSet.GetChildren()))
		}
		if updatedMainTabSet.GetChildren()[1].GetId() != "new-tab" {
			t.Fatalf("expected new tab id 'new-tab', got %q", updatedMainTabSet.GetChildren()[1].GetId())
		}

		// Report the tab count observed after layout model replacement.
		t.Logf("Successfully updated layout model, main tabset now has %d tabs", len(updatedMainTabSet.GetChildren()))
	})

	// Verify navigation accepts a request for the default layout.
	t.Run("NavigateTab", func(t *testing.T) {
		// Create a retained layout engine for the navigation request.
		objectKey := "object-layout/test-navigate-" + t.Name()
		setup, err := tb.SetupLayoutEngine(ctx, objectKey)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer setup.Release()

		// Create a reference to the layout resource
		layoutRef := tb.ResClient.CreateResourceReference(setup.LayoutResourceID)
		defer layoutRef.Release()

		// Get the SRPC client for the layout resource
		layoutSrpcClient, err := layoutRef.GetClient()
		if err != nil {
			t.Fatalf("GetClient failed: %v", err)
		}

		// Create a client for the layout host service
		layoutClient := s4wave_layout.NewSRPCLayoutHostClient(layoutSrpcClient)

		// Call NavigateTab (default implementation returns empty response)
		resp, err := layoutClient.NavigateTab(ctx, &s4wave_layout.NavigateTabRequest{
			TabId: "file-browser",
			Path:  "/some/path",
		})
		if err != nil {
			t.Fatalf("NavigateTab failed: %v", err)
		}

		// Require the navigation RPC to return a response.
		if resp == nil {
			t.Fatal("expected non-nil response")
		}

		// Report successful navigation RPC completion.
		t.Log("NavigateTab returned successfully")
	})

	// Verify Files tab navigation updates the watch and persists across clients.
	t.Run("NavigateTabCriticalPath", func(t *testing.T) {
		// Create a retained layout engine for the navigation sequence.
		objectKey := "object-layout/test-navigate-critical-path-" + t.Name()
		setup, err := tb.SetupLayoutEngine(ctx, objectKey)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer setup.Release()

		// Watch the seeded layout before applying navigation changes.
		layoutClient := openLayoutClient(t, tb, setup.LayoutResourceID)
		strm, err := layoutClient.WatchLayoutModel(ctx)
		if err != nil {
			t.Fatalf("WatchLayoutModel failed: %v", err)
		}

		// Require an unclosable Files tab with an empty initial path.
		initialModel, err := strm.Recv()
		if err != nil {
			t.Fatalf("Recv initial model failed: %v", err)
		}
		initialTab := getMainFilesTabState(t, initialModel)
		if initialTab.tabID != "files" {
			t.Fatalf("expected files tab id, got %q", initialTab.tabID)
		}
		if initialTab.enableClose {
			t.Fatal("expected initial files tab to be unclosable")
		}
		if initialTab.path != "" {
			t.Fatalf("expected empty initial path, got %q", initialTab.path)
		}

		// Navigate the Files tab through successive paths and observe each change.
		paths := []string{
			"/test",
			"/test/a",
			"/test/b",
			"/test/c",
			"/test/final",
		}
		for _, path := range paths {
			// Navigate the Files tab to the next path in the sequence.
			_, err := layoutClient.NavigateTab(ctx, &s4wave_layout.NavigateTabRequest{
				TabId: initialTab.tabID,
				Path:  path,
			})
			if err != nil {
				t.Fatalf("NavigateTab(%q) failed: %v", path, err)
			}

			// Receive the layout model emitted for the navigation change.
			nextModel, err := strm.Recv()
			if err != nil {
				t.Fatalf("Recv updated model failed: %v", err)
			}
			nextTab := getMainFilesTabState(t, nextModel)

			// Require navigation to preserve the tab ID and update its path.
			if nextTab.tabID != initialTab.tabID {
				t.Fatalf("expected tab id %q, got %q", initialTab.tabID, nextTab.tabID)
			}
			if nextTab.path != path {
				t.Fatalf("expected path %q, got %q", path, nextTab.path)
			}
		}

		// Reopen the layout watch and require the final navigation path to persist.
		reopenedClient := openLayoutClient(t, tb, setup.LayoutResourceID)
		reopenedStrm, err := reopenedClient.WatchLayoutModel(ctx)
		if err != nil {
			t.Fatalf("WatchLayoutModel on reopened client failed: %v", err)
		}

		// Receive the persisted layout from the reopened watch.
		reopenedModel, err := reopenedStrm.Recv()
		if err != nil {
			t.Fatalf("Recv reopened model failed: %v", err)
		}

		// Require the reopened Files tab to retain the final path.
		reopenedTab := getMainFilesTabState(t, reopenedModel)
		if reopenedTab.path != "/test/final" {
			t.Fatalf("expected persisted final path %q, got %q", "/test/final", reopenedTab.path)
		}
	})

	// Verify direct tab addition leaves the seeded layout unchanged.
	t.Run("DirectAddTabNoOp", func(t *testing.T) {
		// Create a retained layout engine for the direct tab addition request.
		objectKey := "object-layout/test-add-tab-no-op-" + t.Name()
		setup, err := tb.SetupLayoutEngine(ctx, objectKey)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer setup.Release()

		// Request direct addition of a Files tab through the layout host.
		layoutClient := openLayoutClient(t, tb, setup.LayoutResourceID)
		resp, err := layoutClient.AddTab(ctx, &s4wave_layout.AddTabRequest{
			Tab: &s4wave_layout.TabDef{
				Id:          "direct-add",
				Name:        "Direct Add",
				EnableClose: true,
				Data:        objectLayoutTabData(t, "files", "unixfs/fs-node", "/direct-add", ""),
			},
			Select: true,
		})
		if err != nil {
			t.Fatalf("AddTab failed: %v", err)
		}

		// Require the direct tab addition response to contain no new tab ID.
		if resp.GetTabId() != "" {
			t.Fatalf("expected current ObjectLayout AddTab no-op to return empty tab id, got %q", resp.GetTabId())
		}

		// Reopen the layout watch after direct tab addition.
		reopenedClient := openLayoutClient(t, tb, setup.LayoutResourceID)
		reopenedStrm, err := reopenedClient.WatchLayoutModel(ctx)
		if err != nil {
			t.Fatalf("WatchLayoutModel on reopened client failed: %v", err)
		}

		// Receive the model retained after direct tab addition.
		reopenedModel, err := reopenedStrm.Recv()
		if err != nil {
			t.Fatalf("Recv reopened model failed: %v", err)
		}

		// Require direct tab addition to preserve the original single Files tab.
		mainTabSet := reopenedModel.GetLayout().GetChildren()[0].GetTabSet()
		if got := len(mainTabSet.GetChildren()); got != 1 {
			t.Fatalf("expected direct AddTab no-op to preserve one tab, got %d", got)
		}
		if mainTabSet.GetChildren()[0].GetId() != "files" {
			t.Fatalf("expected original files tab to remain, got %q", mainTabSet.GetChildren()[0].GetId())
		}
	})

	// Verify tab replacement preserves identity and applies target changes.
	t.Run("ReplaceTabCriticalPath", func(t *testing.T) {
		// Create a retained layout engine for tab replacement.
		objectKey := "object-layout/test-replace-critical-path-" + t.Name()
		setup, err := tb.SetupLayoutEngine(ctx, objectKey)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer setup.Release()

		// Watch the seeded layout before replacing its Files tab.
		layoutClient := openLayoutClient(t, tb, setup.LayoutResourceID)
		strm, err := layoutClient.WatchLayoutModel(ctx)
		if err != nil {
			t.Fatalf("WatchLayoutModel failed: %v", err)
		}

		// Require the initial layout to expose the seeded Files tab.
		initialModel, err := strm.Recv()
		if err != nil {
			t.Fatalf("Recv initial model failed: %v", err)
		}
		initialTab := getMainFilesTabState(t, initialModel)
		if initialTab.tabID != "files" {
			t.Fatalf("expected files tab id, got %q", initialTab.tabID)
		}

		// Replace the Files tab target with an ordinary Canvas target.
		_, err = layoutClient.ReplaceTab(ctx, &s4wave_layout.ReplaceTabRequest{
			TabId: initialTab.tabID,
			Tab: &s4wave_layout.TabDef{
				Name:        "Canvas",
				HelpText:    "canvas-1",
				EnableClose: true,
				Data:        objectLayoutTabData(t, "canvas-1", "canvas", "/board", ""),
			},
		})
		if err != nil {
			t.Fatalf("ReplaceTab failed: %v", err)
		}

		// Receive the layout model emitted for the Canvas replacement.
		replacedModel, err := strm.Recv()
		if err != nil {
			t.Fatalf("Recv replaced model failed: %v", err)
		}
		replacedTab := getMainFilesTabState(t, replacedModel)

		// Require Canvas replacement to preserve tab identity and close behavior.
		if replacedTab.tabID != initialTab.tabID {
			t.Fatalf("expected stable tab id %q, got %q", initialTab.tabID, replacedTab.tabID)
		}
		if replacedTab.name != "Canvas" {
			t.Fatalf("expected tab name Canvas, got %q", replacedTab.name)
		}
		if replacedTab.enableClose != initialTab.enableClose {
			t.Fatalf("expected replace to preserve enableClose=%v, got %v", initialTab.enableClose, replacedTab.enableClose)
		}

		// Require Canvas replacement to apply the target, path, and component data.
		if replacedTab.objectKey != "canvas-1" || replacedTab.objectType != "canvas" {
			t.Fatalf("expected canvas target, got key=%q type=%q", replacedTab.objectKey, replacedTab.objectType)
		}
		if replacedTab.path != "/board" {
			t.Fatalf("expected path /board, got %q", replacedTab.path)
		}
		if replacedTab.componentID != "" {
			t.Fatalf("expected ordinary replacement to clear component id, got %q", replacedTab.componentID)
		}

		// Replace the Canvas target with an explicit component target.
		_, err = layoutClient.ReplaceTab(ctx, &s4wave_layout.ReplaceTabRequest{
			TabId: initialTab.tabID,
			Tab: &s4wave_layout.TabDef{
				Name:        "Canvas Component",
				HelpText:    "canvas-1",
				EnableClose: true,
				Data:        objectLayoutTabData(t, "canvas-1", "canvas", "/board", "canvas.component"),
			},
		})
		if err != nil {
			t.Fatalf("ReplaceTab explicit component failed: %v", err)
		}

		// Receive the layout model emitted for the component replacement.
		componentModel, err := strm.Recv()
		if err != nil {
			t.Fatalf("Recv component model failed: %v", err)
		}
		componentTab := getMainFilesTabState(t, componentModel)

		// Require component replacement to preserve close behavior and component ID.
		if componentTab.enableClose != initialTab.enableClose {
			t.Fatalf("expected component replace to preserve enableClose=%v, got %v", initialTab.enableClose, componentTab.enableClose)
		}
		if componentTab.componentID != "canvas.component" {
			t.Fatalf("expected explicit component id, got %q", componentTab.componentID)
		}

		// Require replacement of a missing tab to succeed without changing a tab.
		_, err = layoutClient.ReplaceTab(ctx, &s4wave_layout.ReplaceTabRequest{
			TabId: "missing-tab",
			Tab: &s4wave_layout.TabDef{
				Name: "Missing",
				Data: objectLayoutTabData(t, "files", "unixfs/fs-node", "", ""),
			},
		})
		if err != nil {
			t.Fatalf("ReplaceTab missing tab should be a no-op, got: %v", err)
		}
	})

	// Verify the seeded tab data decodes to its World object target.
	t.Run("TabDataRoundtrip", func(t *testing.T) {
		// Create a retained layout engine for tab data decoding.
		objectKey := "object-layout/test-tabdata-" + t.Name()
		setup, err := tb.SetupLayoutEngine(ctx, objectKey)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer setup.Release()

		// Create a reference to the layout resource
		layoutRef := tb.ResClient.CreateResourceReference(setup.LayoutResourceID)
		defer layoutRef.Release()

		// Get the SRPC client for the layout resource
		layoutSrpcClient, err := layoutRef.GetClient()
		if err != nil {
			t.Fatalf("GetClient failed: %v", err)
		}

		// Create a client for the layout host service
		layoutClient := s4wave_layout.NewSRPCLayoutHostClient(layoutSrpcClient)

		// Start watching the layout model
		strm, err := layoutClient.WatchLayoutModel(ctx)
		if err != nil {
			t.Fatalf("WatchLayoutModel failed: %v", err)
		}

		// Receive the initial layout model
		model, err := strm.Recv()
		if err != nil {
			t.Fatalf("Recv failed: %v", err)
		}

		// Get the file browser tab (first tab in main tabset) and deserialize its data
		mainTabSet := model.GetLayout().GetChildren()[0].GetTabSet()
		fileBrowserTab := mainTabSet.GetChildren()[0]

		// Decode the Files tab data from the initial layout model.
		tabData := &s4wave_layout_world.ObjectLayoutTab{}
		err = tabData.UnmarshalVT(fileBrowserTab.GetData())
		if err != nil {
			t.Fatalf("UnmarshalVT tab data failed: %v", err)
		}

		// Verify the tab data
		worldObjInfo := tabData.GetObjectInfo().GetWorldObjectInfo()
		if worldObjInfo == nil {
			t.Fatal("expected WorldObjectInfo")
		}
		if worldObjInfo.GetObjectKey() != "files" {
			t.Fatalf("expected object key 'files', got %q", worldObjInfo.GetObjectKey())
		}

		// Report the World object target decoded from the Files tab.
		t.Logf("Successfully deserialized tab data: objectKey=%s", worldObjInfo.GetObjectKey())
	})
}
