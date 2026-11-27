package provider_local

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
)

// TestStateWritesRefuseUnownedIds checks that a transfer write to an unlisted
// id and an enrollment with an invalid id write nothing.
func TestStateWritesRefuseUnownedIds(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	_, _, a, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	// A transfer write to an unlisted id fails and stores nothing.
	owner, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	err = a.ReplaceSharedObjectState(ctx, a.le, "unlisted", owner, []byte("fake-world"), ReplaceOrigin{})
	if !errors.Is(err, sobject.ErrSharedObjectNotFound) {
		t.Fatalf("a transfer write to an unlisted id returned %v", err)
	}
	if n := countTestKeys(ctx, t, a, SobjectObjectStorePrefix("unlisted")); n != 0 {
		t.Fatalf("a transfer write to an unlisted id left %d keys", n)
	}

	// An enrollment with an id that is not a DNS label fails before writing.
	// Its ref check is the only one before the placeholder write: the genesis
	// builder does not validate the id, so without the ref check keys land
	// under so/a/.
	_, err = a.mountEnrolledSO(ctx, "a/x", &sobject.SharedObjectMeta{BodyType: "space"}, "shared", nil, &sobject.SOState{}, nil, nil)
	if err == nil {
		t.Fatal("an enrollment with an invalid id succeeded")
	}
	if n := countTestKeys(ctx, t, a, []byte("so/a/")); n != 0 {
		t.Fatalf("an enrollment with an invalid id left %d keys", n)
	}
}
