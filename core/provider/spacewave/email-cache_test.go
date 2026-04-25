package provider_spacewave

import (
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestSetCachedPrimaryEmailUpdatesCachedRowsImmediately(t *testing.T) {
	acc := &ProviderAccount{}
	acc.state.cachedEmailsValid = true
	acc.state.cachedEmails = []*api.AccountEmailInfo{
		{
			Email:    "old@example.com",
			Verified: true,
			Primary:  true,
		},
		{
			Email:    "new@example.com",
			Verified: true,
			Primary:  false,
		},
	}

	prevOld := acc.state.cachedEmails[0]
	prevNew := acc.state.cachedEmails[1]

	acc.SetCachedPrimaryEmail("new@example.com")

	if !acc.state.cachedEmails[1].GetPrimary() {
		t.Fatal("expected new@example.com to become primary in cached emails")
	}
	if acc.state.cachedEmails[0].GetPrimary() {
		t.Fatal("expected old@example.com to lose primary in cached emails")
	}
	if acc.state.cachedEmails[0] == prevOld {
		t.Fatal("expected old primary row to be cloned before mutation")
	}
	if acc.state.cachedEmails[1] == prevNew {
		t.Fatal("expected new primary row to be cloned before mutation")
	}
}
