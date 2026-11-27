//go:build !js

package spacewave_cli

import (
	"context"
	"os"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
	"github.com/s4wave/spacewave/db/volume/restore"
)

// openRefRepairPeer holds a stopped peer snapshot until the repair ends.
// It refuses aliases of the target before either Volume opens for writing.
func openRefRepairPeer(ctx context.Context, volumePath string, opts refRepairOpts) (block.StoreOps, func() error, error) {
	// Keep programmatic peers and runs without a peer unchanged.
	if opts.peerPath == "" {
		if opts.peerOut != "" {
			return nil, nil, errors.New("--peer-out requires --peer-blocks")
		}
		return opts.peer, func() error { return nil }, nil
	}

	// Compare file identities so hard links and symlinks cannot disguise the target.
	info, err := os.Stat(opts.peerPath)
	if err != nil {
		return nil, nil, errors.Wrap(err, "stat repair peer")
	}
	target, err := os.Stat(volumePath)
	if err != nil {
		return nil, nil, err
	}
	if os.SameFile(info, target) {
		return nil, nil, errors.New("peer blocks names the volume file")
	}

	// Serve block directories directly and keep a file's snapshot open for the walk.
	var source packcopy.Source = restore.DirectorySource(opts.peerPath)
	closePeer := func() error { return nil }
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return nil, nil, errors.New("peer blocks must be a directory or a regular Volume file")
		}
		blocks, closeSource, err := restore.OpenS4DBSource(ctx, opts.peerPath)
		if err != nil {
			return nil, nil, err
		}
		source = blocks
		closePeer = func() error { return errors.Wrap(closeSource(), "close repair peer") }
	}
	return packcopy.NewSourceStore([]packcopy.Source{source}), closePeer, nil
}
