package forge_worker

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// Validate checks that a placement names one Worker and one Device peer.
func (p *Placement) Validate() error {
	// Require a Worker key, a peer ID, and a parseable peer ID.
	if p.GetWorkerObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	if p.GetPeerId() == "" {
		return errors.New("placement peer_id cannot be empty")
	}
	_, err := confparse.ParsePeerID(p.GetPeerId())
	return err
}

// ValidateLinked checks that the selected peer belongs to the selected Worker.
func (p *Placement) ValidateLinked(ctx context.Context, ws world.WorldState) error {
	// Require the placement itself to be valid.
	if err := p.Validate(); err != nil {
		return err
	}

	// Require the selected object to be a Worker and collect its keypairs.
	if err := CheckWorkerType(ctx, ws, p.GetWorkerObjectKey()); err != nil {
		return err
	}
	keypairs, _, err := CollectWorkerKeypairs(ctx, ws, p.GetWorkerObjectKey())
	if err != nil {
		return err
	}

	// Require the selected peer to match one of the Worker's keypairs.
	for _, keypair := range keypairs {
		id, err := keypair.ParsePeerID()
		if err != nil {
			return err
		}
		if id.String() == p.GetPeerId() {
			return nil
		}
	}
	return errors.Errorf("peer %s is not linked to Worker %s", p.GetPeerId(), p.GetWorkerObjectKey())
}
