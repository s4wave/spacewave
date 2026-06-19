//go:build !js

// SSH host-key trust pins parse and fingerprint authorized_keys via
// golang.org/x/crypto/ssh, which pulls reflect through its wire codec. Only the
// native terminal SSH client (sdk/terminal/ssh_connect.go, also !js) calls
// these; the browser GoScript closure registers the SSH host op without them, so
// this file stays native-only to keep reflect out of the web build.
package s4wave_sshhost

import (
	"bytes"
	"context"
	"strings"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"golang.org/x/crypto/ssh"
)

// NewSshHostKeyPinFromPublicKey builds the durable SSH Host trust record for a user-accepted key.
func NewSshHostKeyPinFromPublicKey(key ssh.PublicKey, acceptedAt time.Time, acceptedByPeerID string) *SshHostKeyPin {
	return &SshHostKeyPin{
		Algorithm:         key.Type(),
		PublicKey:         strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))),
		Sha256Fingerprint: ssh.FingerprintSHA256(key),
		AcceptedAt:        timestamppb.New(acceptedAt),
		AcceptedByPeerId:  acceptedByPeerID,
	}
}

// SshHostKeyPinsMatchPublicKey checks whether a stored SSH Host pin accepts a presented key.
func SshHostKeyPinsMatchPublicKey(pins []*SshHostKeyPin, key ssh.PublicKey) bool {
	fingerprint := ssh.FingerprintSHA256(key)
	authorizedKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
	for _, pin := range pins {
		if pin == nil {
			continue
		}
		if alg := strings.TrimSpace(pin.GetAlgorithm()); alg != "" && alg != key.Type() {
			continue
		}
		if pinned := strings.TrimSpace(pin.GetSha256Fingerprint()); pinned != "" && pinned == fingerprint {
			return true
		}
		if pinned := normalizeSshHostPinnedPublicKey(pin.GetPublicKey()); pinned != "" {
			parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pinned))
			if err == nil && bytes.Equal(parsed.Marshal(), key.Marshal()) {
				return true
			}
			if pinned == authorizedKey {
				return true
			}
		}
	}
	return false
}

// RememberSshHostKeyPin appends a newly accepted key pin to the SSH Host if it is not already remembered.
func RememberSshHostKeyPin(ctx context.Context, eng world.Engine, objectKey string, pin *SshHostKeyPin) error {
	if eng == nil {
		return errors.New("world engine is required to remember SSH host keys")
	}
	rememberedPin := normalizeRememberedSshHostKeyPin(pin)
	if err := validateHostKeyPin(rememberedPin); err != nil {
		return err
	}
	pinKey, err := parseSshHostPublicKey(rememberedPin.GetPublicKey())
	if err != nil {
		return err
	}
	return world.ExecTransaction(ctx, eng, true, func(ctx context.Context, wtx world.WorldState) error {
		writeState, found, err := wtx.GetObject(ctx, objectKey)
		if err != nil {
			return err
		}
		if !found {
			return world.ErrObjectNotFound
		}
		if err := world_types.CheckObjectType(ctx, wtx, objectKey, SshHostTypeID); err != nil {
			return err
		}
		_, _, err = world.AccessObjectState(ctx, writeState, true, func(bcs *block.Cursor) error {
			host, err := UnmarshalSshHost(ctx, bcs)
			if err != nil {
				return err
			}
			if SshHostKeyPinsMatchPublicKey(host.GetHostKeyPins(), pinKey) {
				return nil
			}
			host.HostKeyPins = append(host.HostKeyPins, rememberedPin.CloneVT())
			host.UpdatedAt = timestamppb.New(time.Now())
			if err := host.Validate(); err != nil {
				return err
			}
			bcs.SetBlock(host, true)
			return nil
		})
		return err
	})
}

func normalizeRememberedSshHostKeyPin(pin *SshHostKeyPin) *SshHostKeyPin {
	if pin == nil {
		return nil
	}
	out := pin.CloneVT()
	out.PublicKey = normalizeSshHostPinnedPublicKey(out.GetPublicKey())
	if out.GetAcceptedAt() == nil {
		out.AcceptedAt = timestamppb.New(time.Now())
	}
	return out
}

func normalizeSshHostPinnedPublicKey(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	for idx := range fields {
		if idx+1 >= len(fields) {
			break
		}
		candidate := fields[idx] + " " + fields[idx+1]
		if _, _, _, _, err := ssh.ParseAuthorizedKey([]byte(candidate)); err == nil {
			return candidate
		}
	}
	return strings.TrimSpace(value)
}

func parseSshHostPublicKey(value string) (ssh.PublicKey, error) {
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(value))
	if err != nil {
		return nil, err
	}
	return key, nil
}
