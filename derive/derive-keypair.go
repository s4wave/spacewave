package identity_derive

import (
	"context"
	"strings"

	auth_method "github.com/aperturerobotics/auth/method"
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/identity"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// deriveKeypairResolver resolves DeriveEntityKeypair directives
type deriveKeypairResolver struct {
	c   *Controller
	ctx context.Context
	di  directive.Instance
	dir identity.DeriveEntityKeypair
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *deriveKeypairResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	keypairList := o.dir.DeriveEntityKeypairList()
	res, err := DeriveEntityKeypair(ctx, o.c.bus, o.c.le, keypairList)
	if err != nil {
		return err
	}
	if res != nil {
		var val identity.DeriveEntityKeypairValue = res
		_, _ = handler.AddValue(val)
	}
	return nil
}

// DeriveEntityKeypair attempts to interactively derive a list of entity keypairs.
func DeriveEntityKeypair(
	ctx context.Context,
	b bus.Bus,
	le *logrus.Entry,
	keypairList []*identity.EntityKeypair,
) (peer.Peer, error) {
	var lastErr error
KeypairLoop:
	for kpi, ekp := range keypairList {
		kp := ekp.GetKeypair()
		methodID := kp.GetAuthMethodId()
		expectedPeerID := kp.GetPeerId()
		if !AuthMethodIdSupported(methodID) || len(expectedPeerID) < 12 {
			// Currently only triplesec is supported.
			continue
		}

		// Lookup auth method.
		authMethod, err := auth_method.ExAuthLookupMethod(ctx, b, methodID, true)
		if err == nil && authMethod == nil {
			err = errors.Errorf("auth method not found: %s", methodID)
		}
		if err != nil {
			if err == context.Canceled {
				return nil, err
			}

			err = errors.Wrapf(err, "keypairs[%d]: lookup auth method", kpi)
			lastErr = err
			continue
		}

		// Unmarshal params
		params, err := authMethod.UnmarshalParameters(kp.GetAuthMethodParams())
		if err != nil {
			le.
				WithError(err).
				Warnf("keypairs[%d]: unable to unmarshal auth params", kpi)
			err = errors.Wrapf(err, "keypairs[%d]: unmarshal auth params", kpi)
			lastErr = err
			continue
		}

		var reasonBuf strings.Builder
		if ekp.GetEntityEmpty() {
			reasonBuf.WriteString("unlock ")
			if peerID := ekp.GetKeypair().GetPeerId(); peerID != "" {
				if len(peerID) > 14 {
					reasonBuf.WriteString(peerID[len(peerID)-13:])
				} else {
					reasonBuf.WriteString(peerID)
				}
			} else {
				reasonBuf.WriteString("keypair")
			}
		} else {
			reasonBuf.WriteString("unlock ")
			reasonBuf.WriteString(ekp.GetEntityId())
		}

		if domainID := ekp.GetDomainId(); domainID != "" {
			reasonBuf.WriteString("@")
			reasonBuf.WriteString(domainID)
		}

		reasonDetail := reasonBuf.String()
		reason := strings.Join([]string{
			ControllerID,
			"derive",
			expectedPeerID,
		}, "/")

		showErr := lastErr
		if showErr != nil {
			showErr = errors.Wrap(showErr, "Other keypair failed")
		}

		// Ask user for password, repeatedly if incorrect.
		for {
			passwordTxt, err := identity.ExPromptPassword(
				ctx,
				b,
				ekp.GetDomainId(),
				reason,
				reasonDetail,
				showErr,
			)
			if err != nil {
				if err == context.Canceled {
					return nil, err
				}
				le.
					WithError(err).
					Warnf("keypairs[%d]: %s", kpi, reason[:len(reason)-1])
				err = errors.Wrapf(err, "keypairs[%d]: prompt password", kpi)
				lastErr = err
			}
			if passwordTxt == "" {
				continue KeypairLoop
			}

			// Attempt to derive keypair
			var incorrectPw bool
			privKey, err := authMethod.Authenticate(params, []byte(passwordTxt))
			if err == nil {
				var derivPeerID peer.ID
				derivPeerID, err = peer.IDFromPrivateKey(privKey)
				if err == nil {
					derivPretty := derivPeerID.Pretty()
					if derivPretty != expectedPeerID {
						incorrectPw = true
						err = errors.Errorf(
							"expected peer %s but got %s",
							expectedPeerID,
							derivPretty,
						)
					}
				}
			}
			if err != nil {
				if incorrectPw {
					err = errors.New("incorrect password")
				} else {
					le.
						WithError(err).
						Warnf("keypairs[%d]", kpi)
					err = errors.Wrapf(err, "keypairs[%d]", kpi)
				}
				lastErr = err
				showErr = err
				continue
			}

			// otherwise we have successfully derived the peer.
			npeer, err := peer.NewPeer(privKey)
			if err != nil {
				le.
					WithError(err).
					Warnf("keypairs[%d]: failed to build peer: %s", kpi, reason[:len(reason)-1])
				err = errors.Wrapf(err, "keypairs[%d]: failed to build peer", kpi)
				lastErr = err
				continue KeypairLoop
			}
			return npeer, nil
		}
	}

	return nil, lastErr
}

// resolveDeriveEntityKeypair returns a resolver for deriving a keypair.
func (c *Controller) resolveDeriveEntityKeypair(
	ctx context.Context,
	di directive.Instance,
	dir identity.DeriveEntityKeypair,
) (directive.Resolver, error) {
	// Return resolver.
	return &deriveKeypairResolver{c: c, ctx: ctx, di: di, dir: dir}, nil
}

// _ is a type assertion
var _ directive.Resolver = ((*deriveKeypairResolver)(nil))
