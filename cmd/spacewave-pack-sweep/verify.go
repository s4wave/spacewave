package main

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
	"github.com/s4wave/spacewave/db/block/packsweep"
	block_packsweep_admin "github.com/s4wave/spacewave/db/block/packsweep/admin"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/s4db"
	"github.com/sirupsen/logrus"
)

// verifyFlags binds the flags of the verify: the store's, without the id of a
// pack index it does not read, the roots, named, from the backend or from a
// cycle, and the candidate packs left out with an optional mutant.
func (a *Args) verifyFlags() []cli.Flag {
	flags := slices.DeleteFunc(a.dialFlags(), func(f cli.Flag) bool {
		return slices.Contains(f.Names(), "store-id")
	})
	return append(append(flags,
		&cli.StringSliceFlag{Name: "root", Usage: "root block, as a b58 ref; repeat for each", Destination: &a.Roots},
		&cli.StringFlag{Name: "roots-file", Usage: "file listing more roots, one b58 ref a line", Destination: &a.RootsFile},
		&cli.StringFlag{Name: "admin-url", Usage: "read every named root and pin from the backend's pack sweep admin service, a ws or wss URL; the Authorization header comes from " + adminAuthEnv, Destination: &a.AdminURL},
		&cli.StringFlag{Name: "scratch", Usage: "a cycle's scratch file: walk its roots too, report the roots it lacks or the others lack, and pick --mutant auto from its judgments", Destination: &a.Scratch},
		&cli.StringFlag{Name: "candidates", Usage: "candidate list, one JSON judgment a line: leave its packs out of the store read, naming a block only they hold " + packsweep.ReasonCandidate, Destination: &a.Candidates},
		&cli.StringFlag{Name: "mutant", Usage: "pack id, or " + mutantAuto + " for the judged pack with the most live blocks, to leave out as well; the verify then passes only when it names a block only that pack holds", Destination: &a.Mutant},
		&cli.IntFlag{Name: "batch", Usage: "frontier keys per transaction", Value: packsweep.DefaultVerifyBatch, Destination: &a.Batch},
		&cli.IntFlag{Name: "reads", Usage: "blocks read at once", Value: packsweep.DefaultVerifyReads, Destination: &a.Reads},
		&cli.IntFlag{Name: "max-bad", Usage: "bad blocks listed; every one is counted", Value: packsweep.DefaultVerifyMaxBad, Destination: &a.MaxBad},
	), a.pacingFlags()...)
}

// runVerify reads every block the roots reach from the bucket alone, through
// a fresh pack store with no local index, and checks each against its hash.
// With --candidates the store leaves those packs out, so a pass proves that
// removing them loses no block the roots reach. It prints the counts per root
// and the first bad blocks, and fails when any block is missing, unreadable,
// corrupt or without its refs. With --mutant it inverts: it fails unless it
// names a block only the mutant holds, since a check that passes with a live
// pack left out proves nothing.
func (a *Args) runVerify(*cli.Context) error {
	// Run until a signal stops the verify.
	ctx, cancel := signalContext()
	defer cancel()
	le := logrus.NewEntry(logrus.StandardLogger())

	// Read the roots, the cycle's, the candidates and the mutant.
	roots, err := a.verifyRoots(ctx)
	if err != nil {
		return err
	}
	var ids map[string]bool
	if a.Candidates != "" {
		if ids, err = readCandidateIDs(a.Candidates); err != nil {
			return errors.Wrap(err, "--candidates")
		}
	}
	cmpRoots, mutant, err := a.verifyCycle(ctx, roots, ids)
	if err != nil {
		return err
	}
	roots = append(roots, cmpRoots.extra...)
	if len(roots) == 0 {
		return errors.New("name the roots with --root, --roots-file, --admin-url or --scratch")
	}

	// Reach the bucket.
	objects, bucket, closeStore, err := a.dialPacedStore(ctx)
	if err != nil {
		return err
	}
	defer closeStore()

	// Leave out the candidates and the mutant, naming only the mutant's
	// blocks candidate in its run, so its pass cannot rest on a real
	// candidate's block.
	left := maps.Clone(ids)
	named := ids
	if mutant != "" {
		if left == nil {
			left = make(map[string]bool, 1)
		}
		left[mutant] = true
		named = map[string]bool{mutant: true}
	}

	// Open the live packs with those left out, the trashed ones, and the
	// left-out ones to name a block only they hold, all read-only.
	ro := readOnly{objects}
	store := block_store_s3.NewPackStore(le, newPackView(ro, a.Prefix, left, false), bucket, a.Prefix, &block_store_s3.PackStoreOptions{KeepPacks: true})
	defer store.Close()
	trash := block_store_s3.NewPackStore(le.WithField("packs", "trash"), ro, bucket, block_store_s3.TrashPrefix(a.Prefix), &block_store_s3.PackStoreOptions{KeepPacks: true})
	defer trash.Close()
	var candidates packcopy.Source
	if len(named) != 0 {
		cs := block_store_s3.NewPackStore(le.WithField("packs", "candidates"), newPackView(ro, a.Prefix, named, true), bucket, a.Prefix, &block_store_s3.PackStoreOptions{KeepPacks: true})
		defer cs.Close()
		candidates = cs
	}

	// Keep the blocks read and the frontier in a scratch file of the
	// verify's own, removed after.
	dir, err := os.MkdirTemp("", "pack-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	db, err := s4db.Open(filepath.Join(dir, "verify"), s4db.Options{})
	if err != nil {
		return err
	}
	defer db.Close()

	// Verify, logging the totals at most every few seconds.
	start, logged := time.Now(), time.Now()
	res, err := packsweep.Verify(ctx, &packsweep.VerifyOptions{
		Store:      db,
		Source:     store,
		Trash:      trash,
		Candidates: candidates,
		Roots:      roots,
		Batch:      a.Batch,
		Reads:      a.Reads,
		MaxBad:     a.MaxBad,
		Progress: func(r *packsweep.VerifyResult) {
			if time.Since(logged) < 5*time.Second {
				return
			}
			logged = time.Now()
			le.WithFields(logrus.Fields{"blocks": r.Blocks, "bytes": r.Bytes, "bad": r.Bad, "roots": len(r.Roots)}).Info("verify batch")
		},
	})
	if err != nil {
		return err
	}
	le.WithField("took", time.Since(start).String()).Info("verify complete")

	// Print the result with what was left out and how the roots compared.
	caught := res.BadReasons[packsweep.ReasonCandidate] != 0
	if err := a.print(func(arena *fastjson.Arena) *fastjson.Value {
		// Add the counts to the verify's result.
		o := res.JSON(arena)
		o.Set("candidates", arena.NewNumberInt(len(ids)))
		cmpRoots.set(arena, o)
		if mutant != "" {
			o.Set("mutant", arena.NewString(mutant))
			o.Set("mutant_caught", arena.NewFalse())
			if caught {
				o.Set("mutant_caught", arena.NewTrue())
			}
		}
		return o
	}); err != nil {
		return err
	}

	// Fail a mutant run that missed its mutant, and a run with a bad block.
	if mutant != "" {
		if !caught {
			return errors.Errorf("mutant %s left out and no block only it holds was found: the check proves nothing", mutant)
		}
		return nil
	}
	if res.Bad != 0 {
		return errors.Errorf("%d blocks missing, trashed, only in a candidate, unreadable, corrupt or without their refs", res.Bad)
	}
	return nil
}

// rootsComparison compares the roots a cycle walked with the ones the verify
// read afresh.
type rootsComparison struct {
	// cycle is set when a cycle's roots were read.
	cycle bool
	// marked is the cycle's root count.
	marked int
	// added are the fresh roots the cycle lacks, as b58 refs.
	added []string
	// dropped are the cycle's roots the fresh ones lack, as b58 refs.
	dropped []string
	// extra are the dropped roots, named for the cycle, which the verify
	// walks too.
	extra []block.NamedRoot
}

// set adds the comparison to the printed object.
func (c *rootsComparison) set(arena *fastjson.Arena, o *fastjson.Value) {
	// Add nothing without a cycle.
	if !c.cycle {
		return
	}

	// Add the count and both lists.
	refs := func(values []string) *fastjson.Value {
		arr := arena.NewArray()
		for i, v := range values {
			arr.SetArrayItem(i, arena.NewString(v))
		}
		return arr
	}
	o.Set("cycle_roots", arena.NewNumberInt(c.marked))
	o.Set("roots_added", refs(c.added))
	o.Set("roots_dropped", refs(c.dropped))
}

// verifyCycle reads the cycle's roots from --scratch, when named, and compares
// them with the fresh roots, and picks the mutant. The verify walks the union,
// so a block the cycle's own roots reach and a candidate alone holds fails it
// as much as one a fresh root reaches. A fresh root the cycle lacks is
// reported, not refused: the head moves with every write, and the walk from
// it proves its blocks directly.
func (a *Args) verifyCycle(ctx context.Context, fresh []block.NamedRoot, ids map[string]bool) (*rootsComparison, string, error) {
	// Without a cycle, only a named mutant can be taken.
	c := &rootsComparison{}
	if a.Scratch == "" {
		if a.Mutant == "" {
			return c, "", nil
		}
		mutant, err := pickMutant(ctx, nil, a.Mutant, ids)
		return c, mutant, err
	}

	// Read the cycle's roots, then release its scratch file.
	db, store, err := a.openScratch()
	if err != nil {
		return nil, "", err
	}
	defer db.Close()
	return compareCycle(ctx, store, a.Mutant, fresh, ids)
}

// compareCycle compares the cycle's roots in store with the fresh roots, and
// picks the mutant: none for an empty name.
func compareCycle(ctx context.Context, store kvtx.Store, mutantName string, fresh []block.NamedRoot, ids map[string]bool) (*rootsComparison, string, error) {
	// Read the cycle's roots.
	c := &rootsComparison{}
	head, err := packsweep.ReadHead(ctx, store)
	if err != nil {
		return nil, "", err
	}
	if !head.Seeded {
		return nil, "", errors.New("--scratch: the cycle has no roots yet; run walk first")
	}

	// Compare the two sets.
	freshRefs := make(map[string]bool, len(fresh))
	for _, root := range fresh {
		if !root.Ref.GetEmpty() {
			freshRefs[root.Ref.MarshalString()] = true
		}
	}
	marked := make(map[string]bool, len(head.Roots))
	c.cycle, c.marked = true, len(head.Roots)
	for _, value := range head.Roots {
		marked[value] = true
		if freshRefs[value] {
			continue
		}
		ref, err := block.UnmarshalBlockRefB58(value)
		if err != nil {
			return nil, "", errors.Wrap(err, "--scratch: cycle root "+value)
		}
		c.dropped = append(c.dropped, value)
		c.extra = append(c.extra, block.NamedRoot{Name: "cycle", Ref: ref})
	}
	for value := range freshRefs {
		if !marked[value] {
			c.added = append(c.added, value)
		}
	}
	slices.Sort(c.added)

	// Pick the mutant.
	if mutantName == "" {
		return c, "", nil
	}
	mutant, err := pickMutant(ctx, store, mutantName, ids)
	return c, mutant, err
}

// verifyRoots returns the roots named on the command line, each named by its
// ref, then every named root and pin the backend lists; none when none are
// named.
func (a *Args) verifyRoots(ctx context.Context) ([]block.NamedRoot, error) {
	// Read the named roots.
	values := a.Roots.Value()
	if a.RootsFile != "" {
		lines, err := readRefLines(a.RootsFile)
		if err != nil {
			return nil, errors.Wrap(err, "--roots-file")
		}
		values = append(values, lines...)
	}
	var roots []block.NamedRoot
	for _, value := range values {
		ref, err := block.UnmarshalBlockRefB58(value)
		if err != nil {
			return nil, errors.Wrap(err, "parse root "+value)
		}
		roots = append(roots, block.NamedRoot{Name: value, Ref: ref})
	}

	// Add the backend's roots and pins.
	if a.AdminURL != "" || a.dialAdmin != nil {
		client, closeClient, err := a.dialAdminService(ctx)
		if err != nil {
			return nil, err
		}
		defer closeClient()
		resp, err := client.Roots(ctx, &block_packsweep_admin.RootsRequest{})
		if err != nil {
			return nil, errors.Wrap(err, "read the backend's roots")
		}
		for _, root := range resp.GetRoots() {
			roots = append(roots, block.NamedRoot{Name: root.GetName(), Ref: root.GetRef()})
		}
		for _, pin := range resp.GetPins() {
			roots = append(roots, block.NamedRoot{Name: "pin", Ref: pin})
		}
	}
	return roots, nil
}

// readOnly is a store that refuses every write and delete, so a verify only
// ever reads the bucket.
type readOnly struct {
	block_store_s3.ObjectStore
}

// errReadOnly refuses a write.
var errReadOnly = errors.New("the verify only reads the store")

// PutObject refuses the write.
func (readOnly) PutObject(context.Context, string, string, []byte, string) error {
	return errReadOnly
}

// DeleteObject refuses the delete.
func (readOnly) DeleteObject(context.Context, string, string) error {
	return errReadOnly
}
