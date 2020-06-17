package auth_challenge

import (
	"github.com/aperturerobotics/bifrost/protocol"
	"github.com/aperturerobotics/bifrost/stream"
	stream_packet "github.com/aperturerobotics/bifrost/stream/packet"
	"github.com/aperturerobotics/identity"
	"github.com/pkg/errors"
)

const (
	// ChallengeProtocolID is the protocol ID for the challenge req/rep
	ChallengeProtocolID = protocol.ID("aperture/auth/challenge/1")

	// MaxMessageSize is the protocol max message size.
	MaxMessageSize uint32 = 1e6 // 1Mb
)

// Session contains a packet session for the stream.
type Session struct {
	*stream_packet.Session
}

// NewSession constructs a new session.
func NewSession(stream stream.Stream) *Session {
	ps := stream_packet.NewSession(stream, MaxMessageSize)
	return &Session{
		Session: ps,
	}
}

// ReadMsg reads the message type from the session.
func (s *Session) ReadMsg(m *Msg) error {
	if err := s.RecvMsg(m); err != nil {
		return err
	}
	return m.Validate()
}

// NewEntityLookupStart constructs a new entity lookup start msg.
func NewEntityLookupStart(domainID, entityID string) *EntityLookupStart {
	return &EntityLookupStart{
		Identifier: &EntityLookupIdentifier{
			DomainId: domainID,
			EntityId: entityID,
		},
	}
}

// NewEntityLookupFinish constructs a new entity lookup finish msg.
func NewEntityLookupFinish(
	domainID, entityID string,
	lookupError error,
	lookupIsNotFound bool,
	lookupEntity *identity.Entity,
) *EntityLookupFinish {
	var errStr string
	if lookupError != nil {
		errStr = lookupError.Error()
	}
	return &EntityLookupFinish{
		Identifier: &EntityLookupIdentifier{
			DomainId: domainID,
			EntityId: entityID,
		},
		LookupError:      errStr,
		LookupIsNotFound: lookupIsNotFound,
		LookupEntity:     lookupEntity,
	}
}

// NewEntityLookupCancel constructs a new entity lookup stop msg.
func NewEntityLookupCancel(domainID, entityID string) *EntityLookupCancel {
	return &EntityLookupCancel{
		Identifier: &EntityLookupIdentifier{
			DomainId: domainID,
			EntityId: entityID,
		},
	}
}

// Validate validates the message.
func (m *Msg) Validate() error {
	mt := m.GetMsgType()
	switch mt {
	case MsgType_MsgType_ENTITY_LOOKUP_CANCEL:
		return m.GetEntityLookupCancel().Validate()
	case MsgType_MsgType_ENTITY_LOOKUP_FINISH:
		return m.GetEntityLookupFinish().Validate()
	case MsgType_MsgType_ENTITY_LOOKUP_START:
		return m.GetEntityLookupStart().Validate()
	default:
		return errors.Errorf("unknown message type: %v", mt.String())
	}
}

// Validate validates the message.
func (m *EntityLookupStart) Validate() error {
	if err := m.GetIdentifier().Validate(); err != nil {
		return err
	}
	return nil
}

// Validate validates the message.
func (m *EntityLookupCancel) Validate() error {
	if err := m.GetIdentifier().Validate(); err != nil {
		return err
	}
	return nil
}

// Validate validates the message.
func (m *EntityLookupFinish) Validate() error {
	if err := m.GetIdentifier().Validate(); err != nil {
		return err
	}
	return nil
}

// Validate validates the message.
func (m *EntityLookupIdentifier) Validate() error {
	if err := identity.ValidateEntityID(m.GetEntityId()); err != nil {
		return err
	}
	if err := identity.ValidateDomainID(m.GetDomainId()); err != nil {
		return err
	}
	return nil
}
