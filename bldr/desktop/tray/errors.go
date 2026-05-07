package desktop_tray

import "github.com/pkg/errors"

var (
	// ErrDesktopTrayEntryRequired is returned when entry is nil.
	ErrDesktopTrayEntryRequired = errors.New("desktop tray entry is required")
	// ErrDesktopTrayEntryIdRequired is returned when entry id is empty.
	ErrDesktopTrayEntryIdRequired = errors.New("desktop tray entry id is required")
	// ErrDesktopTrayEntryNotFound is returned when entry registration is not found.
	ErrDesktopTrayEntryNotFound = errors.New("desktop tray entry not found")
	// ErrDesktopTrayEntryDuplicate is returned when an entry id is already registered.
	ErrDesktopTrayEntryDuplicate = errors.New("desktop tray entry already registered")
	// ErrDesktopTrayEntryNotInvokable is returned when entry cannot be invoked.
	ErrDesktopTrayEntryNotInvokable = errors.New("desktop tray entry is not invokable")
	// ErrDesktopTrayActionHandlerRequired is returned when an invokable entry has no handler.
	ErrDesktopTrayActionHandlerRequired = errors.New("desktop tray action handler is required")
)
