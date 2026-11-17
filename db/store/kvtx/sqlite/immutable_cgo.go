//go:build cgo && !js && !wasip1

package store_kvtx_sqlite

import (
	"context"

	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/common"
)

// OpenImmutable opens a bounded, physically read-only checkpointed capture.
func OpenImmutable(ctx context.Context, path, table string, limit int) (*common.Immutable, error) {
	return common.OpenImmutable(ctx, path, table, "sqlite3", limit)
}
