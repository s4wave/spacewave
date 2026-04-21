package webrtc

import (
	"encoding/json"

	"github.com/aperturerobotics/util/scrub"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// SignalingCryptContext is the encryption context to use for the signaling messages.
var SignalingCryptContext = "github.com/s4wave/spacewave/net 2024-01-15 17:58:55 webrtc signaling"

// EncodeWebRtcSignal marshals and encrypts the WebRtcSignal message.
func EncodeWebRtcSignal(s *WebRtcSignal, dstPeer crypto.PubKey) ([]byte, error) {
	msgSrc, err := s.MarshalVT()
	if err != nil {
		return nil, err
	}
	defer scrub.Scrub(msgSrc)

	return peer.EncryptToPubKey(dstPeer, SignalingCryptContext, msgSrc)
}

// DecodeWebRtcSignal decrypts and unmarshals the WebRtcSignal message.
func DecodeWebRtcSignal(msg []byte, privKey crypto.PrivKey) (*WebRtcSignal, error) {
	msgDec, err := peer.DecryptWithPrivKey(privKey, SignalingCryptContext, msg)
	if err != nil {
		return nil, err
	}
	defer scrub.Scrub(msgDec)

	out := &WebRtcSignal{}
	if err := out.UnmarshalVT(msgDec); err != nil {
		return nil, err
	}
	return out, nil
}

// Validate validates the WebRtcSignal message.
func (m *WebRtcSignal) Validate() error {
	// Validate body
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

// NewWebRtcSdp constructs a new WebRtcSdp from a SessionDescription.
func NewWebRtcSdp(txSeqno uint64, desc *webrtc.SessionDescription) *WebRtcSdp {
	return &WebRtcSdp{
		TxSeqno: txSeqno,
		SdpType: desc.Type.String(),
		Sdp:     desc.SDP,
	}
}

// Validate validates the WebRtcSdp message.
func (s *WebRtcSdp) Validate() error {
	if s.GetSdpType() == "" {
		return errors.New("sdp_type: cannot be empty")
	}
	if s.ParseSDPType() == webrtc.SDPTypeUnknown {
		return errors.Errorf("sdp_type: unknown sdp type: %v", s.GetSdpType())
	}
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
	if _, err := s.ParseICECandidateInit(); err != nil {
		return err
	}
	return nil
}

// NewWebRtcIce constructs a new WebRtcIce from a ICECandidateInit.
func NewWebRtcIce(candidate *webrtc.ICECandidateInit) (*WebRtcIce, error) {
	data, err := json.Marshal(candidate)
	if err != nil {
		return nil, err
	}
	return &WebRtcIce{Candidate: string(data)}, nil
}

// ParseICECandidateInit parses the ICECandidate from the JSON encoded body.
func (s *WebRtcIce) ParseICECandidateInit() (*webrtc.ICECandidateInit, error) {
	msg := &webrtc.ICECandidateInit{}
	if err := json.Unmarshal([]byte(s.GetCandidate()), msg); err != nil {
		return nil, errors.Wrap(err, "invalid ice candidate json")
	}
	return msg, nil
}
