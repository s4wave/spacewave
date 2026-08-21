package webrtc

import (
	"bytes"
	"crypto/sha256"
	"strings"

	jsoniter "github.com/aperturerobotics/json-iterator-lite"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pion/ice/v4"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// SignalingCryptContext is the authenticated encryption domain for the strict
// offer-correlated signaling schema.
const SignalingCryptContext = "github.com/s4wave/spacewave/net 2026-08-12 webrtc signaling offer-id"

// EncodeWebRtcSignal marshals and encrypts the WebRtcSignal message.
func EncodeWebRtcSignal(s *WebRtcSignal, dstPeer crypto.PubKey) ([]byte, error) {
	// Validate the complete signal before it reaches the wire.
	if err := s.Validate(); err != nil {
		return nil, err
	}

	// Marshal the signal before encrypting its serialized bytes.
	msgSrc, err := s.MarshalVT()
	if err != nil {
		return nil, err
	}
	defer scrub.Scrub(msgSrc)

	// Encrypt the serialized signal in the signaling authentication domain.
	return peer.EncryptToPubKey(dstPeer, SignalingCryptContext, msgSrc)
}

// DecodeWebRtcSignal decrypts, unmarshals, and validates a signaling message.
func DecodeWebRtcSignal(msg []byte, privKey crypto.PrivKey) (*WebRtcSignal, error) {
	// Decrypt the signal in the strict schema authentication domain.
	msgDec, err := peer.DecryptWithPrivKey(privKey, SignalingCryptContext, msg)
	if err != nil {
		return nil, err
	}
	defer scrub.Scrub(msgDec)

	// Unmarshal and validate the envelope before returning it for ingress.
	out := &WebRtcSignal{}
	if err := out.UnmarshalVT(msgDec); err != nil {
		return nil, err
	}
	if err := out.Validate(); err != nil {
		return nil, err
	}
	return out, nil
}

// Validate validates the WebRtcSignal message.
func (m *WebRtcSignal) Validate() error {
	// Validate the concrete signal body variant.
	switch b := m.GetBody().(type) {
	case *WebRtcSignal_RequestOffer:
	case *WebRtcSignal_Sdp:
		if err := b.Sdp.Validate(); err != nil {
			return err
		}
	case *WebRtcSignal_Ice:
		if err := b.Ice.Validate(); err != nil {
			return err
		}
	default:
		return errors.New("unknown webrtc signal message type")
	}

	return nil
}

// OfferID returns the SHA-256 identity of the exact offer SDP.
func OfferID(offerSDP string) [sha256.Size]byte { return sha256.Sum256([]byte(offerSDP)) }

// NewWebRtcSdp constructs a new WebRtcSdp for one offer generation.
func NewWebRtcSdp(
	txSeqno uint64,
	desc *webrtc.SessionDescription,
	offerID [sha256.Size]byte,
) *WebRtcSdp {
	return &WebRtcSdp{
		TxSeqno: txSeqno,
		SdpType: desc.Type.String(),
		Sdp:     desc.SDP,
		OfferId: bytes.Clone(offerID[:]),
	}
}

// Validate validates the WebRtcSdp message.
func (s *WebRtcSdp) Validate() error {
	if len(s.GetOfferId()) != sha256.Size {
		return errors.Errorf("offer_id: length %d, want %d", len(s.GetOfferId()), sha256.Size)
	}

	// Require a non-empty and recognized SDP type.
	if s.GetSdpType() == "" {
		return errors.New("sdp_type: cannot be empty")
	}
	sdpType := s.ParseSDPType()
	if sdpType == webrtc.SDPTypeUnknown {
		return errors.Errorf("sdp_type: unknown sdp type: %v", s.GetSdpType())
	}
	if sdpType == webrtc.SDPTypeOffer {
		offerID := OfferID(s.GetSdp())
		if !bytes.Equal(s.GetOfferId(), offerID[:]) {
			return errors.New("offer_id: does not match offer sdp")
		}
	}

	// Parse the SDP payload to validate its structure.
	if _, err := s.ParseSDP(); err != nil {
		return err
	}
	return nil
}

// ParseSDPType parses the SDP type field.
func (s *WebRtcSdp) ParseSDPType() webrtc.SDPType {
	return webrtc.NewSDPType(s.GetSdpType())
}

// ToSessionDescription converts the sdp into a webrtc session description object.
//
// Returns nil if the message is empty.
func (s *WebRtcSdp) ToSessionDescription() *webrtc.SessionDescription {
	if s.GetSdpType() == "" {
		return nil
	}
	return &webrtc.SessionDescription{
		Type: s.ParseSDPType(),
		SDP:  s.GetSdp(),
	}
}

// ParseSDP parses the SDP from the type and sdp fields.
//
// Returns nil, nil if the message is empty.
func (s *WebRtcSdp) ParseSDP() (*sdp.SessionDescription, error) {
	desc := s.ToSessionDescription()
	if desc == nil {
		return nil, nil
	}
	return desc.Unmarshal()
}

// Validate validates the WebRtcIce message.
func (s *WebRtcIce) Validate() error {
	if len(s.GetOfferId()) != sha256.Size {
		return errors.Errorf("offer_id: length %d, want %d", len(s.GetOfferId()), sha256.Size)
	}
	candidate, err := s.ParseICECandidateInit()
	if err != nil {
		return err
	}
	if candidate.Candidate == "" {
		if candidate.SDPMLineIndex == nil || *candidate.SDPMLineIndex != 0 ||
			candidate.SDPMid != nil || candidate.UsernameFragment != nil {
			return errors.New("ice candidate: invalid end-of-candidates shape")
		}
		return nil
	}
	candidateValue, ok := strings.CutPrefix(candidate.Candidate, "candidate:")
	if !ok {
		return errors.New("invalid ice candidate grammar: missing candidate prefix")
	}
	if _, err := ice.UnmarshalCandidate(candidateValue); err != nil {
		return errors.Wrap(err, "invalid ice candidate grammar")
	}
	return nil
}

// NewWebRtcIce constructs a new WebRtcIce from an ICECandidateInit.
func NewWebRtcIce(candidate *webrtc.ICECandidateInit, offerID [sha256.Size]byte) (*WebRtcIce, error) {
	// Marshal the ICE candidate before storing its JSON representation.
	data, err := marshalICECandidateInit(candidate)
	if err != nil {
		return nil, err
	}

	// Return the encoded ICE candidate message.
	return &WebRtcIce{
		Candidate: string(data),
		OfferId:   bytes.Clone(offerID[:]),
	}, nil
}

// ParseICECandidateInit parses the ICECandidate from the JSON encoded body.
func (s *WebRtcIce) ParseICECandidateInit() (*webrtc.ICECandidateInit, error) {
	msg, err := unmarshalICECandidateInit([]byte(s.GetCandidate()))
	if err != nil {
		return nil, errors.Wrap(err, "invalid ice candidate json")
	}
	return msg, nil
}

func marshalICECandidateInit(candidate *webrtc.ICECandidateInit) ([]byte, error) {
	s := jsoniter.NewStream(nil, 128, 0)
	if candidate == nil {
		s.WriteNil()
		return s.Buffer(), nil
	}

	s.WriteObjectStart()
	s.WriteObjectField("candidate")
	s.WriteString(candidate.Candidate)
	s.WriteMore()
	s.WriteObjectField("sdpMid")
	if candidate.SDPMid == nil {
		s.WriteNil()
	} else {
		s.WriteString(*candidate.SDPMid)
	}
	s.WriteMore()
	s.WriteObjectField("sdpMLineIndex")
	if candidate.SDPMLineIndex == nil {
		s.WriteNil()
	} else {
		s.WriteUint32(uint32(*candidate.SDPMLineIndex))
	}
	s.WriteMore()
	s.WriteObjectField("usernameFragment")
	if candidate.UsernameFragment == nil {
		s.WriteNil()
	} else {
		s.WriteString(*candidate.UsernameFragment)
	}
	s.WriteObjectEnd()
	if s.Error != nil {
		return nil, s.Error
	}
	return s.Buffer(), nil
}

func unmarshalICECandidateInit(data []byte) (*webrtc.ICECandidateInit, error) {
	it := jsoniter.ParseBytes(data)
	msg := &webrtc.ICECandidateInit{}
	for key := it.ReadObject(); key != ""; key = it.ReadObject() {
		switch key {
		case "candidate":
			msg.Candidate = it.ReadString()
		case "sdpMid":
			if it.ReadNil() {
				msg.SDPMid = nil
				continue
			}
			v := it.ReadString()
			msg.SDPMid = &v
		case "sdpMLineIndex":
			if it.ReadNil() {
				msg.SDPMLineIndex = nil
				continue
			}
			v := it.ReadUint16()
			msg.SDPMLineIndex = &v
		case "usernameFragment":
			if it.ReadNil() {
				msg.UsernameFragment = nil
				continue
			}
			v := it.ReadString()
			msg.UsernameFragment = &v
		default:
			it.Skip()
		}
	}
	if err := it.Error; err != nil {
		return nil, err
	}
	return msg, nil
}
