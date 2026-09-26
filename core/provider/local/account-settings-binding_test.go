package provider_local_test

import (
	"testing"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
)

// TestAccountSettingsBindingBootstrap verifies the local provider persists a
// bound unique-id account settings ref when the account first initializes.
func TestAccountSettingsBindingBootstrap(t *testing.T) {
	ctx := t.Context()

	_, _, acc, _, release := setupProviderAndSession(ctx, t)
	defer release()

	ref, err := acc.GetAccountSettingsRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ref.GetProviderResourceRef().GetId() == account_settings.BindingPurpose {
		t.Fatalf("expected unique account settings id, got binding purpose %q", account_settings.BindingPurpose)
	}

	soList := acc.GetSOListCtr().GetValue()
	if soList == nil {
		t.Fatal("shared object list is nil")
	}

	var found bool
	for _, entry := range soList.GetSharedObjects() {
		entryRef := entry.GetRef()
		if entryRef.GetProviderResourceRef().GetId() != ref.GetProviderResourceRef().GetId() {
			continue
		}
		if entry.GetMeta().GetBodyType() != account_settings.BodyType {
			t.Fatalf("expected body type %q, got %q", account_settings.BodyType, entry.GetMeta().GetBodyType())
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("bound account settings SO %q not found in shared object list", ref.GetProviderResourceRef().GetId())
	}
}
