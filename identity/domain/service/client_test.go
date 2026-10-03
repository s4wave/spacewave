package identity_domain_service

import (
	"testing"

	"github.com/s4wave/spacewave/identity"
	"github.com/s4wave/spacewave/net/crypto"
	uuid "github.com/satori/go.uuid"
)

// TestValidateLookupEntity tests validating a looked-up entity.
func TestValidateLookupEntity(t *testing.T) {
	// Create a signed entity for the requested domain and entity IDs.
	privKey, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	domainID, entityID := "test-domain", "test-entity"
	ent, err := identity.EntityWithPrivKey(domainID, entityID, uuid.NewV4().String(), privKey, "", nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify entity validation accepts the fixture and rejects missing or mismatched identities.
	if err := ValidateLookupEntity(ent, domainID, entityID); err != nil {
		t.Fatal(err.Error())
	}
	if err := ValidateLookupEntity(nil, domainID, entityID); err == nil {
		t.Fatal("expected error for nil entity")
	}
	if err := ValidateLookupEntity(ent, "other-domain", entityID); err == nil {
		t.Fatal("expected error for mismatched domain id")
	}
	if err := ValidateLookupEntity(ent, domainID, "other-entity"); err == nil {
		t.Fatal("expected error for mismatched entity id")
	}

	// An entity with matching IDs but invalid signatures is rejected.
	bad := ent.CloneVT()
	bad.EntityUuid = "not-a-uuid"
	if err := ValidateLookupEntity(bad, domainID, entityID); err == nil {
		t.Fatal("expected error for invalid entity")
	}
}
