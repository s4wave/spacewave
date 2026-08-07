package s4wave_sql_query_world

import "github.com/pkg/errors"

// ErrQueryAlreadyInitialized is returned when a SQL query already has a root.
var ErrQueryAlreadyInitialized = errors.New("sql/query: already initialized")
