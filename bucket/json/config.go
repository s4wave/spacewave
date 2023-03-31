package bucket_json

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/pkg/errors"
)

// Config implements the bucket configuration JSON marshalling logic.
type Config struct {
	// Id is the bucket identifier.
	Id string `json:"id"`
	// Version is the configuration version.
	Version uint32 `json:"version"`
	// Reconcilers contains the list of bucket reconcilers.
	Reconcilers []ReconcilerConfig `json:"reconcilers,omitempty"`
	// PutOpts contains the put options.
	PutOpts *block.PutOpts `json:"putOpts,omitempty"`
	// Lookup controls the lookup confiuration.
	Lookup *LookupConfig `json:"lookup,omitempty"`
}

// NewConfig builds a new controller config.
func NewConfig(ctx context.Context, b bus.Bus, c *bucket.Config) (*Config, error) {
	if c == nil {
		return nil, nil
	}

	n := &Config{
		Id:      c.GetId(),
		Version: c.GetVersion(),
	}
	n.Reconcilers = make([]ReconcilerConfig, len(c.GetReconcilers()))
	for i, r := range c.GetReconcilers() {
		cc, err := r.GetController().Resolve(ctx, b)
		if err != nil {
			return nil, err
		}
		n.Reconcilers[i] = *NewReconcilerConfig(r.GetId(), cc)
	}
	return n, nil
}

// GetVersion returns the version.
func (c *Config) GetVersion() uint32 {
	return c.Version
}

// ResolveToProto resolves the config to a proto object.
func (c *Config) ResolveToProto(ctx context.Context, b bus.Bus) (*bucket.Config, error) {
	bc := &bucket.Config{
		Id:          c.Id,
		Version:     c.Version,
		PutOpts:     c.PutOpts,
		Reconcilers: make([]*bucket.ReconcilerConfig, len(c.Reconcilers)),
	}
	if c.Lookup != nil {
		bc.Lookup = &bucket.LookupConfig{
			Disable: c.Lookup.GetDisable(),
		}
		if c.Lookup.Controller != nil {
			lookupConf, err := c.Lookup.Controller.Resolve(ctx, b)
			if err != nil {
				return nil, errors.Wrap(err, "lookup controller resolve")
			}
			/*
				lc, ok := lookupConf.GetConfig().(lookup.Config)
				if !ok {
					confID := lookupConf.GetConfig().GetConfigID()
					return nil, errors.Errorf("config does not implement lookup config: %s", confID)
				}
			*/
			bc.Lookup.Controller, err = configset_proto.NewControllerConfig(lookupConf, false)
			if err != nil {
				return nil, errors.Wrap(err, "lookup controller resolve")
			}
		}
	}
	for i := range c.Reconcilers {
		v := &c.Reconcilers[i]
		c, err := v.Controller.Resolve(ctx, b)
		if err != nil {
			return nil, errors.Wrap(err, "reconciler controller config resolve")
		}
		pcc, err := configset_proto.NewControllerConfig(c, false)
		if err != nil {
			return nil, errors.Wrap(err, "reconciler controller config marshal")
		}
		bc.Reconcilers[i] = &bucket.ReconcilerConfig{
			Id:         v.Id,
			Controller: pcc,
		}
	}
	return bc, nil
}
