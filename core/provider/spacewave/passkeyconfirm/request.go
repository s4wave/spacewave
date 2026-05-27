package passkeyconfirm

import api "github.com/s4wave/spacewave/core/provider/spacewave/api"

const (
	// Method is the HTTP verb for passkey confirmation.
	Method = "POST"
	// Path is the cloud API path for passkey confirmation.
	Path = "/api/auth/passkey/confirm"
	// ContentType is the binary protobuf request content type.
	ContentType = "application/octet-stream"
)

// Request is the passkey confirmation wire payload.
type Request struct {
	CredentialJSON   string
	Username         string
	WrappedEntityKey string
	EntityPeerID     string
	SessionPeerID    string
	PinWrapped       bool
	PrfCapable       bool
	PrfSalt          string
	AuthParams       string
}

// DesktopResponse is the native desktop passkey confirmation result.
type DesktopResponse struct {
	AccountID     string
	SessionPeerID string
}

// BuildRequest builds the protobuf request for the passkey confirm endpoint.
func BuildRequest(req *Request) *api.PasskeyConfirmRequest {
	return &api.PasskeyConfirmRequest{
		CredentialJson:   req.CredentialJSON,
		Username:         req.Username,
		WrappedEntityKey: req.WrappedEntityKey,
		EntityPeerId:     req.EntityPeerID,
		SessionPeerId:    req.SessionPeerID,
		PinWrapped:       req.PinWrapped,
		PrfCapable:       req.PrfCapable,
		PrfSalt:          req.PrfSalt,
		AuthParams:       req.AuthParams,
	}
}

// MarshalRequest marshals the passkey confirm request body.
func MarshalRequest(req *Request) ([]byte, error) {
	return BuildRequest(req).MarshalVT()
}

// ParseDesktopResponse parses the native desktop passkey confirmation result.
func ParseDesktopResponse(body []byte) (*DesktopResponse, error) {
	resp := &api.PasskeyConfirmResponse{}
	if err := resp.UnmarshalVT(body); err != nil {
		return nil, err
	}
	return &DesktopResponse{
		AccountID:     resp.GetAccountId(),
		SessionPeerID: resp.GetSessionPeerId(),
	}, nil
}
