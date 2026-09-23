package sobject

import (
	"bytes"
	"testing"
)

// TestCreateInviteOverLimit rejects a finite invitation whose initial count exceeds its limit.
func TestCreateInviteOverLimit(t *testing.T) {
	peers := createMockPeers(t, 1)
	owner, err := peers[0].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	initial := createMockSOState(peers, nil)
	host, held := newTestSOHost(t.Context(), initial)
	invite := &SOInvite{InviteId: "bounded", TokenHash: []byte{1}, MaxUses: 1, Uses: 2}
	if err := host.CreateInvite(t.Context(), owner, invite); err == nil {
		t.Fatal("accepted an initial use count above its finite limit")
	}
	if !(*held).EqualVT(initial) {
		t.Fatal("rejected creation changed the checkpoint")
	}
}

// TestInviteLockedCheckpoint validates invitations against the checkpoint held for mutation.
func TestInviteLockedCheckpoint(t *testing.T) {
	peers := createMockPeers(t, 1)
	owner, err := peers[0].GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	initial := createMockSOState(peers, nil)
	initial.Config.ConfigChainHash = bytes.Repeat([]byte{1}, 32)
	initial.Root = createMockSORoot(t, 1, peers[0])
	initial.Invites = []*SOInvite{{InviteId: "bounded", TokenHash: []byte{1}, MaxUses: 1}}
	for _, action := range []string{"create", "increment", "revoke"} {
		t.Run(action, func(t *testing.T) {
			host, held := newTestSOHost(t.Context(), initial.CloneVT())
			// A trusted replacement can retain the configuration while changing
			// invitations after the watched snapshot was read and before locking.
			locked := initial.CloneVT()
			invite := &SOInvite{InviteId: "new", TokenHash: []byte{2}}
			switch action {
			case "create":
				locked.Invites = append(locked.Invites, invite.CloneVT())
			case "increment":
				locked.Invites[0].Uses = 1
			case "revoke":
				locked.Invites[0].Revoked = true
			}
			*held = locked
			before := locked.CloneVT()
			var err error
			switch action {
			case "create":
				err = host.CreateInvite(t.Context(), owner, invite)
			case "increment":
				err = host.IncrementInviteUses(t.Context(), owner, "bounded")
			case "revoke":
				err = host.RevokeInvite(t.Context(), owner, "bounded")
			}
			if err == nil {
				t.Fatal("accepted an invitation mutation invalid under the locked checkpoint")
			}
			if !(*held).EqualVT(before) {
				t.Fatal("rejected invitation mutation changed the held checkpoint")
			}
		})
	}
}
