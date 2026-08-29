//go:build !js

package spacewave_cli

import (
	"os"
	"slices"
	"strings"
	"time"

	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_worker "github.com/s4wave/spacewave/forge/worker"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/identity"
	"github.com/s4wave/spacewave/net/peer"
)

// newForgeCommand builds the forge command group.
func newForgeCommand(getBus func() cli_entrypoint.CliBus) *cli.Command {
	var statePath string
	var sessionIdx uint
	var spaceID string
	commonFlags := append(
		clientFlags(&statePath, &sessionIdx),
		&cli.StringFlag{
			Name:        "space",
			Aliases:     []string{"space-id"},
			Usage:       "space ID (auto-detected if only one space)",
			EnvVars:     []string{"SPACEWAVE_SPACE"},
			Destination: &spaceID,
		},
	)
	return &cli.Command{
		Name:  "forge",
		Usage: "manage forge entities (clusters, jobs, workers)",
		Subcommands: []*cli.Command{
			buildForgeCreateClusterCommand(&statePath, &sessionIdx, &spaceID, slices.Clone(commonFlags)),
			buildForgeCreateJobCommand(&statePath, &sessionIdx, &spaceID, slices.Clone(commonFlags)),
			buildForgeCreateWorkerCommand(&statePath, &sessionIdx, &spaceID, slices.Clone(commonFlags)),
		},
	}
}

// buildForgeCreateClusterCommand builds the forge create-cluster subcommand.
func buildForgeCreateClusterCommand(statePath *string, sessionIdx *uint, spaceID *string, commonFlags []cli.Flag) *cli.Command {
	var name string
	return &cli.Command{
		Name:      "create-cluster",
		Usage:     "create a forge cluster in a space",
		ArgsUsage: "<key>",
		Flags: append(commonFlags, &cli.StringFlag{
			Name:        "name",
			Usage:       "cluster name",
			Destination: &name,
		}),
		Action: func(c *cli.Context) error {
			key := c.Args().First()
			if key == "" {
				return errors.New("cluster key required")
			}
			ctx := c.Context
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()

			sess, err := client.mountSession(ctx, uint32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			info, err := sess.GetSessionInfo(ctx)
			if err != nil {
				return errors.Wrap(err, "get Cluster session info")
			}
			entityPeerID, err := peer.IDB58Decode(info.GetPeerId())
			if err != nil {
				return errors.Wrap(err, "parse Cluster session peer")
			}

			sid, err := client.resolveSpaceID(ctx, sess, *spaceID)
			if err != nil {
				return err
			}

			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
			if err != nil {
				return err
			}
			defer spaceCleanup()

			engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
			if err != nil {
				return err
			}
			defer engineCleanup()

			tx, err := engine.NewTransaction(ctx, true)
			if err != nil {
				return errors.Wrap(err, "new transaction")
			}
			defer tx.Discard()

			op := forge_cluster.NewClusterCreateOp(key, name, entityPeerID)
			_, _, err = tx.ApplyWorldOp(ctx, op, entityPeerID)
			if err != nil {
				return errors.Wrap(err, "create cluster")
			}

			if err := tx.Commit(ctx); err != nil {
				return errors.Wrap(err, "commit transaction")
			}

			os.Stdout.WriteString("Created cluster \"" + key + "\".\n")
			return nil
		},
	}
}

// buildForgeCreateJobCommand builds the forge create-job subcommand.
func buildForgeCreateJobCommand(statePath *string, sessionIdx *uint, spaceID *string, commonFlags []cli.Flag) *cli.Command {
	var name string
	return &cli.Command{
		Name:      "create-job",
		Usage:     "create a forge job in a space",
		ArgsUsage: "<key>",
		Flags: append(commonFlags, &cli.StringFlag{
			Name:        "name",
			Usage:       "job name",
			Destination: &name,
		}),
		Action: func(c *cli.Context) error {
			key := c.Args().First()
			if key == "" {
				return errors.New("job key required")
			}
			ctx := c.Context
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()

			sess, err := client.mountSession(ctx, uint32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()

			sid, err := client.resolveSpaceID(ctx, sess, *spaceID)
			if err != nil {
				return err
			}

			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
			if err != nil {
				return err
			}
			defer spaceCleanup()

			engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
			if err != nil {
				return err
			}
			defer engineCleanup()

			tx, err := engine.NewTransaction(ctx, true)
			if err != nil {
				return errors.Wrap(err, "new transaction")
			}
			defer tx.Discard()

			// Create job with no tasks (tasks can be added later).
			njob := &forge_job.Job{
				JobState:  forge_job.State_JobState_PENDING,
				Timestamp: timestamppb.New(time.Now()),
			}
			_, _, err = world.CreateWorldObject(ctx, tx, key, func(bcs *block.Cursor) error {
				bcs.ClearAllRefs()
				bcs.SetBlock(njob, true)
				return nil
			})
			if err != nil {
				return errors.Wrap(err, "create job object")
			}
			err = world_types.SetObjectType(ctx, tx, key, forge_job.JobTypeID)
			if err != nil {
				return errors.Wrap(err, "set job type")
			}

			if err := tx.Commit(ctx); err != nil {
				return errors.Wrap(err, "commit transaction")
			}

			os.Stdout.WriteString("Created job \"" + key + "\".\n")
			return nil
		},
	}
}

// buildForgeCreateWorkerCommand builds the forge create-worker subcommand.
func buildForgeCreateWorkerCommand(statePath *string, sessionIdx *uint, spaceID *string, commonFlags []cli.Flag) *cli.Command {
	var name, peerID, clusterKey string
	return &cli.Command{
		Name:      "create-worker",
		Usage:     "create and cluster-assign a Forge Worker for one session peer",
		ArgsUsage: "<key>",
		Flags: append(commonFlags,
			&cli.StringFlag{
				Name:        "name",
				Usage:       "worker name",
				Required:    true,
				Destination: &name,
			},
			&cli.StringFlag{
				Name:        "peer-id",
				Usage:       "exact session peer that will run the Worker",
				Required:    true,
				Destination: &peerID,
			},
			&cli.StringFlag{
				Name:        "cluster",
				Usage:       "existing Cluster object key",
				Required:    true,
				Destination: &clusterKey,
			},
		),
		Action: func(c *cli.Context) error {
			key := strings.TrimSpace(c.Args().First())
			if key == "" {
				return errors.New("worker key required")
			}
			peerID = strings.TrimSpace(peerID)
			if peerID == "" {
				return errors.New("worker peer ID required")
			}
			clusterKey = strings.TrimSpace(clusterKey)
			if clusterKey == "" {
				return errors.New("worker Cluster required")
			}
			ctx := c.Context
			client, err := connectDaemonFromContext(ctx, c, *statePath)
			if err != nil {
				return err
			}
			defer client.close()

			sess, err := client.mountSession(ctx, uint32(*sessionIdx))
			if err != nil {
				return err
			}
			defer sess.Release()
			info, err := sess.GetSessionInfo(ctx)
			if err != nil {
				return errors.Wrap(err, "get Worker session info")
			}
			if info.GetPeerId() != peerID {
				return errors.Errorf(
					"worker peer %q does not match mounted session peer %q",
					peerID,
					info.GetPeerId(),
				)
			}
			publicKeyPEM := strings.TrimSpace(info.GetCryptoInfo().GetPublicKeyPem())
			if publicKeyPEM == "" {
				return errors.New("mounted Worker session has no public key")
			}
			keypair := &identity.Keypair{PeerId: peerID, PubKey: publicKeyPEM}
			if err := keypair.Validate(); err != nil {
				return errors.Wrap(err, "validate Worker session keypair")
			}
			workerPeerID, err := keypair.ParsePeerID()
			if err != nil {
				return errors.Wrap(err, "parse Worker session peer")
			}

			sid, err := client.resolveSpaceID(ctx, sess, *spaceID)
			if err != nil {
				return err
			}
			spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
			if err != nil {
				return err
			}
			defer spaceCleanup()
			engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
			if err != nil {
				return err
			}
			defer engineCleanup()
			tx, err := engine.NewTransaction(ctx, true)
			if err != nil {
				return errors.Wrap(err, "new transaction")
			}
			defer tx.Discard()

			if _, _, err = tx.ApplyWorldOp(
				ctx,
				forge_worker.NewWorkerCreateOp(key, name, []*identity.Keypair{keypair}),
				workerPeerID,
			); err != nil {
				return errors.Wrap(err, "create worker")
			}
			if _, _, err = tx.ApplyWorldOp(
				ctx,
				forge_cluster.NewClusterAssignWorkerOp(clusterKey, key),
				workerPeerID,
			); err != nil {
				return errors.Wrap(err, "assign worker to Cluster")
			}
			if err := tx.Commit(ctx); err != nil {
				return errors.Wrap(err, "commit transaction")
			}

			os.Stdout.WriteString("Created worker \"" + key + "\" in Cluster \"" + clusterKey + "\".\n")
			return nil
		},
	}
}
