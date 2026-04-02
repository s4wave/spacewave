//go:build !cgo && !js && !wasip1 && sqlite_purego_modernc

package store_kvtx_sqlite

import (
	"context"
	"database/sql"
	"os"

	"github.com/aperturerobotics/hydra/store/kvtx/sqlite/purego"
)

func open(ctx context.Context, path string, table string) (Store, error) {
	return purego.Open(ctx, path, table)
}

func openWithMode(ctx context.Context, path string, mode os.FileMode, table string) (Store, error) {
	return purego.OpenWithMode(ctx, path, mode, table)
}

func newStore(db *sql.DB, table string) (Store, error) {
	return purego.NewStore(db, table)
}
