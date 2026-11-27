package boot

import (
	"context"
	"errors"
	"net"
	"path/filepath"

	"github.com/aperturerobotics/starpc/srpc"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
	block_rpc_client "github.com/s4wave/spacewave/db/block/rpc/client"
	"github.com/s4wave/spacewave/db/bucket"
	os_image "github.com/s4wave/spacewave/os/image"
	os_system_stager "github.com/s4wave/spacewave/os/system/stager"
	"github.com/sirupsen/logrus"
)

// daemonSocket selects the daemon's socket in its state directory, as
// core/daemon's SocketName does.
const daemonSocket = "spacewave.sock"

// StageReleaseArgs returns the init arguments of the child that stages the
// next release the Device's os/system node serves into the image store in
// system, verified against the signers system and data pin. The console runs
// it as a child because its own process holds the store's read-only handle.
func StageReleaseArgs(system, data string) []string {
	return []string{"stage-release", system, data}
}

// RunStageRelease stages one release from the os/system node served by the
// daemon whose state lives under data, with arguments from StageReleaseArgs.
// RunStageRelease reports the image store's revisions to the node, which serves
// the revisions to the Device's update state, then watches the node's release
// and stages the first one a pinned signer authorized that is neither the
// accepted revision nor the trial that last rolled back, reporting the staging
// and then the trial. It returns nil once the trial is staged, so the console
// reboots into it. It returns an error when the daemon or the node stops
// serving, or when a trial is already pending, and the console runs it again
// after a backoff. Running the child again reports the revisions again.
func RunStageRelease(args []string) error {
	// Select the store, the daemon's state and the log.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	system, data := args[0], args[1]

	// Read the revisions a new release must differ from.
	pinned, err := PinSigners(system, data)
	if err != nil {
		return err
	}
	revs, err := readRevisions(ctx, system, pinned)
	if err != nil {
		return err
	}

	// Connect to the node through the daemon's socket.
	conn, err := net.Dial("unix", filepath.Join(data, "daemon", daemonSocket))
	if err != nil {
		return err
	}
	defer conn.Close()
	client, err := srpc.NewClientWithConn(conn, true, nil)
	if err != nil {
		return err
	}
	stager := os_system_stager.NewSRPCStagerClient(client)

	// Report the revisions, and stage nothing while a trial is pending.
	report := revs.report()
	if _, err := stager.ReportRevision(ctx, report); err != nil {
		return err
	}
	if revs.trial.auth != nil {
		return errors.New("stage: a trial is pending")
	}
	strm, err := stager.WatchRelease(ctx, &os_system_stager.WatchReleaseRequest{})
	if err != nil {
		return err
	}
	blocks := block_rpc.NewSRPCBlockStoreClientWithServiceID(client, os_system_stager.BlocksServiceID)

	// Stage the first new release a pinned signer authorized.
	for {
		resp, err := strm.Recv()
		if err != nil {
			return errors.Join(errors.New("watch release"), err)
		}
		signed := resp.GetRelease().GetAuthorization()
		image, _, err := os_image.VerifyImage(signed, pinned)
		if err != nil {
			println("SPACEWAVE_OS_RELEASE_REFUSED " + err.Error())
			continue
		}
		root := image.GetRootRef()
		if skip, err := revs.skipRelease(image); err != nil {
			return err
		} else if skip != "" {
			println("SPACEWAVE_OS_RELEASE_SKIPPED root=" + root.MarshalString() + " reason=" + skip)
			continue
		}
		report.Update = &os_system_stager.Revision_Staging{Staging: root}
		if _, err := stager.ReportRevision(ctx, report); err != nil {
			return err
		}
		src := block_rpc_client.NewBlockStore(blocks, root.GetHash().GetHashType(), true)
		copied, err := StageRevision(ctx, le, system, src, signed, image)
		if err != nil {
			return err
		}
		le.Infof("staged release %s; copied %d blocks", root.MarshalString(), copied)
		// The trial is published, so the console must reboot into it
		// even when this report fails. The trial's boot reports again.
		report.Update = &os_system_stager.Revision_Trial{Trial: root}
		if _, err := stager.ReportRevision(ctx, report); err != nil {
			le.WithError(err).Warn("cannot report the staged trial")
		}
		return nil
	}
}

// skipRelease returns why image needs no staging, or "" when it does: it is
// the accepted revision, or the trial that last rolled back, which would roll
// back again.
func (r *revisions) skipRelease(image *bucket.ObjectRef) (string, error) {
	// Skip the running revision.
	root := image.GetRootRef()
	if root.EqualsRef(r.accepted.image.GetRootRef()) {
		return "accepted", nil
	}

	// Skip the trial that last rolled back.
	record, err := rolledBackRecord(root)
	if err != nil {
		return "", err
	}
	if record.Ref.EqualsRef(r.rolledBack) {
		return "rolled-back", nil
	}
	return "", nil
}
