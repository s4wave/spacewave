//go:build js || tinygo || sql_lite

package s4wave_sql_world

import (
	"context"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
)

func (o *SqlSetRootOp) rebaseRoot(
	ctx context.Context,
	os world.ObjectState,
	currentRoot *bucket.ObjectRef,
) (*bucket.ObjectRef, error) {
	return nil, ErrSqlRebaseUnsupported
}
