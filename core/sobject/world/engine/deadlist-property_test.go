//go:build !js

package sobject_world_engine_test

import (
	"errors"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
)

// TestDeadlistsRandomWrites drives one World through random writes, renames,
// payload roots, bursts of writes with the change log on, abandoned writes,
// failed publications with a fork behind them, and engine restarts, and
// checks after each step that no deadlist record lists a page the World it
// runs to reaches.
// DEADLIST_SEED replays a seed the log printed.
func TestDeadlistsRandomWrites(t *testing.T) {
	// Pick and log the seed.
	seed := uint64(time.Now().UnixNano())
	if s := os.Getenv("DEADLIST_SEED"); s != "" {
		var err error
		if seed, err = strconv.ParseUint(s, 10, 64); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("DEADLIST_SEED=%d", seed)

	// Start a World that also resolves the Space settings operation, which
	// switches the change log.
	w, so := startGatedWorld(t, t.TempDir(), nil)
	so.ctrl.SetOpTypes(sobject_world_engine.OpTypes{
		Lookup: world.LookupOpSlice{world_mock.LookupMockOp, world.NewSchema(space_world_ops.Types).LookupOp}.LookupOp,
	})
	flows := scrapeWriteFlows(t)
	r := &deadlistRun{t: t, w: w, so: so, rng: rand.New(rand.NewPCG(seed, 0)), checked: make(map[uint64]bool)}

	// Run the random steps, checking each record once it is named.
	for range 240 {
		r.step()
		r.check()
	}

	// End with writes enough for a checkpoint whose whole interval follows
	// the last.
	for range 2 * sobject.MinCheckpointOperations {
		r.create(false)
	}
	waitSeqIdle(t, w)
	r.check()

	// The run named records with deaths in them. Restarts and change log
	// switches forfeit intervals, so how many records a seed names varies.
	after := scrapeWriteFlows(t)
	deaths := after["spacewave_world_deaths_total"] - flows["spacewave_world_deaths_total"]
	forfeits := after["spacewave_world_death_forfeits_total"] - flows["spacewave_world_death_forfeits_total"]
	t.Logf("records %d, entries %d, deaths %v, forfeits %v, steps %v", len(r.checked), r.entries, deaths, forfeits, r.ran)
	if r.entries == 0 {
		t.Fatalf("checked %d records with no entries", len(r.checked))
	}
}

// deadlistRun is the state of one random run of TestDeadlistsRandomWrites.
type deadlistRun struct {
	t   *testing.T
	w   *localWorld
	so  *gatedSharedObject
	rng *rand.Rand

	// live holds the keys of the objects the World holds.
	live []string
	// next numbers the next new key, so no two objects share content.
	next int
	// checked holds the sequence of each record checked.
	checked map[uint64]bool
	// entries counts the entries of the checked records.
	entries int
	// ran counts the steps run by kind.
	ran map[string]int
}

// step runs one random step.
func (r *deadlistRun) step() {
	// Pick the step, falling back to a create when it has no object.
	steps := []struct {
		name   string
		weight int
		run    func()
	}{
		{"create", 24, func() { r.create(false) }},
		{"payload", 10, func() { r.create(true) }},
		{"delete", 14, r.delete},
		{"rename", 10, r.rename},
		{"batch", 10, r.batch},
		{"rev", 10, r.rev},
		{"abandon", 5, r.abandon},
		{"fork", 5, r.fork},
		{"restart", 3, r.restart},
		{"changelog", 2, r.changelogBurst},
	}
	total := 0
	for _, s := range steps {
		total += s.weight
	}
	n := r.rng.IntN(total)
	for _, s := range steps {
		if n -= s.weight; n < 0 {
			if len(r.live) == 0 && s.name != "restart" {
				s.name, s.run = "create", func() { r.create(false) }
			}
			if r.ran == nil {
				r.ran = make(map[string]int)
			}
			r.ran[s.name]++
			s.run()
			return
		}
	}
}

// newKey returns a key no object has had.
func (r *deadlistRun) newKey() string {
	r.next++
	return "deadlist/" + strconv.Itoa(r.next)
}

// pick removes and returns a random live key.
func (r *deadlistRun) pick() string {
	i := r.rng.IntN(len(r.live))
	key := r.live[i]
	r.live = slices.Delete(r.live, i, i+1)
	return key
}

// create commits a write that creates one object, with a root block it puts
// when payload is set.
func (r *deadlistRun) create(payload bool) {
	key := r.newKey()
	if payload {
		r.w.createWithPayload(r.t, key)
	} else {
		r.w.create(r.t, key)
	}
	r.live = append(r.live, key)
}

// delete commits a write that deletes a random object.
func (r *deadlistRun) delete() {
	key := r.pick()
	r.write(func(tx world.Tx) error {
		_, err := tx.DeleteObject(r.t.Context(), key)
		return err
	})
}

// rename commits a write that renames a random object to a new key.
func (r *deadlistRun) rename() {
	from, to := r.pick(), r.newKey()
	r.write(func(tx world.Tx) error {
		obj, err := tx.RenameObject(r.t.Context(), from, to, false)
		world.ReleaseObjectState(obj)
		return err
	})
	r.live = append(r.live, to)
}

// batch commits one write that creates two objects and deletes one.
func (r *deadlistRun) batch() {
	gone, a, b := r.pick(), r.newKey(), r.newKey()
	r.write(func(tx world.Tx) error {
		for _, key := range []string{a, b} {
			obj, err := tx.CreateObject(r.t.Context(), key, nil)
			world.ReleaseObjectState(obj)
			if err != nil {
				return err
			}
		}
		_, err := tx.DeleteObject(r.t.Context(), gone)
		return err
	})
	r.live = append(r.live, a, b)
}

// rev commits a write that bumps a random object's revision.
func (r *deadlistRun) rev() {
	key := r.live[r.rng.IntN(len(r.live))]
	r.write(func(tx world.Tx) error {
		// Bump the object's revision.
		obj, err := world.MustGetObject(r.t.Context(), tx, key)
		if err != nil {
			return err
		}
		defer world.ReleaseObjectState(obj)
		_, err = obj.IncrementRev(r.t.Context())
		return err
	})
}

// abandon discards a write that created an object.
func (r *deadlistRun) abandon() {
	r.w.newWrite(r.t, r.newKey()).Discard()
}

// fork admits two writes behind a held publication, the second forking from
// the first's World, then fails the publication, so neither is accepted.
func (r *deadlistRun) fork() {
	// Admit both behind the held publication, once a checkpoint in flight
	// has trimmed the operations it covers.
	waitSeqIdle(r.t, r.w)
	before := admittedOps(r.t, r.so)
	injected := errors.New("injected publication failure")
	applying, release := r.so.hold(injected)
	first := commitAsync(r.t, r.w.newWrite(r.t, r.newKey()))
	waitReached(r.t, applying)
	second := commitAsync(r.t, r.w.newWrite(r.t, r.newKey()))
	waitAdmittedOps(r.t, r.so, before+2)

	// Fail it: both Commits fail.
	release()
	for _, done := range []<-chan error{first, second} {
		if err := <-done; err == nil {
			r.t.Fatal("Commit behind the failed publication succeeded")
		}
	}
}

// restart stops the engine and serves the World again, which drops the
// engine's memory of the interval's transitions.
func (r *deadlistRun) restart() {
	// Replace the engine and write through the new one.
	r.so.stop()
	r.so.serve(r.t)
	eng, err := r.so.ctrl.GetWorldEngine(r.t.Context())
	if err != nil {
		r.t.Fatal(err)
	}
	r.w.eng = eng
}

// changelogBurst turns the change log on, bumps a random object's revision
// with it on, and turns it off, so the interval holding the burst forfeits.
func (r *deadlistRun) changelogBurst() {
	r.setChangelog(true)
	r.rev()
	r.setChangelog(false)
}

// setChangelog commits a write that turns the change log on or off.
func (r *deadlistRun) setChangelog(on bool) {
	settings := &space_world.SpaceSettings{ChangelogEnabled: on}
	op := space_world_ops.NewSetSpaceSettingsOp("", settings, true, time.Now())
	r.write(func(tx world.Tx) error {
		_, _, err := tx.ApplyWorldOp(r.t.Context(), op, "")
		return err
	})
}

// write commits a write that fn builds.
func (r *deadlistRun) write(fn func(tx world.Tx) error) {
	// Build the write and commit it.
	r.t.Helper()
	tx, err := r.w.eng.NewTransaction(r.t.Context(), true)
	if err != nil {
		r.t.Fatal(err)
	}
	defer tx.Discard()
	if err := fn(tx); err != nil {
		r.t.Fatal(err)
	}
	if err := tx.Commit(r.t.Context()); err != nil {
		r.t.Fatal(err)
	}
}

// check reads every named record not checked yet and fails when one lists a
// page the World it runs to reaches.
func (r *deadlistRun) check() {
	// Read the records not checked yet.
	r.t.Helper()
	ctx := r.t.Context()
	store := r.so.GetBlockStore()
	for _, rec := range r.w.deadlists(r.t, store) {
		seq := rec.head.GetSeq()
		if r.checked[seq] {
			continue
		}
		r.checked[seq] = true

		// Gather the pages its World reaches.
		to := rec.head.GetTo()
		if to.GetEmpty() {
			r.t.Fatalf("%s names no World", rec.name)
		}
		live := make(map[string]struct{})
		err := block.WalkGraph(ctx, store, []*block.BlockRef{to}, 0, func(ref *block.BlockRef, _ *block.StoredBlock) error {
			live[ref.MarshalString()] = struct{}{}
			return nil
		})
		if err != nil {
			r.t.Fatalf("%s: walk its World: %v", rec.name, err)
		}

		// Check each entry against them.
		entries := slices.Concat(rec.dead.Replaced, rec.dead.Removed)
		r.entries += len(entries)
		for _, ref := range entries {
			if _, ok := live[ref.MarshalString()]; ok {
				r.t.Fatalf("%s lists %s, which its World reaches; steps so far %v", rec.name, ref.MarshalString(), r.ran)
			}
		}
	}
}
