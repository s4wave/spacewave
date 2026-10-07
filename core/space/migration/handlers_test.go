package space_migration_test

import (
	"context"
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
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	s4wave_unixfs_world "github.com/s4wave/spacewave/sdk/unixfs/world"
)

func TestBuiltInHandlersDecodeAndSerializePopulatedWorldPayloads(t *testing.T) {
	// Open a World testbed for built-in payload inspection and rewriting.
	ctx := context.Background()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Define and store one payload for each built-in ObjectType.
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
		{"canvas", s4wave_canvas_world.CanvasTypeID, s4wave_canvas.NewCanvasStorage()},
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
		{"device", s4wave_device.DeviceTypeID, s4wave_device.NewDeviceBlock()},
		{"flowgraph", s4wave_flowgraph.FlowgraphTypeID, s4wave_flowgraph.NewFlowgraphBlock()},
		{"terminal", s4wave_terminal.TerminalTypeID, s4wave_terminal.NewTerminalBlock()},
		{"ssh-host", s4wave_sshhost.SshHostTypeID, s4wave_sshhost.NewSshHostBlock()},
		{"secret", s4wave_secret.SecretTypeID, s4wave_secret.NewSecretBlock()},
	}
	for _, fixture := range fixtures {
		setObjectBlock(t, ctx, tb.WorldState, fixture.key, fixture.typeID, fixture.payload)
	}

	// Construct the built-in registry and identity mappings.
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}
	mapping := space_migration.NewIdentityMap()
	for _, fixture := range fixtures {
		mapping.ObjectKeys[fixture.key] = fixture.key
	}

	// Inspect and rewrite every stored built-in payload.
	for _, fixture := range fixtures {
		// Require the built-in handler for this fixture ObjectType.
		handler := registry.Lookup(fixture.typeID)
		if handler == nil {
			t.Fatalf("handler missing for %s", fixture.typeID)
		}

		// Verify the built-in handler can inspect the fixture payload.
		object := &space_migration.ObjectDescriptor{ObjectKey: fixture.key, ObjectType: fixture.typeID, World: tb.WorldState}
		if _, err := handler.Inspect(ctx, object); err != nil {
			t.Fatalf("Inspect(%s): %v", fixture.typeID, err)
		}

		// Rewrite the fixture payload through its built-in handler.
		result, err := handler.Rewrite(ctx, object, mapping)
		if err != nil {
			t.Fatalf("Rewrite(%s): %v", fixture.typeID, err)
		}

		// Verify the fixture rewrite returns a result.
		if result == nil {
			t.Fatalf("Rewrite(%s) returned no result", fixture.typeID)
		}
	}
}

func TestForgeHandlersOwnPopulatedIdentities(t *testing.T) {
	// Open a World testbed for Forge identity inspection and rewriting.
	ctx := context.Background()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Define populated Forge fixtures and store their payloads.
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

	// Construct the Forge registry and destination identity mappings.
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

	// Inspect and rewrite every Forge fixture through its registered handler.
	for _, fixture := range fixtures {
		// Inspect the Forge fixture and require the expected source references.
		handler := registry.Lookup(fixture.typeID)
		object := &space_migration.ObjectDescriptor{ObjectKey: fixture.key, ObjectType: fixture.typeID, World: tb.WorldState}
		inspection, err := handler.Inspect(ctx, object)
		if err != nil {
			t.Fatalf("Inspect(%s): %v", fixture.typeID, err)
		}

		// Verify the Forge inspection preserves all source identities.
		assertForgeReferences(t, fixture.typeID+" inspect", inspection.References, fixture.before)

		// Rewrite the Forge fixture with the destination mappings.
		result, err := handler.Rewrite(ctx, object, mapping)
		if err != nil {
			t.Fatalf("Rewrite(%s): %v", fixture.typeID, err)
		}

		// Verify the Forge rewrite reports the expected destination identities.
		assertForgeReferences(t, fixture.typeID+" rewrite", result.References, fixture.after)
	}
}

func TestFlowgraphHandlerRewritesTargetAndPlacementIdentities(t *testing.T) {
	// Store a Flowgraph whose Step Target reads an object and an object snapshot.
	ctx := context.Background()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	graph := &s4wave_flowgraph.Flowgraph{
		Name: "graph",
		Nodes: map[string]*s4wave_flowgraph.FlowgraphNode{
			"step": {
				TypeId: s4wave_flowgraph.StepNodeTypeID,
				Step: &s4wave_flowgraph.FlowgraphStep{
					Prompt: "unchanged",
					Target: &forge_target.Target{
						Inputs: []*forge_target.Input{
							{
								Name:        "object",
								InputType:   forge_target.InputType_InputType_WORLD_OBJECT,
								WorldObject: &forge_target.InputWorldObject{ObjectKey: "input-object"},
							},
							{
								Name:      "snapshot",
								InputType: forge_target.InputType_InputType_VALUE,
								Value: &forge_value.Value{
									ValueType: forge_value.ValueType_ValueType_WORLD_OBJECT_SNAPSHOT,
									WorldObjectSnapshot: &forge_value.WorldObjectSnapshot{
										Key: "snapshot-object", RootRef: &bucket.ObjectRef{BucketId: "source-store"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	setObjectBlock(t, ctx, tb.WorldState, "graph", s4wave_flowgraph.FlowgraphTypeID, graph)

	// Describe the graph with placement edges on a Device and an actor.
	object := &space_migration.ObjectDescriptor{
		ObjectKey:  "graph",
		ObjectType: s4wave_flowgraph.FlowgraphTypeID,
		World:      tb.WorldState,
		GraphReferences: []string{
			"<graph>", s4wave_flowgraph.DevicePlacementPredicate, "<device>", `"tcp"`,
			"<graph>", s4wave_flowgraph.ActorPlacementPredicate, "<actor>", `"step"`,
		},
	}
	registry, err := space_migration.BuiltInRegistry()
	if err != nil {
		t.Fatal(err)
	}
	handler := registry.Lookup(s4wave_flowgraph.FlowgraphTypeID)

	// Verify the inspection reports the Target and placement identities.
	inspection, err := handler.Inspect(ctx, object)
	if err != nil {
		t.Fatalf("Inspect(flowgraph): %v", err)
	}
	var objectKeys []space_migration.TypedReference
	for _, reference := range inspection.References {
		if reference.Kind != space_migration.ReferenceGraphIRI {
			objectKeys = append(objectKeys, reference)
		}
	}
	assertForgeReferences(t, "flowgraph inspect", objectKeys, []space_migration.TypedReference{
		{Kind: space_migration.ReferenceObjectKey, Value: "input-object"},
		{Kind: space_migration.ReferenceObjectKey, Value: "snapshot-object"},
		{Kind: space_migration.ReferenceBlockStore, Value: "source-store"},
		{Kind: space_migration.ReferenceObjectKey, Value: "device"},
		{Kind: space_migration.ReferenceObjectKey, Value: "actor"},
	})

	// Rewrite the graph with destination identities.
	mapping := space_migration.NewIdentityMap()
	mapping.BlockStoreIDs["source-store"] = "destination-store"
	for _, key := range []string{"input-object", "snapshot-object", "device", "actor"} {
		mapping.ObjectKeys[key] = key + "-destination"
	}
	result, err := handler.Rewrite(ctx, object, mapping)
	if err != nil {
		t.Fatalf("Rewrite(flowgraph): %v", err)
	}

	// Verify the rewrite reports the destination identities.
	assertForgeReferences(t, "flowgraph rewrite", result.References, []space_migration.TypedReference{
		{Kind: space_migration.ReferenceObjectKey, Value: "input-object-destination"},
		{Kind: space_migration.ReferenceObjectKey, Value: "snapshot-object-destination"},
		{Kind: space_migration.ReferenceBlockStore, Value: "destination-store"},
		{Kind: space_migration.ReferenceObjectKey, Value: "device-destination"},
		{Kind: space_migration.ReferenceObjectKey, Value: "actor-destination"},
	})

	// Verify the payload carries the destination identities and keeps the rest.
	var rewritten s4wave_flowgraph.Flowgraph
	if err := rewritten.UnmarshalVT(result.Payload); err != nil {
		t.Fatal(err)
	}
	step := rewritten.GetNodes()["step"].GetStep()
	inputs := step.GetTarget().GetInputs()
	if got := inputs[0].GetWorldObject().GetObjectKey(); got != "input-object-destination" {
		t.Fatalf("rewritten input object key = %q", got)
	}
	snapshot := inputs[1].GetValue().GetWorldObjectSnapshot()
	if snapshot.GetKey() != "snapshot-object-destination" || snapshot.GetRootRef().GetBucketId() != "destination-store" {
		t.Fatalf("rewritten snapshot = %v", snapshot)
	}
	if rewritten.GetName() != "graph" || step.GetPrompt() != "unchanged" {
		t.Fatalf("rewrite changed unreferenced fields: %v", &rewritten)
	}
}

func TestGitWorktreeLinksAreOwnedThroughRegistryAndPlanner(t *testing.T) {
	// Open source and destination Worlds for Git worktree migration.
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

	// Populate Git repository, worktree, and workdir objects with graph links.
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

	// Construct the registry and Git worktree descriptor for inspection.
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

	// Inspect the Git worktree through its registered handler.
	inspection, err := registry.Inspect(ctx, worktree)
	if err != nil {
		t.Fatalf("Registry.Inspect(worktree): %v", err)
	}

	// Verify the Git worktree inspection owns repository and workdir links.
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

	// Rewrite the Git worktree using destination object keys.
	mapping := space_migration.NewIdentityMap()
	mapping.ObjectKeys["repo"] = "repo-destination"
	mapping.ObjectKeys["workdir"] = "workdir-destination"
	rewrite, err := registry.Lookup(git_world.GitWorktreeTypeID).Rewrite(ctx, worktree, mapping)
	if err != nil {
		t.Fatalf("Registry.Rewrite(worktree): %v", err)
	}

	// Verify the Git worktree rewrite reports mapped object references.
	assertForgeReferences(t, "worktree rewrite", rewrite.References, []space_migration.TypedReference{
		{Kind: space_migration.ReferenceObjectKey, Value: "repo-destination"},
		{Kind: space_migration.ReferenceObjectKey, Value: "workdir-destination"},
	})

	// Plan migration of the Git worktree and its dependency closure.
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

	// Verify the planned Git closure contains all three objects and no blockers.
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
