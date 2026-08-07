package s4wave_sql_workbench_world

import "github.com/pkg/errors"

// ErrWorkbenchAlreadyInitialized is returned when a SQL workbench already has a root.
var ErrWorkbenchAlreadyInitialized = errors.New("sql/workbench: already initialized")
