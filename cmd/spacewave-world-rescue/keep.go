//go:build !js && !wasip1 && !windows

package main

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
)

// Pack orders --order takes.
const (
	// orderWanted orders by wanted refs, most first (round 1).
	orderWanted = "wanted"
	// orderTime orders by wanted refs per second of fetch: per byte of the
	// pack plus a per-pack overhead in bytes, most first.
	orderTime = "keys-per-time"
)

// keyTail is the last 16 bytes of a block key, the h row's key.
type keyTail [16]byte

// tailOf returns the tail of a block key.
func tailOf(key []byte) (t keyTail) {
	copy(t[:], key[max(len(key)-16, 0):])
	return t
}

// keepSet is a sorted set of key tails: the blocks of a fetched pack, other
// than the wanted, to keep.
type keepSet []keyTail

// has reports whether the set holds key's tail.
func (s keepSet) has(key []byte) bool {
	_, ok := slices.BinarySearchFunc(s, tailOf(key), func(e, t keyTail) int { return bytes.Compare(e[:], t[:]) })
	return ok
}

// readKeepSet reads a list of hex block keys or 16-byte hex tails, one a
// line, the first tab-separated field taken. Blank lines are skipped.
func readKeepSet(path string) (keepSet, error) {
	// Open the list.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Take the tail of every key or tail line, refusing any other line.
	var s keepSet
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for n := 1; sc.Scan(); n++ {
		field, _, _ := strings.Cut(strings.TrimSpace(sc.Text()), "\t")
		if field == "" {
			continue
		}
		raw, err := hex.DecodeString(field)
		if err != nil || len(raw) < 16 {
			return nil, errors.Errorf("%s:%d: not a hex key or tail", path, n)
		}
		s = append(s, tailOf(raw))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	// Sort the tails for has and drop repeats.
	slices.SortFunc(s, func(a, b keyTail) int { return bytes.Compare(a[:], b[:]) })
	return slices.Compact(s), nil
}

// readWholePackList reads a list of pack ids, one a line, the first
// tab-separated field taken, in file order without repeats. Blank lines and
// lines starting with # are skipped; any other line must be a v1 pack id.
func readWholePackList(path string) ([]string, error) {
	// Open the list.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Take each id once, refusing a line that is not one.
	var ids []string
	seen := make(map[string]bool)
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		field, _, _ := strings.Cut(strings.TrimSpace(sc.Text()), "\t")
		if field == "" || strings.HasPrefix(field, "#") {
			continue
		}
		if err := identity.ValidatePackID(field); err != nil {
			return nil, errors.Wrapf(err, "%s:%d", path, n)
		}
		if !seen[field] {
			seen[field] = true
			ids = append(ids, field)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, errors.Errorf("%s: no pack ids", path)
	}
	return ids, nil
}

// keepOptions choose the blocks of a whole pack kept beside the wanted.
type keepOptions struct {
	// Set is the blocks to keep; nil keeps the wanted alone.
	Set keepSet
	// MaxSize drops a kept block larger than this, listing it instead; 0 is
	// no cut.
	MaxSize uint64
	// Kept gets one "<pack id>\t<hex key>\t<size>" line per kept block
	// written; Dropped one "<pack id>\t<hex key>\t<offset>\t<size>" line per
	// block past MaxSize. Either may be nil.
	Kept, Dropped io.Writer
}

// extra is a block of the keep set found in a fetched pack.
type extra struct {
	key  []byte
	off  uint64
	size uint64
	// blk is the verified block, nil when dropped by size.
	blk *block.PutBatchEntry
	// dup is set by the flush when the run already stored the key, wanted
	// or kept from another pack, so it is not stored again.
	dup bool
}

// keeper takes the keep set's blocks out of whole packs, each block once
// across packs. It is safe for concurrent use.
type keeper struct {
	o    keepOptions
	mtx  sync.Mutex
	seen map[keyTail]struct{}
	// Kept, Dropped and Failed count blocks; Dup those the run had already
	// stored; KeptBytes the kept data.
	Kept, Dropped, Failed, Dup int
	KeptBytes                  uint64
}

// newKeeper returns a keeper, nil when the options keep nothing.
func newKeeper(o keepOptions) *keeper {
	if o.Set == nil {
		return nil
	}
	return &keeper{o: o, seen: make(map[keyTail]struct{})}
}

// take returns the keep set's blocks of a pack, verified against their keys,
// skipping the wanted and those another pack gave. index is the pack's own
// index; data the whole pack.
func (k *keeper) take(index []*kvfile.IndexEntry, data []byte, wanted map[string]bool) []extra {
	var out []extra
	for _, ie := range index {
		key := ie.GetKey()
		if wanted[string(key)] || !k.o.Set.has(key) {
			continue
		}
		off, size := ie.GetOffset(), ie.GetSize()
		if off+size > uint64(len(data)) {
			continue
		}
		if !k.claim(key) {
			continue
		}
		if k.o.MaxSize != 0 && size > k.o.MaxSize {
			out = append(out, extra{key: key, off: off, size: size})
			continue
		}
		blk, ok := verifyKeyed(key, data[off:off+size])
		if !ok {
			k.mtx.Lock()
			k.Failed++
			delete(k.seen, tailOf(key))
			k.mtx.Unlock()
			continue
		}
		out = append(out, extra{key: key, off: off, size: size, blk: blk})
	}
	return out
}

// claim marks key taken, reporting false when a pack already gave it.
func (k *keeper) claim(key []byte) bool {
	// Mark the tail seen unless a pack already gave it.
	k.mtx.Lock()
	defer k.mtx.Unlock()
	t := tailOf(key)
	if _, ok := k.seen[t]; ok {
		return false
	}
	k.seen[t] = struct{}{}
	return true
}

// record writes the lines of a pack's extras after they are stored.
func (k *keeper) record(id string, xs []extra) error {
	// Count each extra and log it as dropped by size or kept.
	k.mtx.Lock()
	defer k.mtx.Unlock()
	for _, x := range xs {
		if x.dup {
			k.Dup++
			continue
		}
		if x.blk == nil {
			k.Dropped++
			if k.o.Dropped != nil {
				if _, err := fmt.Fprintf(k.o.Dropped, "%s\t%x\t%d\t%d\n", id, x.key, x.off, x.size); err != nil {
					return err
				}
			}
			continue
		}
		k.Kept++
		k.KeptBytes += uint64(len(x.blk.Data))
		if k.o.Kept != nil {
			if _, err := fmt.Fprintf(k.o.Kept, "%s\t%x\t%d\n", id, x.key, len(x.blk.Data)); err != nil {
				return err
			}
		}
	}
	return nil
}

// verifyKeyed decodes a pack value and checks its bytes hash to the block
// key, the only ref a block the list does not name has.
func verifyKeyed(key, value []byte) (*block.PutBatchEntry, bool) {
	// Build the ref the key names.
	h, err := packfile.ParseBlockKey(key)
	if err != nil {
		return nil, false
	}
	ref := block.NewBlockRef(h)

	// Decode the stored object and check its data hashes to that ref.
	stored, err := block.DecodeBlockObject(value)
	if err != nil {
		return nil, false
	}
	if err := ref.VerifyData(stored.Data, false); err != nil {
		return nil, false
	}
	return &block.PutBatchEntry{Ref: ref, Data: stored.Data, Refs: stored.Refs, RefsKnown: stored.RefsKnown}, true
}

// packCost is the bytes a pack's whole read is ordered by: its size plus the
// per-pack overhead, which carries the per-request time in bytes.
func packCost(p *packWork, overhead uint64) uint64 {
	return p.entry.GetSizeBytes() + overhead
}

// orderPacksBy orders packs by mode: orderWanted as orderPacks, orderTime by
// wanted refs per (size + overhead), most first, then by id. A pack of
// unknown size goes last under orderTime.
func orderPacksBy(packs []*packWork, mode string, overhead uint64) {
	// Leave every mode but keys-per-time to orderPacks.
	if mode != orderTime {
		orderPacks(packs)
		return
	}

	// Sort by wanted refs per cost byte, unknown sizes last, ties by id.
	slices.SortFunc(packs, func(a, b *packWork) int {
		// Put a pack of unknown size after every sized one.
		if (a.entry.GetSizeBytes() == 0) != (b.entry.GetSizeBytes() == 0) {
			if a.entry.GetSizeBytes() == 0 {
				return 1
			}
			return -1
		}

		// Compare wanted refs per byte of size plus overhead, then ids.
		ca, cb := packCost(a, overhead), packCost(b, overhead)
		ra := float64(len(a.wants)) / float64(max(ca, 1))
		rb := float64(len(b.wants)) / float64(max(cb, 1))
		if c := cmp.Compare(rb, ra); c != 0 {
			return c
		}
		return cmp.Compare(a.id, b.id)
	})
}

// wholeInPacks fails when packs bounds the plan and a listed whole pack is
// outside it, as the plan would skip it in silence. The error names and
// counts none.
func wholeInPacks(whole []string, packs map[string]bool) error {
	// Pass when no pack list bounds the plan.
	if packs == nil {
		return nil
	}

	// Refuse any listed whole pack outside the pack list, naming none and
	// counting none, so the message holds no digit to read as a reply code.
	for _, id := range whole {
		if !packs[id] {
			return errors.New("whole pack not in --packs: some --whole-packs ids are not in the --packs list; add them or drop them")
		}
	}
	return nil
}
