//go:build linux
// +build linux

package main

import (
	"context"
	"os"
	"os/signal"
	"time"

	bfuse "bazil.org/fuse"
	"github.com/aperturerobotics/cli"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/file"
	hcli "github.com/s4wave/spacewave/db/cli"
	"github.com/s4wave/spacewave/db/daemon/prof"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/unixfs/fuse"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_vlogger "github.com/s4wave/spacewave/db/world/vlogger"
	"github.com/sirupsen/logrus"
)

type hDaemonArgs = hcli.DaemonArgs

var daemonFlags struct {
	hDaemonArgs
}

var (
	fuseRoot   = "./fuseroot"
	verbose    bool
	dotOut     string
	profListen string
)

func main() {
	// Configure the command-line application for the FUSE demo.
	app := cli.NewApp()
	app.Usage = "unixfs filesystem demo"

	// Add storage and demo-specific command flags.
	dflags := (&daemonFlags.hDaemonArgs).BuildFlags()
	dflags = append(
		dflags,
		&cli.BoolFlag{
			Name:        "verbose",
			Usage:       "enable verbose logging",
			Destination: &verbose,
		},
		&cli.StringFlag{
			Name:        "viz-dot-out",
			Usage:       "dot visualization output (if set) (e.x. demo.dot)",
			Destination: &dotOut,
			Value:       dotOut,
		},
		&cli.StringFlag{
			Name:        "prof-listen",
			Usage:       "if set, debug profiler will be hosted on the port, ex :8080",
			Destination: &profListen,
		},
	)
	app.Flags = dflags
	app.Action = func(c *cli.Context) error {
		ctx := context.Background()
		sctx, sctxStop := signal.NotifyContext(ctx, os.Interrupt, os.Kill)
		defer sctxStop()

		return execute(sctx)
	}
	if err := app.Run(os.Args); err != nil {
		os.Stderr.WriteString(err.Error())
		os.Exit(1)
	}
}

func execute(rctx context.Context) error {
	// Cancel the program context on return.
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// Build the debug logger.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	testbed.Verbose = verbose

	// Serve the profiler when requested.
	if profListen != "" {
		go prof.ListenProf(le, profListen)
	}

	// Build the storage and World testbeds.
	volConfig := daemonFlags.BuildSingleVolume("", nil)
	tb, err := testbed.NewTestbed(
		ctx,
		le,
		testbed.WithVolumeConfig(volConfig),
	)
	if err != nil {
		return err
	}
	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		return err
	}
	sender := tb.Volume.GetPeerID()

	// Provide the filesystem op handlers to the bus.
	opc := world.NewLookupOpController("test-fs-ops", wtb.EngineID, unixfs_world.LookupFsOp)
	relOpc, err := tb.Bus.AddController(ctx, opc, nil)
	if err != nil {
		return err
	}
	defer relOpc()

	// Use the Engine directly: BusEngine looks up the engine on the bus for
	// every call, which is slow.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	if verbose {
		ws = world_vlogger.NewWorldState(le, ws)
	}

	// Initialize the filesystem if it does not exist.
	objKey := "test-filesystem"
	obj, exists, err := ws.GetObject(ctx, objKey)
	if err != nil {
		world.ReleaseObjectState(obj)
		return err
	}
	if exists {
		world.ReleaseObjectState(obj)
	}
	if !exists {
		_, _, err = ws.ApplyWorldOp(
			ctx,
			unixfs_world.NewFsInitOp(objKey, unixfs_world.FSType_FSType_FS_NODE, nil, true, time.Now()),
			sender,
		)
		if err != nil {
			return err
		}
	}

	// Name the sample file in the test filesystem.
	testFilename := "test-file.txt"

	// Create or update the file inside the World object.
	_, _, err = world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
		// Open the filesystem tree and find the demo file.
		ftree, err := unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_DIRECTORY)
		if err != nil {
			return err
		}

		// Reuse the existing file entry when present.
		fnode, _, err := ftree.LookupFollowDirent(testFilename)
		if err != nil {
			return err
		}

		// Create the demo file entry when it does not exist.
		if fnode == nil {
			now := timestamp.Now()
			fnode, err = ftree.Mknod(testFilename, unixfs_block.NodeType_NodeType_FILE, nil, 0, now)
			if err != nil {
				return err
			}
		}

		// Open the file handle for writing.
		fh, err := fnode.BuildFileHandle(ctx)
		if err != nil {
			return err
		}

		// Write the demo contents to the file.
		fw := file.NewWriter(fh, nil, nil)
		return fw.WriteBytes(0, []byte("Hello world from FUSE!\n"))
	})
	if err != nil {
		return err
	}

	// clone a repo to the store
	/*
		ts := timestamp.Now()
		gitRepoKey := "repo/test-repo"
		_, gitRepoFound, err := ws.GetObject(gitRepoKey)
		if err != nil {
			return err
		}
		if !gitRepoFound {
			cloneOpts := &git_block.CloneOpts{
				Url:      "https://github.com/pkg/errors",
				Insecure: true,
			}
			createWtOp := &git_world.GitCreateWorktreeOp{
				ObjectKey:     "repo/test-repo/worktree",
				CreateWorkdir: true,
				WorkdirRef: &unixfs_world.UnixfsRef{
					ObjectKey: objKey,
					FsType:    unixfs_world.FSType_FSType_FS_NODE,
					Path:      unixfs_block.NewFSPath([]string{"workdir"}),
				},
				Timestamp: &ts,
			}
			_, err := git_world.GitClone(ctx, ws, gitRepoKey, sender.GetPeerID(), cloneOpts, nil, os.Stderr, createWtOp, nil)
			if err != nil {
				return err
			}
		}
	*/

	// start the filesystem
	rootFSCursor, err := unixfs_world.FollowUnixfsRef(
		ctx,
		le,
		ws,
		&unixfs_world.UnixfsRef{
			ObjectKey: objKey,
			FsType:    unixfs_world.FSType_FSType_FS_NODE,
		},
		sender,
		true,
	)
	if err != nil {
		return err
	}

	// Open a handle to the filesystem root.
	rref, err := unixfs.NewFSHandle(rootFSCursor)
	if err != nil {
		return err
	}
	defer rref.Release()

	// Mount the root over FUSE.
	le.Debug("mounting rootfs fuse")
	rootFS, err := fuse.Mount(ctx, le, fuseRoot, rref, verbose, false, []fuse.MountOption{
		bfuse.AllowOther(),
		bfuse.DefaultPermissions(),
	})
	if err != nil {
		return errors.Wrap(err, "build rootfs fuse")
	}

	// Serve requests, stopping the program when the server exits.
	go func() {
		err := rootFS.Serve()
		if err != nil {
			select {
			case <-ctx.Done():
			default:
				le.WithError(err).Warn("server exited with error")
			}
		}
		ctxCancel()
	}()

	// Run until the program stops.
	le.Info("startup complete")
	<-ctx.Done()

	// Close and unmount the filesystem.
	le.Info("shutting down")
	rootFS.Close()
	_ = fuse.Unmount(fuseRoot)
	return nil
}
