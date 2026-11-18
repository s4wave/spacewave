package main

import (
	"context"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/packsweep"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/s4db"
	"github.com/sirupsen/logrus"
)

// checkFlags binds the flags of the pack check: the verify's, with the cycle
// and the sample required, and the candidates the cycle's trash set unless a
// list names others.
func (a *Args) checkFlags() []cli.Flag {
	flags := slices.DeleteFunc(a.dialFlags(), func(f cli.Flag) bool {
		return slices.Contains(f.Names(), "store-id")
	})
	return append(append(flags,
		&cli.StringFlag{Name: "scratch", Usage: "the judged cycle's scratch file: its catalog gives each block's children, and its roots are walked with the fresh ones", Destination: &a.Scratch, Required: true},
		&cli.StringFlag{Name: "candidates", Usage: "candidate list, one JSON judgment a line, to check in place of the cycle's trash set, the packs that passed their recheck; a pass over another set writes no receipt", Destination: &a.Candidates},
		&cli.StringSliceFlag{Name: "root", Usage: "root block, as a b58 ref; repeat for each", Destination: &a.Roots},
		&cli.StringFlag{Name: "roots-file", Usage: "file listing more roots, one b58 ref a line", Destination: &a.RootsFile},
		&cli.StringFlag{Name: "admin-url", Usage: "read every named root and pin from the backend's pack sweep admin service, a ws or wss URL; the Authorization header comes from " + adminAuthEnv, Destination: &a.AdminURL},
		&cli.BoolFlag{Name: "cycle-roots-only", Usage: "walk the cycle's roots with no fresh root from --root, --roots-file or --admin-url; the receipt records it, and a hide refuses such a receipt", Destination: &a.CycleRootsOnly},
		a.framesFlag(),
		&cli.StringFlag{Name: "accept-missing", Usage: "file listing missing blocks accepted as lost, one b58 ref a line", Destination: &a.AcceptMissing},
		&cli.StringFlag{Name: "mutant", Usage: "catalogued pack id, or " + mutantAuto + " for the judged pack with the most live blocks, to add to the candidates; the check then passes only when it finds that pack live and nothing else bad", Destination: &a.Mutant},
		&cli.Float64Flag{Name: "sample", Usage: "fraction of the other reached blocks read by value and checked against the catalog's refs, 0 to 1; the receipt records it, and a hide refuses one under " + strconv.FormatFloat(packsweep.DefaultReceiptMinSample, 'g', -1, 64), Destination: &a.Sample, Required: true},
		&cli.Uint64Flag{Name: "sample-seed", Usage: "seed choosing the blocks the sample takes", Destination: &a.SampleSeed},
		&cli.IntFlag{Name: "batch", Usage: "frontier keys per transaction", Value: packsweep.DefaultVerifyBatch, Destination: &a.Batch},
		&cli.IntFlag{Name: "reads", Usage: "reads at once", Value: packsweep.DefaultVerifyReads, Destination: &a.Reads},
		&cli.IntFlag{Name: "max-bad", Usage: "bad blocks listed; every one is counted", Value: packsweep.DefaultVerifyMaxBad, Destination: &a.MaxBad},
	), a.pacingFlags()...)
}

// runCheck proves that removing the candidates loses no block the roots
// reach, reading their indexes and few values: see packsweep.Check. It prints
// the counts and the first bad, and fails on any bad block or pack; a pass
// over the cycle's trash set writes the receipt a hide needs. With --mutant
// it tests itself: it passes only when it finds the mutant live, by a block
// only the mutant holds, and nothing bad once the mutant's findings are set
// aside.
func (a *Args) runCheck(*cli.Context) error {
	// Run until a signal stops the check.
	ctx, cancel := signalContext()
	defer cancel()
	le := logrus.NewEntry(logrus.StandardLogger())

	// Read the fresh roots, which a check needs unless told to walk the
	// cycle's alone, and the accepted holes.
	roots, err := a.verifyRoots(ctx)
	if err != nil {
		return err
	}
	if (len(roots) == 0) != a.CycleRootsOnly {
		return errors.New("name the fresh roots with --root, --roots-file or --admin-url, or none with --cycle-roots-only")
	}
	accepted := make(map[string]bool)
	if a.AcceptMissing != "" {
		lines, err := readRefLines(a.AcceptMissing)
		if err != nil {
			return errors.Wrap(err, "--accept-missing")
		}
		for _, line := range lines {
			accepted[line] = true
		}
	}

	// Open the cycle for the whole check, which reads its catalog, take its
	// trash set or the list named, and walk its roots with the fresh ones.
	cycleDB, cycle, err := a.openScratch()
	if err != nil {
		return err
	}
	defer cycleDB.Close()
	ids, err := a.checkCandidates(ctx, cycle)
	if err != nil {
		return err
	}
	cmpRoots, mutant, err := compareCycle(ctx, cycle, a.Mutant, roots, ids)
	if err != nil {
		return err
	}
	roots = append(roots, cmpRoots.extra...)

	// Add the mutant to the candidates.
	left := maps.Clone(ids)
	if mutant != "" {
		left[mutant] = true
	}

	// Reach the bucket, read-only, and open the packs with the candidates
	// left out.
	objects, bucket, closeStore, err := a.dialPacedStore(ctx)
	if err != nil {
		return err
	}
	defer closeStore()
	ro := readOnly{objects}
	store := block_store_s3.NewPackStore(le, newPackView(ro, a.Prefix, left, false), bucket, a.Prefix, &block_store_s3.PackStoreOptions{KeepPacks: true})
	defer store.Close()

	// Open the frames that serve the refs of blocks waiting to upload.
	refs, closeFrames, err := a.openFrames()
	if err != nil {
		return err
	}
	defer closeFrames()

	// Keep the check's rows in a scratch file of its own, removed after.
	dir, err := os.MkdirTemp("", "pack-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	db, err := s4db.Open(filepath.Join(dir, "check"), s4db.Options{})
	if err != nil {
		return err
	}
	defer db.Close()

	// Check, logging the totals at most every few seconds.
	start, logged := time.Now(), time.Now()
	res, err := packsweep.Check(ctx, &packsweep.CheckOptions{
		Cycle:      cycle,
		Store:      db,
		Candidates: slices.Sorted(maps.Keys(left)),
		Mutant:     mutant,
		PackKeys: func(ctx context.Context, id string, fn func(key []byte) error) error {
			return readPackKeys(ctx, ro, bucket, a.Prefix, id, fn)
		},
		Source:         store,
		Refs:           refs,
		Roots:          roots,
		CycleRootsOnly: a.CycleRootsOnly,
		Accepted:       accepted,
		Sample:         a.Sample,
		SampleSeed:     a.SampleSeed,
		Batch:          a.Batch,
		Reads:          a.Reads,
		MaxBad:         a.MaxBad,
		Progress: func(r *packsweep.CheckResult) {
			if time.Since(logged) < 5*time.Second {
				return
			}
			logged = time.Now()
			le.WithFields(logrus.Fields{"reached": r.Reached, "in_candidates": r.InCandidates, "bad": r.Bad}).Info("check batch")
		},
	})
	if err != nil {
		return err
	}
	le.WithField("took", time.Since(start).String()).Info("check complete")

	// Print the result with the roots compared and the mutant.
	caught := mutant != "" && res.LivePacks[mutant] != 0
	rest := res.Bad - res.BadPacks[mutant]
	if err := a.print(func(arena *fastjson.Arena) *fastjson.Value {
		// Add the comparison and the mutant to the check's result.
		o := res.JSON(arena)
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

	// Fail a mutant run that missed its mutant, and any run with a bad block
	// beside the mutant's.
	if mutant != "" && !caught {
		return errors.Errorf("mutant %s added to the candidates and not found live: the check proves nothing", mutant)
	}
	if rest != 0 {
		return errors.Errorf("%d blocks or candidate packs bad: removing the candidates is not proven safe", rest)
	}
	return nil
}

// checkCandidates returns the packs the check takes: the list --candidates
// names, or else the cycle's trash set. Neither may be empty.
func (a *Args) checkCandidates(ctx context.Context, cycle kvtx.Store) (map[string]bool, error) {
	if a.Candidates == "" {
		return trashSetIDs(ctx, cycle)
	}
	ids, err := readCandidateIDs(a.Candidates)
	return ids, errors.Wrap(err, "--candidates")
}

// readPackKeys calls fn with each key of pack id's own index, read from the
// bucket: its entry for the size and count, then the index at its end.
func readPackKeys(ctx context.Context, objects block_store_s3.ObjectStore, bucket, prefix, id string, fn func(key []byte) error) error {
	// Read the pack's entry.
	body, err := objects.GetObject(ctx, bucket, block_store_s3.EntryObjectKey(prefix, id))
	if err != nil {
		return errors.Wrap(err, "read entry of "+id)
	}
	data, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		return errors.Wrap(err, "read entry of "+id)
	}
	entry := &packfile.PackfileEntry{}
	if err := entry.UnmarshalVT(data); err != nil {
		return errors.Wrap(err, "decode entry of "+id)
	}

	// Read the index from the pack's end, and scan its keys.
	key := block_store_s3.PackObjectKey(prefix, id)
	size := entry.GetSizeBytes()
	tail, err := packfile.ReadIndexTail(ctx, size, packfile.IndexTailReadSize(entry.GetBlockCount()), func(ctx context.Context, off int64, length int) ([]byte, error) {
		return objects.GetObjectRange(ctx, bucket, key, off, length)
	})
	if err != nil {
		return errors.Wrap(err, "read index of "+id)
	}
	return packfile.ScanIndexTail(tail, size, func(ie *kvfile.IndexEntry) error {
		return fn(ie.GetKey())
	})
}
