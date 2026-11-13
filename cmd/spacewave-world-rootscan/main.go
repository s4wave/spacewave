//go:build !js && !wasip1 && !windows

// Command spacewave-world-rootscan reads every block body in a set of local
// pack dirs (pfv1_* packs with their .entry files, the layout
// spacewave-world-rescue --layout adopt writes) as the root of a World Object
// and writes the child refs it finds, for the next rescue round.
//
// Each body is decoded through the World's transform from the head file,
// then typed by shape: a blob.Blob whose fields re-encode to the same bytes
// and whose chunks tile its size gives its chunk refs (blob-chunk); else a
// sql_mysql Root that re-encodes the same, every database named with a ref,
// gives its database refs (sql-node); else the body is a leaf, scanned blind
// for the shape of a BlockRef message at any depth (other). Root and head are
// read at the wire level, so the scan links neither db/sql/mysql nor
// core/space/world. A body the transform cannot decode is scanned blind as
// stored.
//
// Outputs go to -out (must not exist):
//
//	refs.tsv    "<parent hex key>\t<shape>\t<child hex key>" per ref found
//	stats.json  counts only
//
// It never prints a key.
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/sirupsen/logrus"
)

// Shapes a ref is found under.
const (
	shapeBlobChunk = "blob-chunk"
	shapeSQLNode   = "sql-node"
	shapeOther     = "other"
)

// stats are the counts written to stats.json.
type stats struct {
	Packs, Bodies                        uint64
	BlobRoots, SQLRoots, Leaves          uint64
	Undecodable, Empty                   uint64
	BlobChunks, BlobZeroChunks, SQLNodes uint64
	OtherRefs, SelfRefs, LeavesWithRefs  uint64
	Errors                               map[string]uint64
}

// json encodes the counts as one JSON object, errors by sorted reason.
func (st *stats) json() []byte {
	// Set each count under its field name.
	var a fastjson.Arena
	o := a.NewObject()
	for _, f := range []struct {
		name string
		n    uint64
	}{
		{"Packs", st.Packs}, {"Bodies", st.Bodies},
		{"BlobRoots", st.BlobRoots}, {"SQLRoots", st.SQLRoots}, {"Leaves", st.Leaves},
		{"Undecodable", st.Undecodable}, {"Empty", st.Empty},
		{"BlobChunks", st.BlobChunks}, {"BlobZeroChunks", st.BlobZeroChunks}, {"SQLNodes", st.SQLNodes},
		{"OtherRefs", st.OtherRefs}, {"SelfRefs", st.SelfRefs}, {"LeavesWithRefs", st.LeavesWithRefs},
	} {
		o.Set(f.name, a.NewNumberString(strconv.FormatUint(f.n, 10)))
	}

	// Add the error counts in reason order.
	errs := a.NewObject()
	for _, k := range slices.Sorted(maps.Keys(st.Errors)) {
		errs.Set(k, a.NewNumberString(strconv.FormatUint(st.Errors[k], 10)))
	}
	o.Set("Errors", errs)
	return o.MarshalTo(nil)
}

func main() {
	// Parse the flags, all three required.
	packs := flag.String("packs", "", "comma-separated pack dirs")
	headPath := flag.String("head", "", "World head file (WorldHead: inner_state = 1, seqno = 2)")
	out := flag.String("out", "", "output dir (must not exist)")
	flag.Parse()
	if *packs == "" || *headPath == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: spacewave-world-rootscan -packs DIR[,DIR] -head FILE -out DIR")
		os.Exit(2)
	}

	// Build the World's transform from the head file.
	xfrm, err := headTransform(*headPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "head:", err)
		os.Exit(1)
	}

	// Scan the packs and print the counts.
	st, err := run(strings.Split(*packs, ","), xfrm, *out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rootscan:", err)
		os.Exit(1)
	}
	fmt.Println(string(st.json()))
}

// headTransform builds the World's transform from a head file, nil when the
// head names none. The head is WorldHead{inner_state = 1}, InnerState{head_ref
// = 1}, bucket.ObjectRef{transform_conf = 4}; it is read at the wire level so
// the scan does not link core/space/world.
func headTransform(path string) (block.Transformer, error) {
	// Read the head file.
	b, err := os.ReadFile(path) //nolint:gosec // the head path is the operator's flag.
	if err != nil {
		return nil, err
	}

	// Walk WorldHead to InnerState to its head ObjectRef.
	inner, err := wireField(b, 1)
	if err != nil {
		return nil, errors.Wrap(err, "inner_state")
	}
	headRef, err := wireField(inner, 1)
	if err != nil {
		return nil, errors.Wrap(err, "head_ref")
	}

	// Take the ObjectRef's transform_conf, absent when it has none.
	fs, err := wireFields(headRef)
	if err != nil {
		return nil, errors.Wrap(err, "head_ref")
	}
	var raw []byte
	for _, f := range fs {
		if f.num == 4 && f.typ == 2 {
			raw = f.val
		}
	}

	// Decode the transform config and build it, nil when it is empty.
	conf := &block_transform.Config{}
	if err := conf.UnmarshalVT(raw); err != nil {
		return nil, errors.Wrap(err, "transform_conf")
	}
	if conf.GetEmpty() {
		return nil, nil
	}
	return newTransform(conf)
}

// newTransform builds a transformer for conf from every known step.
func newTransform(conf *block_transform.Config) (block.Transformer, error) {
	le := logrus.NewEntry(logrus.StandardLogger())
	return block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, transform_all.BuildFactorySet(), conf)
}

// wf is one protobuf wire field; val is set for length-delimited fields.
type wf struct {
	num, typ uint64
	val      []byte
}

// wireFields splits a message into its fields, failing on malformed wire.
func wireFields(b []byte) ([]wf, error) {
	var out []wf
	for i := 0; i < len(b); {
		tag, n := binary.Uvarint(b[i:])
		if n <= 0 || tag>>3 == 0 {
			return nil, errors.New("bad tag")
		}
		i += n
		f := wf{num: tag >> 3, typ: tag & 7}
		switch f.typ {
		case 0:
			_, n := binary.Uvarint(b[i:])
			if n <= 0 {
				return nil, errors.New("bad varint")
			}
			i += n
		case 1, 5:
			w := 8
			if f.typ == 5 {
				w = 4
			}
			if i+w > len(b) {
				return nil, errors.New("short fixed field")
			}
			i += w
		case 2:
			l, n := binary.Uvarint(b[i:])
			if n <= 0 || uint64(i+n)+l > uint64(len(b)) {
				return nil, errors.New("bad length")
			}
			i += n
			f.val = b[i : i+int(l)] //nolint:gosec // l is bounded by len(b) above.
			i += int(l)             //nolint:gosec // l is bounded by len(b) above.
		default:
			return nil, errors.Errorf("wire type %d", f.typ)
		}
		out = append(out, f)
	}
	return out, nil
}

// wireField returns the last length-delimited field num of a message.
func wireField(b []byte, num uint64) ([]byte, error) {
	// Split the message into fields.
	fs, err := wireFields(b)
	if err != nil {
		return nil, err
	}

	// Keep the last length-delimited field num.
	var out []byte
	found := false
	for _, f := range fs {
		if f.num == num && f.typ == 2 {
			out, found = f.val, true
		}
	}
	if !found {
		return nil, errors.Errorf("no field %d", num)
	}
	return out, nil
}

// sqlDB is one sql_mysql RootDb: name = 1, ref = 2.
type sqlDB struct {
	name string
	ref  *block.BlockRef
}

// sqlShape reports whether body is a sql_mysql Root{repeated RootDb databases
// = 1}: only that field, each database only a non-empty name and a valid
// non-empty ref, and the known fields re-encode to the same bytes. It reads
// the wire directly so the scan does not link db/sql/mysql.
func sqlShape(body []byte) ([]sqlDB, bool) {
	// Split the Root into fields; an empty body is no Root.
	fs, err := wireFields(body)
	if err != nil || len(fs) == 0 {
		return nil, false
	}

	// Read each RootDb, refusing any other field, and re-encode it.
	var dbs []sqlDB
	var canon []byte
	for _, f := range fs {
		// Split the RootDb, the Root's only field.
		if f.num != 1 || f.typ != 2 {
			return nil, false
		}
		inner, err := wireFields(f.val)
		if err != nil {
			return nil, false
		}

		// Take its name and ref, refusing any other field.
		var d sqlDB
		var refRaw []byte
		for _, g := range inner {
			switch {
			case g.num == 1 && g.typ == 2:
				d.name = string(g.val)
			case g.num == 2 && g.typ == 2:
				refRaw = g.val
			default:
				return nil, false
			}
		}

		// Require a name and a valid non-empty ref.
		d.ref = &block.BlockRef{}
		if d.name == "" || refRaw == nil || d.ref.UnmarshalVT(refRaw) != nil || d.ref.GetEmpty() || d.ref.Validate(false) != nil {
			return nil, false
		}

		// Re-encode the RootDb from its known fields.
		ref, err := d.ref.MarshalVT()
		if err != nil {
			return nil, false
		}
		var db []byte
		db = appendLD(db, 1, []byte(d.name))
		db = appendLD(db, 2, ref)
		canon = appendLD(canon, 1, db)
		dbs = append(dbs, d)
	}

	// Accept only a Root whose re-encoding is the body itself.
	return dbs, bytes.Equal(canon, body)
}

// appendLD appends length-delimited field num holding v.
func appendLD(b []byte, num uint64, v []byte) []byte {
	b = binary.AppendUvarint(b, num<<3|2)
	b = binary.AppendUvarint(b, uint64(len(v)))
	return append(b, v...)
}

// packFiles lists dir's pfv1_* packs, each with its .entry file.
func packFiles(dir string) ([]string, error) {
	// List the dir's names.
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, e := range ents {
		have[e.Name()] = true
	}

	// Take each pack, refusing one without its entry file.
	var out []string
	for _, e := range ents {
		n := e.Name()
		if !strings.HasPrefix(n, "pfv1_") || strings.HasSuffix(n, ".entry") {
			continue
		}
		if !have[n+".entry"] {
			return nil, errors.Errorf("%s: pack %s without an entry file", dir, n)
		}
		out = append(out, filepath.Join(dir, n))
	}

	// Scan in name order.
	slices.Sort(out)
	return out, nil
}

// run scans every body of the packs in dirs into out.
func run(dirs []string, xfrm block.Transformer, out string) (*stats, error) {
	// Create the new output dir and its refs.tsv.
	if err := os.Mkdir(out, 0o700); err != nil {
		return nil, err
	}
	f, err := os.Create(filepath.Join(out, "refs.tsv"))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Scan every pack of every dir, refusing a dir without packs.
	w := bufio.NewWriterSize(f, 1<<20)
	st := &stats{Errors: map[string]uint64{}}
	for _, d := range dirs {
		files, err := packFiles(d)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, errors.Errorf("%s: no packs", d)
		}
		for _, p := range files {
			if err := scanPack(p, xfrm, st, w); err != nil {
				return nil, errors.Wrap(err, filepath.Base(p))
			}
		}
	}

	// Flush the refs and write the counts beside them.
	if err := w.Flush(); err != nil {
		return nil, err
	}
	return st, os.WriteFile(filepath.Join(out, "stats.json"), append(st.json(), '\n'), 0o644)
}

// scanPack scans one pack's bodies.
func scanPack(path string, xfrm block.Transformer, st *stats, w *bufio.Writer) error {
	// Read the whole pack and its index.
	data, err := os.ReadFile(path) //nolint:gosec // pack paths come from the operator's dirs.
	if err != nil {
		return err
	}
	index, err := packfile.ReadIndex(bytes.NewReader(data), uint64(len(data)))
	if err != nil {
		return err
	}
	st.Packs++

	// Classify each body and write its child refs, skipping self refs.
	for _, ie := range index {
		// Decode the stored object at the entry's extent.
		off, size := ie.GetOffset(), ie.GetSize()
		if off+size > uint64(len(data)) {
			st.Errors["extent past pack"]++
			continue
		}
		key := ie.GetKey()
		_, stored, err := packfile.DecodeBlockValue(key, data[off:off+size])
		if err != nil {
			st.Errors["decode value"]++
			continue
		}

		// Type the body and write a line per child ref.
		st.Bodies++
		shape, refs := classify(stored.Data, xfrm, st)
		parent := hex.EncodeToString(key)
		for _, r := range refs {
			child := packfile.BlockKey(r.GetHash())
			if bytes.Equal(child, key) {
				st.SelfRefs++
				continue
			}
			if _, err := fmt.Fprintf(w, "%s\t%s\t%x\n", parent, shape, child); err != nil {
				return err
			}
		}
	}
	return nil
}

// classify types a stored body by shape and returns its child refs.
func classify(stored []byte, xfrm block.Transformer, st *stats) (string, []*block.BlockRef) {
	// Decode through the transform; an undecodable body is scanned blind.
	body := stored
	if xfrm != nil {
		dec, err := xfrm.DecodeBlock(stored)
		if err != nil {
			st.Undecodable++
			refs := blindRefs(stored)
			st.OtherRefs += uint64(len(refs))
			return shapeOther, refs
		}
		body = dec
	}

	// An empty body is a leaf with no refs.
	if len(body) == 0 {
		st.Empty++
		st.Leaves++
		return shapeOther, nil
	}

	// A blob gives its non-empty chunk refs.
	if b, ok := blobShape(body); ok {
		st.BlobRoots++
		var refs []*block.BlockRef
		for _, c := range b.GetChunkIndex().GetChunks() {
			if c.GetDataRef().GetEmpty() {
				st.BlobZeroChunks++
				continue
			}
			refs = append(refs, c.GetDataRef())
		}
		st.BlobChunks += uint64(len(refs))
		return shapeBlobChunk, refs
	}

	// A sql Root gives its database refs.
	if r, ok := sqlShape(body); ok {
		st.SQLRoots++
		var refs []*block.BlockRef
		for _, d := range r {
			refs = append(refs, d.ref)
		}
		st.SQLNodes += uint64(len(refs))
		return shapeSQLNode, refs
	}

	// Anything else is a leaf scanned blind for embedded refs.
	st.Leaves++
	refs := blindRefs(body)
	if len(refs) != 0 {
		st.LeavesWithRefs++
	}
	st.OtherRefs += uint64(len(refs))
	return shapeOther, refs
}

// blobShape reports whether body is a blob.Blob: it re-encodes from its known
// fields to the same bytes (no unknown fields, canonical order), its type is
// raw or chunked, a raw blob holds exactly its size inline, and a chunked
// blob's chunks start where the previous ended and sum to its size.
func blobShape(body []byte) (*blob.Blob, bool) {
	// Decode the Blob; a zero size is no blob.
	b := &blob.Blob{}
	if err := b.UnmarshalVT(body); err != nil || b.GetTotalSize() == 0 {
		return nil, false
	}

	// Rebuild it from its known fields, checking the inline or chunked size.
	canon := &blob.Blob{BlobType: b.GetBlobType(), TotalSize: b.GetTotalSize(), RawData: b.GetRawData()}
	switch b.GetBlobType() {
	case blob.BlobType_BlobType_RAW:
		if b.GetChunkIndex() != nil || uint64(len(b.GetRawData())) != b.GetTotalSize() {
			return nil, false
		}
	case blob.BlobType_BlobType_CHUNKED:
		ci := b.GetChunkIndex()
		if len(b.GetRawData()) != 0 || len(ci.GetChunks()) == 0 {
			return nil, false
		}
		cci := &blob.ChunkIndex{ChunkerArgs: ci.GetChunkerArgs(), TailStart: ci.GetTailStart()}
		var at uint64
		for _, c := range ci.GetChunks() {
			if c == nil || c.GetStart() != at || c.GetSize() == 0 {
				return nil, false
			}
			if !c.GetDataRef().GetEmpty() && c.GetDataRef().Validate(false) != nil {
				return nil, false
			}
			at += c.GetSize()
			cci.Chunks = append(cci.Chunks, &blob.Chunk{DataRef: c.GetDataRef(), Size: c.GetSize(), Start: c.GetStart()})
		}
		if at != b.GetTotalSize() {
			return nil, false
		}
		canon.ChunkIndex = cci
	default:
		return nil, false
	}

	// Accept only a Blob whose re-encoding is the body itself.
	re, err := canon.MarshalVT()
	return b, err == nil && bytes.Equal(re, body)
}

// refShape is a BlockRef's hash field at any depth: length 36, then Hash
// {hash_type = SHA256, hash = 32 bytes}.
var refShape = []byte{0x24, 0x08, 0x01, 0x12, 0x20}

// blindRefs returns the block refs whose shape appears in b.
func blindRefs(b []byte) []*block.BlockRef {
	var out []*block.BlockRef
	for off := 0; ; {
		i := bytes.Index(b[off:], refShape)
		if i < 0 {
			return out
		}
		s := off + i + 1
		if s+36 > len(b) {
			return out
		}
		ref := &block.BlockRef{}
		// A BlockRef's field 1 is its Hash: wrap the 36 bytes as that field.
		if err := ref.UnmarshalVT(append([]byte{0x0a, 0x24}, b[s:s+36]...)); err == nil && ref.GetHash() != nil {
			out = append(out, ref)
		}
		off = s + 4
	}
}
