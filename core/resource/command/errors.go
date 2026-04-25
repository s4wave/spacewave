package resource_command

import "github.com/pkg/errors"

// ErrCommandRequired is returned when command is nil.
var ErrCommandRequired = errors.New("command is required")

// ErrCommandIdRequired is returned when command_id is empty.
var ErrCommandIdRequired = errors.New("command_id is required")

// ErrResourceIdRequired is returned when resource_id is empty.
var ErrResourceIdRequired = errors.New("resource_id is required")

// ErrRegistrationNotFound is returned when a registration is not found.
var ErrRegistrationNotFound = errors.New("command registration not found")

// ErrCommandNotFound is returned when a command is not found.
var ErrCommandNotFound = errors.New("command not found")

// ErrNoHandler is returned when a command has no handler.
var ErrNoHandler = errors.New("command has no handler")

// ErrMultipleActiveRegistrations is returned when more than one registration is active.
var ErrMultipleActiveRegistrations = errors.New("multiple active command registrations")
