package kvtx

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/db/volume"
)

// TestObsoleteKindsSpareLiveKinds fails when a kind a format upgrade deletes is
// one format 10 writes, since the upgrade would delete its live records.
func TestObsoleteKindsSpareLiveKinds(t *testing.T) {
	for _, c := range []struct {
		name  string
		kinds []byte
	}{
		{"legacyGraphKinds", legacyGraphKinds},
		{"Format9Kinds", block_gc.Format9Kinds},
	} {
		for _, kind := range c.kinds {
			if slices.Contains(block_gc.LiveKinds, kind) {
				t.Errorf("%s deletes live kind %q", c.name, kind)
			}
		}
	}
}

// TestFormatUpgradeCarriesLiveness opens a store of format 6 and checks that
// format 7 keeps its child lists in place, carries its named roots into the
// root table, leaves it unadopted, sweeps the rest of the old graph, and that
// an adoption then counts its holders. A second store whose root references a
// block stored without a child list refuses adoption.
func TestFormatUpgradeCarriesLiveness(t *testing.T) {
	// Write a format 6 store: R1 -> C, its child lists, a root owner edge
	// holding R1, and old graph records.
	ctx := t.Context()
	keys := store_kvkey.NewDefaultKVKey()
	store := newGuardedStore(t)
	tr := testTree{}
	tr.add(t, "C")
	r1 := tr.add(t, "R1", "C")
	writeLegacyStore(t, store, keys, tr, "R1", true, "C", "R1")

	// An open that refuses the upgrade fails and leaves the store at format
	// 6, while an empty store opens under the same refusal.
	refuse := &store_kvtx.Config{RefuseFormatUpgrade: true}
	_, err := NewVolume(ctx, "test/liveness", keys, store, refuse, false, false, nil, nil)
	if !errors.Is(err, volume.ErrFormatUpgradeRefused) || !volume.IsPermanent(err) {
		t.Fatalf("refused upgrade = %v, want a permanent ErrFormatUpgradeRefused", err)
	}
	if version, err := readFormatVersion(ctx, store, keys); err != nil || version != 6 {
		t.Fatalf("version after a refused upgrade = %d err %v, want 6", version, err)
	}
	openConfVolume(t, newGuardedStore(t), refuse)

	// Open it: the lists and the root carry over, and it is unadopted.
	v, _ := openUnadoptedVolume(t, store)
	if from, upgraded := v.FormatUpgrade(); !upgraded || from != 6 {
		t.Fatalf("format upgrade = %d %v, want from 6", from, upgraded)
	}
	if v.obsoleteDone != nil {
		<-v.obsoleteDone
	}
	if got, err := v.GetBucketRoot(ctx, "space", "world"); err != nil || !got.EqualsRef(r1) {
		t.Fatalf("carried root = %v err %v, want R1", got, err)
	}
	if children, listed, err := v.GetChildren(ctx, r1); err != nil || !listed || len(children) != 1 || !children[0].EqualsRef(tr["C"].Ref) {
		t.Fatalf("carried child list = %v listed %v err %v", children, listed, err)
	}
	err = v.readRecords(ctx, func(rec *block_gc.Records) error {
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		if state.GetAdoption() != block_gc.Adoption_ADOPTION_UNADOPTED || len(state.GetObsoleteKinds()) != 0 || state.GetGeneration() != block_gc.Generation_GENERATION_OPEN {
			t.Fatalf("state %v, want unadopted, unverified, with the old graph swept", state)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range legacyGraphKinds {
		if n := countPrefix(t, store, append([]byte{livenessPrefix[0]}, kind)); n != 0 {
			t.Fatalf("%d old graph records of kind %q remain", n, kind)
		}
	}

	// Adoption counts the unverified lists as they are: nothing is garbage.
	res, err := v.Adopt(ctx)
	if err != nil || res.Blocks != 2 || res.Roots != 1 || res.Queued != 0 {
		t.Fatalf("adopt = %+v err %v, want 2 blocks, 1 root, none queued", res, err)
	}
	requireAudit(t, v, 0)

	// A store whose root references a block stored without a child list
	// refuses adoption and stays unadopted.
	store2 := newGuardedStore(t)
	writeLegacyStore(t, store2, keys, tr, "R1", false)
	v2, _ := openUnadoptedVolume(t, store2)
	setGeneration(t, v2, block_gc.Generation_GENERATION_SETTLED)
	if _, err := v2.Adopt(ctx); !errors.Is(err, block_gc.ErrAdoptUnknown) {
		t.Fatalf("adopt of a listless root = %v, want ErrAdoptUnknown", err)
	}
	if res, err := v2.Collect(ctx); err != nil || res.Netted != 0 {
		t.Fatalf("collect after a refused adoption netted %d err %v", res.Netted, err)
	}

	// A format 6 store opened with frees on adopts in the background with
	// its lists unverified, which the generation rule makes safe.
	store4 := newGuardedStore(t)
	writeLegacyStore(t, store4, keys, tr, "R1", true, "C", "R1")
	v4, _ := openLivenessVolume(t, store4, true)
	waitAdopted(t, v4)

	// An adopted format 7 store where main holds B, which a fill stored
	// referencing C with a fill mark and no holder for C, opens unadopted,
	// its adoption counts C under B, and the obsolete sweep deletes the
	// mark.
	store3 := newGuardedStore(t)
	tr3 := testTree{}
	tr3.add(t, "C")
	b := tr3.add(t, "B", "C")
	writeFormat7Fill(t, store3, keys, tr3, b)
	v3, clk3 := openUnadoptedVolume(t, store3)
	err = v3.readRecords(ctx, func(rec *block_gc.Records) error {
		state, err := rec.GetState(ctx)
		if err == nil && state.GetAdoption() != block_gc.Adoption_ADOPTION_UNADOPTED {
			t.Fatalf("format 7 state %v, want unadopted", state)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if res, err := v3.Adopt(ctx); err != nil || res.Blocks != 2 || res.Queued != 0 {
		t.Fatalf("adopt of format 7 = %+v err %v, want 2 blocks, none queued", res, err)
	}
	if v3.obsoleteDone != nil {
		<-v3.obsoleteDone
	}
	if n := countPrefix(t, store3, []byte{livenessPrefix[0], 'F'}); n != 0 {
		t.Fatalf("%d fill marks remain after the obsolete sweep", n)
	}
	clk3.Add(block_gc.DefaultGrace + time.Second)
	collectAll(t, v3)
	requireStored(t, v3, tr3, true, "B", "C")
	requireAudit(t, v3, 0)

	// A format 11 store opens stamped at format 12 with every other record
	// as it was, since its records are format 12's: the version is what
	// keeps a build whose sweep gives a released park's downs again off it.
	t.Run("format 11 stamped", func(t *testing.T) {
		store := newGuardedStore(t)
		writeLegacyStore(t, store, keys, tr, "R1", true, "C", "R1")
		v, err := NewVolume(ctx, "test/liveness", keys, store, &store_kvtx.Config{}, false, false, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if v.obsoleteDone != nil {
			<-v.obsoleteDone
		}
		if err := v.Close(); err != nil {
			t.Fatal(err)
		}
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(11)); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		before := storeRecords(t, store, keys)

		// A refusing open leaves it at 11; an open that may upgrade stamps it.
		if _, err := NewVolume(ctx, "test/liveness", keys, store, refuse, false, false, nil, nil); !errors.Is(err, volume.ErrFormatUpgradeRefused) {
			t.Fatalf("refused stamp = %v, want ErrFormatUpgradeRefused", err)
		}
		v, _ = openUnadoptedVolume(t, store)
		if from, upgraded := v.FormatUpgrade(); !upgraded || from != 11 {
			t.Fatalf("format upgrade = %d %v, want from 11", from, upgraded)
		}
		if version, err := readFormatVersion(ctx, store, keys); err != nil || version != store_kvkey.FormatVersion {
			t.Fatalf("version after the stamp = %d err %v, want %d", version, err, store_kvkey.FormatVersion)
		}
		if after := storeRecords(t, store, keys); !maps.EqualFunc(before, after, bytes.Equal) {
			t.Fatalf("the stamp changed records: %d before, %d after", len(before), len(after))
		}
	})

	// An adopted format 9 store that owes releases, P's through a cascade
	// record and R2's through a paged release, opens unadopted with its
	// release dropped, its old kinds obsolete and its pop mark moved. The
	// adoption recounts from the lists, so neither owed release moves C
	// again, and the collection frees the dead P, G and H2 and keeps every
	// block a root reaches, whatever the stale queue entries reference.
	t.Run("format 9 with owed releases", func(t *testing.T) {
		store := newGuardedStore(t)
		tr := format9Owed(t)
		writeFormat9Owed(t, store, keys, tr)
		v, clk := openLivenessVolume(t, store, true)
		state := waitAdopted(t, v)
		if state.GetRelease() != nil {
			t.Fatalf("the adopted state keeps the release %v", state.GetRelease())
		}
		if v.obsoleteDone != nil {
			<-v.obsoleteDone
		}
		for _, kind := range slices.Concat(block_gc.Format9Kinds, []byte{'p', 'D'}) {
			if n := countPrefix(t, store, []byte{livenessPrefix[0], kind}); n != 0 {
				t.Fatalf("%d format 9 records of kind %q remain", n, kind)
			}
		}
		if holders, _ := blockHolders(t, v, tr, "C"); holders != 2 {
			t.Fatalf("C has %d holders after the recount, want B's and P's", holders)
		}
		collectSettled(t, v, clk)
		collectSettled(t, v, clk)
		requireStored(t, v, tr, true, "A", "B", "C", "S")
		requireStored(t, v, tr, false, "P", "G", "H2")
		if holders, _ := blockHolders(t, v, tr, "C"); holders != 1 {
			t.Fatalf("C has %d holders after P's free, want B's", holders)
		}
		requireAudit(t, v, 0)
	})
}

// format9Owed builds the blocks of a format 9 store that owes releases: main
// holds A -> B -> C and s holds S. P -> {C, G} reached zero and owes its
// children their release through a cascade record. R2 -> {C, H2} was freed and
// its paged release moved C but owes H2. format9Owed returns the tree. R2's
// bytes and list are gone.
func format9Owed(t *testing.T) testTree {
	t.Helper()
	tr := testTree{}
	tr.add(t, "C")
	tr.add(t, "B", "C")
	tr.add(t, "A", "B")
	tr.add(t, "S")
	tr.add(t, "G")
	tr.add(t, "P", "C", "G")
	tr.add(t, "H2")
	tr.add(t, "R2", "C", "H2")
	return tr
}

// writeFormat9Owed writes the adopted format 9 store of format9Owed's tr: the
// blocks but R2, their lists, the roots main on A and s on S, format 9's counts
// with P's zero record in its uvarint encoding, P's cascade record owing its
// release, the paged release of R2 at its second child, stale queue entries
// referencing the live S and C, a journal record and the pop mark.
func writeFormat9Owed(t *testing.T, store kvtx.Store, keys *store_kvkey.KVKey, tr testTree) {
	t.Helper()
	ctx := t.Context()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	set := func(key, value []byte) {
		if err := tx.Set(ctx, key, value); err != nil {
			t.Fatal(err)
		}
	}
	raw := func(kind byte, parts ...[]byte) []byte {
		return slices.Concat(append([][]byte{{livenessPrefix[0], kind}}, parts...)...)
	}
	be := func(n uint64) []byte { return binary.BigEndian.AppendUint64(nil, n) }

	// Write the blocks but the freed R2, their lists and the roots.
	rec := block_gc.NewRecords(tx, livenessPrefix)
	for name, entry := range tr {
		if name == "R2" {
			continue
		}
		key, err := entry.Ref.MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		set(keys.GetBlockKey(key), entry.Data)
		if err := rec.PutChildren(ctx, entry.Ref, entry.Refs); err != nil {
			t.Fatal(err)
		}
	}
	for name, root := range map[string]string{"main": "A", "s": "S"} {
		if err := rec.SetRoot(ctx, "b", name, &block_gc.RootEntry{Ref: block_gc.BlockKey(tr[root].Ref)}); err != nil {
			t.Fatal(err)
		}
	}

	// Write format 9's counts: C counts B and the Dying P, R2's release
	// having moved it, P is at zero at ripe 7 in format 9's encoding, and
	// every other block has its one implied holder.
	if err := rec.SetHolders(ctx, tr["C"].Ref, 2, 0); err != nil {
		t.Fatal(err)
	}
	set(raw('H', block_gc.BlockKey(tr["P"].Ref)), binary.AppendUvarint([]byte{0}, 7))
	set(raw('C', block_gc.BlockKey(tr["P"].Ref)), []byte{'d'})

	// Write the release of R2 owing H2, the stale queue entries, a journal
	// record and the pop mark.
	set(raw('D', be(0)), block_gc.EncodeChildren([]*block.BlockRef{tr["C"].Ref, tr["H2"].Ref}))
	set(raw('Q', be(3), block_gc.BlockKey(tr["S"].Ref)), nil)
	set(raw('Q', be(2), block_gc.BlockKey(tr["C"].Ref)), nil)
	set(raw('J', be(1)), []byte{0x08, 0x01})
	set(raw('p'), be(9))
	state := &block_gc.LivenessState{
		Adoption:   block_gc.Adoption_ADOPTION_ADOPTED,
		Generation: block_gc.Generation_GENERATION_OPEN,
		Frees:      true,
		Release:    &block_gc.Release{Block: block_gc.BlockKey(tr["R2"].Ref), Children: 2, Next: 1},
	}
	if err := rec.SetState(ctx, state); err != nil {
		t.Fatal(err)
	}
	set(keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(9))
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// collectSettled lets the grace period pass and collects until the
// generation is Settled, failing after 16 passes.
func collectSettled(t *testing.T, v *Volume, clk *testClock) {
	t.Helper()
	for range 16 {
		clk.Add(block_gc.DefaultGrace + time.Second)
		collectAll(t, v)
		if livenessGeneration(t, v) == block_gc.Generation_GENERATION_SETTLED {
			return
		}
	}
	t.Fatalf("collection never settled; the generation is %v", livenessGeneration(t, v))
}

// writeFormat7Fill writes an adopted format 7 store holding the blocks of tr
// with their child lists, a named root main of bucket b on root, and a fill
// mark on root, as a format 7 fill left a block whose children it gave no
// holder.
func writeFormat7Fill(t *testing.T, store kvtx.Store, keys *store_kvkey.KVKey, tr testTree, root *block.BlockRef) {
	t.Helper()
	ctx := t.Context()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Write the blocks, their lists, the root, the mark and the state.
	rec := block_gc.NewRecords(tx, livenessPrefix)
	for _, entry := range tr {
		key, err := entry.Ref.MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, keys.GetBlockKey(key), entry.Data); err != nil {
			t.Fatal(err)
		}
		if err := rec.PutChildren(ctx, entry.Ref, entry.Refs); err != nil {
			t.Fatal(err)
		}
	}
	if err := rec.SetRoot(ctx, "b", "main", &block_gc.RootEntry{Ref: block_gc.BlockKey(root)}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, append([]byte{livenessPrefix[0], 'F'}, block_gc.BlockKey(root)...), nil); err != nil {
		t.Fatal(err)
	}
	if err := rec.SetState(ctx, &block_gc.LivenessState{Adoption: block_gc.Adoption_ADOPTION_ADOPTED}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(7)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// writeLegacyStore writes a format 6 store holding the blocks of tr, a named
// root world of bucket space on root, the child lists of listed when lists is
// set, and a few old graph records of other kinds.
func writeLegacyStore(t *testing.T, store kvtx.Store, keys *store_kvkey.KVKey, tr testTree, root string, lists bool, listed ...string) {
	t.Helper()
	ctx := t.Context()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	set := func(key, value []byte) {
		if err := tx.Set(ctx, key, value); err != nil {
			t.Fatal(err)
		}
	}

	// Write the version, the blocks and their child lists.
	set(keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(6))
	for _, entry := range tr {
		key, err := entry.Ref.MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		set(keys.GetBlockKey(key), entry.Data)
	}
	if lists {
		for _, name := range listed {
			key := append([]byte{livenessPrefix[0], 'c'}, block_gc.BlockKey(tr[name].Ref)...)
			set(key, block_gc.EncodeChildren(tr[name].Refs))
		}
	}

	// Write the root owner edge and old records of other kinds.
	owner := legacyRootOwnerPrefix + base64.RawURLEncoding.EncodeToString([]byte("space")) + "/" + base64.RawURLEncoding.EncodeToString([]byte("world"))
	edge := append([]byte{livenessPrefix[0], legacyForwardKind, legacyTextNode}, binary.AppendUvarint(nil, uint64(len(owner)))...)
	edge = append(append(edge, owner...), block_gc.BlockKey(tr[root].Ref)...)
	set(edge, nil)
	set(append([]byte{livenessPrefix[0], 'h'}, block_gc.BlockKey(tr[root].Ref)...), []byte{1})
	set([]byte{livenessPrefix[0], 'w'}, []byte("sharing"))
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
