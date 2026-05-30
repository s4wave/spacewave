package s4wave_sshhost

import (
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/testbed"
)

func TestCreateSshHostOpCreatesTypedHostWithoutCredentialPayload(t *testing.T) {
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	op := NewCreateSshHostOp(
		"hosts/prod",
		"Prod Host",
		&SshHostEndpoint{Host: "prod.example.com", Username: "deploy"},
		&SshHostCredentialRefs{PrivateKeySecretObjectKey: "secrets/ssh/prod-key"},
		[]*SshHostKeyPin{{
			Algorithm:         "ssh-ed25519",
			Sha256Fingerprint: "SHA256:example",
		}},
		time.Unix(100, 0),
	)
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, op, ""); err != nil {
		t.Fatalf("ApplyWorldOp: %v", err)
	}

	typeID, err := world_types.GetObjectType(ctx, tb.WorldState, "hosts/prod")
	if err != nil {
		t.Fatalf("GetObjectType: %v", err)
	}
	if typeID != SshHostTypeID {
		t.Fatalf("type id = %q, want %q", typeID, SshHostTypeID)
	}

	obj, found, err := tb.WorldState.GetObject(ctx, "hosts/prod")
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	if !found {
		t.Fatal("ssh host object not found")
	}

	var host *SshHost
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var uerr error
		host, uerr = UnmarshalSshHost(ctx, bcs)
		return uerr
	})
	if err != nil {
		t.Fatalf("AccessObjectState: %v", err)
	}
	if host.GetEndpoint().GetPort() != DefaultSshPort {
		t.Fatalf("endpoint port = %d, want %d", host.GetEndpoint().GetPort(), DefaultSshPort)
	}
	if host.GetCredentials().GetPrivateKeySecretObjectKey() != "secrets/ssh/prod-key" {
		t.Fatalf("private key secret ref = %q", host.GetCredentials().GetPrivateKeySecretObjectKey())
	}
	if host.GetCredentials().GetPasswordSecretObjectKey() != "" {
		t.Fatal("ssh host should not contain raw password payload")
	}
}
