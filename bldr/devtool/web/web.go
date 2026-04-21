package devtool_web

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// HostServiceIDPrefix is the prefix used for the devtool RPC services. This
// ID can be prepended to RPC service IDs to indicate the service is located on
// the devtool (while running within the web runtime).
const HostServiceIDPrefix = "devtool/"

// HostServerID is the server ID used for devtool-host originating RPC calls.
const HostServerID = "devtool/web"

// HostProtocolID is the protocol ID used for devtool-host RPC calls.
const HostProtocolID = protocol.ID("devtool/web/rpc")

// BrowserProtocolID is the protocol ID used for devtool-to-browser RPC calls.
// The browser serves incoming streams on this protocol, allowing the devtool
// (or a Go test harness) to call any RPC service on the browser bus.
const BrowserProtocolID = protocol.ID("devtool/web/browser-rpc")

// EntrypointClientID is the client ID used for devtool-entrypoint originating RPC calls.
const EntrypointClientID = "devtool/web/entrypoint"

// HostVolumeID is the volume ID used for devtool-host volume.
const HostVolumeID = "devtool"

// HostVolumeServiceIDPrefix is the service ID prefix for the host ProxyVolume.
const HostVolumeServiceIDPrefix = "devtool-volume/"

// Validate validates the DevtoolInitBrowser.
func (i *DevtoolInitBrowser) Validate() error {
	if i.GetAppId() == "" {
		return errors.New("app id cannot be empty")
	}

	pid, err := i.ParsePeerID()
	if err == nil && pid == "" {
		err = peer.ErrEmptyPeerID
	}
	if err != nil {
		return errors.Wrap(err, "devtool_peer_id")
	}

	if err := i.GetDevtoolVolumeInfo().Validate(); err != nil {
		return errors.Wrap(err, "devtool_volume_info")
	}

	return nil
}

// ParseDevtoolPeerID parses the devtool peer id field.
func (i *DevtoolInitBrowser) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(i.GetDevtoolPeerId())
}
