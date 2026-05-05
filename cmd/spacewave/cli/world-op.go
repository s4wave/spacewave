//go:build !js

package spacewave_cli

import (
	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	sdk_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// applyWorldOp opens a writable engine transaction, applies the op, and
// commits. Shared by canvas, git, and other CLI subcommands that mutate
// world state through a typed object.
func applyWorldOp(c *cli.Context, engine *sdk_engine.SDKEngine, op world.Operation) error {
	ctx := c.Context
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()

	_, _, err = tx.ApplyWorldOp(ctx, op, "")
	if err != nil {
		return errors.Wrap(err, "apply "+op.GetOperationTypeId())
	}
	return tx.Commit(ctx)
}
