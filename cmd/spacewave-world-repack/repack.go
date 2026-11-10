package main

import (
	"bufio"
	"bytes"
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
	// Frames are the input frame files of hyperline's world block-export,
	// only read, after the directories' packs.
	Frames []string
	// Out is the output directory: missing or empty.
	Out string
	// PackBytes bounds an output pack file.
	PackBytes int64
	// MaxBlocks bounds the blocks of an output pack; zero is no bound, so
	// packs fill by bytes alone. The command defaults it to
	// defaultMaxBlocks.
	MaxBlocks uint64
	// SliceBytes bounds the files of a slice of the plan; zero prints none.
	SliceBytes uint64
	// ResourceID scopes the pack ids.
	ResourceID string
	// CreatedAt is every output entry's created-at; nil takes the latest
	// of the input entries.
	CreatedAt *time.Time
	// AllowExtra skips the input files that are not packs or entries,
	// counted, instead of failing on them. A pack without its entry or an
	// entry without its pack always fails.
	AllowExtra bool
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

// maxKeyLen is the longest block key: the encoded hash of a 32 byte digest.
const maxKeyLen = 36

// blockKey is a block key held as a map key: its length, then its bytes.
type blockKey [maxKeyLen + 1]byte

// toBlockKey returns key as a blockKey, or an error for a key too long.
func toBlockKey(key []byte) (blockKey, error) {
	// Store the length, then the bytes.
	var k blockKey
	if len(key) > maxKeyLen {
		return k, errors.Errorf("block key %x is longer than %d bytes", key, maxKeyLen)
	}
	k[0] = byte(len(key)) //nolint:gosec // at most maxKeyLen.
	copy(k[1:], key)
	return k, nil
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
	// FrameFiles counts the input frame files and FrameBytes their bytes.
	FrameFiles int
	FrameBytes uint64
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
	sums      map[blockKey]blockSum
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
	if len(o.In) == 0 && len(o.Frames) == 0 {
		return nil, errors.New("no input directory or frame file")
	}
	if err := freshOut(o.Out, slices.Concat(o.In, o.Frames)); err != nil {
		return nil, err
	}

	// List and check the inputs' entries before reading a block.
	packs, sameIDs, err := listInputs(o.In, o.AllowExtra, o.Log)
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
		sums:      make(map[blockKey]blockSum),
	}
	r.rep.PacksIn, r.rep.SameIDs = len(packs), sameIDs

	// Read every input pack in id order, each in key order, into the
	// output packs.
	for _, p := range packs {
		if err := r.readPack(p); err != nil {
			return &r.rep, errors.Wrapf(err, "input pack %s", filepath.Join(p.dir, p.id))
		}
	}
	for _, path := range o.Frames {
		if err := r.readFrames(path); err != nil {
			return &r.rep, errors.Wrapf(err, "input frames %s", path)
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
		printSlices(o.Log, r.rep.Packs, o.SliceBytes)
	}
	return &r.rep, nil
}

// freshOut checks the output directory is missing or empty and neither is,
// holds or lies inside an input, by their paths with symlinks resolved. It
// creates nothing.
func freshOut(out string, in []string) error {
	// Refuse an output that is, holds or lies inside an input.
	realOut, err := resolvePath(out)
	if err != nil {
		return err
	}
	for _, d := range in {
		realIn, err := resolvePath(d)
		if err != nil {
			return err
		}
		if within(realIn, realOut) || within(realOut, realIn) {
			return errors.Errorf("output %s (%s) and input %s (%s) are inside one another", out, realOut, d, realIn)
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

// resolvePath returns p absolute with its symlinks resolved. Where p does not
// exist, the nearest existing parent is resolved and the rest kept.
func resolvePath(p string) (string, error) {
	// Resolve the path when it exists.
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}

	// Resolve its parent when it is missing, and keep its name.
	parent := filepath.Dir(abs)
	if parent == abs {
		return abs, nil
	}
	rp, err := resolvePath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(rp, filepath.Base(abs)), nil
}

// within reports whether path p is dir or inside it.
func within(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// validID reports whether name may be a pack's file name in the adopt
// layout, as the rescue's adopt layout names them.
func validID(name string) bool {
	return name != "" && !strings.ContainsAny(name, `/\`) && !strings.HasPrefix(name, ".") &&
		!strings.HasSuffix(name, entrySuffix) && !strings.HasSuffix(name, ".part")
}

// isPackID reports whether name is a v1 pack id, the name of a pack file.
func isPackID(name string) bool {
	return validID(name) && identity.ValidatePackID(name) == nil
}

// dirCounts counts the names of an input directory by kind.
type dirCounts struct {
	pairs, parts, dirs, symlinks, other int
}

// extra returns the count of names that are not packs or entries.
func (c dirCounts) extra() int {
	return c.parts + c.dirs + c.symlinks + c.other
}

// classifyDir sorts the names of an input directory: each {id} regular file,
// named by a v1 pack id, with its {id}.entry regular file is a pair, returned in byte order of id;
// everything else is counted by kind and named in extras. A pack without its
// entry, or an entry without its pack, is an error.
func classifyDir(dir string) ([]string, dirCounts, []string, error) {
	// Read the directory.
	var c dirCounts
	names, err := os.ReadDir(dir)
	if err != nil {
		return nil, c, nil, err
	}

	// Find the regular packs and entries; count the rest.
	packs := make(map[string]bool)
	entries := make(map[string]bool)
	var extras []string
	for _, n := range names {
		name := n.Name()
		switch {
		case n.Type()&fs.ModeSymlink != 0:
			c.symlinks++
		case n.IsDir():
			c.dirs++
		case !n.Type().IsRegular():
			c.other++
		case strings.HasSuffix(name, ".part"):
			c.parts++
		case isPackID(name):
			packs[name] = true
			continue
		case strings.HasSuffix(name, entrySuffix) && isPackID(strings.TrimSuffix(name, entrySuffix)):
			entries[strings.TrimSuffix(name, entrySuffix)] = true
			continue
		default:
			c.other++
		}
		extras = append(extras, name)
	}

	// Pair them: a pack without its entry or an entry without its pack fails.
	var ids, unpaired []string
	for id := range packs {
		if !entries[id] {
			unpaired = append(unpaired, id+" (no "+id+entrySuffix+")")
			continue
		}
		ids = append(ids, id)
	}
	for id := range entries {
		if !packs[id] {
			unpaired = append(unpaired, id+entrySuffix+" (no regular "+id+")")
		}
	}
	if len(unpaired) != 0 {
		slices.Sort(unpaired)
		return nil, c, nil, errors.Errorf("input %s holds %d unpaired packs or entries: %s", dir, len(unpaired), strings.Join(head(unpaired, 10), ", "))
	}
	slices.Sort(ids)
	slices.Sort(extras)
	c.pairs = len(ids)
	return ids, c, extras, nil
}

// head returns at most n of list.
func head(list []string, n int) []string {
	return list[:min(len(list), n)]
}

// listInputs returns every {id} with its {id}.entry of the directories, in
// byte order of id and then directory order, with its entry read and checked
// against the pack file's name and size, and prints each directory's counts.
// A name that is not part of a pair fails unless allowExtra skips it. It also
// counts the packs whose id an earlier directory holds.
func listInputs(dirs []string, allowExtra bool, log io.Writer) ([]*inPack, int, error) {
	// Classify each directory's names.
	type found struct {
		id  string
		dir int
	}
	var list []found
	for i, dir := range dirs {
		ids, c, extras, err := classifyDir(dir)
		if err != nil {
			return nil, 0, err
		}
		_, _ = fmt.Fprintf(log, "in %s: pairs %d extra %d (part %d dir %d symlink %d other %d)\n", dir, c.pairs, c.extra(), c.parts, c.dirs, c.symlinks, c.other)
		if len(extras) != 0 && !allowExtra {
			return nil, 0, errors.Errorf("input %s holds %d names that are not pack and entry pairs, such as %s; --allow-extra skips them", dir, len(extras), strings.Join(head(extras, 10), ", "))
		}
		for _, id := range ids {
			list = append(list, found{id, i})
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
		st, err := os.Lstat(filepath.Join(dir, f.id))
		if err != nil {
			return nil, 0, err
		}
		if !st.Mode().IsRegular() {
			return nil, 0, errors.Errorf("input pack %s is not a regular file", filepath.Join(dir, f.id))
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
		h, stored, err := decodeBlock(key, value)
		if err != nil {
			return err
		}
		return r.take(key, h, stored, value)
	})
	if err != nil {
		return err
	}
	if n != rdr.Size() {
		return errors.Errorf("scanned %d blocks of %d", n, rdr.Size())
	}
	return nil
}

// decodeBlock decodes a block value, verifying its bytes against its key,
// and requires the key be the canonical key of its hash, as the writer
// writes it.
func decodeBlock(key, value []byte) (*hash.Hash, *block.StoredBlock, error) {
	// Decode it, verifying its bytes against its key.
	ref, stored, err := packfile.DecodeBlockValue(key, value)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "block %x", key)
	}

	// Require the canonical key.
	if want := packfile.BlockKey(ref.GetHash()); !bytes.Equal(key, want) {
		return nil, nil, errors.Errorf("block key %x is not the canonical key %x of its hash", key, want)
	}
	return ref.GetHash(), stored, nil
}

// frameReader reads a frame file, counting its offset.
type frameReader struct {
	r *bufio.Reader
	// n is the bytes consumed; size the file's.
	n, size int64
}

// ReadByte reads one byte and counts it.
func (f *frameReader) ReadByte() (byte, error) {
	b, err := f.r.ReadByte()
	if err == nil {
		f.n++
	}
	return b, err
}

// uvarint reads a uvarint; a cut file is io.ErrUnexpectedEOF.
func (f *frameReader) uvarint() (uint64, error) {
	v, err := binary.ReadUvarint(f)
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return v, err
}

// read reads n bytes; n past the file's end is io.ErrUnexpectedEOF.
func (f *frameReader) read(n uint64) ([]byte, error) {
	// Refuse a length past the file's end before allocating it.
	if n > uint64(f.size-f.n) { //nolint:gosec // n never passes size.
		return nil, io.ErrUnexpectedEOF
	}

	// Read and count the bytes.
	buf := make([]byte, n)
	k, err := io.ReadFull(f.r, buf)
	f.n += int64(k)
	return buf, err
}

// readFrames reads every frame of a frame file of hyperline's world
// block-export, in file order, verifying each block against its key and its
// refs flag against its object, and adds the new ones to the output. A
// frame is the uvarint length of the marshaled hash, the hash, a uvarint 1
// when the block's refs are known and 0 when not, the uvarint length of the
// encoded block.BlockObject and the object. A cut or malformed frame fails,
// named by its offset.
func (r *repacker) readFrames(path string) error {
	// Open the file read-only.
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return errors.New("not a regular file")
	}
	r.rep.FrameFiles++
	r.rep.FrameBytes += uint64(st.Size()) //nolint:gosec // sizes are not negative.

	// Read each frame until the end of the file.
	fr := &frameReader{r: bufio.NewReaderSize(f, 1<<20), size: st.Size()}
	for fr.n < fr.size {
		start := fr.n
		if err := r.readFrame(fr); err != nil {
			return errors.Wrapf(err, "frame at offset %d", start)
		}
	}
	if _, err := fr.r.ReadByte(); err != io.EOF {
		return errors.Errorf("file grew past its %d bytes while read", fr.size)
	}
	return nil
}

// readFrame reads one frame and takes its block.
func (r *repacker) readFrame(fr *frameReader) error {
	// Read the key.
	keyLen, err := fr.uvarint()
	if err != nil {
		return errors.Wrap(err, "key length")
	}
	if keyLen == 0 || keyLen > maxKeyLen {
		return errors.Errorf("key length %d is not 1 to %d", keyLen, maxKeyLen)
	}
	key, err := fr.read(keyLen)
	if err != nil {
		return errors.Wrap(err, "key")
	}

	// Read the refs flag and the object.
	known, err := fr.uvarint()
	if err != nil {
		return errors.Wrap(err, "refs flag")
	}
	if known > 1 {
		return errors.Errorf("refs flag %d is not 0 or 1", known)
	}
	objLen, err := fr.uvarint()
	if err != nil {
		return errors.Wrap(err, "object length")
	}
	if objLen > uint64(r.o.PackBytes) { //nolint:gosec // positive.
		return errors.Errorf("object length %d is past the %d byte pack limit", objLen, r.o.PackBytes)
	}
	obj, err := fr.read(objLen)
	if err != nil {
		return errors.Wrap(err, "object")
	}

	// Verify the block, and its refs flag against its object.
	h, stored, err := decodeBlock(key, obj)
	if err != nil {
		return err
	}
	if stored.RefsKnown != (known == 1) {
		return errors.Errorf("block %x: refs flag %d, its object's refs known %v", key, known, stored.RefsKnown)
	}
	return r.take(key, h, stored, obj)
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
// when the block would take it past its limits. raw is the block's value as
// the input pack holds it, which the writer's encoding must reproduce. A
// block already taken is dropped as a duplicate, and must equal the one
// taken.
func (r *repacker) take(key []byte, h *hash.Hash, stored *block.StoredBlock, raw []byte) error {
	// Encode it as the writer will and require the input's bytes.
	sum, value, err := sumBlock(stored)
	if err != nil {
		return err
	}
	if !bytes.Equal(value, raw) {
		return errors.Errorf("block %x: its value re-encoded is %d bytes that differ from the input's %d bytes, so the output would not hold the input's value bytes", key, len(value), len(raw))
	}

	// Count it and check a duplicate.
	mk, err := toBlockKey(key)
	if err != nil {
		return err
	}
	r.rep.BlocksIn++
	r.rep.DataIn += uint64(len(stored.Data))
	if prev, ok := r.sums[mk]; ok {
		if prev.data != sum.data || prev.obj != sum.obj {
			return errors.Errorf("block %x is held twice with different contents", key)
		}
		r.rep.Duplicates++
		return nil
	}
	r.sums[mk] = sum
	r.rep.DataUnique += uint64(len(stored.Data))

	// Close the open pack when this block would overflow it.
	size, index := r.packSizeWith(key, uint64(len(value)))
	if len(r.cur) != 0 && (size > uint64(r.o.PackBytes) || (r.maxBlocks != 0 && uint64(len(r.cur)) >= r.maxBlocks)) { //nolint:gosec // positive.
		if err := r.flush(); err != nil {
			return err
		}
		size, index = r.packSizeWith(key, uint64(len(value)))
	}
	if size > uint64(r.o.PackBytes) { //nolint:gosec // positive.
		return errors.Errorf("block %x alone makes a pack of %d bytes, past the %d byte limit", key, size, r.o.PackBytes)
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
	_, _ = fmt.Fprintf(w, "frame files in %d bytes %d\n", p.FrameFiles, p.FrameBytes)
}

// printSlices prints the output packs, in write order, grouped into slices of
// at most limit bytes of files, one slice per line. A pack is never split: a
// pack with its entry past limit gets a slice of its own, after a warning
// line.
func printSlices(w io.Writer, packs []outPack, limit uint64) {
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
		if total+size > limit {
			emit()
		}
		if size > limit {
			_, _ = fmt.Fprintf(w, "WARN pack %s of %d bytes with its entry is past the %d byte slice: it gets a slice of its own\n", p.id, size, limit)
		}
		ids = append(ids, p.id)
		total += size
		if size > limit {
			emit()
		}
	}
	emit()
}
