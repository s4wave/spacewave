package s4wave_sshhost

import (
	"bytes"
	"context"
	"testing"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	"github.com/s4wave/spacewave/testbed"
)

func TestValidateSshHostCredentialSecretsChecksSecretKinds(t *testing.T) {
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	createSecretParent(ctx, t, tb.WorldState, "secrets/ssh/key", s4wave_secret.SecretKindSSHPrivateKey)
	createSecretParent(ctx, t, tb.WorldState, "secrets/ssh/password", s4wave_secret.SecretKindSSHPassword)
	createSecretParent(ctx, t, tb.WorldState, "secrets/ssh/passphrase", s4wave_secret.SecretKindSSHPassphrase)
	createSecretParent(ctx, t, tb.WorldState, "secrets/ssh/wrong", s4wave_secret.SecretKindMatrixAccessToken)

	refs := &SshHostCredentialRefs{
		PrivateKeySecretObjectKey: "secrets/ssh/key",
		PasswordSecretObjectKey:   "secrets/ssh/password",
		PassphraseSecretObjectKey: "secrets/ssh/passphrase",
	}
	if err := ValidateSshHostCredentialSecrets(ctx, tb.WorldState, refs); err != nil {
		t.Fatalf("ValidateSshHostCredentialSecrets: %v", err)
	}

	refs.PrivateKeySecretObjectKey = "secrets/ssh/wrong"
	if err := ValidateSshHostCredentialSecrets(ctx, tb.WorldState, refs); err == nil {
		t.Fatal("expected mismatched SSH credential kind to fail")
	}

	refs.PrivateKeySecretObjectKey = "secrets/ssh/missing"
	if err := ValidateSshHostCredentialSecrets(ctx, tb.WorldState, refs); err == nil {
		t.Fatal("expected missing SSH credential Secret to fail")
	}
}

func TestCredentialMaterialAbsentFromHostDeviceTerminalComputersBlocks(t *testing.T) {
	rawCredential := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nspacewave-secret\n-----END OPENSSH PRIVATE KEY-----")
	host := &SshHost{
		Label: "SSH Host",
		Endpoint: &SshHostEndpoint{
			Host:     "prod.example.com",
			Port:     22,
			Username: "deploy",
		},
		Credentials: &SshHostCredentialRefs{
			PrivateKeySecretObjectKey: "secrets/ssh/key",
		},
		HostKeyPins: []*SshHostKeyPin{{
			Algorithm:         "ssh-ed25519",
			Sha256Fingerprint: "SHA256:example",
		}},
	}
	device := &s4wave_device.Device{
		PeerId: "device-peer",
		Label:  "Device",
		Capabilities: []*s4wave_device.DeviceCapability{{
			Id:    "terminal",
			Kind:  "terminal",
			Label: "Terminal",
			Link: &s4wave_device.DeviceCapabilityLink{
				ObjectKey:  "hosts/prod",
				TypeId:     SshHostTypeID,
				ProtocolId: "alpha/remote-shell/v0",
			},
		}},
	}
	computers := &s4wave_device.ComputersDashboard{Name: "Computers"}

	for name, block := range map[string]block.Block{
		"ssh host":  host,
		"device":    device,
		"computers": computers,
	} {
		data, err := block.MarshalBlock()
		if err != nil {
			t.Fatalf("%s MarshalBlock: %v", name, err)
		}
		if bytes.Contains(data, rawCredential) {
			t.Fatalf("%s block contains raw SSH credential bytes", name)
		}
	}
}

func createSecretParent(
	ctx context.Context,
	t *testing.T,
	ws world.WorldState,
	objectKey string,
	kind string,
) {
	t.Helper()
	secret := &s4wave_secret.Secret{
		DisplayName: objectKey,
		Kind:        kind,
		CreatedAt:   timestamppb.New(time.Unix(100, 0)),
		UpdatedAt:   timestamppb.New(time.Unix(100, 0)),
	}
	if _, _, err := world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(secret, true)
		return nil
	}); err != nil {
		t.Fatalf("CreateWorldObject %s: %v", objectKey, err)
	}
	if err := world_types.SetObjectType(ctx, ws, objectKey, s4wave_secret.SecretTypeID); err != nil {
		t.Fatalf("SetObjectType %s: %v", objectKey, err)
	}
}
