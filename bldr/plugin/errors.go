package bldr_plugin

import (
	"errors"

	"github.com/s4wave/spacewave/bldr/plugin/pluginid"
)

// ErrEmptyPluginID is returned if the plugin ID was empty.
var ErrEmptyPluginID = pluginid.ErrEmptyPluginID

// ErrWorkerGone fails the calls of a plugin worker that exited or was
// released, so a caller can tell a worker that left from one that is slow.
var ErrWorkerGone = errors.New("plugin worker is gone")

// IsWorkerGone reports whether err ended a call because its worker left. The
// RPC client may return the close error itself or a copy of its message.
func IsWorkerGone(err error) bool {
	return err != nil && (errors.Is(err, ErrWorkerGone) || err.Error() == ErrWorkerGone.Error())
}
