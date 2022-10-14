package git_block

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	peer_ssh "github.com/aperturerobotics/bifrost/peer/ssh"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/go-git/go-git/v5/plumbing/transport"
	transport_ssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// ResolveAuth resolves authentication on a bus from the config.
// Returns nil, nil if auth not configured.
func (a *AuthOpts) ResolveAuth(ctx context.Context, b bus.Bus) (transport.AuthMethod, error) {
	// ssh authentication
	sshAuth := &transport_ssh.PublicKeys{
		User: a.GetUsername(),
	}
	if a.GetPeerId() != "" {
		peerID, err := a.ParsePeerId()
		if err != nil {
			return nil, err
		}

		peerPriv, peerPrivRef, err := peer.GetPeerWithID(ctx, b, peerID)
		if err != nil {
			return nil, err
		}
		defer peerPrivRef.Release()

		privKey, err := peerPriv.GetPrivKey(ctx)
		if err != nil {
			return nil, err
		}
		if privKey != nil {
			sshAuth.Signer, err = peer_ssh.NewSigner(privKey)
			if err != nil {
				return nil, err
			}
		}
	}
	if len(sshAuth.User) != 0 || sshAuth.Signer != nil {
		return sshAuth, nil
	}

	return nil, nil
}

// Validate checks the auth object.
func (a *AuthOpts) Validate() error {
	if _, err := a.ParsePeerId(); err != nil {
		return err
	}
	return nil
}

// ParsePeerId parses the authentication peer id.
func (a *AuthOpts) ParsePeerId() (peer.ID, error) {
	return confparse.ParsePeerID(a.GetPeerId())
}
