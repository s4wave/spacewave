//go:build !js

package spacewave_cli

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// dropCountBlocks bounds the blocks one drop check counts, as in the copy.
const dropCountBlocks = 1000

// dropCountDepth is the most key segments a family name keeps.
const dropCountDepth = 8

// dropCountClasses bounds the key families printed, largest first.
const dropCountClasses = 60

// The block families of a counted dropping copy.
const (
	dropCountDropped = "blocks dropped: SQL holds"
	dropCountMarked  = "blocks kept: upload marker"
	dropCountLacks   = "blocks kept: SQL lacks"
)

// countVolumeDrop walks the stopped volume at src as a dropping copy would,
// without writing anything, and prints to out the keys and bytes it would
// keep and drop by key family. Progress goes to w.
func countVolumeDrop(ctx context.Context, src string, drop *sqlDrop, out, w io.Writer) error {
	// Open the source, refusing a missing file, which the open would create,
	// and a file a daemon has open.
	if _, err := os.Stat(src); err != nil {
		return err
	}
	sdb, err := s4db.Open(src, s4db.Options{})
	if err != nil {
		return err
	}
	defer sdb.Close()
	shared, err := sdb.Shared()
	if err != nil {
		return err
	}
	if shared {
		return fmt.Errorf("%s is open in another process; stop the daemon first", src)
	}
	keys := store_kvkey.NewDefaultKVKey()
	if err := drop.open(ctx, sdb, keys, w); err != nil {
		return err
	}
	defer drop.close()

	// Read every key and value length from one snapshot.
	tx, err := sdb.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	classes := make(map[string]*volumeKeyClass)
	add := func(class string, keyLen, n int) {
		kc := classes[class]
		if kc == nil {
			kc = &volumeKeyClass{class: class}
			classes[class] = kc
		}
		kc.keys++
		kc.keyBytes += uint64(keyLen) //nolint:gosec // lengths are never negative.
		kc.valueBytes += uint64(n)    //nolint:gosec // lengths are never negative.
	}

	// Check the held blocks as the copy would.
	type heldBlock struct {
		ref       *block.BlockRef
		keyLen, n int
	}
	var held []heldBlock
	checkHeld := func() error {
		if len(held) == 0 {
			return nil
		}
		refs := make([]*block.BlockRef, len(held))
		for i, hb := range held {
			refs[i] = hb.ref
		}
		dropped, err := drop.drop(ctx, refs)
		if err != nil {
			return err
		}
		for i, hb := range held {
			class := dropCountLacks
			if dropped[i] {
				class = dropCountDropped
			} else if drop.isMarked(hb.ref) {
				class = dropCountMarked
			}
			add(class, hb.keyLen, hb.n)
		}
		held = held[:0]
		return nil
	}

	// Walk the keys, reporting progress at most once per interval.
	blockPrefix := keys.GetBlockFullPrefix()
	start := time.Now()
	last := start
	var walked uint64
	err = tx.(*s4db.Tx).ScanPrefixLens(ctx, nil, func(key []byte, n int) error {
		walked++
		if time.Since(last) >= copyReportInterval {
			last = time.Now()
			fmt.Fprintf(w, "%s: %d keys walked\n", time.Since(start).Round(time.Second), walked)
		}
		data, ok := bytes.CutPrefix(key, blockPrefix)
		if !ok {
			add(dropCountClassOf(key), len(key), n)
			return nil
		}
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(bytes.Clone(data)); err != nil {
			return fmt.Errorf("decode block key %q: %w", key, err)
		}
		held = append(held, heldBlock{ref: ref, keyLen: len(key), n: n})
		if len(held) == dropCountBlocks {
			return checkHeld()
		}
		return nil
	})
	if err == nil {
		err = checkHeld()
	}
	if err != nil {
		return err
	}

	// Sum what the copy keeps and drops, then print the families largest
	// first, folding the smallest into one line.
	var kept, dropped volumeKeyClass
	ranked := make([]*volumeKeyClass, 0, len(classes))
	for _, kc := range classes {
		sum := &kept
		if kc.class == dropCountDropped {
			sum = &dropped
		}
		sum.keys += kc.keys
		sum.keyBytes += kc.keyBytes
		sum.valueBytes += kc.valueBytes
		ranked = append(ranked, kc)
	}
	slices.SortFunc(ranked, func(a, b *volumeKeyClass) int {
		return cmp.Or(cmp.Compare(b.keyBytes+b.valueBytes, a.keyBytes+a.valueBytes), cmp.Compare(a.class, b.class))
	})
	fmt.Fprintf(out, "walked %d keys of %s in %s\n", walked, src, time.Since(start).Round(time.Second))
	fmt.Fprintf(out, "keep: %d keys, %s\n", kept.keys, humanize.IBytes(kept.keyBytes+kept.valueBytes))
	fmt.Fprintf(out, "drop: %d keys, %s\n", dropped.keys, humanize.IBytes(dropped.keyBytes+dropped.valueBytes))
	var rest volumeKeyClass
	for i, kc := range ranked {
		if i >= dropCountClasses {
			rest.keys += kc.keys
			rest.keyBytes += kc.keyBytes
			rest.valueBytes += kc.valueBytes
			continue
		}
		fmt.Fprintf(out, "%12d keys %10s  %s\n", kc.keys, humanize.IBytes(kc.keyBytes+kc.valueBytes), kc.class)
	}
	if rest.keys != 0 {
		fmt.Fprintf(out, "%12d keys %10s  (%d smaller families)\n", rest.keys, humanize.IBytes(rest.keyBytes+rest.valueBytes), len(ranked)-dropCountClasses)
	}
	return nil
}

// dropCountClassOf names the family of a key outside the blocks: its first
// byte when that is binary, as in the graph records, or else its segments
// up to the last separator within dropCountDepth, so a key's own name never
// splits a family.
func dropCountClassOf(key []byte) string {
	if len(key) != 0 && key[0] < 0x20 {
		return volumeKeyClassOf(key[:1], []byte("/"), 0, 1)
	}
	end := 0
	for range dropCountDepth {
		i := bytes.IndexByte(key[end:], '/')
		if i < 0 {
			break
		}
		end += i + 1
	}
	if end == 0 {
		end = len(key)
	}
	return volumeKeyClassOf(key[:end], []byte("/"), 0, 96)
}
