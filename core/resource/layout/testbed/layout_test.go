//go:build !js

package layout_testbed_test

import (
	"context"
	"testing"

	layout_testbed "github.com/s4wave/spacewave/core/resource/layout/testbed"
	s4wave_layout "github.com/s4wave/spacewave/sdk/layout"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
)

type layoutTabState struct {
	tabID string
	path  string
}

func getMainFilesTabState(t *testing.T, model *s4wave_layout.LayoutModel) layoutTabState {
	t.Helper()

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

	var tabData s4wave_layout_world.ObjectLayoutTab
	if err := tabData.UnmarshalVT(tab.GetData()); err != nil {
		t.Fatalf("unmarshal tab data failed: %v", err)
	}

	tabID := tab.GetId()
	if tabID == "" {
		t.Fatal("expected files tab id")
	}
	return layoutTabState{
		tabID: tabID,
		path:  tabData.GetPath(),
	}
}

func openLayoutClient(t *testing.T, tb *layout_testbed.Testbed, resourceID uint32) s4wave_layout.SRPCLayoutHostClient {
	t.Helper()

	layoutRef := tb.ResClient.CreateResourceReference(resourceID)
	t.Cleanup(layoutRef.Release)

	layoutSrpcClient, err := layoutRef.GetClient()
	if err != nil {
		t.Fatalf("GetClient failed: %v", err)
	}
	return s4wave_layout.NewSRPCLayoutHostClient(layoutSrpcClient)
}

// TestLayoutResource tests the LayoutResource functionality.
func TestLayoutResource(t *testing.T) {
	ctx := context.Background()

	tb, err := layout_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	t.Run("WatchLayoutModel", func(t *testing.T) {
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

		t.Logf("Successfully received initial layout model with main tabset containing %d tabs", len(mainTabSet.GetChildren()))
	})

	t.Run("SetModel", func(t *testing.T) {
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

		t.Logf("Successfully updated layout model, main tabset now has %d tabs", len(updatedMainTabSet.GetChildren()))
	})

	t.Run("NavigateTab", func(t *testing.T) {
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

		if resp == nil {
			t.Fatal("expected non-nil response")
		}

		t.Log("NavigateTab returned successfully")
	})

	t.Run("NavigateTabCriticalPath", func(t *testing.T) {
		objectKey := "object-layout/test-navigate-critical-path-" + t.Name()
		setup, err := tb.SetupLayoutEngine(ctx, objectKey)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer setup.Release()

		layoutClient := openLayoutClient(t, tb, setup.LayoutResourceID)
		strm, err := layoutClient.WatchLayoutModel(ctx)
		if err != nil {
			t.Fatalf("WatchLayoutModel failed: %v", err)
		}

		initialModel, err := strm.Recv()
		if err != nil {
			t.Fatalf("Recv initial model failed: %v", err)
		}
		initialTab := getMainFilesTabState(t, initialModel)
		if initialTab.tabID != "files" {
			t.Fatalf("expected files tab id, got %q", initialTab.tabID)
		}
		if initialTab.path != "" {
			t.Fatalf("expected empty initial path, got %q", initialTab.path)
		}

		paths := []string{
			"/test",
			"/test/a",
			"/test/b",
			"/test/c",
			"/test/final",
		}
		for _, path := range paths {
			_, err := layoutClient.NavigateTab(ctx, &s4wave_layout.NavigateTabRequest{
				TabId: initialTab.tabID,
				Path:  path,
			})
			if err != nil {
				t.Fatalf("NavigateTab(%q) failed: %v", path, err)
			}

			nextModel, err := strm.Recv()
			if err != nil {
				t.Fatalf("Recv updated model failed: %v", err)
			}
			nextTab := getMainFilesTabState(t, nextModel)
			if nextTab.tabID != initialTab.tabID {
				t.Fatalf("expected tab id %q, got %q", initialTab.tabID, nextTab.tabID)
			}
			if nextTab.path != path {
				t.Fatalf("expected path %q, got %q", path, nextTab.path)
			}
		}

		reopenedClient := openLayoutClient(t, tb, setup.LayoutResourceID)
		reopenedStrm, err := reopenedClient.WatchLayoutModel(ctx)
		if err != nil {
			t.Fatalf("WatchLayoutModel on reopened client failed: %v", err)
		}
		reopenedModel, err := reopenedStrm.Recv()
		if err != nil {
			t.Fatalf("Recv reopened model failed: %v", err)
		}
		reopenedTab := getMainFilesTabState(t, reopenedModel)
		if reopenedTab.path != "/test/final" {
			t.Fatalf("expected persisted final path %q, got %q", "/test/final", reopenedTab.path)
		}
	})

	t.Run("TabDataRoundtrip", func(t *testing.T) {
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

		t.Logf("Successfully deserialized tab data: objectKey=%s", worldObjInfo.GetObjectKey())
	})
}
