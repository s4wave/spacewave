package identity_domain_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/identity"
	aidentity "github.com/aperturerobotics/identity"
	"github.com/sirupsen/logrus"
)

// selectEntityResolver resolves select entity directives
type selectEntityResolver struct {
	c   *Controller
	ctx context.Context
	dir aidentity.SelectIdentityEntity
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *selectEntityResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Ask the user for an entity id.
	purpose := o.dir.SelectIdentityEntityPurpose()
	domainID := o.dir.SelectIdentityEntityDomainID()
	prevErr := o.dir.SelectIdentityEntityPrevError()

	entityID, err := aidentity.ExSelectEntityId(ctx, o.c.bus, purpose, domainID, prevErr)
	if err != nil {
		return err
	}
	if entityID == "" {
		o.c.le.Info("user provided empty entity id")
		handler.AddValue(aidentity.SelectIdentityEntityValue(nil))
		return nil
	}

	// Lookup the entity - via directive so other controllers can handle it.
	le := o.c.le.
		WithField("entity-id", entityID).
		WithField("domain-id", domainID)
	le.Info("looking up entity")
	v1, err := identity.ExIdentityLookupEntity(ctx, o.c.bus, domainID, entityID)
	if err != nil {
		return err
	}

	if !v1.IsNotFound() {
		if err := v1.GetError(); err != nil {
			le.WithError(err).Error("entity lookup failed")
			return err
		}
	}

	var val aidentity.SelectIdentityEntityValue = v1.GetEntity()
	if val == nil || v1.IsNotFound() {
		le.WithError(err).Warn("entity lookup returned not found")
	} else {
		// validate: ensure keypair signatures are valid
		kps, err := val.UnmarshalVerifyKeypairs()
		if err != nil {
			le.WithError(err).Error("entity lookup returned invalid entity")
			return err
		}

		le.WithFields(logrus.Fields{
			"entity-uuid":     val.GetEntityUuid(),
			"entity-epoch":    val.GetEpoch(),
			"entity-keypairs": len(kps),
		}).Info("retrieved entity")
	}

	handler.AddValue(val)
	return nil
}

// resolveSelectEntity returns a resolver for selecting an entity.
func (c *Controller) resolveSelectEntity(
	ctx context.Context,
	di directive.Instance,
	dir aidentity.SelectIdentityEntity,
) (directive.Resolver, error) {
	domainID := dir.SelectIdentityEntityDomainID()
	if c.domainID != domainID {
		return nil, nil
	}

	// Return resolver.
	return &selectEntityResolver{c: c, ctx: ctx, dir: dir}, nil
}
