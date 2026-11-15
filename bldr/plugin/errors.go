package bldr_plugin

import (
	"errors"
	"strings"

	"github.com/s4wave/spacewave/bldr/plugin/pluginid"
)

// ErrEmptyPluginID is returned if the plugin ID was empty.
var ErrEmptyPluginID = pluginid.ErrEmptyPluginID

// ErrWorkerGone fails the calls of a plugin worker that exited or was
// released, so a caller can tell a worker that left from one that is slow.
var ErrWorkerGone = errors.New("plugin worker is gone")

// ErrNotHostPlugin refuses a request to run controllers on the host bus from a
// plugin the host did not start itself, such as one loaded for a Space or
// approved by a person. Retrying cannot succeed.
var ErrNotHostPlugin = errors.New("only plugins the host starts may run controllers on the host bus")

// IsNotHostPlugin reports whether err is the host's refusal to run controllers
// for a plugin it did not start. The RPC client may return the error itself or
// a copy of its message, possibly wrapped by the controllers that relayed it.
func IsNotHostPlugin(err error) bool {
	return err != nil && (errors.Is(err, ErrNotHostPlugin) || strings.Contains(err.Error(), ErrNotHostPlugin.Error()))
}

// IsWorkerGone reports whether err ended a call because its worker left. The
// RPC client may return the close error itself or a copy of its message.
func IsWorkerGone(err error) bool {
	return err != nil && (errors.Is(err, ErrWorkerGone) || err.Error() == ErrWorkerGone.Error())
}
