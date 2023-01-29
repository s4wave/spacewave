package identity_domain_server

import (
	"context"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/protocol"
	stream_srpc_server "github.com/aperturerobotics/bifrost/stream/srpc/server"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/identity"
	identity_domain_service "github.com/aperturerobotics/identity/domain/service"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "identity/server"

// Server is an identity authority server.
type Server struct {
	// le is the logger
	le *logrus.Entry
	// b is the bus
	b bus.Bus
	// c is the config
	c *Config

	// server is the srpc server
	server *stream_srpc_server.Server
}

// NewServer constructs a new server, looking up the world handle.
func NewServer(le *logrus.Entry, b bus.Bus, c *Config) (*Server, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	srv := &Server{
		le: le,
		b:  b,
		c:  c,
	}
	var err error
	srv.server, err = stream_srpc_server.NewServer(
		b,
		le,
		controller.NewInfo(ControllerID, Version, "identity domain server"),
		[]protocol.ID{identity_domain_service.IdentityDomainProtocol},
		c.GetPeerIds(),
		[]stream_srpc_server.RegisterFn{
			func(mux srpc.Mux) error {
				return identity_domain_service.SRPCRegisterIdentityDomain(mux, srv)
			},
		},
	)
	if err != nil {
		return nil, err
	}
	return srv, nil
}

// GetControllerInfo returns information about the controller.
func (s *Server) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"session storage server",
	)
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (s *Server) Execute(ctx context.Context) error {
	return nil
}

// LookupEntity requests the Entity corresponding to an entity_id.
func (s *Server) LookupEntity(
	ctx context.Context,
	sreq *peer.SignedMsg,
) (*identity_domain_service.LookupEntityResp, error) {
	req := &identity_domain_service.LookupEntityReq{}
	pubKey, err := req.UnmarshalFrom(sreq)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if err := req.CheckTimestamp(now); err != nil {
		return nil, err
	}

	// NOTE: The identity of the peer making the request is not checked here.
	reqPeerID, err := peer.IDFromPublicKey(pubKey)
	if err != nil {
		return nil, err
	}

	lookupId := req.GetIdentifier()
	entityID, domainID := lookupId.GetEntityId(), lookupId.GetDomainId()
	if !s.DomainIdMatches(domainID) {
		return &identity_domain_service.LookupEntityResp{
			Identifier:  lookupId,
			LookupError: errors.Errorf("domain not found: %s", domainID).Error(),
			NotFound:    true,
		}, nil
	}

	le := s.le.
		WithField("request-peer", reqPeerID.Pretty()).
		WithField("entity-id", entityID).
		WithField("domain-id", domainID)

	// Lookup the desired entity.
	le.Debug("looking up entity for peer")
	lookupRes, err := identity.ExIdentityLookupEntity(
		ctx,
		s.b,
		domainID,
		entityID,
	)
	if err != nil {
		// note: this is a exception (not a lookup error)
		return nil, err
	}

	var lookupErr string
	if err := lookupRes.GetError(); err != nil {
		lookupErr = err.Error()
	}

	var ent *identity.Entity
	notFound := lookupRes.IsNotFound()
	if lookupRes != nil && !notFound {
		ent = lookupRes.GetEntity()
	}

	le.Debugf("entity lookup finished: found(%v)", ent != nil)
	return &identity_domain_service.LookupEntityResp{
		Identifier:   lookupId,
		LookupError:  lookupErr,
		NotFound:     notFound,
		LookupEntity: ent,
	}, nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (s *Server) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	return s.server.HandleDirective(ctx, di)
}

// DomainIdMatches checks if we will service domain id.
func (s *Server) DomainIdMatches(domainID string) bool {
	domainIDs := s.c.GetDomainIds()
	if len(domainIDs) == 0 {
		return true
	}
	for _, dm := range domainIDs {
		if dm == domainID {
			return true
		}
	}
	return false
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (s *Server) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ controller.Controller                            = ((*Server)(nil))
	_ identity_domain_service.SRPCIdentityDomainServer = ((*Server)(nil))
)
