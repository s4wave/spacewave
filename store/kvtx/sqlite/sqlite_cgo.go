//go:build cgo && !js && !wasip1

package store_kvtx_sqlite

import (
	"database/sql"
	"os"

	"github.com/aperturerobotics/hydra/store/kvtx/sqlite/cgo"
)

func open(path string, table string) (Store, error) {
	return cgo.Open(path, table)
}

func openWithMode(path string, mode os.FileMode, table string) (Store, error) {
	return cgo.OpenWithMode(path, mode, table)
}

func newStore(db *sql.DB, table string) Store {
	return cgo.NewStore(db, table)
}
