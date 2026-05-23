//go:build goscript

package git_block

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/go-git/go-git/v6/plumbing/client"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ErrAuthUnsupported reports SSH authentication in GoScript builds.
var ErrAuthUnsupported = errors.New("git ssh auth is not supported in goscript")

// ResolveAuth resolves authentication on a bus from the config.
// Returns nil, nil if auth not configured.
func (a *AuthOpts) ResolveAuth(ctx context.Context, b bus.Bus) (client.SSHAuth, error) {
	if a.GetUsername() == "" && a.GetPeerId() == "" {
		return nil, nil
	}
	return nil, ErrAuthUnsupported
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
