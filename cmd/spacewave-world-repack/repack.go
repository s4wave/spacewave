package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
)

// entrySuffix ends the name of a pack's encoded PackfileEntry.
const entrySuffix = ".entry"

// tempPrefix starts the name of an output file being written.
const tempPrefix = ".tmp-"

// options configure a re-pack.
type options struct {
	// In are the input directories, only read.
	In []string
	// Out is the output directory: missing or empty.
	Out string
	// PackBytes bounds an output pack file.
	PackBytes int64
	// MaxBlocks bounds the blocks of an output pack; zero is no bound, so
	// packs fill by bytes alone.
	MaxBlocks uint64
	// SliceBytes bounds the files of a slice of the plan; zero prints none.
	SliceBytes uint64
	// ResourceID scopes the pack ids.
	ResourceID string
	// CreatedAt is every output entry's created-at; nil takes the latest
	// of the input entries.
	CreatedAt *time.Time
	// Log receives the report.
	Log io.Writer
}

// inPack is one input pack.
type inPack struct {
	id, dir string
	entry   *packfile.PackfileEntry
	// size is the pack file's size; entryBytes the entry file's.
	size, entryBytes int64
}

// blockSum identifies a block's contents: the sha256 of its bytes, and a
// short sum of its encoded block object, which adds its refs.
type blockSum struct {
	data [32]byte
	obj  uint64
	// seen is set when the output's read back finds the block.
	seen bool
}

// outPack is one written output pack, in write order.
type outPack struct {
	id                string
	size, entryBytes  int64
	blocks, dataBytes uint64
}

// report is the outcome of a re-pack.
type report struct {
	PacksIn, PacksOut int
	// SameIDs counts input packs whose id another input directory also
	// holds.
	SameIDs int
	// BlocksIn counts every input block, duplicates included; Duplicates
	// the ones dropped as already written.
	BlocksIn, Duplicates, BlocksOut uint64
	// DataIn is the input block bytes, duplicates included; DataUnique
	// without them; DataOut the output's.
	DataIn, DataUnique, DataOut uint64
	// FilesIn and FilesOut are the bytes of the packs and entries.
	FilesIn, FilesOut uint64
	// Packs are the output packs in write order.
	Packs []outPack
}

// pending is a block waiting for its output pack.
type pending struct {
	h   *hash.Hash
	blk *block.StoredBlock
}

// repacker carries a re-pack's state.
type repacker struct {
	o         *options
	createdAt *timestamppb.Timestamp
	maxBlocks uint64
	sums      map[string]*blockSum
	rep       report

	// The open output pack: its blocks, and the kvfile value and index
	// bytes they take.
	cur              []pending
	curPos, curIndex uint64
	curData          uint64
}

// run re-packs o.In into o.Out, proves the output equal and prints the
// report and slice plan.
func run(o *options) (*report, error) {
	// Check the output is fresh and not an input.
	if o.PackBytes <= 0 || o.PackBytes > maxPackMiB<<20 {
		return nil, errors.Errorf("pack size must be 1 to %d MiB", maxPackMiB)
	}
	if len(o.In) == 0 {
		return nil, errors.New("no input directory")
	}
	if err := freshOut(o.Out, o.In); err != nil {
		return nil, err
	}

	// List and check the inputs' entries before reading a block.
	packs, sameIDs, err := listInputs(o.In)
	if err != nil {
		return nil, err
	}
	created := o.CreatedAt
	if created == nil {
		for _, p := range packs {
			if t := p.entry.GetCreatedAt(); t != nil && (created == nil || t.AsTime().After(*created)) {
				at := t.AsTime()
				created = &at
			}
		}
	}
	if created == nil {
		return nil, errors.New("no input entry has a created-at: set --created-at")
	}
	if err := os.MkdirAll(o.Out, 0o755); err != nil {
		return nil, err
	}

	// Start the re-pack.
	r := &repacker{
		o:         o,
		createdAt: timestamppb.New(created.UTC()),
		maxBlocks: o.MaxBlocks,
		sums:      make(map[string]*blockSum),
	}
	r.rep.PacksIn, r.rep.SameIDs = len(packs), sameIDs

	// Read every input pack in id order, each in key order, into the
	// output packs.
	for _, p := range packs {
		if err := r.readPack(p); err != nil {
			return &r.rep, errors.Wrapf(err, "input pack %s", filepath.Join(p.dir, p.id))
		}
	}
	if err := r.flush(); err != nil {
		return &r.rep, err
	}

	// Read the output back and prove it equal to the input.
	if err := r.prove(); err != nil {
		r.print()
		return &r.rep, errors.Wrap(err, "proof failed")
	}
	r.print()
	if o.SliceBytes != 0 {
		if err := printSlices(o.Log, r.rep.Packs, o.SliceBytes); err != nil {
			return &r.rep, err
		}
	}
	return &r.rep, nil
}

// freshOut checks the output directory is missing or empty and is not one of
// the inputs. It creates nothing.
func freshOut(out string, in []string) error {
	// Refuse an input as the output.
	absOut, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	for _, d := range in {
		absIn, err := filepath.Abs(d)
		if err != nil {
			return err
		}
		if absIn == absOut {
			return errors.Errorf("output %s is an input", out)
		}
		if a, err := os.Stat(d); err == nil {
			if b, err := os.Stat(out); err == nil && os.SameFile(a, b) {
				return errors.Errorf("output %s is input %s", out, d)
			}
		}
	}

	// Take a missing or empty directory.
	names, err := os.ReadDir(out)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(names) != 0 {
		return errors.Errorf("output %s is not empty", out)
	}
	return nil
}

// validID reports whether name may be a pack's file name in the adopt
// layout, as the rescue's adopt layout names them.
func validID(name string) bool {
	return name != "" && !strings.ContainsAny(name, `/\`) && !strings.HasPrefix(name, ".") &&
		!strings.HasSuffix(name, entrySuffix) && !strings.HasSuffix(name, ".part")
}

// listInputs returns every {id} with an {id}.entry of the directories, in
// byte order of id and then directory order, with its entry read and checked
// against the pack file's name and size. It also counts the packs whose id
// an earlier directory holds.
func listInputs(dirs []string) ([]*inPack, int, error) {
	// Find the pairs in each directory.
	type found struct {
		id  string
		dir int
	}
	var list []found
	for i, dir := range dirs {
		names, err := os.ReadDir(dir)
		if err != nil {
			return nil, 0, err
		}
		have := make(map[string]bool, len(names))
		for _, n := range names {
			have[n.Name()] = true
		}
		for _, n := range names {
			if n.Type().IsRegular() && validID(n.Name()) && have[n.Name()+entrySuffix] {
				list = append(list, found{n.Name(), i})
			}
		}
	}
	slices.SortFunc(list, func(a, b found) int {
		if c := strings.Compare(a.id, b.id); c != 0 {
			return c
		}
		return a.dir - b.dir
	})

	// Read and check each entry.
	packs := make([]*inPack, 0, len(list))
	var same int
	for i, f := range list {
		if i != 0 && list[i-1].id == f.id {
			same++
		}
		dir := dirs[f.dir]
		raw, err := os.ReadFile(filepath.Join(dir, f.id+entrySuffix))
		if err != nil {
			return nil, 0, err
		}
		entry := &packfile.PackfileEntry{}
		if err := entry.UnmarshalVT(raw); err != nil {
			return nil, 0, errors.Wrapf(err, "decode entry %s", filepath.Join(dir, f.id+entrySuffix))
		}
		st, err := os.Stat(filepath.Join(dir, f.id))
		if err != nil {
			return nil, 0, err
		}
		if entry.GetId() != f.id || entry.GetSizeBytes() != uint64(st.Size()) { //nolint:gosec // sizes are not negative.
			return nil, 0, errors.Errorf("entry %s names %s of %d bytes, the file is %d", filepath.Join(dir, f.id+entrySuffix), entry.GetId(), entry.GetSizeBytes(), st.Size())
		}
		packs = append(packs, &inPack{id: f.id, dir: dir, entry: entry, size: st.Size(), entryBytes: int64(len(raw))})
	}
	return packs, same, nil
}

// readPack reads every block of an input pack in key order, verifying each
// against its key, and adds the new ones to the output.
func (r *repacker) readPack(p *inPack) error {
	// Open the pack read-only and its index.
	f, err := os.Open(filepath.Join(p.dir, p.id))
	if err != nil {
		return err
	}
	defer f.Close()
	rdr, err := kvfile.BuildReader(f, uint64(p.size)) //nolint:gosec // sizes are not negative.
	if err != nil {
		return errors.Wrap(err, "read index")
	}
	if rdr.Size() != p.entry.GetBlockCount() {
		return errors.Errorf("index holds %d blocks, the entry %d", rdr.Size(), p.entry.GetBlockCount())
	}
	r.rep.FilesIn += uint64(p.size + p.entryBytes) //nolint:gosec // sizes are not negative.

	// Verify and take each block.
	var n uint64
	err = rdr.ScanPrefix(nil, func(key, value []byte) error {
		n++
		ref, stored, err := packfile.DecodeBlockValue(key, value)
		if err != nil {
			return errors.Wrapf(err, "block %x", key)
		}
		return r.take(string(key), ref.GetHash(), stored)
	})
	if err != nil {
		return err
	}
	if n != rdr.Size() {
		return errors.Errorf("scanned %d blocks of %d", n, rdr.Size())
	}
	return nil
}

// sumBlock returns the contents sum of a block and its encoded value as the
// pack writer encodes it.
func sumBlock(stored *block.StoredBlock) (blockSum, []byte, error) {
	value, err := block.EncodeBlockObject(stored.Data, stored.Refs, stored.GetRefsKnown())
	if err != nil {
		return blockSum{}, nil, err
	}
	obj := sha256.Sum256(value)
	return blockSum{data: sha256.Sum256(stored.Data), obj: binary.BigEndian.Uint64(obj[:8])}, value, nil
}

// take adds a verified block to the open output pack, closing the pack first
// when the block would take it past its limits. A block already taken is
// dropped as a duplicate, and must equal the one taken.
func (r *repacker) take(key string, h *hash.Hash, stored *block.StoredBlock) error {
	// Count it and check a duplicate.
	sum, value, err := sumBlock(stored)
	if err != nil {
		return err
	}
	r.rep.BlocksIn++
	r.rep.DataIn += uint64(len(stored.Data))
	if prev, ok := r.sums[key]; ok {
		if prev.data != sum.data || prev.obj != sum.obj {
			return errors.Errorf("block %x is held twice with different contents", []byte(key))
		}
		r.rep.Duplicates++
		return nil
	}
	r.sums[key] = &sum
	r.rep.DataUnique += uint64(len(stored.Data))

	// Close the open pack when this block would overflow it.
	size, index := r.packSizeWith([]byte(key), uint64(len(value)))
	if len(r.cur) != 0 && (size > uint64(r.o.PackBytes) || (r.maxBlocks != 0 && uint64(len(r.cur)) >= r.maxBlocks)) { //nolint:gosec // positive.
		if err := r.flush(); err != nil {
			return err
		}
		size, index = r.packSizeWith([]byte(key), uint64(len(value)))
	}
	if size > uint64(r.o.PackBytes) { //nolint:gosec // positive.
		return errors.Errorf("block %x alone makes a pack of %d bytes, past the %d byte limit", []byte(key), size, r.o.PackBytes)
	}

	// Add it.
	r.cur = append(r.cur, pending{h: h, blk: stored})
	r.curPos += uint64(len(value))
	r.curIndex = index
	r.curData += uint64(len(stored.Data))
	return nil
}

// packSizeWith returns the exact size the open pack's kvfile would have with
// one more value of vlen bytes under key, and its index bytes then, by the
// go-kvfile layout: the values, then per entry its IndexEntry, the varint of
// that entry's size and an 8 byte position, then an 8 byte count.
func (r *repacker) packSizeWith(key []byte, vlen uint64) (size, index uint64) {
	// Add the new entry's index bytes to the values and index so far.
	ie := (&kvfile.IndexEntry{Key: key, Offset: r.curPos, Size: vlen}).SizeVT()
	index = r.curIndex + uint64(ie) + uint64(len(binary.AppendUvarint(nil, uint64(ie)))) + 8 //nolint:gosec // positive.
	return r.curPos + vlen + index + 8, index
}

// flush writes the open pack and its entry, if it holds any block.
func (r *repacker) flush() error {
	// Take the open pack's blocks and planned size, and start the next.
	if len(r.cur) == 0 {
		return nil
	}
	want := r.curPos + r.curIndex + 8
	blocks, data := r.cur, r.curData
	r.cur, r.curPos, r.curIndex, r.curData = nil, 0, 0, 0

	// Open a temporary file for the pack.
	tmp, err := os.CreateTemp(r.o.Out, tempPrefix)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck // gone once renamed.

	// Pack the blocks in order with the PackStore's packing code.
	bw := bufio.NewWriterSize(tmp, 1<<20)
	i := 0
	res, err := writer.PackBlocks(bw, func() (*hash.Hash, *block.StoredBlock, error) {
		if i == len(blocks) {
			return nil, nil, nil
		}
		i++
		return blocks[i-1].h, blocks[i-1].blk, nil
	})

	// Flush, sync and close it, and check it is the size planned.
	if err == nil {
		err = bw.Flush()
	}
	if err == nil {
		err = tmp.Chmod(0o644)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return errors.Wrap(err, "write pack")
	}
	if res.BytesWritten != want || res.BytesWritten > uint64(r.o.PackBytes) || res.BlockCount != uint64(len(blocks)) { //nolint:gosec // positive.
		return errors.Errorf("pack of %d blocks is %d bytes, planned %d, limit %d", res.BlockCount, res.BytesWritten, want, r.o.PackBytes)
	}

	// Name it from its bytes and build its entry as the PackStore's
	// writePack does.
	id, err := identity.BuildPackID(r.o.ResourceID, res)
	if err != nil {
		return err
	}
	entry := &packfile.PackfileEntry{
		Id:                 id,
		BloomFilter:        res.BloomFilter,
		BloomFormatVersion: packfile.BloomFormatVersionV1,
		BlockCount:         res.BlockCount,
		SizeBytes:          res.BytesWritten,
		CreatedAt:          r.createdAt,
	}
	raw, err := entry.MarshalVT()
	if err != nil {
		return err
	}

	// Publish the pack, then its entry.
	final := filepath.Join(r.o.Out, id)
	if _, err := os.Lstat(final); !errors.Is(err, fs.ErrNotExist) {
		return errors.Errorf("output pack %s already exists", id)
	}
	if err := os.Rename(tmpName, final); err != nil {
		return err
	}
	if err := writeFile(r.o.Out, id+entrySuffix, raw); err != nil {
		return err
	}
	if err := syncDir(r.o.Out); err != nil {
		return err
	}
	r.rep.Packs = append(r.rep.Packs, outPack{
		id: id, size: int64(res.BytesWritten), entryBytes: int64(len(raw)), //nolint:gosec // below 128 MiB.
		blocks: res.BlockCount, dataBytes: data,
	})
	return nil
}

// writeFile writes data to dir/name through a synced temporary file renamed
// into place.
func writeFile(dir, name string, data []byte) error {
	// Open a temporary file beside the final name.
	tmp, err := os.CreateTemp(dir, tempPrefix)
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // gone once renamed.

	// Write, sync and close it, then rename it into place.
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Chmod(0o644)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(dir, name))
	}
	return errors.Wrap(err, "write "+name)
}

// syncDir syncs a directory so the renames in it are durable.
func syncDir(dir string) error {
	// Open the directory, sync it and close it.
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	if cerr := d.Close(); err == nil {
		err = cerr
	}
	return err
}

// print writes the report's totals.
func (r *repacker) print() {
	// Print one line per total.
	p := &r.rep
	w := r.o.Log
	_, _ = fmt.Fprintf(w, "packs in %d (ids also in another input dir %d) out %d\n", p.PacksIn, p.SameIDs, p.PacksOut)
	_, _ = fmt.Fprintf(w, "blocks in %d unique %d duplicates dropped %d out %d\n", p.BlocksIn, p.BlocksIn-p.Duplicates, p.Duplicates, p.BlocksOut)
	_, _ = fmt.Fprintf(w, "block bytes in %d unique %d out %d\n", p.DataIn, p.DataUnique, p.DataOut)
	_, _ = fmt.Fprintf(w, "file bytes in %d out %d\n", p.FilesIn, p.FilesOut)
}

// printSlices prints the output packs, in write order, grouped into slices of
// at most limit bytes of files, one slice per line. A pack is never split.
func printSlices(w io.Writer, packs []outPack, limit uint64) error {
	// Collect a slice's ids and print it when the next pack would overflow it.
	var ids []string
	var total uint64
	n := 0
	emit := func() {
		if len(ids) == 0 {
			return
		}
		n++
		_, _ = fmt.Fprintf(w, "slice %d bytes %d packs %d ids %s\n", n, total, len(ids), strings.Join(ids, " "))
		ids, total = nil, 0
	}
	for _, p := range packs {
		size := uint64(p.size + p.entryBytes) //nolint:gosec // positive.
		if size > limit {
			return errors.Errorf("pack %s of %d bytes with its entry exceeds the %d byte slice", p.id, size, limit)
		}
		if total+size > limit {
			emit()
		}
		ids = append(ids, p.id)
		total += size
	}
	emit()
	return nil
}
