package space_migration

import "github.com/pkg/errors"

// ErrPlanBlocked identifies a preview with one or more typed blockers.
var ErrPlanBlocked = errors.New("migration preview is blocked")

// ErrStalePlan identifies source or destination drift after preview.
var ErrStalePlan = errors.New("migration preview is stale")

// ErrCapacityInsufficient identifies a destination capacity preflight failure.
var ErrCapacityInsufficient = errors.New("destination capacity is insufficient")

// ErrUnknownOperation identifies an unsupported migration operation enum.
var ErrUnknownOperation = errors.New("unknown migration operation")

// ErrPayloadSchemaRefused identifies a built-in payload whose schema is not admitted in Phase 0.
var ErrPayloadSchemaRefused = errors.New("migration payload schema refused")
