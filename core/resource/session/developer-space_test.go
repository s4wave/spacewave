package resource_session_test

import (
	"context"
	"sync"
	"testing"
	"time"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/space"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// ensureDeveloperSpace returns the developer Space id the resource reports.
func ensureDeveloperSpace(ctx context.Context, t *testing.T, r *resource_session.SessionResource) string {
	t.Helper()
	resp, err := r.EnsureDeveloperSpace(ctx, &s4wave_session.EnsureDeveloperSpaceRequest{})
	if err != nil {
		t.Fatalf("EnsureDeveloperSpace: %v", err)
	}
	return resp.GetSharedObjectId()
}

// countSpaces returns the number of Spaces on the account.
func countSpaces(t *testing.T, acc *provider_local.ProviderAccount) int {
	t.Helper()
	listCtr, release, err := acc.AccessSharedObjectList(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	n := 0
	for _, entry := range listCtr.GetValue().GetSharedObjects() {
		if entry.GetMeta().GetBodyType() == space.SpaceBodyType {
			n++
		}
	}
	return n
}

// TestEnsureDeveloperSpace checks that an account creates its developer Space
// on first use, records it in the account settings, and returns the same Space
// to every later call from any resource of the account.
func TestEnsureDeveloperSpace(t *testing.T) {
	// Bound the test.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Create a Session and a second resource over the same account.
	env := setupTestEnv(ctx, t)
	sessRef, _ := env.createSession(ctx, t)
	acc := env.accessAccount(ctx, t, sessRef)
	first := env.buildSessionResource(ctx, t, sessRef)
	second := env.buildSessionResource(ctx, t, sessRef)

	// The first use creates the Space.
	id := ensureDeveloperSpace(ctx, t, first)
	if id == "" {
		t.Fatal("expected a developer space id")
	}
	if n := countSpaces(t, acc); n != 1 {
		t.Fatalf("expected 1 space, got %d", n)
	}

	// The account settings record it.
	ref, err := acc.GetAccountSettingsRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	so, release, err := acc.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := account_settings.ReadSnapshot(ctx, snap)
	if err != nil {
		t.Fatal(err)
	}
	if settings.GetDeveloperSpaceId() != id {
		t.Fatalf("expected the settings to record %q, got %q", id, settings.GetDeveloperSpaceId())
	}

	// A second device reuses it without creating another Space.
	if got := ensureDeveloperSpace(ctx, t, second); got != id {
		t.Fatalf("expected developer space %q, got %q", id, got)
	}
	if n := countSpaces(t, acc); n != 1 {
		t.Fatalf("expected 1 space, got %d", n)
	}
}

// ensureDeveloperSpaceTogether ensures the developer Space from every resource
// at once and returns the ids they report.
func ensureDeveloperSpaceTogether(ctx context.Context, t *testing.T, resources []*resource_session.SessionResource) []string {
	t.Helper()
	ids := make([]string, len(resources))
	var wg sync.WaitGroup
	for i, r := range resources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids[i] = ensureDeveloperSpace(ctx, t, r)
		}()
	}
	wg.Wait()
	return ids
}

// requireOneDeveloperSpace fails unless every id names one Space, which is
// the only Space on the account.
func requireOneDeveloperSpace(t *testing.T, acc *provider_local.ProviderAccount, ids []string) {
	t.Helper()
	for _, id := range ids {
		if id == "" || id != ids[0] {
			t.Fatalf("expected one developer space, got %v", ids)
		}
	}
	if n := countSpaces(t, acc); n != 1 {
		t.Fatalf("expected 1 space, got %d", n)
	}
}

// TestEnsureDeveloperSpaceRace checks that devices adding their first
// repository at the same time agree on one developer Space and delete the
// Spaces that lost.
func TestEnsureDeveloperSpaceRace(t *testing.T) {
	// Bound the test.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Ensure the developer Space from several resources at once.
	env := setupTestEnv(ctx, t)
	sessRef, _ := env.createSession(ctx, t)
	acc := env.accessAccount(ctx, t, sessRef)
	resources := make([]*resource_session.SessionResource, 4)
	for i := range resources {
		resources[i] = env.buildSessionResource(ctx, t, sessRef)
	}
	requireOneDeveloperSpace(t, acc, ensureDeveloperSpaceTogether(ctx, t, resources))
}

// TestEnsureDeveloperSpaceDangling checks that a developer Space deleted after
// it was recorded is replaced once, even when several devices notice at the
// same time.
func TestEnsureDeveloperSpaceDangling(t *testing.T) {
	// Bound the test.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Create the developer Space and delete it behind the settings' back.
	env := setupTestEnv(ctx, t)
	sessRef, _ := env.createSession(ctx, t)
	acc := env.accessAccount(ctx, t, sessRef)
	resources := make([]*resource_session.SessionResource, 4)
	for i := range resources {
		resources[i] = env.buildSessionResource(ctx, t, sessRef)
	}
	gone := ensureDeveloperSpace(ctx, t, resources[0])
	if err := acc.DeleteSharedObject(ctx, gone); err != nil {
		t.Fatal(err)
	}

	// A device creates a replacement and records it.
	replaced := ensureDeveloperSpace(ctx, t, resources[0])
	if replaced == "" || replaced == gone {
		t.Fatalf("expected a replacement for %q, got %q", gone, replaced)
	}
	requireOneDeveloperSpace(t, acc, []string{replaced})
	if got := ensureDeveloperSpace(ctx, t, resources[1]); got != replaced {
		t.Fatalf("expected developer space %q, got %q", replaced, got)
	}

	// Several devices replace the next missing Space at once and agree.
	if err := acc.DeleteSharedObject(ctx, replaced); err != nil {
		t.Fatal(err)
	}
	ids := ensureDeveloperSpaceTogether(ctx, t, resources)
	if ids[0] == replaced {
		t.Fatalf("expected a replacement for %q", replaced)
	}
	requireOneDeveloperSpace(t, acc, ids)
}
