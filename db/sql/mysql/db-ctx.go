package mysql

import (
	"context"

	"github.com/dolthub/go-mysql-server/sql"
)

// GetDbContext gets the sql context or returns background.
func GetDbContext(ctx *sql.Context) context.Context {
	if ctx != nil && ctx.Context != nil {
		return ctx.Context
	}
	return context.Background()
}
