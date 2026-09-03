package space_migration_test

import (
	"context"
	"slices"
	"testing"

	forge_dashboard "github.com/s4wave/spacewave/core/forge/dashboard"
	space_migration "github.com/s4wave/spacewave/core/space/migration"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	world_db "github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_value "github.com/s4wave/spacewave/forge/value"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
	s4wave_canvas_world "github.com/s4wave/spacewave/sdk/canvas/world"
	s4wave_chat "github.com/s4wave/spacewave/sdk/chat"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	s4wave_unixfs_world "github.com/s4wave/spacewave/sdk/unixfs/world"
)

func TestChatMigrationRemapsLinksReplyAndReceiptKeys(t *testing.T) {
	ctx := context.Background()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	ws := tb.WorldState

	createChannel := func(key string) {
		setObjectBlock(t, ctx, ws, key, s4wave_chat.ChatChannelTypeID, &s4wave_chat.ChatChannel{Name: key})
	}
	createChannel("ch")
	createChannel("target")
	setObjectBlock(t, ctx, ws, "msg", s4wave_chat.ChatMessageTypeID, &s4wave_chat.ChatMessage{
		SenderPeerId:     "peer-a",
		ReplyToKey:       "ch",
		LinkedObjectKeys: []string{"target", "target"},
	})
	setObjectBlock(t, ctx, ws, "receipt", s4wave_chat.ChatMessageReceiptTypeID, &s4wave_chat.ChatMessageReceipt{
		SenderPeerId:    "peer-a",
		ClientMessageId: "m1",
		MessageKey:      "msg",
	})
	if err := ws.SetGraphQuad(ctx, world_db.NewGraphQuadWithKeys("msg", s4wave_chat.PredChannelMessage.String(), "ch", "")); err != nil {
		t.Fatalf("SetGraphQuad(channel message): %v", err)
	}
	if err := ws.SetGraphQuad(ctx, world_db.NewGraphQuadWithKeys("msg", s4wave_chat.PredMessageLink.String(), "target", "")); err != nil {
		t.Fatalf("SetGraphQuad(message link): %v", err)
	}

	mapping := space_migration.NewIdentityMap()
	for _, pair := range [][2]string{{"ch", "new-ch"}, {"target", "new-target"}, {"msg", "new-msg"}, {"receipt", "new-receipt"}} {
		mapping.ObjectKeys[pair[0]] = pair[1]
	}
	for _, pair := range [][2]string{{"msg", "<new-msg>"}, {"ch", "<new-ch>"}, {"target", "<new-target>"}} {
		mapping.GraphIRIs[pair[0]] = pair[1]
	}

	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}

	msgHandler := registry.Lookup(s4wave_chat.ChatMessageTypeID)
	if msgHandler == nil {
		t.Fatal("handler missing for chat message")
	}
	// Mirror the read-only scan: quads arrive as subject/predicate/object/label groups.
	msgObject := &space_migration.ObjectDescriptor{
		ObjectKey: "msg", ObjectType: s4wave_chat.ChatMessageTypeID, World: ws,
		GraphReferences: []string{
			"msg", s4wave_chat.PredChannelMessage.String(), "ch", "",
			"msg", s4wave_chat.PredMessageLink.String(), "target", "",
		},
	}
	if _, err := msgHandler.Inspect(ctx, msgObject); err != nil {
		t.Fatalf("Inspect(message): %v", err)
	}
	msgResult, err := msgHandler.Rewrite(ctx, msgObject, mapping)
	if err != nil {
		t.Fatalf("Rewrite(message): %v", err)
	}
	var gotMsg s4wave_chat.ChatMessage
	if err := gotMsg.UnmarshalVT(msgResult.Payload); err != nil {
		t.Fatalf("unmarshal rewritten message: %v", err)
	}
	if gotMsg.GetReplyToKey() != "new-ch" {
		t.Fatalf("rewritten reply_to_key = %q, want new-ch", gotMsg.GetReplyToKey())
	}
	if gotLinks := gotMsg.GetLinkedObjectKeys(); len(gotLinks) != 1 || gotLinks[0] != "new-target" {
		t.Fatalf("rewritten linked_object_keys = %v, want canonical [new-target]", gotLinks)
	}
	if gotMsg.GetSenderPeerId() != "peer-a" {
		t.Fatalf("rewritten sender = %q, want peer-a preserved", gotMsg.GetSenderPeerId())
	}
	wantGraph := []string{
		"<new-msg>", s4wave_chat.PredChannelMessage.String(), "<new-ch>", "",
		"<new-msg>", s4wave_chat.PredMessageLink.String(), "<new-target>", "",
	}
	if !slices.Equal(msgResult.GraphReferences, wantGraph) {
		t.Fatalf("rewritten graph references = %v, want %v", msgResult.GraphReferences, wantGraph)
	}

	receiptHandler := registry.Lookup(s4wave_chat.ChatMessageReceiptTypeID)
	if receiptHandler == nil {
		t.Fatal("handler missing for chat message receipt")
	}
	receiptObject := &space_migration.ObjectDescriptor{
		ObjectKey: "receipt", ObjectType: s4wave_chat.ChatMessageReceiptTypeID, World: ws,
	}
	receiptInspection, err := receiptHandler.Inspect(ctx, receiptObject)
	if err != nil {
		t.Fatalf("Inspect(receipt): %v", err)
	}
	wantRefs := []space_migration.TypedReference{
		{Kind: space_migration.ReferenceExternal, Value: "peer-a"},
		{Kind: space_migration.ReferenceObjectKey, Value: "msg"},
	}
	if !slices.EqualFunc(receiptInspection.References, wantRefs, func(a, b space_migration.TypedReference) bool {
		return a.Kind == b.Kind && a.Value == b.Value
	}) {
		t.Fatalf("receipt inspection references = %v, want %v", receiptInspection.References, wantRefs)
	}
	receiptResult, err := receiptHandler.Rewrite(ctx, receiptObject, mapping)
	if err != nil {
		t.Fatalf("Rewrite(receipt): %v", err)
	}
	var gotReceipt s4wave_chat.ChatMessageReceipt
	if err := gotReceipt.UnmarshalVT(receiptResult.Payload); err != nil {
		t.Fatalf("unmarshal rewritten receipt: %v", err)
	}
	if gotReceipt.GetMessageKey() != "new-msg" {
		t.Fatalf("rewritten receipt message_key = %q, want new-msg", gotReceipt.GetMessageKey())
	}
	if gotReceipt.GetSenderPeerId() != "peer-a" || gotReceipt.GetClientMessageId() != "m1" {
		t.Fatalf(
			"rewritten receipt identity = (%q, %q), want (peer-a, m1) preserved",
			gotReceipt.GetSenderPeerId(), gotReceipt.GetClientMessageId(),
		)
	}
}

func TestBuiltInHandlersDecodeAndSerializePopulatedWorldPayloads(t *testing.T) {
	ctx := context.Background()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	fixtures := []struct {
		key     string
		typeID  string
		payload block.Block
	}{
		{"settings", "github.com/s4wave/spacewave/core/space/world.SpaceSettings", space_world.NewSpaceSettingsBlock()},
		{"layout", s4wave_layout_world.ObjectLayoutTypeID, s4wave_layout_world.NewObjectLayoutBlock()},
		{"unixfs", s4wave_unixfs_world.UnixFSTypeID, unixfs_block.NewFSNodeBlock()},
		{"git-repo", git_world.GitRepoTypeID, git_block.NewRepo()},
		{"git-worktree", git_world.GitWorktreeTypeID, git_world.NewWorktreeBlock()},
		{"canvas", s4wave_canvas_world.CanvasTypeID, &s4wave_canvas.CanvasState{}},
		{"kv", s4wave_kv_world.KvStoreTypeID, kvtx_block.NewKeyValueStore(0)},
		{"cluster", forge_cluster.ClusterTypeID, forge_cluster.NewClusterBlock()},
		{"job", forge_job.JobTypeID, forge_job.NewJobBlock()},
		{"task", forge_task.TaskTypeID, forge_task.NewTaskBlock()},
		{"pass", forge_pass.PassTypeID, forge_pass.NewPassBlock()},
		{"execution", forge_execution.ExecutionTypeID, forge_execution.NewExecutionBlock()},
		{"worker", forge_worker.WorkerTypeID, forge_worker.NewWorkerBlock()},
		{"dashboard", forge_dashboard.ForgeDashboardTypeID, &forge_dashboard.ForgeDashboard{}},
		{"channel", s4wave_chat.ChatChannelTypeID, s4wave_chat.NewChatChannelBlock()},
		{"message", s4wave_chat.ChatMessageTypeID, s4wave_chat.NewChatMessageBlock()},
		{"message-receipt", s4wave_chat.ChatMessageReceiptTypeID, s4wave_chat.NewChatMessageReceiptBlock()},
		{"device", s4wave_device.DeviceTypeID, s4wave_device.NewDeviceBlock()},
		{"terminal", s4wave_terminal.TerminalTypeID, s4wave_terminal.NewTerminalBlock()},
		{"ssh-host", s4wave_sshhost.SshHostTypeID, s4wave_sshhost.NewSshHostBlock()},
		{"secret", s4wave_secret.SecretTypeID, s4wave_secret.NewSecretBlock()},
	}
	for _, fixture := range fixtures {
		setObjectBlock(t, ctx, tb.WorldState, fixture.key, fixture.typeID, fixture.payload)
	}
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mapping := space_migration.NewIdentityMap()
	for _, fixture := range fixtures {
		mapping.ObjectKeys[fixture.key] = fixture.key
	}
	for _, fixture := range fixtures {
		handler := registry.Lookup(fixture.typeID)
		if handler == nil {
			t.Fatalf("handler missing for %s", fixture.typeID)
		}
		object := &space_migration.ObjectDescriptor{ObjectKey: fixture.key, ObjectType: fixture.typeID, World: tb.WorldState}
		if _, err := handler.Inspect(ctx, object); err != nil {
			t.Fatalf("Inspect(%s): %v", fixture.typeID, err)
		}
		result, err := handler.Rewrite(ctx, object, mapping)
		if err != nil {
			t.Fatalf("Rewrite(%s): %v", fixture.typeID, err)
		}
		if result == nil {
			t.Fatalf("Rewrite(%s) returned no result", fixture.typeID)
		}
	}
}

func TestForgeHandlersOwnPopulatedIdentities(t *testing.T) {
	ctx := context.Background()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	value := func(key, parent string) *forge_value.Value {
		return &forge_value.Value{
			ValueType: forge_value.ValueType_ValueType_WORLD_OBJECT_SNAPSHOT,
			WorldObjectSnapshot: &forge_value.WorldObjectSnapshot{
				Key: key, ObjectParent: parent, RootRef: &bucket.ObjectRef{BucketId: "source-store"},
			},
		}
	}
	fixtures := []struct {
		key, typeID string
		payload     block.Block
		before      []space_migration.TypedReference
		after       []space_migration.TypedReference
	}{
		{
			key: "cluster", typeID: forge_cluster.ClusterTypeID,
			payload: &forge_cluster.Cluster{PeerId: "cluster-peer"},
			before:  []space_migration.TypedReference{{Kind: space_migration.ReferenceExternal, Value: "cluster-peer"}},
			after:   []space_migration.TypedReference{{Kind: space_migration.ReferenceExternal, Value: "cluster-peer"}},
		},
		{
			key: "job", typeID: forge_job.JobTypeID,
			payload: &forge_job.Job{Result: &forge_value.Result{Success: true}},
		},
		{
			key: "task", typeID: forge_task.TaskTypeID,
			payload: &forge_task.Task{
				PeerId:   "task-peer",
				ValueSet: &forge_target.ValueSet{Inputs: []*forge_value.Value{value("task-object", "task-parent")}},
			},
			before: []space_migration.TypedReference{
				{Kind: space_migration.ReferenceExternal, Value: "task-peer"},
				{Kind: space_migration.ReferenceObjectKey, Value: "task-object"},
				{Kind: space_migration.ReferenceObjectKey, Value: "task-parent"},
				{Kind: space_migration.ReferenceBlockStore, Value: "source-store"},
			},
			after: []space_migration.TypedReference{
				{Kind: space_migration.ReferenceExternal, Value: "task-peer"},
				{Kind: space_migration.ReferenceObjectKey, Value: "task-object-destination"},
				{Kind: space_migration.ReferenceObjectKey, Value: "task-parent-destination"},
				{Kind: space_migration.ReferenceBlockStore, Value: "destination-store"},
			},
		},
		{
			key: "pass", typeID: forge_pass.PassTypeID,
			payload: &forge_pass.Pass{
				PeerId: "pass-peer",
				ExecStates: []*forge_pass.ExecState{{
					ObjectKey: "pass-execution", PeerId: "execution-peer",
					ValueSet: &forge_target.ValueSet{Outputs: []*forge_value.Value{value("pass-object", "pass-parent")}},
				}},
			},
			before: []space_migration.TypedReference{
				{Kind: space_migration.ReferenceExternal, Value: "pass-peer"},
				{Kind: space_migration.ReferenceObjectKey, Value: "pass-execution"},
				{Kind: space_migration.ReferenceExternal, Value: "execution-peer"},
				{Kind: space_migration.ReferenceObjectKey, Value: "pass-object"},
				{Kind: space_migration.ReferenceObjectKey, Value: "pass-parent"},
				{Kind: space_migration.ReferenceBlockStore, Value: "source-store"},
			},
			after: []space_migration.TypedReference{
				{Kind: space_migration.ReferenceExternal, Value: "pass-peer"},
				{Kind: space_migration.ReferenceObjectKey, Value: "pass-execution-destination"},
				{Kind: space_migration.ReferenceExternal, Value: "execution-peer"},
				{Kind: space_migration.ReferenceObjectKey, Value: "pass-object-destination"},
				{Kind: space_migration.ReferenceObjectKey, Value: "pass-parent-destination"},
				{Kind: space_migration.ReferenceBlockStore, Value: "destination-store"},
			},
		},
		{
			key: "execution", typeID: forge_execution.ExecutionTypeID,
			payload: &forge_execution.Execution{
				PeerId:   "execution-owner",
				ValueSet: &forge_target.ValueSet{Inputs: []*forge_value.Value{value("execution-object", "execution-parent")}},
			},
			before: []space_migration.TypedReference{
				{Kind: space_migration.ReferenceExternal, Value: "execution-owner"},
				{Kind: space_migration.ReferenceObjectKey, Value: "execution-object"},
				{Kind: space_migration.ReferenceObjectKey, Value: "execution-parent"},
				{Kind: space_migration.ReferenceBlockStore, Value: "source-store"},
			},
			after: []space_migration.TypedReference{
				{Kind: space_migration.ReferenceExternal, Value: "execution-owner"},
				{Kind: space_migration.ReferenceObjectKey, Value: "execution-object-destination"},
				{Kind: space_migration.ReferenceObjectKey, Value: "execution-parent-destination"},
				{Kind: space_migration.ReferenceBlockStore, Value: "destination-store"},
			},
		},
		{
			key: "worker", typeID: forge_worker.WorkerTypeID,
			payload: &forge_worker.Worker{Name: "worker-name"},
		},
		{
			key: "dashboard", typeID: forge_dashboard.ForgeDashboardTypeID,
			payload: &forge_dashboard.ForgeDashboard{Name: "dashboard-name"},
		},
	}
	for _, fixture := range fixtures {
		setObjectBlock(t, ctx, tb.WorldState, fixture.key, fixture.typeID, fixture.payload)
	}
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mapping := space_migration.NewIdentityMap()
	mapping.BlockStoreIDs["source-store"] = "destination-store"
	for _, fixture := range fixtures {
		mapping.ObjectKeys[fixture.key] = fixture.key + "-destination"
	}
	for _, key := range []string{"task-object", "task-parent", "pass-execution", "pass-object", "pass-parent", "execution-object", "execution-parent"} {
		mapping.ObjectKeys[key] = key + "-destination"
	}
	for _, fixture := range fixtures {
		handler := registry.Lookup(fixture.typeID)
		object := &space_migration.ObjectDescriptor{ObjectKey: fixture.key, ObjectType: fixture.typeID, World: tb.WorldState}
		inspection, err := handler.Inspect(ctx, object)
		if err != nil {
			t.Fatalf("Inspect(%s): %v", fixture.typeID, err)
		}
		assertForgeReferences(t, fixture.typeID+" inspect", inspection.References, fixture.before)
		result, err := handler.Rewrite(ctx, object, mapping)
		if err != nil {
			t.Fatalf("Rewrite(%s): %v", fixture.typeID, err)
		}
		assertForgeReferences(t, fixture.typeID+" rewrite", result.References, fixture.after)
	}
}

func TestGitWorktreeLinksAreOwnedThroughRegistryAndPlanner(t *testing.T) {
	ctx := context.Background()
	source, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Release()
	destination, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Release()

	setObjectBlock(t, ctx, source.WorldState, "repo", git_world.GitRepoTypeID, git_block.NewRepo())
	setObjectBlock(t, ctx, source.WorldState, "worktree", git_world.GitWorktreeTypeID, &git_world.Worktree{
		HeadRefStore: &git_world.HeadRefStore{SubmoduleName: "main"},
	})
	setObjectBlock(t, ctx, source.WorldState, "workdir", s4wave_unixfs_world.UnixFSTypeID, &unixfs_block.FSNode{
		NodeType: unixfs_block.NodeType_NodeType_FILE,
	})
	for _, quad := range []world_db.GraphQuad{
		world_db.NewGraphQuadWithKeys("worktree", git_world.GitRepoPred, "repo", ""),
		world_db.NewGraphQuadWithKeys("worktree", git_world.GitWorktreeWorkdirPred, "workdir", ""),
	} {
		if err := source.WorldState.SetGraphQuad(ctx, quad); err != nil {
			t.Fatal(err)
		}
	}

	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}
	worktree := &space_migration.ObjectDescriptor{
		ObjectKey:  "worktree",
		ObjectType: git_world.GitWorktreeTypeID,
		World:      source.WorldState,
		GraphReferences: []string{
			"<worktree>", git_world.GitRepoPred, "<repo>", "",
			"<worktree>", git_world.GitWorktreeWorkdirPred, "<workdir>", "",
		},
	}
	inspection, err := registry.Inspect(ctx, worktree)
	if err != nil {
		t.Fatalf("Registry.Inspect(worktree): %v", err)
	}
	assertForgeReferences(t, "worktree inspect", inspection.References, []space_migration.TypedReference{
		{Kind: space_migration.ReferenceObjectKey, Value: "repo"},
		{Kind: space_migration.ReferenceObjectKey, Value: "workdir"},
		{Kind: space_migration.ReferenceGraphIRI, Value: "<worktree>"},
		{Kind: space_migration.ReferenceGraphIRI, Value: git_world.GitRepoPred},
		{Kind: space_migration.ReferenceGraphIRI, Value: "<repo>"},
		{Kind: space_migration.ReferenceGraphIRI, Value: "<worktree>"},
		{Kind: space_migration.ReferenceGraphIRI, Value: git_world.GitWorktreeWorkdirPred},
		{Kind: space_migration.ReferenceGraphIRI, Value: "<workdir>"},
	})
	mapping := space_migration.NewIdentityMap()
	mapping.ObjectKeys["repo"] = "repo-destination"
	mapping.ObjectKeys["workdir"] = "workdir-destination"
	rewrite, err := registry.Lookup(git_world.GitWorktreeTypeID).Rewrite(ctx, worktree, mapping)
	if err != nil {
		t.Fatalf("Registry.Rewrite(worktree): %v", err)
	}
	assertForgeReferences(t, "worktree rewrite", rewrite.References, []space_migration.TypedReference{
		{Kind: space_migration.ReferenceObjectKey, Value: "repo-destination"},
		{Kind: space_migration.ReferenceObjectKey, Value: "workdir-destination"},
	})

	preview, err := space_migration.NewPlanner(registry).Plan(ctx, &space_migration.PlannerInput{
		SourceSpaceID:      "source-space",
		DestinationSpaceID: "destination-space",
		Source:             source.WorldState,
		Destination:        destination.WorldState,
		SelectedObjectKeys: []string{"worktree"},
	})
	if err != nil {
		t.Fatalf("Planner.Plan(worktree): %v", err)
	}
	if preview.GetProgress().GetObjectsPlanned() != 3 {
		t.Fatalf("planned closure = %d, want repo/worktree/workdir", preview.GetProgress().GetObjectsPlanned())
	}
	if len(preview.GetBlockers()) != 0 {
		t.Fatalf("planner blockers = %#v", preview.GetBlockers())
	}
}

func assertForgeReferences(t *testing.T, label string, got, want []space_migration.TypedReference) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s references = %#v, want %#v", label, got, want)
	}
	for index := range want {
		if got[index].Kind != want[index].Kind || got[index].Value != want[index].Value {
			t.Fatalf("%s references = %#v, want %#v", label, got, want)
		}
	}
}
