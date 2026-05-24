package emailcache

import (
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestSetPrimaryEmailUpdatesCachedRowsImmediately(t *testing.T) {
	emails := []*api.AccountEmailInfo{
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

	prevOld := emails[0]
	prevNew := emails[1]

	next, changed := SetPrimaryEmail(emails, "new@example.com")
	if !changed {
		t.Fatal("expected cache update")
	}
	if !next[1].GetPrimary() {
		t.Fatal("expected new@example.com to become primary in cached emails")
	}
	if next[0].GetPrimary() {
		t.Fatal("expected old@example.com to lose primary in cached emails")
	}
	if next[0] == prevOld {
		t.Fatal("expected old primary row to be cloned before mutation")
	}
	if next[1] == prevNew {
		t.Fatal("expected new primary row to be cloned before mutation")
	}
}
