package main

import (
	"bufio"
	"context"
	"io"
	"os"
	"strings"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/packsweep"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/kvtx"
)

// mutantAuto picks the mutant pack from the cycle's judgments.
const mutantAuto = "auto"

// packView shows a store's packs with a set of them left out, or only that
// set. Its keys are the packs' objects and entries under one prefix; every
// other key passes through. A pack store opened over it with no index of its
// own sees the bucket as if the left-out packs were gone, so a check through
// it proves what their removal would lose before anything is removed.
//
// Invariant: a left-out pack is unreachable through the view by any read. The
// view embeds ObjectStore alone, so a pack store cannot find an interface on
// it, such as ObjectHider's version reads, that would reach the pack's bytes
// some other way.
type packView struct {
	block_store_s3.ObjectStore
	// packDir and entryDir are the prefixes of the packs and their entries.
	packDir, entryDir string
	// ids are the packs in the set.
	ids map[string]bool
	// only shows the set alone, instead of leaving it out.
	only bool
}

// newPackView returns the view of store's packs under prefix that leaves out
// ids, or, with only, shows ids alone.
func newPackView(store block_store_s3.ObjectStore, prefix string, ids map[string]bool, only bool) packView {
	return packView{
		ObjectStore: store,
		packDir:     block_store_s3.PackObjectKey(prefix, ""),
		entryDir:    block_store_s3.EntryObjectKey(prefix, ""),
		ids:         ids,
		only:        only,
	}
}

// hidden reports whether the view leaves the key out: the object or entry of
// a pack on the wrong side of the set.
func (v packView) hidden(key string) bool {
	id, ok := strings.CutPrefix(key, v.packDir)
	if !ok {
		id, ok = strings.CutPrefix(key, v.entryDir)
	}
	return ok && v.ids[id] != v.only
}

// GetObject opens the object, or returns ErrNotFound for a left-out key.
func (v packView) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	if v.hidden(key) {
		return nil, block_store_s3.ErrNotFound
	}
	return v.ObjectStore.GetObject(ctx, bucket, key)
}

// GetObjectRange reads the range, or returns ErrNotFound for a left-out key.
func (v packView) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	if v.hidden(key) {
		return nil, block_store_s3.ErrNotFound
	}
	return v.ObjectStore.GetObjectRange(ctx, bucket, key, off, length)
}

// ListObjects lists the keys under prefix the view shows.
func (v packView) ListObjects(ctx context.Context, bucket, prefix string, fn func(key string, size int64) error) error {
	return v.ObjectStore.ListObjects(ctx, bucket, prefix, func(key string, size int64) error {
		if v.hidden(key) {
			return nil
		}
		return fn(key, size)
	})
}

// readCandidateIDs reads the pack ids of a candidate list, one JSON judgment
// a line as the candidates step writes it. A line without an id is refused,
// since skipping it would leave a candidate in the store the check reads.
func readCandidateIDs(path string) (map[string]bool, error) {
	// Open the list.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Take each line's id.
	ids := make(map[string]bool)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	var p fastjson.Parser
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		v, err := p.Parse(line)
		if err != nil {
			return nil, errors.Wrapf(err, "line %d", n)
		}
		id := string(v.GetStringBytes("id"))
		if id == "" {
			return nil, errors.Errorf("line %d has no pack id", n)
		}
		ids[id] = true
	}
	return ids, sc.Err()
}

// pickMutant returns the pack a mutant run adds to the candidates: the one
// named, or for mutantAuto the judged pack outside the candidates with the
// most live blocks, which a correct check must then find live.
func pickMutant(ctx context.Context, store kvtx.Store, name string, candidates map[string]bool) (string, error) {
	// Take a named pack as it is.
	if name != mutantAuto {
		if candidates[name] {
			return "", errors.Errorf("mutant %s is already a candidate", name)
		}
		return name, nil
	}

	// Pick the judged pack with the most live blocks.
	if store == nil {
		return "", errors.New("--mutant auto needs --scratch to read the judgments")
	}
	var best string
	var most uint64
	err := packsweep.Judgments(ctx, store, func(j *packsweep.Judgment) error {
		if !candidates[j.ID] && j.LiveBlocks > most {
			best, most = j.ID, j.LiveBlocks
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if best == "" {
		return "", errors.New("no judged pack outside the candidates holds a live block")
	}
	return best, nil
}
