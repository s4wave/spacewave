package volume

import (
	"context"
	"crypto"

	"github.com/aperturerobotics/bifrost/keypem"
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/controllerbus/controller"
)

// NewVolumeInfo constructs volume info from a volume.
func NewVolumeInfo(ctx context.Context, ci *controller.Info, vol Volume) (*VolumeInfo, error) {
	peerID := vol.GetPeerID().String()
	peerInfo, err := vol.GetPeer(ctx, false)
	if err != nil {
		return nil, err
	}
	peerPub := peerInfo.GetPubKey()

	pkPem, err := keypem.MarshalPubKeyPem(peerPub)
	if err != nil {
		return nil, err
	}

	return &VolumeInfo{
		VolumeId:       vol.GetID(),
		PeerId:         peerID,
		PeerPub:        string(pkPem),
		ControllerInfo: ci.Clone(),
		HashType:       vol.GetHashType(),
	}, nil
}

// Validate validates the VolumeInfo object.
func (i *VolumeInfo) Validate() error {
	peerID, err := i.ParsePeerID()
	if err == nil && len(peerID) == 0 {
		err = peer.ErrEmptyPeerID
	}
	if err != nil {
		return err
	}
	if _, err := i.ParseToPeer(); err != nil {
		return err
	}
	// note: allows zero value
	if err := i.GetHashType().Validate(); err != nil {
		return err
	}
	return nil
}

// ParsePeerID parses the peer ID.
func (i *VolumeInfo) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(i.GetPeerId())
}

// ParsePeerPub parses the public key.
func (i *VolumeInfo) ParsePeerPub() (crypto.PublicKey, error) {
	return confparse.ParsePublicKey(i.GetPeerPub())
}

// ParseToPeer parses the fields and builds the corresponding Peer.
func (i *VolumeInfo) ParseToPeer() (peer.Peer, error) {
	return confparse.ParsePeer("", i.GetPeerPub(), i.GetPeerId())
}
