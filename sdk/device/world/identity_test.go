package s4wave_device_world_test

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_device_world "github.com/s4wave/spacewave/sdk/device/world"
)

// TestEnsureDevice checks caller-owned atomic pairing and subsequent session readiness.
func TestEnsureDevice(t *testing.T) {
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	local, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	id := local.GetPeerID()
	key := s4wave_device_world.EnrolledDeviceObjectKey(id)
	edge := world.NewGraphQuadWithKeys("dashboards/computers", "test/paired-device", key, "")
	seed, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(seed.Discard)
	_, _, err = world.AccessWorldObject(ctx, seed, "dashboards/computers", true, func(cursor *block.Cursor) error {
		cursor.SetBlock(&s4wave_device.ComputersDashboard{Name: "Computers"}, true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	seed.Discard()

	// A failed application binding can discard the Device and its edge together.
	preview, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(preview.Discard)
	createdKey, created, err := s4wave_device_world.EnsureDevice(ctx, preview, id, "My PC")
	if err != nil {
		t.Fatal(err)
	}
	if createdKey != key || created.IsSelectable() || created.GetLastStatus() != nil || len(created.Capabilities) != 0 {
		t.Fatal("pairing asserted native readiness or changed Device identity", createdKey, created)
	}
	if err := preview.SetGraphQuad(ctx, edge); err != nil {
		t.Fatal(err)
	}
	preview.Discard()
	read, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(read.Discard)
	if exists, err := read.HasObject(ctx, key); err != nil || exists {
		t.Fatal("discarded Device retained", exists, err)
	}
	if edges, err := read.LookupGraphQuads(ctx, edge, 0); err != nil || len(edges) != 0 {
		t.Fatal("discarded pairing edge retained", edges, err)
	}
	read.Discard()

	// Commit identity and the application's association in one transaction.
	tx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tx.Discard)
	_, paired, err := s4wave_device_world.EnsureDevice(ctx, tx, id, "My PC")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.SetGraphQuad(ctx, edge); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx.Discard()
	before, err := tb.Engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	check, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(check.Discard)
	_, repeated, err := s4wave_device_world.EnsureDevice(ctx, check, id, "")
	if err != nil || !paired.EqualVT(repeated) {
		t.Fatal("repeated pairing changed saved metadata", repeated, err)
	}
	if edges, err := check.LookupGraphQuads(ctx, edge, 0); err != nil || len(edges) != 1 {
		t.Fatal("committed pairing edge missing", edges, err)
	}
	if err := check.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	check.Discard()
	if after, err := tb.Engine.GetSeqno(ctx); err != nil || after != before {
		t.Fatal("repeat pairing created a revision", before, after, err)
	}

	// Actual session enrollment promotes the same object, retaining pairing metadata.
	if readyKey, err := s4wave_device_world.EnsureEnrolledDevice(ctx, tb.Engine, id, "New session label"); err != nil || readyKey != key {
		t.Fatal("session enrollment changed paired Device identity", readyKey, err)
	}
	ready, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ready.Discard()
	device, err := world.LookupObjectBody[*s4wave_device.Device](ctx, ready, key, s4wave_device.NewDeviceBlock)
	if err != nil {
		t.Fatal(err)
	}
	if !device.IsSelectable() || device.Label != paired.Label || !device.CreatedAt.EqualVT(paired.CreatedAt) {
		t.Fatal("session enrollment lost pairing metadata or readiness", device)
	}
}
