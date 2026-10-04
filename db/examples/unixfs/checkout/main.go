//go:build !js && !wasip1

package main

import (
	"context"
	"os"
	"os/signal"
	"time"

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
	checkout "github.com/s4wave/spacewave/db/unixfs/checkout"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

type hDaemonArgs = hcli.DaemonArgs

var daemonFlags struct {
	hDaemonArgs
}

var (
	checkoutRoot = "./output"
	verbose      bool
	dotOut       string
	profListen   string
)

func main() {
	// Configure the command-line app for the UnixFS filesystem demo.
	app := cli.NewApp()
	app.Usage = "unixfs filesystem demo"

	// Add daemon and demo-specific flags to the CLI.
	dflags := daemonFlags.BuildFlags()
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

// execute writes a test file to a UnixFS World object and checks it out to
// checkoutRoot.
func execute(rctx context.Context) error {
	// Scope the run and configure logging and profiling.
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	testbed.Verbose = verbose
	if profListen != "" {
		go prof.ListenProf(le, profListen)
	}

	// Start the storage testbed on the configured volume.
	volConfig := daemonFlags.BuildSingleVolume("", nil)
	tb, err := testbed.NewTestbed(
		ctx,
		le,
		testbed.WithVolumeConfig(volConfig),
	)
	if err != nil {
		return err
	}

	// Start the World engine on the testbed.
	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		return err
	}
	engineID := wtb.EngineID
	senderPeerID := tb.Volume.GetPeerID()

	// Provide the filesystem op handlers to the bus.
	opc := world.NewLookupOpController("test-fs-ops", engineID, unixfs_world.LookupFsOp)
	relOpc, err := tb.Bus.AddController(ctx, opc, nil)
	if err != nil {
		return err
	}
	defer relOpc()

	// Use the Engine directly: BusEngine looks up the engine on the bus for
	// every call, which is slow.
	ws := world.NewEngineWorldState(wtb.Engine, true)

	// Initialize the filesystem if it does not exist.
	objKey := "test-filesystem"
	objectState, exists, err := ws.GetObject(ctx, objKey)
	world.ReleaseObjectState(objectState)
	if err != nil {
		return err
	}
	if !exists {
		_, _, err = ws.ApplyWorldOp(
			ctx,
			unixfs_world.NewFsInitOp(objKey, unixfs_world.FSType_FSType_FS_NODE, nil, true, time.Now()),
			senderPeerID,
		)
		if err != nil {
			return err
		}
	}

	// Write the test file.
	if err := writeTestFile(ctx, ws, objKey); err != nil {
		return err
	}

	// Open the filesystem.
	fsType := unixfs_world.FSType_FSType_FS_NODE
	writer := unixfs_world.NewFSWriter(ws, objKey, fsType, senderPeerID)
	rootFSCursor := unixfs_world.NewFSCursor(le, ws, objKey, fsType, writer, true)
	rref, err := unixfs.NewFSHandle(rootFSCursor)
	if err != nil {
		return err
	}
	defer rref.Release()

	// Check it out to the destination path.
	le.Debugf("checking out to path: %s", checkoutRoot)
	if err := checkout.Checkout(ctx, checkoutRoot, rref, nil); err != nil {
		return errors.Wrap(err, "checkout fs")
	}
	le.Info("checkout complete")
	return nil
}

// writeTestFile creates test-file.txt in the filesystem at objKey if needed
// and writes a greeting to it.
func writeTestFile(ctx context.Context, ws world.WorldState, objKey string) error {
	_, _, err := world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
		// Find the file, creating it when it is missing.
		const testFilename = "test-file.txt"
		ftree, err := unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_DIRECTORY)
		if err != nil {
			return err
		}
		fnode, _, err := ftree.LookupFollowDirent(testFilename)
		if err != nil {
			return err
		}
		if fnode == nil {
			fnode, err = ftree.Mknod(
				testFilename,
				unixfs_block.NodeType_NodeType_FILE,
				nil,
				0,
				timestamp.Now(),
			)
			if err != nil {
				return err
			}
		}

		// Write the greeting at the start of the file.
		fh, err := fnode.BuildFileHandle(ctx)
		if err != nil {
			return err
		}
		fw := file.NewWriter(fh, nil, nil)
		return fw.WriteBytes(0, []byte("Hello world from UnixFS\n"))
	})
	return err
}
