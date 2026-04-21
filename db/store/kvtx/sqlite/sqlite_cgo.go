//go:build cgo && !js && !wasip1

package store_kvtx_sqlite

import (
	"context"
	"database/sql"
	"os"

	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/cgo"
)

func open(ctx context.Context, path string, table string) (Store, error) {
	return cgo.Open(ctx, path, table)
}

func openWithMode(ctx context.Context, path string, mode os.FileMode, table string) (Store, error) {
	return cgo.OpenWithMode(ctx, path, mode, table)
}

func newStore(db *sql.DB, table string) (Store, error) {
	return cgo.NewStore(db, table)
}
