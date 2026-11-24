package resource_root_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	resource_root "github.com/s4wave/spacewave/core/resource/root"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
	s4wave_block_cursor "github.com/s4wave/spacewave/sdk/block/cursor"
	s4wave_block_transaction "github.com/s4wave/spacewave/sdk/block/transaction"
	s4wave_bucket_lookup "github.com/s4wave/spacewave/sdk/bucket/lookup"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
	s4wave_cdn "github.com/s4wave/spacewave/sdk/cdn"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
	s4wave_command_registry "github.com/s4wave/spacewave/sdk/command/registry"
	s4wave_configtype_registry "github.com/s4wave/spacewave/sdk/configtype/registry"
	s4wave_debugdb "github.com/s4wave/spacewave/sdk/debugdb"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_registry "github.com/s4wave/spacewave/sdk/flowgraph/registry"
	s4wave_git "github.com/s4wave/spacewave/sdk/git"
	s4wave_layout "github.com/s4wave/spacewave/sdk/layout"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
	s4wave_plugin_registration "github.com/s4wave/spacewave/sdk/plugin/registration"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	s4wave_provider "github.com/s4wave/spacewave/sdk/provider"
	s4wave_provider_local "github.com/s4wave/spacewave/sdk/provider/local"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_quickstart_registry "github.com/s4wave/spacewave/sdk/quickstart/registry"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	s4wave_sql_query "github.com/s4wave/spacewave/sdk/sql/query"
	s4wave_sql_query_result "github.com/s4wave/spacewave/sdk/sql/query-result"
	s4wave_sql_schema "github.com/s4wave/spacewave/sdk/sql/schema"
	s4wave_sql_table_view "github.com/s4wave/spacewave/sdk/sql/table-view"
	s4wave_sql_workbench "github.com/s4wave/spacewave/sdk/sql/workbench"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	s4wave_trace "github.com/s4wave/spacewave/sdk/trace"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
	s4wave_viewer_registry "github.com/s4wave/spacewave/sdk/viewer/registry"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
	s4wave_worldop_registry "github.com/s4wave/spacewave/sdk/worldop/registry"
	"github.com/sirupsen/logrus"
)

// TestPluginCallerFilterNestedSession calls protected session methods through
// PluginRpc on a session the root mounts beneath it, as a plugin would. A
// caller outside the host set is refused; a host-set caller reaches the session.
func TestPluginCallerFilterNestedSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// Serve a root whose MountSessionByIdx mounts a session beneath it, behind
	// the filter, through a Plugin service that serves stamped PluginRpc calls.
	sess := &fakeSession{}
	rootMux := srpc.NewMux()
	if err := s4wave_root.SRPCRegisterRootResourceService(rootMux, &fakeRoot{session: sess}); err != nil {
		t.Fatal(err)
	}
	coreMux := srpc.NewMux()
	server := resource_server.NewFilteredResourceServer(rootMux, resource_root.NewPluginCallerFilter(logrus.NewEntry(logrus.New()), nil))
	if err := server.Register(coreMux); err != nil {
		t.Fatal(err)
	}
	pluginMux := srpc.NewMux()
	if err := bldr_plugin.SRPCRegisterPlugin(pluginMux, stampedPlugin{inv: coreMux}); err != nil {
		t.Fatal(err)
	}
	pluginClient := bldr_plugin.NewSRPCPluginClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(pluginMux))))

	// mountSession mounts the session as caller and returns its session client.
	mountSession := func(caller *bldr_plugin.PluginCaller) s4wave_session.SRPCSessionResourceServiceClient {
		t.Helper()
		conn := rpcstream.NewRpcStreamClient(pluginClient.PluginRpc, caller.ComponentID(), true)
		client, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(conn))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(client.Release)
		rootRef := client.AccessRootResource()
		t.Cleanup(rootRef.Release)
		rootClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err)
		}
		resp, err := s4wave_root.NewSRPCRootResourceServiceClient(rootClient).MountSessionByIdx(ctx, &s4wave_root.MountSessionByIdxRequest{SessionIdx: 1})
		if err != nil {
			t.Fatal(err)
		}
		ref := client.CreateResourceReference(resp.GetResourceId())
		t.Cleanup(ref.Release)
		sessClient, err := ref.GetClient()
		if err != nil {
			t.Fatal(err)
		}
		return s4wave_session.NewSRPCSessionResourceServiceClient(sessClient)
	}

	// A plugin outside the host set is refused both methods.
	outside := mountSession(&bldr_plugin.PluginCaller{PluginId: "colors", BindingKey: "colors"})
	_, err := outside.SetPluginApproval(ctx, &s4wave_session.SetPluginApprovalRequest{PluginId: "colors"})
	requireRefused(t, "SetPluginApproval", err)
	_, err = outside.DeleteSpace(ctx, &s4wave_session.DeleteSpaceRequest{SharedObjectId: "space"})
	requireRefused(t, "DeleteSpace", err)
	if n := sess.calls.Load(); n != 0 {
		t.Fatalf("session served %d refused calls", n)
	}

	// A host-set plugin reaches the session for both.
	host := mountSession(&bldr_plugin.PluginCaller{PluginId: "spacewave-app", HostSet: true})
	if _, err := host.SetPluginApproval(ctx, &s4wave_session.SetPluginApprovalRequest{PluginId: "colors"}); err != nil {
		t.Fatal(err)
	}
	if _, err := host.DeleteSpace(ctx, &s4wave_session.DeleteSpaceRequest{SharedObjectId: "space"}); err != nil {
		t.Fatal(err)
	}
	if n := sess.calls.Load(); n != 2 {
		t.Fatalf("session served %d host-set calls, want 2", n)
	}
}

// TestPluginCallerFilterSecretGrant calls a Space's Secret methods through
// PluginRpc under the Device policy's grant. Only the granted plugin, bound to
// the granted Space, reaches the read, create and write methods; granting
// participants and deleting stay refused to it.
func TestPluginCallerFilterSecretGrant(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// Grant glados-core the Secrets of Space s1 in a real Device policy.
	stateRoot := t.TempDir()
	policy := &device_policy.DevicePolicy{LaunchApproval: []*device_policy.LaunchApproval{
		{SpaceId: "s1", SecretPluginIds: []string{"glados-core"}},
		{SpaceId: "s2"},
	}}
	if err := device_policy.WriteFile(stateRoot, policy); err != nil {
		t.Fatal(err)
	}
	store, err := device_policy.NewPolicyStore(stateRoot)
	if err != nil {
		t.Fatal(err)
	}

	// Serve a Space Resource service on the root, behind the granted filter.
	sp := &fakeSpace{}
	rootMux := srpc.NewMux()
	if err := s4wave_space.SRPCRegisterSpaceResourceService(rootMux, sp); err != nil {
		t.Fatal(err)
	}
	coreMux := srpc.NewMux()
	server := resource_server.NewFilteredResourceServer(rootMux, resource_root.NewPluginCallerFilter(logrus.NewEntry(logrus.New()), store))
	if err := server.Register(coreMux); err != nil {
		t.Fatal(err)
	}
	pluginMux := srpc.NewMux()
	if err := bldr_plugin.SRPCRegisterPlugin(pluginMux, stampedPlugin{inv: coreMux}); err != nil {
		t.Fatal(err)
	}
	pluginClient := bldr_plugin.NewSRPCPluginClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(pluginMux))))

	// spaceAs returns the Space Resource client a plugin stamped as caller sees.
	spaceAs := func(pluginID, bindingKey string) s4wave_space.SRPCSpaceResourceServiceClient {
		t.Helper()
		caller := &bldr_plugin.PluginCaller{PluginId: pluginID, BindingKey: bindingKey}
		conn := rpcstream.NewRpcStreamClient(pluginClient.PluginRpc, caller.ComponentID(), true)
		client, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(conn))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(client.Release)
		rootRef := client.AccessRootResource()
		t.Cleanup(rootRef.Release)
		rootClient, err := rootRef.GetClient()
		if err != nil {
			t.Fatal(err)
		}
		return s4wave_space.NewSRPCSpaceResourceServiceClient(rootClient)
	}

	// The granted plugin in its Space reaches read, create and write.
	granted := spaceAs("glados-core", "space/local/acct/s1")
	if _, err := granted.ReadSecretPayload(ctx, &s4wave_space.ReadSecretPayloadRequest{ObjectKey: "k"}); err != nil {
		t.Fatal(err)
	}
	if _, err := granted.CreateSecret(ctx, &s4wave_space.CreateSecretRequest{ObjectKey: "k"}); err != nil {
		t.Fatal(err)
	}
	if _, err := granted.WriteSecretPayload(ctx, &s4wave_space.WriteSecretPayloadRequest{ObjectKey: "k"}); err != nil {
		t.Fatal(err)
	}

	// Participants and deletion stay refused to the granted plugin.
	_, err = granted.AddSecretParticipant(ctx, &s4wave_space.AddSecretParticipantRequest{ObjectKey: "k"})
	requireRefused(t, "AddSecretParticipant", err)
	_, err = granted.DeleteSecret(ctx, &s4wave_space.DeleteSecretRequest{ObjectKey: "k"})
	requireRefused(t, "DeleteSecret", err)

	// Another plugin, the plugin in an ungranted Space, and a binding that is
	// no Space are refused.
	for _, caller := range [][2]string{
		{"colors", "space/local/acct/s1"},
		{"glados-core", "space/local/acct/s2"},
		{"glados-core", "space/local/acct/s3"},
		{"glados-core", "s1"},
	} {
		_, err := spaceAs(caller[0], caller[1]).ReadSecretPayload(ctx, &s4wave_space.ReadSecretPayloadRequest{ObjectKey: "k"})
		requireRefused(t, "ReadSecretPayload as "+caller[0]+"@"+caller[1], err)
	}
	if n := sp.calls.Load(); n != 3 {
		t.Fatalf("space served %d Secret calls, want 3", n)
	}
}

// fakeSpace counts the Secret calls it serves. Its other methods are
// unimplemented.
type fakeSpace struct {
	s4wave_space.SRPCSpaceResourceServiceServer
	calls atomic.Int32
}

// CreateSecret counts the call.
func (s *fakeSpace) CreateSecret(context.Context, *s4wave_space.CreateSecretRequest) (*s4wave_space.CreateSecretResponse, error) {
	s.calls.Add(1)
	return &s4wave_space.CreateSecretResponse{}, nil
}

// ReadSecretPayload counts the call.
func (s *fakeSpace) ReadSecretPayload(context.Context, *s4wave_space.ReadSecretPayloadRequest) (*s4wave_space.ReadSecretPayloadResponse, error) {
	s.calls.Add(1)
	return &s4wave_space.ReadSecretPayloadResponse{}, nil
}

// WriteSecretPayload counts the call.
func (s *fakeSpace) WriteSecretPayload(context.Context, *s4wave_space.WriteSecretPayloadRequest) (*s4wave_space.WriteSecretPayloadResponse, error) {
	s.calls.Add(1)
	return &s4wave_space.WriteSecretPayloadResponse{}, nil
}

// AddSecretParticipant counts the call.
func (s *fakeSpace) AddSecretParticipant(context.Context, *s4wave_space.AddSecretParticipantRequest) (*s4wave_space.AddSecretParticipantResponse, error) {
	s.calls.Add(1)
	return &s4wave_space.AddSecretParticipantResponse{}, nil
}

// DeleteSecret counts the call.
func (s *fakeSpace) DeleteSecret(context.Context, *s4wave_space.DeleteSecretRequest) (*s4wave_space.DeleteSecretResponse, error) {
	s.calls.Add(1)
	return &s4wave_space.DeleteSecretResponse{}, nil
}

// requireRefused fails unless err is the filter's refusal.
func requireRefused(t *testing.T, method string, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "outside the host set") {
		t.Fatalf("%s: expected refusal, got %v", method, err)
	}
}

// stampedPlugin serves PluginRpc calls with inv, as a plugin's server does.
type stampedPlugin struct {
	inv srpc.Invoker
}

// PluginRpc serves the stamped stream with inv.
func (p stampedPlugin) PluginRpc(strm bldr_plugin.SRPCPlugin_PluginRpcStream) error {
	return bldr_plugin.ServePluginRpc(strm, func(*bldr_plugin.PluginCaller) srpc.Invoker { return p.inv })
}

// fakeRoot mounts session for every MountSessionByIdx. Its other methods are
// unimplemented.
type fakeRoot struct {
	s4wave_root.SRPCRootResourceServiceServer
	session *fakeSession
}

// MountSessionByIdx adds the session as a child Resource of the call.
func (r *fakeRoot) MountSessionByIdx(ctx context.Context, _ *s4wave_root.MountSessionByIdxRequest) (*s4wave_root.MountSessionByIdxResponse, error) {
	client, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	mux := srpc.NewMux()
	if err := s4wave_session.SRPCRegisterSessionResourceService(mux, r.session); err != nil {
		return nil, err
	}
	id, err := client.AddResource(mux, func() {})
	if err != nil {
		return nil, err
	}
	return &s4wave_root.MountSessionByIdxResponse{ResourceId: id}, nil
}

// fakeSession counts the protected calls it serves. Its other methods are
// unimplemented.
type fakeSession struct {
	s4wave_session.SRPCSessionResourceServiceServer
	calls atomic.Int32
}

// SetPluginApproval counts the call.
func (s *fakeSession) SetPluginApproval(context.Context, *s4wave_session.SetPluginApprovalRequest) (*s4wave_session.SetPluginApprovalResponse, error) {
	s.calls.Add(1)
	return &s4wave_session.SetPluginApprovalResponse{}, nil
}

// DeleteSpace counts the call.
func (s *fakeSession) DeleteSpace(context.Context, *s4wave_session.DeleteSpaceRequest) (*s4wave_session.DeleteSpaceResponse, error) {
	s.calls.Add(1)
	return &s4wave_session.DeleteSpaceResponse{}, nil
}

// TestPluginCallerMethodsClassified walks every method of the services the
// root Resource tree serves and requires each to be on exactly one of the
// deny list and pluginCallerAllowed, so a new method is classified before it
// reaches a plugin outside the host set.
func TestPluginCallerMethodsClassified(t *testing.T) {
	walked := make(map[string]bool)
	for _, h := range rootTreeHandlers {
		allowed := make(map[string]bool)
		for _, m := range pluginCallerAllowed[h.GetServiceID()] {
			allowed[m] = true
		}
		for _, m := range h.GetMethodIDs() {
			key := h.GetServiceID() + "/" + m
			walked[key] = true
			denied := resource_root.PluginCallerDenied(h.GetServiceID(), m)
			switch {
			case denied && allowed[m]:
				t.Errorf("%s is on both the deny list and pluginCallerAllowed", key)
			case !denied && !allowed[m]:
				t.Errorf("%s is unclassified: add it to the deny list or pluginCallerAllowed", key)
			}
		}
	}

	// Every listed method must exist, so a renamed one cannot linger.
	for _, key := range resource_root.PluginCallerDeniedMethods() {
		if !walked[key] {
			t.Errorf("denied %s is not served by a walked service", key)
		}
	}
	for svc, methods := range pluginCallerAllowed {
		for _, m := range methods {
			if !walked[svc+"/"+m] {
				t.Errorf("allowed %s/%s is not served by a walked service", svc, m)
			}
		}
	}
}

// rootTreeHandlers holds a handler for every service the root Resource tree
// can serve beneath it. Add a service here when the tree starts serving it:
// each Resource registers its services on its own mux when it is built, and
// no generated list of them exists to walk instead.
var rootTreeHandlers = []srpc.Handler{
	s4wave_account.NewSRPCAccountResourceServiceHandler(nil, ""),
	s4wave_block_cursor.NewSRPCBlockCursorResourceServiceHandler(nil, ""),
	s4wave_block_transaction.NewSRPCBlockTransactionResourceServiceHandler(nil, ""),
	s4wave_bucket_lookup.NewSRPCBucketLookupCursorResourceServiceHandler(nil, ""),
	s4wave_canvas.NewSRPCCanvasResourceServiceHandler(nil, ""),
	s4wave_cdn.NewSRPCCdnResourceServiceHandler(nil, ""),
	spacewave_chat_rpc.NewSRPCChatResourceServiceHandler(nil, ""),
	s4wave_command_registry.NewSRPCCommandRegistryResourceServiceHandler(nil, ""),
	s4wave_configtype_registry.NewSRPCConfigTypeRegistryResourceServiceHandler(nil, ""),
	s4wave_debugdb.NewSRPCDebugDbResourceServiceHandler(nil, ""),
	s4wave_debugdb.NewSRPCDebugDbBenchmarkServiceHandler(nil, ""),
	s4wave_device.NewSRPCDeviceResourceServiceHandler(nil, ""),
	s4wave_flowgraph.NewSRPCFlowgraphResourceServiceHandler(nil, ""),
	flowgraph_registry.NewSRPCNodeTypeRegistryResourceServiceHandler(nil, ""),
	s4wave_git.NewSRPCGitRepoResourceServiceHandler(nil, ""),
	s4wave_git.NewSRPCGitWorktreeResourceServiceHandler(nil, ""),
	s4wave_layout.NewSRPCLayoutHostHandler(nil, ""),
	s4wave_objecttype_registry.NewSRPCObjectTypeRegistryResourceServiceHandler(nil, ""),
	s4wave_org.NewSRPCOrgResourceServiceHandler(nil, ""),
	s4wave_plugin_registration.NewSRPCRegistrationServiceHandler(nil, ""),
	s4wave_plugin_registration.NewSRPCGenerationServiceHandler(nil, ""),
	s4wave_process.NewSRPCPersistentExecutionServiceHandler(nil, ""),
	s4wave_provider_local.NewSRPCLocalProviderResourceServiceHandler(nil, ""),
	s4wave_provider.NewSRPCProviderResourceServiceHandler(nil, ""),
	s4wave_provider_spacewave.NewSRPCSpacewaveProviderResourceServiceHandler(nil, ""),
	s4wave_quickstart_registry.NewSRPCQuickstartRegistryResourceServiceHandler(nil, ""),
	s4wave_root.NewSRPCRootResourceServiceHandler(nil, ""),
	s4wave_secret.NewSRPCSecretResourceServiceHandler(nil, ""),
	s4wave_session.NewSRPCLocalSessionResourceServiceHandler(nil, ""),
	s4wave_session.NewSRPCSessionResourceServiceHandler(nil, ""),
	s4wave_session.NewSRPCSharedObjectSelfEnrollmentResourceServiceHandler(nil, ""),
	s4wave_session.NewSRPCSpacewaveSessionResourceServiceHandler(nil, ""),
	s4wave_sobject.NewSRPCSharedObjectResourceServiceHandler(nil, ""),
	s4wave_space.NewSRPCSpaceResourceServiceHandler(nil, ""),
	s4wave_space.NewSRPCSpaceContentsResourceServiceHandler(nil, ""),
	s4wave_sql_query_result.NewSRPCSqlQueryResultResourceServiceHandler(nil, ""),
	s4wave_sql_query.NewSRPCSqlQueryResourceServiceHandler(nil, ""),
	s4wave_sql_schema.NewSRPCSqlSchemaResourceServiceHandler(nil, ""),
	s4wave_sql_table_view.NewSRPCSqlTableViewResourceServiceHandler(nil, ""),
	s4wave_sql_workbench.NewSRPCSqlWorkbenchResourceServiceHandler(nil, ""),
	s4wave_sshhost.NewSRPCSshHostResourceServiceHandler(nil, ""),
	s4wave_status.NewSRPCSystemStatusServiceHandler(nil, ""),
	s4wave_terminal.NewSRPCTerminalResourceServiceHandler(nil, ""),
	s4wave_testbed.NewSRPCTestbedResourceServiceHandler(nil, ""),
	s4wave_trace.NewSRPCTraceServiceHandler(nil, ""),
	s4wave_unixfs.NewSRPCFSHandleResourceServiceHandler(nil, ""),
	s4wave_viewer_registry.NewSRPCViewerRegistryResourceServiceHandler(nil, ""),
	s4wave_vm.NewSRPCV86RuntimeStatusServiceHandler(nil, ""),
	s4wave_wizard.NewSRPCObjectWizardRegistryResourceServiceHandler(nil, ""),
	s4wave_wizard.NewSRPCWizardResourceServiceHandler(nil, ""),
	s4wave_world.NewSRPCEngineResourceServiceHandler(nil, ""),
	s4wave_world.NewSRPCWorldStageResourceServiceHandler(nil, ""),
	s4wave_world.NewSRPCWorldStateResourceServiceHandler(nil, ""),
	s4wave_world.NewSRPCWatchWorldStateResourceServiceHandler(nil, ""),
	s4wave_world.NewSRPCTxResourceServiceHandler(nil, ""),
	s4wave_world.NewSRPCObjectIteratorResourceServiceHandler(nil, ""),
	s4wave_world.NewSRPCGraphPathQueryResourceServiceHandler(nil, ""),
	s4wave_world.NewSRPCObjectStateResourceServiceHandler(nil, ""),
	s4wave_world.NewSRPCTypedObjectResourceServiceHandler(nil, ""),
	s4wave_worldop_registry.NewSRPCWorldOpRegistryResourceServiceHandler(nil, ""),
}

// pluginCallerAllowed lists, by service ID, the methods a plugin outside the
// host set may call on the root Resource tree. Every method of
// rootTreeHandlers is here or on the deny list, never both.
var pluginCallerAllowed = map[string][]string{
	s4wave_account.SRPCAccountResourceServiceServiceID: {
		"WatchAccountInfo", "WatchAuthMethods", "WatchSessions",
		"WatchKeybindingOverrides", "ReplaceKeybindingOverrideSet",
		"WatchEntityKeypairs",
	},
	s4wave_block_cursor.SRPCBlockCursorResourceServiceServiceID: {
		"Fetch", "SetBlock", "FollowRef", "GetRef", "IsDirty", "MarkDirty",
		"GetBlock", "Unmarshal", "IsSubBlock", "FollowSubBlock", "SetAsSubBlock",
		"ClearRef", "ClearAllRefs", "SetRef", "GetExistingRef", "GetAllRefs",
		"Detach", "DetachTransaction", "DetachRecursive", "Parents",
	},
	s4wave_block_transaction.SRPCBlockTransactionResourceServiceServiceID: {
		"Write", "GetRootCursor",
	},
	s4wave_bucket_lookup.SRPCBucketLookupCursorResourceServiceServiceID: {
		"GetRef", "FollowRef", "GetBlock", "PutBlock", "PutBlockBatch",
		"GetBlockExistsBatch", "BuildTransaction", "BuildTransactionAtRef", "Clone",
		"Release", "Unmarshal",
	},
	s4wave_canvas.SRPCCanvasResourceServiceServiceID: {
		"GetCanvasState", "UpdateCanvas", "WatchCanvasState",
	},
	s4wave_cdn.SRPCCdnResourceServiceServiceID: {
		"GetCdnSpaceId", "MountCdnSpace", "CopyV86ImageToSpace",
	},
	spacewave_chat_rpc.SRPCChatResourceServiceServiceID: {
		"GetChannelInfo", "WatchChannelInfo", "GetState", "GetMessage",
		"ListMessages", "ListThreads", "WatchMessages", "SendMessage",
		"AttachEnvelope", "GetReadPositions", "UpdateReadPosition",
	},
	s4wave_command_registry.SRPCCommandRegistryResourceServiceServiceID: {
		"RegisterCommand", "SetActive", "SetEnabled", "WatchCommands", "GetSubItems",
		"InvokeCommand",
	},
	s4wave_configtype_registry.SRPCConfigTypeRegistryResourceServiceServiceID: {
		"RegisterConfigType", "WatchConfigTypes",
	},
	s4wave_debugdb.SRPCDebugDbResourceServiceServiceID: {
		"GetStorageInfo", "StartBenchmark",
	},
	s4wave_debugdb.SRPCDebugDbBenchmarkServiceServiceID: {
		"WatchProgress", "GetResults",
	},
	s4wave_device.SRPCDeviceResourceServiceServiceID: {
		"WatchDeviceState", "WatchDeviceAvailability", "ReportDeviceStatus",
		"AccessCheckoutRoot",
	},
	s4wave_flowgraph.SRPCFlowgraphResourceServiceServiceID: {
		"OpenNodePort", "GetFlowgraph", "UpdateFlowgraph", "WatchFlowgraph",
		"ListFlowgraphChanges",
	},
	flowgraph_registry.SRPCNodeTypeRegistryResourceServiceServiceID: {
		"RegisterNodeType",
	},
	s4wave_git.SRPCGitRepoResourceServiceServiceID: {
		"ListRefs", "ResolveRef", "GetRepoInfo", "GetTreeResource",
		"GetRepoFilesystemResource", "Log", "GetCommit", "GetDiffStat",
		"GetDiffPatch",
	},
	s4wave_git.SRPCGitWorktreeResourceServiceServiceID: {
		"GetWorktreeInfo", "GetRepoResource", "GetWorkdirResource", "WatchStatus",
		"StageFiles", "UnstageFiles", "CommitFiles",
	},
	s4wave_layout.SRPCLayoutHostServiceID: {
		"WatchLayoutModel", "NavigateTab", "ReplaceTab", "AddTab",
	},
	s4wave_objecttype_registry.SRPCObjectTypeRegistryResourceServiceServiceID: {
		"RegisterObjectType", "WatchObjectTypes",
	},
	s4wave_org.SRPCOrgResourceServiceServiceID: {
		"WatchOrgState",
	},
	s4wave_plugin_registration.SRPCRegistrationServiceServiceID: {
		"Prepare",
	},
	s4wave_plugin_registration.SRPCGenerationServiceServiceID: {
		"Activate",
	},
	s4wave_process.SRPCPersistentExecutionServiceServiceID: {
		"Execute",
	},
	s4wave_provider_local.SRPCLocalProviderResourceServiceServiceID: {
		"CreateAccount", "AttachAccount", "CompleteSpaceLinkEnrollment",
	},
	s4wave_provider.SRPCProviderResourceServiceServiceID: {
		"GetProviderInfo", "AccessProviderAccount",
	},
	s4wave_provider_spacewave.SRPCSpacewaveProviderResourceServiceServiceID: {
		"GetCloudProviderConfig", "GetLinkedCloudSession",
		"GenerateAuthKeypairs", "WrapPemWithPin", "UnwrapPemWithPin",
		"GeneratePasskeyPrfSalt", "WrapWithPasskeyPrf", "UnwrapWithPasskeyPrf",
		"CreateAccount", "LoginAccount", "LoginOrCreateAccount",
		"LoginWithEntityKey", "MountLinkedDeviceSession", "StartBrowserHandoff",
		"SSOCodeExchange", "PrepareBrowserSSO", "SSONonceExchange",
		"StartDesktopSSO", "ConfirmDesktopSSO", "PasskeyCheckUsername",
		"PasskeyRegisterChallenge", "PasskeyConfirmSignup",
		"PasskeyAuthOptions", "PasskeyAuthVerify", "StartDesktopPasskey",
		"ConfirmDesktopPasskey", "RelayDesktopPasskey",
	},
	s4wave_quickstart_registry.SRPCQuickstartRegistryResourceServiceServiceID: {
		"RegisterQuickstart", "ListQuickstarts", "WatchQuickstarts",
		"ExecuteQuickstart",
	},
	s4wave_root.SRPCRootResourceServiceServiceID: {
		"MountApp", "ListProviders", "LookupProvider", "MountSession",
		"MountSessionByIdx", "ListSessions", "WatchSessions",
		"WatchAllAccountStatuses", "GetSessionMetadata", "WatchSessionMetadata",
		"AccessStateAtom", "WatchStateAtoms", "ListSpaceRootAliases",
		"WatchSpaceRootAliases", "WatchSpaceRootRuntime", "MarshalHash",
		"ParseHash", "HashSum", "HashValidate", "GetChangelog", "GatherMetrics",
		"GetDebugDb", "GetCdn", "AccessWebListener", "WatchWebListeners",
		"WatchListenerYieldPrompts", "WatchRuntimeHandoff",
		"WatchListenerStatus",
	},
	s4wave_secret.SRPCSecretResourceServiceServiceID: {
		"WatchState", "BeginReadPayload", "ReadPayload",
	},
	s4wave_session.SRPCLocalSessionResourceServiceServiceID: {
		"SetDisplayName", "WatchDisplayName",
	},
	s4wave_session.SRPCSessionResourceServiceServiceID: {
		"GetSessionInfo", "WatchResourcesList", "CreateSpace",
		"MountSharedObject", "WatchSharedObjectHealth", "WatchSyncStatus",
		"WatchStorageStats", "RenameSpace", "WatchLockState", "GetSASEmoji",
		"AccessStateAtom", "AccessPeerTransport", "WatchStateAtoms",
		"GetTransferInventory", "WatchTransferProgress", "GetTransferStatus",
		"WatchPairedDevices", "WatchPairingStatus", "ListSpaceInvites",
		"ListSpaceParticipants", "WithdrawSpaceJoinRequest",
		"EnsureDeveloperSpace", "WatchPluginApprovals", "WatchStorageBackends",
		"CheckStorageBackend", "WatchSpaceStorage",
	},
	s4wave_session.SRPCSharedObjectSelfEnrollmentResourceServiceServiceID: {
		"WatchState",
	},
	s4wave_session.SRPCSpacewaveSessionResourceServiceServiceID: {
		"WatchOnboardingStatus", "GetLinkedLocalSession",
		"WatchSubscriptionStatus", "WatchBillingState", "WatchCheckoutStatus",
		"RefreshBillingState", "ListManagedBillingAccounts",
		"WatchOrganizations", "CreateOrganization", "WatchOrganizationState",
		"ResolveUsername", "ListTargetedInvitations",
		"WatchTargetedInvitations", "GetTargetedInvitation",
		"RepairSharedObject", "GetSpacePublicOrigin",
		"MountSharedObjectSelfEnrollment", "WatchEmails", "LookupInviteCode",
		"PreviewSpaceLink", "GetAdminJson",
	},
	s4wave_sobject.SRPCSharedObjectResourceServiceServiceID: {
		"WatchSharedObjectHealth", "WatchSharedObjectParticipation",
		"OpenReadCheckpoint", "MountSharedObjectBody", "GetSharedObjectTrim",
		"InspectSharedObjectOrder",
	},
	s4wave_space.SRPCSpaceResourceServiceServiceID: {
		"WatchSpaceState", "WatchProcessBindings", "WatchSpaceSharingState",
		"AccessWorld", "MountSpaceContents", "HoldNodeTypePlugins",
		"FetchPluginRepository", "ValidatePluginRepository", "OpenPluginFrontend",
		"SetSpaceBackfill", "WatchSpaceBackfill",
	},
	s4wave_space.SRPCSpaceContentsResourceServiceServiceID: {
		"WatchState", "SetProcessBinding", "BindAttachedRpcService",
	},
	s4wave_sql_query_result.SRPCSqlQueryResultResourceServiceServiceID: {
		"GetResultGrid",
	},
	s4wave_sql_query.SRPCSqlQueryResourceServiceServiceID: {
		"Initialize", "GetQueryText", "SetQueryText", "SetParameters", "Run",
	},
	s4wave_sql_schema.SRPCSqlSchemaResourceServiceServiceID: {
		"GetSchema", "ListTables",
	},
	s4wave_sql_table_view.SRPCSqlTableViewResourceServiceServiceID: {
		"GetTableView", "GetDriverCapability", "FetchRows", "UpdateRow",
	},
	s4wave_sql_workbench.SRPCSqlWorkbenchResourceServiceServiceID: {
		"Initialize", "GetWorkbench", "AddPin", "RemovePin", "SetLayout",
	},
	s4wave_sshhost.SRPCSshHostResourceServiceServiceID: {
		"WatchSshHostState",
	},
	s4wave_status.SRPCSystemStatusServiceServiceID: {
		"WatchControllers", "WatchDirectives", "WatchPlugins", "WatchNetworkStats",
		"ReportRecoveryStatus", "WatchRecoveryStatus",
	},
	s4wave_terminal.SRPCTerminalResourceServiceServiceID: {
		"WatchTerminalState", "ConnectTerminal",
	},
	s4wave_testbed.SRPCTestbedResourceServiceServiceID: {
		"CreateWorld", "MarkTestResult", "AccessStateAtom",
	},
	s4wave_trace.SRPCTraceServiceServiceID: {
		"StartTrace", "StopTrace", "CaptureCPUProfile", "CaptureMemoryProfile",
		"CaptureGoroutineProfile",
	},
	s4wave_unixfs.SRPCFSHandleResourceServiceServiceID: {
		"Lookup", "LookupPath", "ReadAt", "ReadStream", "WriteAt", "Truncate", "Sync",
		"GetSize", "GetFileInfo", "GetNodeType", "Readdir", "Mknod", "Remove",
		"MkdirAll", "Rename", "Clone", "Readlink", "UploadFile", "UploadTree",
		"WatchReaddir", "WatchFiles",
	},
	s4wave_viewer_registry.SRPCViewerRegistryResourceServiceServiceID: {
		"RegisterViewer", "ListViewers", "WatchViewers",
	},
	s4wave_vm.SRPCV86RuntimeStatusServiceServiceID: {
		"ReportStatus",
	},
	s4wave_wizard.SRPCObjectWizardRegistryResourceServiceServiceID: {
		"RegisterWizard", "ListWizards", "WatchWizards",
	},
	s4wave_wizard.SRPCWizardResourceServiceServiceID: {
		"WatchWizardState", "UpdateWizardState", "StartGitClone",
		"WatchGitCloneProgress",
	},
	s4wave_world.SRPCEngineResourceServiceServiceID: {
		"GetEngineInfo", "GetWorldRootSnapshot", "WatchWorldRootSnapshots",
		"SetRetainedRoot", "NewTransaction", "ExecuteWorldOp", "ExecuteWorldBatch",
		"Sync", "SyncThrough", "GetSeqno", "WaitSeqno", "WatchChanges",
		"WaitObjectRev", "BuildStorageCursor", "AccessWorldState", "StageWorldState",
		"WatchTypeCounts", "SearchObjectsWithType",
	},
	s4wave_world.SRPCWorldStageResourceServiceServiceID: {
		"BuildStorageCursor", "AccessWorldState", "ReleaseRoots",
	},
	s4wave_world.SRPCWorldStateResourceServiceServiceID: {
		"CompareObjectRecords", "GetReadOnly", "Sync", "GetSeqno", "WaitSeqno",
		"BuildStorageCursor", "AccessWorldState", "StageWorldState",
		"OpenNestedWorld", "OpenOuterWorld", "CreateObject", "GetObject",
		"IterateObjects", "ListObjects", "RenameObject", "DeleteObject",
		"SetGraphQuad", "DeleteGraphQuad", "LookupGraphQuads",
		"LookupGraphQuadsBatch", "ListGraphEdgeBuckets", "ListObjectsWithType",
		"GetObjectRootRefsBatch", "GetObjectMetadataBatch", "GetObjectBodiesBatch",
		"QueryGraphPath", "DeleteGraphObject", "ApplyWorldOp", "ListTypeCounts",
		"WatchTypeCounts", "SearchObjectsWithType",
	},
	s4wave_world.SRPCWatchWorldStateResourceServiceServiceID: {
		"WatchWorldState",
	},
	s4wave_world.SRPCTxResourceServiceServiceID: {
		"CommitTx", "Discard",
	},
	s4wave_world.SRPCObjectIteratorResourceServiceServiceID: {
		"Err", "Valid", "Key", "Next", "Seek", "Close",
	},
	s4wave_world.SRPCGraphPathQueryResourceServiceServiceID: {
		"Next", "Close",
	},
	s4wave_world.SRPCObjectStateResourceServiceServiceID: {
		"GetKey", "GetRootRef", "GetModSeqno", "SetRootRef", "AccessWorldState",
		"ApplyObjectOp", "IncrementRev", "WaitRev",
	},
	s4wave_world.SRPCTypedObjectResourceServiceServiceID: {
		"AccessTypedObject", "WatchTypedObject",
	},
	s4wave_worldop_registry.SRPCWorldOpRegistryResourceServiceServiceID: {
		"RegisterWorldOp", "WatchWorldOps",
	},
}
