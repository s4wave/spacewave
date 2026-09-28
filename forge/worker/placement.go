package forge_worker

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// Validate checks that a placement names one Worker and one Device peer.
func (p *Placement) Validate() error {
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
	if err := p.Validate(); err != nil {
		return err
	}
	if err := CheckWorkerType(ctx, ws, p.GetWorkerObjectKey()); err != nil {
		return err
	}
	keypairs, _, err := CollectWorkerKeypairs(ctx, ws, p.GetWorkerObjectKey())
	if err != nil {
		return err
	}
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
