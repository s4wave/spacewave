package s4wave_sshhost

import (
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/testbed"
	"golang.org/x/crypto/ssh"
)

func TestRememberSshHostKeyPinNormalizesAndDeduplicatesAcceptedKey(t *testing.T) {
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	op := NewCreateSshHostOp(
		"hosts/prod",
		"Prod Host",
		&SshHostEndpoint{Host: "192.168.1.15", Port: 1940, Username: "root"},
		nil,
		nil,
		time.Unix(10, 0),
	)
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, op, ""); err != nil {
		t.Fatalf("ApplyWorldOp: %v", err)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	pin := NewSshHostKeyPinFromPublicKey(
		signer.PublicKey(),
		time.Unix(20, 0),
		"12D3KooWReviewer",
	)
	pin.PublicKey = "[192.168.1.15]:1940 " + pin.GetPublicKey()

	if err := RememberSshHostKeyPin(ctx, tb.Engine, "hosts/prod", pin); err != nil {
		t.Fatalf("RememberSshHostKeyPin: %v", err)
	}
	if err := RememberSshHostKeyPin(ctx, tb.Engine, "hosts/prod", pin); err != nil {
		t.Fatalf("RememberSshHostKeyPin duplicate: %v", err)
	}

	host, _, err := world.LookupObject[*SshHost](
		ctx,
		tb.WorldState,
		"hosts/prod",
		NewSshHostBlock,
	)
	if err != nil {
		t.Fatalf("LookupObject: %v", err)
	}
	if got := len(host.GetHostKeyPins()); got != 1 {
		t.Fatalf("host key pins = %d, want 1", got)
	}
	remembered := host.GetHostKeyPins()[0]
	if remembered.GetAcceptedByPeerId() != "12D3KooWReviewer" {
		t.Fatalf("accepted by peer id = %q", remembered.GetAcceptedByPeerId())
	}
	if remembered.GetPublicKey() != strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))) {
		t.Fatalf("remembered public key was not normalized: %q", remembered.GetPublicKey())
	}
	if !SshHostKeyPinsMatchPublicKey(host.GetHostKeyPins(), signer.PublicKey()) {
		t.Fatal("remembered pin does not match accepted key")
	}
}
