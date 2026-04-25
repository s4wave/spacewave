package resource_testbed_test

import (
	"context"
	"testing"
	"time"

	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	s4wave_layout "github.com/s4wave/spacewave/sdk/layout"
	"github.com/sirupsen/logrus"
)

func TestBrowserTestServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Create and start server
	server := resource_testbed.NewBrowserTestServer(le, nil)
	port, err := server.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop(ctx)

	t.Logf("server started on port %d", port)

	// Verify port is valid
	if port <= 0 {
		t.Fatalf("invalid port: %d", port)
	}

	// Test setting layout model
	initialModel := &s4wave_layout.LayoutModel{
		Layout: &s4wave_layout.RowDef{
			Id: "root",
			Children: []*s4wave_layout.RowOrTabSetDef{
				{
					Node: &s4wave_layout.RowOrTabSetDef_TabSet{
						TabSet: &s4wave_layout.TabSetDef{
							Id: "tabset-1",
							Children: []*s4wave_layout.TabDef{
								{Id: "tab-1", Name: "Tab 1"},
								{Id: "tab-2", Name: "Tab 2"},
							},
						},
					},
				},
			},
		},
	}
	server.SetLayoutModel(initialModel)

	// Verify we can get the model back
	model := server.GetLayoutModel()
	if model == nil {
		t.Fatal("expected model to be set")
	}
	if model.GetLayout().GetId() != "root" {
		t.Fatalf("expected root id, got %s", model.GetLayout().GetId())
	}

	t.Log("server test passed")
}
