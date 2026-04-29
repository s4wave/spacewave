//go:build tinygo

package mysql

import (
	"strings"
)

func parseDSN(dsn string) (string, error) {
	if i := strings.LastIndexByte(dsn, '/'); i >= 0 {
		dsn = dsn[i+1:]
	}
	if i := strings.IndexByte(dsn, '?'); i >= 0 {
		dsn = dsn[:i]
	}
	return dsn, nil
}
