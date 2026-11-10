package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/bloom"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/s4wave/spacewave/db/packfile/writer"
)

// prove reads the output back from disk and checks it against the input:
// the directory holds only pack and entry pairs; each entry names its pack's
// id, size, block count and bloom filter, and the id is the pack's v1 id of
// its bytes; each pack is within the size limit; every block verifies
// against its key; and the set of (ref, block sums) out equals the set in,
// with no ref twice. It fills the output totals of the report.
func (r *repacker) prove() error {
	// List the output: only complete pairs, nothing else.
	names, err := os.ReadDir(r.o.Out)
	if err != nil {
		return err
	}
	have := make(map[string]bool, len(names))
	for _, n := range names {
		have[n.Name()] = true
	}
	var ids []string
	for _, n := range names {
		name := n.Name()
		if !n.Type().IsRegular() {
			return errors.Errorf("output %s is not a regular file", name)
		}
		if id, ok := strings.CutSuffix(name, entrySuffix); ok {
			if !have[id] {
				return errors.Errorf("output entry %s has no pack", name)
			}
			continue
		}
		if !validID(name) || !have[name+entrySuffix] {
			return errors.Errorf("output %s is not a pack with its entry", name)
		}
		ids = append(ids, name)
	}
	slices.Sort(ids)

	// Check each pack and its blocks.
	var problems []string
	for _, id := range ids {
		probs, err := r.provePack(id)
		if err != nil {
			return errors.Wrapf(err, "output pack %s", id)
		}
		problems = append(problems, probs...)
	}

	// Every input block is in the output.
	var missing int
	for _, s := range r.sums {
		if !s.seen {
			missing++
		}
	}
	if missing != 0 {
		problems = append(problems, fmt.Sprintf("%d input blocks are missing from the output", missing))
	}

	// The totals agree.
	written := uint64(0)
	for _, p := range r.rep.Packs {
		written += p.blocks
	}
	if r.rep.PacksOut != len(r.rep.Packs) {
		problems = append(problems, fmt.Sprintf("output holds %d packs, %d were written", r.rep.PacksOut, len(r.rep.Packs)))
	}
	if r.rep.BlocksOut != r.rep.BlocksIn-r.rep.Duplicates || r.rep.BlocksOut != written {
		problems = append(problems, fmt.Sprintf("output holds %d blocks, input %d unique, %d written", r.rep.BlocksOut, r.rep.BlocksIn-r.rep.Duplicates, written))
	}
	if r.rep.DataOut != r.rep.DataUnique {
		problems = append(problems, fmt.Sprintf("output holds %d block bytes, input %d unique", r.rep.DataOut, r.rep.DataUnique))
	}
	if len(problems) != 0 {
		for _, p := range problems {
			_, _ = fmt.Fprintln(r.o.Log, "DIFF "+p)
		}
		return errors.Errorf("%d differences", len(problems))
	}
	_, _ = fmt.Fprintln(r.o.Log, "proof ok: output blocks equal input blocks, each once; every entry matches its pack")
	return nil
}

// provePack checks one output pack and its entry. A broken pack or entry is
// an error; a block that differs from the input is a problem returned.
func (r *repacker) provePack(id string) ([]string, error) {
	// Read the pack and its entry.
	data, err := os.ReadFile(filepath.Join(r.o.Out, id))
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(r.o.Out, id+entrySuffix))
	if err != nil {
		return nil, err
	}
	entry := &packfile.PackfileEntry{}
	if err := entry.UnmarshalVT(raw); err != nil {
		return nil, errors.Wrap(err, "decode entry")
	}
	r.rep.PacksOut++
	r.rep.FilesOut += uint64(len(data) + len(raw))

	// The entry names the pack at its size, within the limit.
	if err := identity.ValidatePackID(id); err != nil {
		return nil, err
	}
	if entry.GetId() != id || entry.GetSizeBytes() != uint64(len(data)) {
		return nil, errors.Errorf("entry names %s of %d bytes, the file is %d", entry.GetId(), entry.GetSizeBytes(), len(data))
	}
	if int64(len(data)) > r.o.PackBytes {
		return nil, errors.Errorf("pack of %d bytes exceeds the %d byte limit", len(data), r.o.PackBytes)
	}
	if entry.GetBloomFormatVersion() != packfile.BloomFormatVersionV1 || entry.GetCreatedAt() == nil {
		return nil, errors.New("entry lacks its bloom format version or created-at")
	}

	// Open the pack's index.
	rdr, err := kvfile.BuildReader(bytes.NewReader(data), uint64(len(data)))
	if err != nil {
		return nil, errors.Wrap(err, "read index")
	}
	if rdr.Size() != entry.GetBlockCount() || rdr.Size() == 0 {
		return nil, errors.Errorf("index holds %d blocks, the entry %d", rdr.Size(), entry.GetBlockCount())
	}

	// Read every block in key order, verifying it against its key.
	var problems []string
	var keys [][]byte
	err = rdr.ScanPrefix(nil, func(key, value []byte) error {
		// Verify and count it.
		_, stored, err := packfile.DecodeBlockValue(key, value)
		if err != nil {
			return errors.Wrapf(err, "block %x", key)
		}
		keys = append(keys, bytes.Clone(key))
		r.rep.BlocksOut++
		r.rep.DataOut += uint64(len(stored.Data))

		// Match it to the input's block, once.
		sum, _, err := sumBlock(stored)
		if err != nil {
			return err
		}
		mk, err := toBlockKey(key)
		if err != nil {
			return err
		}
		in, ok := r.sums[mk]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("block %x is not in the input", key))
		case in.seen:
			problems = append(problems, fmt.Sprintf("block %x is in the output twice", key))
		case in.data != sum.data || in.obj != sum.obj:
			problems = append(problems, fmt.Sprintf("block %x differs from the input", key))
		}
		if ok {
			in.seen = true
			r.sums[mk] = in
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if uint64(len(keys)) != entry.GetBlockCount() {
		return nil, errors.Errorf("scanned %d blocks, the entry %d", len(keys), entry.GetBlockCount())
	}

	// The bloom filter is the one the pack writer builds of the keys.
	bf := writer.DefaultPolicy().NewBloomFilter(uint64(len(keys)))
	for _, k := range keys {
		bf.Add(k)
	}
	bloomBytes, err := bloom.NewBloom(bf).MarshalVT()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(bloomBytes, entry.GetBloomFilter()) {
		return nil, errors.New("entry bloom filter is not the pack's")
	}

	// The id is the pack's own v1 id.
	digest := sha256.Sum256(data)
	want, err := identity.BuildPackID(r.o.ResourceID, &writer.PackResult{
		SortedKeyDigest:  writer.DigestSortedKeys(keys),
		PackBytesDigest:  digest[:],
		PolicyTag:        writer.PolicyTag(writer.DefaultPolicy()),
		ValueOrderPolicy: writer.ValueOrderIterator,
	})
	if err != nil {
		return nil, err
	}
	if want != id {
		return nil, errors.Errorf("pack bytes give id %s", want)
	}
	return problems, nil
}
