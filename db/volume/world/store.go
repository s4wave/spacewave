package volume_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// worldStore uses the World transaction's writer lifetime for the entire KV
// operation, including reads that determine subsequent metadata writes.
type worldStore struct {
	// engine serves the World that holds the KV root.
	engine world.Engine
	// b resolves an explicitly configured backing volume.
	b bus.Bus
	// le logs lookups on an explicitly configured backing volume.
	le *logrus.Entry
	// sfs builds the transforms used by an explicit backing cursor.
	sfs *block_transform.StepFactorySet
	// conf is the mounted volume's cloned configuration.
	conf *Config
}

// SupportsAtomicCommit reports that one World root publishes the KV tree's
// blocks and metadata together, and Commit fences the enclosing engine.
func (*worldStore) SupportsAtomicCommit() bool { return true }

// NewTransaction opens the current object root while retaining its World writer.
func (s *worldStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	// Expose a KV handle only after the World-backed transaction opens.
	t, err := newWorldTransaction(ctx, s.le, s.b, s.engine, s.sfs, s.conf, write)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// _ is a type assertion.
var (
	_ kvtx.Store             = (*worldStore)(nil)
	_ kvtx.AtomicCommitStore = (*worldStore)(nil)
)
