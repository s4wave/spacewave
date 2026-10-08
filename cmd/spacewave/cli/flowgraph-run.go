//go:build !js

package spacewave_cli

import (
	"context"
	"io"
	"os"
	"strconv"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	"github.com/s4wave/spacewave/net/peer"
	flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_run "github.com/s4wave/spacewave/sdk/flowgraph/run"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// flowgraphRunArgs selects a run and adapts its operations to the daemon.
type flowgraphRunArgs struct {
	flowgraphArgs
	// action selects the run operation.
	action string
	// cluster is the Cluster that receives the run's Job.
	cluster string
	// worker optionally restricts the Job to one Worker.
	worker string
	// workerPeer selects a Device peer linked to the placed Worker.
	workerPeer string
}

// BuildFlags returns the run command's daemon and placement flags.
func (a *flowgraphRunArgs) BuildFlags() []cli.Flag {
	return append(a.flowgraphArgs.BuildFlags(),
		&cli.StringFlag{Name: "cluster", Usage: "Cluster object key for a new run", Destination: &a.cluster},
		&cli.StringFlag{Name: "worker", Usage: "place a new run on this Worker object key", Destination: &a.worker},
		&cli.StringFlag{Name: "peer-id", Usage: "placed Worker peer (default: this session)", Destination: &a.workerPeer},
	)
}

// Run creates, resumes, or reads a run through the granting daemon session.
func (a *flowgraphRunArgs) Run(c *cli.Context) error {
	// Require one run key before mounting the session.
	if c.NArg() != 1 {
		return errors.New("one Flowgraph run object key is required")
	}
	key := c.Args().First()
	if a.action == "start" && (a.cluster == "" || a.key == "") {
		return errors.New("start requires --cluster and --flowgraph")
	}

	// Read-only commands borrow the selected World engine.
	if a.action == "show" || a.action == "tasks" || a.action == "watch" {
		return a.read(c, key)
	}

	// Connect to the daemon before borrowing the initiating session.
	client, err := connectDaemonFromContext(c.Context, c, a.statePath)
	if err != nil {
		return err
	}
	defer client.close()

	// Borrow the initiating session and resolve its Space.
	idx, err := sessionIndexFromInt(a.sessionIdx)
	if err != nil {
		return err
	}
	session, err := client.mountSession(c.Context, idx)
	if err != nil {
		return err
	}
	defer session.Release()
	sid, err := client.resolveSpaceID(c.Context, session, a.spaceID)
	if err != nil {
		return err
	}

	// Mount the Space and its World engine under that session's capability.
	space, releaseSpace, err := client.mountSpace(c.Context, session, sid)
	if err != nil {
		return err
	}
	defer releaseSpace()
	engine, releaseEngine, err := client.accessWorldEngine(c.Context, space)
	if err != nil {
		return err
	}
	defer releaseEngine()

	// Apply the durable run transition in one transaction.
	info, err := session.GetSessionInfo(c.Context)
	if err != nil {
		return err
	}
	sender, err := peer.IDB58Decode(info.GetPeerId())
	if err != nil {
		return err
	}
	err = world.ExecTransaction(c.Context, engine, true, func(ctx context.Context, tx world.WorldState) error {
		if a.action == "resume" {
			return flowgraph_run.ResumeRun(ctx, tx, key)
		}
		var placement *forge_worker.Placement
		if a.worker != "" {
			pid := a.workerPeer
			if pid == "" {
				pid = sender.String()
			}
			placement = &forge_worker.Placement{WorkerObjectKey: a.worker, PeerId: pid}
		}
		return flowgraph_run.StartRun(ctx, tx, sender, key, a.key, a.cluster, placement)
	})
	if err != nil {
		return err
	}

	// Persist execution approval for this session's Space runtime.
	contents, releaseContents, err := client.mountSpaceContents(c.Context, space)
	if err != nil {
		return errors.Wrap(err, "run committed; mount its process binding")
	}
	defer releaseContents()
	if _, err := contents.SetProcessBinding(c.Context, &s4wave_space.SetProcessBindingRequest{
		ObjectKey: key, TypeId: flowgraph.FlowgraphRunTypeID, Approved: true,
	}); err != nil {
		return errors.Wrap(err, "run committed; approve its process binding")
	}
	_, err = io.WriteString(os.Stdout, key+"\n")
	return err
}

// read prints generated run or Task JSON and watches committed run revisions.
func (a *flowgraphRunArgs) read(c *cli.Context, key string) error {
	// Borrow the World engine for the whole read or watch.
	engine, release, err := a.mountEngine(c)
	if err != nil {
		return err
	}
	defer release()

	// Emit each committed run snapshot, waiting on the World's revision watch.
	ws := world.NewEngineWorldState(engine, false)
	for {
		run, rev, err := flowgraph.ReadFlowgraphRun(c.Context, ws, key)
		if err != nil {
			return err
		}
		if a.action == "tasks" {
			return a.writeTasks(c.Context, ws, run)
		}
		data, err := run.MarshalJSON()
		if err != nil {
			return err
		}
		if _, err := os.Stdout.Write(append(data, '\n')); err != nil {
			return err
		}
		if a.action != "watch" || run.GetState() != flowgraph.FlowgraphRunState_FLOWGRAPH_RUN_STATE_RUNNING {
			return nil
		}
		if _, err := engine.WaitObjectRev(c.Context, key, rev+1, false); err != nil {
			return err
		}
	}
}

// writeTasks prints each activation's typed Task with its World object key.
func (a *flowgraphRunArgs) writeTasks(ctx context.Context, ws world.WorldState, run *flowgraph.FlowgraphRun) error {
	for _, activation := range run.GetActivations() {
		// Read and encode the Task through Forge's production lookup API.
		key := activation.GetTaskKey()
		task, err := forge_task.LookupTaskBody(ctx, ws, key)
		if err != nil {
			return err
		}
		data, err := task.MarshalJSON()
		if err != nil {
			return err
		}
		if _, err := io.WriteString(os.Stdout, `{"taskKey":`+strconv.Quote(key)+`,"task":`+string(data)+"}\n"); err != nil {
			return err
		}
	}
	return nil
}

// buildFlowgraphRunCommand builds commands for the durable run and its binding.
func buildFlowgraphRunCommand() *cli.Command {
	// Keep every command's mutable flag destinations independent.
	group := &cli.Command{Name: "run", Usage: "start, read and resume Flowgraph runs"}
	for _, spec := range [][2]string{
		{"start", "create a run and approve its local process binding"},
		{"show", "show a run as protobuf JSON"},
		{"tasks", "show the run's activation Tasks as protobuf JSON"},
		{"watch", "watch run revisions until it pauses or completes"},
		{"resume", "resume a paused run on the current graph revision"},
	} {
		args := &flowgraphRunArgs{action: spec[0]}
		group.Subcommands = append(group.Subcommands, &cli.Command{
			Name: spec[0], Usage: spec[1], ArgsUsage: "<run-object-key>",
			Flags: args.BuildFlags(), Action: args.Run,
		})
	}
	return group
}
