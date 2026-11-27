package mysql

import (
	"context"

	"github.com/dolthub/go-mysql-server/sql"
)

// sessionContextKey selects the SQL session of an attached protocol client.
type sessionContextKey struct{}

// WithSession retains a client's SQL variables across storage transactions.
// The caller serializes commands using the session. The context does not extend
// a transaction's lifetime or transfer ownership of its storage locks.
func WithSession(ctx context.Context, session sql.Session) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, session)
}
