package s4wave_sshhost

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

const (
	// SshHostTypeID is the type identifier for SSH-only Host objects.
	SshHostTypeID = "spacewave/ssh-host"
	// DefaultSshPort is the default SSH port applied by create paths.
	DefaultSshPort uint32 = 22
)

// NewSshHostBlock constructs a new SSH Host block.
func NewSshHostBlock() block.Block {
	return &SshHost{}
}

// UnmarshalSshHost unmarshals an SSH Host from a cursor.
func UnmarshalSshHost(ctx context.Context, bcs *block.Cursor) (*SshHost, error) {
	return block.UnmarshalBlock[*SshHost](ctx, bcs, NewSshHostBlock)
}

// MarshalBlock marshals the SSH Host to bytes.
func (h *SshHost) MarshalBlock() ([]byte, error) {
	return h.MarshalVT()
}

// UnmarshalBlock unmarshals the SSH Host from bytes.
func (h *SshHost) UnmarshalBlock(data []byte) error {
	return h.UnmarshalVT(data)
}

// NormalizeSshHostEndpoint fills endpoint defaults without changing identity.
func NormalizeSshHostEndpoint(endpoint *SshHostEndpoint) *SshHostEndpoint {
	if endpoint == nil {
		return &SshHostEndpoint{Port: DefaultSshPort}
	}
	out := endpoint.CloneVT()
	out.Host = strings.TrimSpace(out.GetHost())
	out.Username = strings.TrimSpace(out.GetUsername())
	if out.GetPort() == 0 {
		out.Port = DefaultSshPort
	}
	return out
}

// Validate performs cursory checks on the SSH Host block.
func (h *SshHost) Validate() error {
	if strings.TrimSpace(h.GetLabel()) == "" {
		return errors.New("ssh host label is required")
	}
	endpoint := h.GetEndpoint()
	if endpoint == nil {
		return errors.New("ssh host endpoint is required")
	}
	if strings.TrimSpace(endpoint.GetHost()) == "" {
		return errors.New("ssh host endpoint host is required")
	}
	if endpoint.GetPort() == 0 || endpoint.GetPort() > 65535 {
		return errors.New("ssh host endpoint port is invalid")
	}
	if strings.TrimSpace(endpoint.GetUsername()) == "" {
		return errors.New("ssh host endpoint username is required")
	}
	if err := validateCredentialRefs(h.GetCredentials()); err != nil {
		return err
	}
	for _, pin := range h.GetHostKeyPins() {
		if err := validateHostKeyPin(pin); err != nil {
			return err
		}
	}
	return nil
}

func validateCredentialRefs(refs *SshHostCredentialRefs) error {
	if refs == nil {
		return nil
	}
	for _, ref := range []struct {
		name string
		key  string
	}{
		{name: "private_key_secret_object_key", key: refs.GetPrivateKeySecretObjectKey()},
		{name: "password_secret_object_key", key: refs.GetPasswordSecretObjectKey()},
		{name: "passphrase_secret_object_key", key: refs.GetPassphraseSecretObjectKey()},
	} {
		key := strings.TrimSpace(ref.key)
		if ref.key != "" && (key == "" || key != ref.key) {
			return errors.Errorf("ssh host credential %s is empty", ref.name)
		}
		if strings.ContainsAny(ref.key, " \r\n\t") {
			return errors.Errorf("ssh host credential %s must be a Secret object key", ref.name)
		}
	}
	return nil
}

func validateHostKeyPin(pin *SshHostKeyPin) error {
	if pin == nil {
		return errors.New("ssh host key pin is required")
	}
	if strings.TrimSpace(pin.GetAlgorithm()) == "" {
		return errors.New("ssh host key pin algorithm is required")
	}
	if strings.TrimSpace(pin.GetPublicKey()) == "" && strings.TrimSpace(pin.GetSha256Fingerprint()) == "" {
		return errors.New("ssh host key pin public_key or sha256_fingerprint is required")
	}
	if pin.GetAcceptedAt() != nil {
		if err := pin.GetAcceptedAt().Validate(false); err != nil {
			return err
		}
	}
	return nil
}

// _ is a type assertion.
var _ block.Block = (*SshHost)(nil)
