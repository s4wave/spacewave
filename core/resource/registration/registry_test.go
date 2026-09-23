package registration_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	objecttypes "github.com/s4wave/spacewave/core/resource/objecttype/registry"
	quickstarts "github.com/s4wave/spacewave/core/resource/quickstart/registry"
	"github.com/s4wave/spacewave/core/resource/registration"
	viewers "github.com/s4wave/spacewave/core/resource/viewer/registry"
	worldops "github.com/s4wave/spacewave/core/resource/worldop/registry"
	objecttype "github.com/s4wave/spacewave/sdk/objecttype/registry"
	plugin_registration "github.com/s4wave/spacewave/sdk/plugin/registration"
	quickstart "github.com/s4wave/spacewave/sdk/quickstart/registry"
	viewer "github.com/s4wave/spacewave/sdk/viewer/registry"
	worldop "github.com/s4wave/spacewave/sdk/worldop/registry"
)

// TestPreparedRegistrationReplacement drives the real Resource protocol across
// all four registries, including incomplete candidates and retired Resources.
func TestPreparedRegistrationReplacement(t *testing.T) {
	// Compose the same shared admission boundary as the core Resource root.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	groups := registration.NewRegistry()
	types := objecttypes.NewObjectTypeRegistryResource(groups)
	ops := worldops.NewWorldOpRegistryResource(groups)
	uis := viewers.NewViewerRegistryResource(groups)
	seeds := quickstarts.NewQuickstartRegistryResource(nil, nil, groups)
	root := srpc.NewMux(types.GetMux(), ops.GetMux(), uis.GetMux(), seeds.GetMux())
	if err := groups.Register(root); err != nil {
		t.Fatal(err)
	}
	mux := srpc.NewMux()
	if err := resource_server.NewResourceServer(root).Register(mux); err != nil {
		t.Fatal(err)
	}
	client, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Release()
	rootRef := client.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	prepare := plugin_registration.NewSRPCRegistrationServiceClient(rootClient)

	// Every generation registers the same type, operation, viewer, and Quickstart.
	open := func(pluginID, manifest, instanceKey string) (resource_client.ResourceRef, srpc.Client) {
		t.Helper()
		response, err := prepare.Prepare(ctx, &plugin_registration.PrepareRequest{PluginId: pluginID, ManifestRoot: manifest, InstanceKey: instanceKey})
		if err != nil {
			t.Fatal(err)
		}
		ref := client.CreateResourceReference(response.GetResourceId())
		t.Cleanup(ref.Release)
		rpc, err := ref.GetClient()
		if err != nil {
			t.Fatal(err)
		}
		return ref, rpc
	}
	register := func(rpc srpc.Client, label string) {
		t.Helper()
		if _, err := objecttype.NewSRPCObjectTypeRegistryResourceServiceClient(rpc).RegisterObjectType(ctx, &objecttype.RegisterObjectTypeRequest{
			TypeId: "colors/app", PluginId: "colors", Metadata: &objecttype.ObjectTypeMetadata{DisplayName: label},
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := worldop.NewSRPCWorldOpRegistryResourceServiceClient(rpc).RegisterWorldOp(ctx, &worldop.RegisterWorldOpRequest{
			OperationTypeId: "colors/like", PluginId: "colors",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := viewer.NewSRPCViewerRegistryResourceServiceClient(rpc).RegisterViewer(ctx, &viewer.RegisterViewerRequest{Registration: &viewer.ViewerRegistration{
			TypeId: "colors/app", ComponentId: "Colors", ScriptPath: "/" + label + ".js", Surface: viewer.ViewerSurface_VIEWER_SURFACE_WEB,
		}}); err != nil {
			t.Fatal(err)
		}
		if _, err := quickstart.NewSRPCQuickstartRegistryResourceServiceClient(rpc).RegisterQuickstart(ctx, &quickstart.RegisterQuickstartRequest{Registration: &quickstart.QuickstartRegistration{
			QuickstartId: "colors/app", PluginId: "colors", Name: label, Description: "Colors", Category: "apps",
		}}); err != nil {
			t.Fatal(err)
		}
	}
	activate := func(rpc srpc.Client) {
		t.Helper()
		if _, err := plugin_registration.NewSRPCGenerationServiceClient(rpc).Activate(ctx, &plugin_registration.ActivateRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	check := func(instanceKey, label string) {
		t.Helper()
		if got := types.LookupRegistration("colors/app", instanceKey).GetMetadata().GetDisplayName(); got != label {
			t.Fatalf("type = %q, want %q", got, label)
		}
		if got := seeds.LookupRegistration("colors/app", instanceKey); got.GetName() != label || got.GetManifestRoot() != label {
			t.Fatalf("Quickstart = %v, want generation %q", got, label)
		}
		list, err := uis.ListViewers(ctx, &viewer.ListViewersRequest{Surface: viewer.ViewerSurface_VIEWER_SURFACE_WEB, InstanceKey: instanceKey})
		if err != nil {
			t.Fatal(err)
		}
		if label == "" {
			if len(list.GetRegistrations()) != 0 || ops.LookupRegistrationByOpType("colors/like", instanceKey) != nil {
				t.Fatal("private generation leaked into visible registries")
			}
			return
		}
		if len(list.GetRegistrations()) != 1 || list.GetRegistrations()[0].GetScriptPath() != "/"+label+".js" || ops.LookupRegistrationByOpType("colors/like", instanceKey) == nil {
			t.Fatalf("incomplete visible generation: %v", list)
		}
	}

	// Preparation is invisible, and a failed candidate never hides the old group.
	oldRef, old := open("colors", "old", "")
	register(old, "old")
	check("", "")
	activate(old)
	check("", "old")
	failedRef, failed := open("colors", "failed", "")
	if _, err := objecttype.NewSRPCObjectTypeRegistryResourceServiceClient(failed).RegisterObjectType(ctx, &objecttype.RegisterObjectTypeRequest{TypeId: "colors/app", PluginId: "colors"}); err != nil {
		t.Fatal(err)
	}
	if _, err := viewer.NewSRPCViewerRegistryResourceServiceClient(failed).RegisterViewer(ctx, &viewer.RegisterViewerRequest{}); err == nil {
		t.Fatal("invalid candidate viewer succeeded")
	}
	failedRef.Release()
	check("", "old")

	// Losing a candidate after publication but before the old worker retires
	// restores the retained old group, including its immutable viewer path.
	interruptedRef, interrupted := open("colors", "interrupted", "")
	register(interrupted, "interrupted")
	activate(interrupted)
	check("", "interrupted")
	interruptedRef.Release()
	rollbackStream, err := quickstart.NewSRPCQuickstartRegistryResourceServiceClient(rootClient).WatchQuickstarts(ctx, &quickstart.WatchQuickstartsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer rollbackStream.Close()
	for {
		response, err := rollbackStream.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if len(response.GetRegistrations()) == 1 && response.GetRegistrations()[0].GetName() == "old" {
			break
		}
	}
	check("", "old")

	// Admission switches all readers, and releasing the old group preserves it.
	newRef, next := open("colors", "new", "")
	register(next, "new")
	check("", "old")
	activate(next)
	activate(next)
	check("", "new")
	if _, err := plugin_registration.NewSRPCGenerationServiceClient(old).Activate(ctx, &plugin_registration.ActivateRequest{}); err == nil {
		t.Fatal("retired generation was admitted again")
	}
	if _, err := objecttype.NewSRPCObjectTypeRegistryResourceServiceClient(next).RegisterObjectType(ctx, &objecttype.RegisterObjectTypeRequest{TypeId: "colors/late", PluginId: "colors"}); err == nil {
		t.Fatal("admitted generation accepted a partial extension")
	}
	oldRef.Release()
	check("", "new")

	// Two Spaces independently select the same names. Preparing, replacing, and
	// releasing one Space leaves the other Space and the global defaults intact.
	spaceARef, spaceA := open("colors", "space-a-one", "space/a")
	register(spaceA, "space-a-one")
	spaceBRef, spaceB := open("colors", "space-b-one", "space/b")
	register(spaceB, "space-b-one")
	check("space/a", "new")
	activate(spaceA)
	activate(spaceB)
	check("space/a", "space-a-one")
	check("space/b", "space-b-one")
	check("", "new")
	check("space/c", "new")
	if ops.LookupRegistrationByOpType("colors/like", "space/a").GetRegistrationId() == ops.LookupRegistrationByOpType("colors/like", "space/b").GetRegistrationId() {
		t.Fatal("independent Spaces selected the same operation registration")
	}
	spaceANextRef, spaceANext := open("colors", "space-a-two", "space/a")
	register(spaceANext, "space-a-two")
	check("space/a", "space-a-one")
	activate(spaceANext)
	check("space/a", "space-a-two")
	check("space/b", "space-b-one")
	spaceARef.Release()
	spaceANextRef.Release()
	spaceWatch, err := quickstart.NewSRPCQuickstartRegistryResourceServiceClient(rootClient).WatchQuickstarts(ctx, &quickstart.WatchQuickstartsRequest{InstanceKey: "space/a"})
	if err != nil {
		t.Fatal(err)
	}
	defer spaceWatch.Close()
	for {
		response, err := spaceWatch.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if len(response.GetRegistrations()) == 1 && response.GetRegistrations()[0].GetName() == "new" {
			break
		}
	}
	check("space/a", "new")
	check("space/b", "space-b-one")
	spaceBRef.Release()

	// A different family cannot reserve a type already held by this family.
	foreignRef, foreign := open("other", "foreign", "")
	if _, err := objecttype.NewSRPCObjectTypeRegistryResourceServiceClient(foreign).RegisterObjectType(ctx, &objecttype.RegisterObjectTypeRequest{TypeId: "colors/app", PluginId: "other"}); err == nil {
		t.Fatal("foreign plugin replaced another family's type")
	}
	foreignRef.Release()
	newRef.Release()
	// Resource release is asynchronous; use the actual watch to observe removal.
	stream, err := quickstart.NewSRPCQuickstartRegistryResourceServiceClient(rootClient).WatchQuickstarts(ctx, &quickstart.WatchQuickstartsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	for {
		response, err := stream.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if len(response.GetRegistrations()) == 0 {
			break
		}
	}
	check("", "")
}
