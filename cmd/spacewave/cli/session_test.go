//go:build !js

package spacewave_cli

import (
	"testing"

	core_provider "github.com/s4wave/spacewave/core/provider"
	core_session "github.com/s4wave/spacewave/core/session"
)

func TestValidateSessionPeerIDRejectsAccountID(t *testing.T) {
	err := validateSessionPeerID("01kq3t7sv1anq7gnfcxbn09xbh")
	if err == nil {
		t.Fatal("expected account id to be rejected")
	}
}

func TestResolveSessionLogoutEntryMatchesIndexAndAccount(t *testing.T) {
	sessions := []*core_session.SessionListEntry{
		testSessionListEntry(1, "sess-local", "local", "acct-local"),
		testSessionListEntry(2, "sess-cloud", "spacewave", "acct-cloud"),
	}

	entry, err := resolveSessionLogoutEntry(sessions, sessionLogoutTarget{Positional: "2"}, 1)
	if err != nil {
		t.Fatalf("resolve index: %v", err)
	}
	if entry.GetSessionIndex() != 2 {
		t.Fatalf("index selector: got %d", entry.GetSessionIndex())
	}

	entry, err = resolveSessionLogoutEntry(sessions, sessionLogoutTarget{AccountID: "acct-cloud"}, 1)
	if err != nil {
		t.Fatalf("resolve account: %v", err)
	}
	if entry.GetSessionIndex() != 2 {
		t.Fatalf("account selector: got %d", entry.GetSessionIndex())
	}

	entry, err = resolveSessionLogoutEntry(sessions, sessionLogoutTarget{}, 1)
	if err != nil {
		t.Fatalf("resolve default: %v", err)
	}
	if entry.GetSessionIndex() != 1 {
		t.Fatalf("default selector: got %d", entry.GetSessionIndex())
	}
}

func TestResolveSessionLogoutEntryRejectsAmbiguousAccount(t *testing.T) {
	sessions := []*core_session.SessionListEntry{
		testSessionListEntry(1, "sess-a", "spacewave", "acct"),
		testSessionListEntry(2, "sess-b", "spacewave", "acct"),
	}
	_, err := resolveSessionLogoutEntry(sessions, sessionLogoutTarget{AccountID: "acct"}, 1)
	if err == nil {
		t.Fatal("expected ambiguous account error")
	}
}

func testSessionListEntry(idx uint32, sessionID, providerID, accountID string) *core_session.SessionListEntry {
	return &core_session.SessionListEntry{
		SessionIndex: idx,
		SessionRef: &core_session.SessionRef{
			ProviderResourceRef: &core_provider.ProviderResourceRef{
				Id:                sessionID,
				ProviderId:        providerID,
				ProviderAccountId: accountID,
			},
		},
	}
}
