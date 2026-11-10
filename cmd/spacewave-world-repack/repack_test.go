package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
)

// testBlock is a synthetic block.
type testBlock struct {
	ref  *block.BlockRef
	data []byte
}

// newBlocks returns n blocks of size bytes from a seeded source.
func newBlocks(t *testing.T, rng *rand.Rand, n, size int) []testBlock {
	t.Helper()
	out := make([]testBlock, n)
	for i := range out {
		data := make([]byte, size)
		for j := range data {
			data[j] = byte(rng.Uint32())
		}
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = testBlock{ref: ref, data: data}
	}
	return out
}

// writeInput writes blks as one pack and its entry in the adopt layout of
// dir, as the PackStore writes them, and returns the pack's id.
func writeInput(t *testing.T, dir string, blks []testBlock) string {
	t.Helper()
	var buf bytes.Buffer
	i := 0
	res, err := writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		if i == len(blks) {
			return nil, nil, nil
		}
		i++
		return blks[i-1].ref.GetHash(), &block.StoredBlock{Data: blks[i-1].data, RefsKnown: true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	id := identity.NewPackID()
	entry := &packfile.PackfileEntry{
		Id: id, BloomFilter: res.BloomFilter, BloomFormatVersion: packfile.BloomFormatVersionV1,
		BlockCount: res.BlockCount, SizeBytes: res.BytesWritten,
		CreatedAt: timestamppb.New(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)),
	}
	raw, err := entry.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+entrySuffix), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return id
}

// snapshot returns every file of dir by name with its bytes.
func snapshot(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	names, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string][]byte, len(names))
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(dir, n.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[n.Name()] = data
	}
	return out
}

// sameFiles reports whether two snapshots hold the same names and bytes.
func sameFiles(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || !bytes.Equal(v, w) {
			return false
		}
	}
	return true
}

// testOpts returns options re-packing in into out at packBytes.
func testOpts(in []string, out string, packBytes int64) *options {
	return &options{In: in, Out: out, PackBytes: packBytes, SliceBytes: 1e9, ResourceID: defaultResourceID, Log: io.Discard}
}

// TestRepackRoundTrip re-packs several small packs in two directories into
// fewer packs that hold every block once, and leaves the inputs unchanged.
func TestRepackRoundTrip(t *testing.T) {
	// Write five packs of eight 1 KiB blocks across two directories.
	rng := rand.New(rand.NewPCG(1, 2))
	root := t.TempDir()
	in := []string{filepath.Join(root, "a"), filepath.Join(root, "b")}
	for i := range 5 {
		blks := newBlocks(t, rng, 8, 1024)
		writeInput(t, in[i%2], blks)
	}
	before := []map[string][]byte{snapshot(t, in[0]), snapshot(t, in[1])}

	// Re-pack into packs of at most 24 KiB.
	out := filepath.Join(root, "out")
	var log bytes.Buffer
	o := testOpts(in, out, 24<<10)
	o.Log = &log
	rep, err := run(o)
	if err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	if rep.PacksIn != 5 || rep.PacksOut >= 5 || rep.PacksOut < 2 || rep.BlocksIn != 40 || rep.BlocksOut != 40 || rep.Duplicates != 0 {
		t.Fatalf("report %+v", rep)
	}
	if rep.DataIn != 40*1024 || rep.DataOut != 40*1024 {
		t.Fatalf("block bytes in %d out %d", rep.DataIn, rep.DataOut)
	}
	if !strings.Contains(log.String(), "proof ok") || !strings.Contains(log.String(), "slice 1 ") {
		t.Fatalf("log:\n%s", log.String())
	}

	// Every block is in the output; the inputs are as they were.
	got := snapshot(t, out)
	if len(got) != 2*rep.PacksOut {
		t.Fatalf("output files %d", len(got))
	}
	for i, d := range in {
		if !sameFiles(before[i], snapshot(t, d)) {
			t.Fatalf("input %s changed", d)
		}
	}

	// A second run into the now full output is refused.
	if _, err := run(testOpts(in, out, 24<<10)); err == nil {
		t.Fatal("a non-empty output was taken")
	}
}

// TestRepackDuplicate writes a block held by two input packs once.
func TestRepackDuplicate(t *testing.T) {
	// Two packs in two directories share two blocks.
	rng := rand.New(rand.NewPCG(3, 4))
	root := t.TempDir()
	in := []string{filepath.Join(root, "a"), filepath.Join(root, "b")}
	shared := newBlocks(t, rng, 2, 500)
	writeInput(t, in[0], append(newBlocks(t, rng, 3, 500), shared...))
	writeInput(t, in[1], append(shared, newBlocks(t, rng, 4, 500)...))

	// The output holds the nine distinct blocks.
	rep, err := run(testOpts(in, filepath.Join(root, "out"), 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	if rep.BlocksIn != 11 || rep.Duplicates != 2 || rep.BlocksOut != 9 || rep.DataOut != 9*500 || rep.PacksOut != 1 {
		t.Fatalf("report %+v", rep)
	}
}

// TestRepackCorrupt fails on an input block whose bytes do not match its
// ref, and leaves the input as it was.
func TestRepackCorrupt(t *testing.T) {
	// Flip a byte inside the first value's block data.
	rng := rand.New(rand.NewPCG(5, 6))
	dir := filepath.Join(t.TempDir(), "in")
	id := writeInput(t, dir, newBlocks(t, rng, 4, 300))
	path := filepath.Join(dir, id)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[20] ^= 0xff
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, dir)

	// The run fails and the input is unchanged.
	if _, err := run(testOpts([]string{dir}, filepath.Join(t.TempDir(), "out"), 1<<20)); err == nil {
		t.Fatal("a corrupt block was taken")
	}
	if !sameFiles(before, snapshot(t, dir)) {
		t.Fatal("input changed")
	}
}

// TestRepackDeterministic gives byte-identical output for two runs over the
// same inputs.
func TestRepackDeterministic(t *testing.T) {
	// Write six packs of mixed sizes.
	rng := rand.New(rand.NewPCG(7, 8))
	root := t.TempDir()
	in := []string{filepath.Join(root, "a"), filepath.Join(root, "b"), filepath.Join(root, "c")}
	for i := range 6 {
		writeInput(t, in[i%3], newBlocks(t, rng, 5+i, 700+100*i))
	}

	// Two runs give the same files.
	var outs []map[string][]byte
	var logs []string
	for _, name := range []string{"out1", "out2"} {
		var log bytes.Buffer
		o := testOpts(in, filepath.Join(root, name), 16<<10)
		o.Log = &log
		if _, err := run(o); err != nil {
			t.Fatal(err)
		}
		outs = append(outs, snapshot(t, filepath.Join(root, name)))
		logs = append(logs, log.String())
	}
	if len(outs[0]) < 4 || !sameFiles(outs[0], outs[1]) || logs[0] != logs[1] {
		t.Fatalf("runs differ: %d and %d files\n%s\n%s", len(outs[0]), len(outs[1]), logs[0], logs[1])
	}
}

// TestRepackSizeCap fills packs close to the size limit and never past it.
func TestRepackSizeCap(t *testing.T) {
	// Write 200 KiB of blocks of uneven sizes.
	rng := rand.New(rand.NewPCG(9, 10))
	root := t.TempDir()
	in := filepath.Join(root, "in")
	for range 4 {
		var blks []testBlock
		for range 25 {
			blks = append(blks, newBlocks(t, rng, 1, 1000+rng.IntN(3000))...)
		}
		writeInput(t, in, blks)
	}

	// Re-pack at 32 KiB: every pack within it and all but the last within
	// one largest block of it.
	const limit = 32 << 10
	rep, err := run(testOpts([]string{in}, filepath.Join(root, "out"), limit))
	if err != nil {
		t.Fatal(err)
	}
	if rep.PacksOut < 5 {
		t.Fatalf("packs out %d", rep.PacksOut)
	}
	for i, p := range rep.Packs {
		if p.size > limit {
			t.Fatalf("pack %s of %d bytes past %d", p.id, p.size, limit)
		}
		if i != len(rep.Packs)-1 && p.size < limit-4200 {
			t.Fatalf("pack %s of %d bytes is not filled", p.id, p.size)
		}
	}

	// A block larger than the limit is refused.
	big := filepath.Join(root, "big")
	writeInput(t, big, newBlocks(t, rng, 1, limit))
	if _, err := run(testOpts([]string{big}, filepath.Join(root, "out2"), limit)); err == nil {
		t.Fatal("a block past the limit was taken")
	}
}

// TestPrintSlices groups packs into slices without splitting one.
func TestPrintSlices(t *testing.T) {
	packs := []outPack{{id: "a", size: 400, entryBytes: 10}, {id: "b", size: 500, entryBytes: 10}, {id: "c", size: 80, entryBytes: 10}, {id: "d", size: 900, entryBytes: 10}}
	var buf bytes.Buffer
	printSlices(&buf, packs, 1000)
	want := "slice 1 bytes 920 packs 2 ids a b\nslice 2 bytes 1000 packs 2 ids c d\n"
	if buf.String() != want {
		t.Fatalf("got\n%s", buf.String())
	}

	// A pack past the slice gets a slice of its own after a warning.
	buf.Reset()
	printSlices(&buf, packs, 500)
	want = "slice 1 bytes 410 packs 1 ids a\n" +
		"WARN pack b of 510 bytes with its entry is past the 500 byte slice: it gets a slice of its own\n" +
		"slice 2 bytes 510 packs 1 ids b\n" +
		"slice 3 bytes 90 packs 1 ids c\n" +
		"WARN pack d of 910 bytes with its entry is past the 500 byte slice: it gets a slice of its own\n" +
		"slice 4 bytes 910 packs 1 ids d\n"
	if buf.String() != want {
		t.Fatalf("got\n%s", buf.String())
	}
}

// TestRepackUnpaired fails on a pack left beside a staged entry, with or
// without --allow-extra, and on a stray file without it; with it, the stray
// is skipped and counted.
func TestRepackUnpaired(t *testing.T) {
	// A pack whose entry is still staged as {id}.entry.part.
	rng := rand.New(rand.NewPCG(11, 12))
	root := t.TempDir()
	in := filepath.Join(root, "in")
	writeInput(t, in, newBlocks(t, rng, 3, 200))
	id := writeInput(t, in, newBlocks(t, rng, 3, 200))
	if err := os.Rename(filepath.Join(in, id+entrySuffix), filepath.Join(in, id+entrySuffix+".part")); err != nil {
		t.Fatal(err)
	}
	for _, allow := range []bool{false, true} {
		o := testOpts([]string{in}, filepath.Join(root, "out"), 1<<20)
		o.AllowExtra = allow
		if _, err := run(o); err == nil || !strings.Contains(err.Error(), "unpaired") {
			t.Fatalf("allow-extra %v: a pack without its entry was taken: %v", allow, err)
		}
	}

	// An entry without its pack fails too.
	in2 := filepath.Join(root, "in2")
	id2 := writeInput(t, in2, newBlocks(t, rng, 3, 200))
	if err := os.Remove(filepath.Join(in2, id2)); err != nil {
		t.Fatal(err)
	}
	o := testOpts([]string{in2}, filepath.Join(root, "out"), 1<<20)
	o.AllowExtra = true
	if _, err := run(o); err == nil || !strings.Contains(err.Error(), "unpaired") {
		t.Fatalf("an entry without its pack was taken: %v", err)
	}

	// A stray file fails, unless --allow-extra skips and counts it.
	in3 := filepath.Join(root, "in3")
	writeInput(t, in3, newBlocks(t, rng, 3, 200))
	if err := os.WriteFile(filepath.Join(in3, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(in3, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := run(testOpts([]string{in3}, filepath.Join(root, "out3"), 1<<20)); err == nil {
		t.Fatal("a stray file was taken")
	}
	var log bytes.Buffer
	o = testOpts([]string{in3}, filepath.Join(root, "out3"), 1<<20)
	o.AllowExtra, o.Log = true, &log
	if rep, err := run(o); err != nil || rep.BlocksOut != 3 {
		t.Fatalf("allow-extra run: %v", err)
	}
	if !strings.Contains(log.String(), "pairs 1 extra 2 (part 0 dir 1 symlink 0 other 1)") {
		t.Fatalf("log:\n%s", log.String())
	}
}

// TestRepackRawValue fails on an input value that decodes to a block but is
// not the bytes the writer's encoding gives.
func TestRepackRawValue(t *testing.T) {
	// Encode a block's data field twice: it decodes to the second, and its
	// hash matches, but the value re-encoded is shorter.
	rng := rand.New(rand.NewPCG(13, 14))
	blk := newBlocks(t, rng, 1, 100)[0]
	junk, err := block.EncodeBlockObject([]byte("junk"), nil, true)
	if err != nil {
		t.Fatal(err)
	}
	real, err := block.EncodeBlockObject(blk.data, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	value := append(junk, real...)
	if _, stored, err := packfile.DecodeBlockValue(packfile.BlockKey(blk.ref.GetHash()), value); err != nil || !bytes.Equal(stored.Data, blk.data) {
		t.Fatalf("the doubled value does not decode to the block: %v", err)
	}

	// Write it as a pack and its entry.
	var buf bytes.Buffer
	kw := kvfile.NewWriter(&buf)
	if err := kw.WriteValue(packfile.BlockKey(blk.ref.GetHash()), bytes.NewReader(value)); err != nil {
		t.Fatal(err)
	}
	if err := kw.Close(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "in")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := identity.NewPackID()
	raw, err := (&packfile.PackfileEntry{
		Id: id, BlockCount: 1, SizeBytes: uint64(buf.Len()), BloomFormatVersion: packfile.BloomFormatVersionV1,
		CreatedAt: timestamppb.New(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)),
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+entrySuffix), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	// The run fails and says why.
	_, err = run(testOpts([]string{dir}, filepath.Join(t.TempDir(), "out"), 1<<20))
	if err == nil || !strings.Contains(err.Error(), "re-encoded") {
		t.Fatalf("a non-canonical value was taken: %v", err)
	}
}

// TestRepackOutInsideIn refuses an output inside an input, through a
// symlink too, and an input inside the output.
func TestRepackOutInsideIn(t *testing.T) {
	// One input pack, and a symlink to its directory.
	rng := rand.New(rand.NewPCG(15, 16))
	root := t.TempDir()
	in := filepath.Join(root, "in")
	writeInput(t, in, newBlocks(t, rng, 2, 100))
	link := filepath.Join(root, "link")
	if err := os.Symlink(in, link); err != nil {
		t.Fatal(err)
	}

	// Each nesting is refused before anything is written.
	for _, c := range []struct{ in, out string }{
		{in, in},
		{in, filepath.Join(in, "out")},
		{in, filepath.Join(link, "out")},
		{link, filepath.Join(in, "sub", "out")},
		{filepath.Join(root, "out", "in"), filepath.Join(root, "out")},
	} {
		if _, err := run(testOpts([]string{c.in}, c.out, 1<<20)); err == nil || !strings.Contains(err.Error(), "inside") {
			t.Fatalf("in %s out %s: %v", c.in, c.out, err)
		}
	}
	if _, err := os.Stat(filepath.Join(in, "out")); err == nil {
		t.Fatal("an output was made inside the input")
	}
}

// frameBlock is a block for a frame file, with its refs and their flag.
type frameBlock struct {
	testBlock
	refs  []*block.BlockRef
	known bool
}

// appendFrame appends b as hyperline's world block-export frames it.
func appendFrame(t *testing.T, buf []byte, b frameBlock) []byte {
	t.Helper()
	object, err := block.EncodeBlockObject(b.data, b.refs, b.known)
	if err != nil {
		t.Fatal(err)
	}
	key := packfile.BlockKey(b.ref.GetHash())
	var known uint64
	if b.known {
		known = 1
	}
	buf = binary.AppendUvarint(buf, uint64(len(key)))
	buf = append(buf, key...)
	buf = binary.AppendUvarint(buf, known)
	buf = binary.AppendUvarint(buf, uint64(len(object)))
	return append(buf, object...)
}

// writeFrames writes blks as one frame file at path.
func writeFrames(t *testing.T, path string, blks []frameBlock) {
	t.Helper()
	var buf []byte
	for _, b := range blks {
		buf = appendFrame(t, buf, b)
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
}

// frameOpts returns options re-packing the frame files into out.
func frameOpts(in, frames []string, out string) *options {
	o := testOpts(in, out, 1<<20)
	o.Frames = frames
	at := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	o.CreatedAt = &at
	return o
}

// TestRepackFrames re-packs a frame file of refs-known and refs-unknown
// blocks, proves it, and keeps each block's refs and their flag.
func TestRepackFrames(t *testing.T) {
	// Write five blocks: three refs known, one with refs, two unknown.
	rng := rand.New(rand.NewPCG(21, 22))
	root := t.TempDir()
	blks := newBlocks(t, rng, 5, 400)
	frames := []frameBlock{
		{testBlock: blks[0], known: true},
		{testBlock: blks[1], known: true, refs: []*block.BlockRef{blks[0].ref}},
		{testBlock: blks[2], known: false},
		{testBlock: blks[3], known: true},
		{testBlock: blks[4], known: false},
	}
	path := filepath.Join(root, "blocks.frames")
	writeFrames(t, path, frames)

	// Re-pack and prove it.
	var log bytes.Buffer
	o := frameOpts(nil, []string{path}, filepath.Join(root, "out"))
	o.Log = &log
	rep, err := run(o)
	if err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	if rep.FrameFiles != 1 || rep.BlocksIn != 5 || rep.BlocksOut != 5 || rep.Duplicates != 0 || rep.PacksOut != 1 || !strings.Contains(log.String(), "proof ok") {
		t.Fatalf("report %+v\n%s", rep, log.String())
	}

	// The output pack holds each block with its refs and flag.
	want := make(map[string]frameBlock)
	for _, f := range frames {
		want[string(packfile.BlockKey(f.ref.GetHash()))] = f
	}
	data, err := os.ReadFile(filepath.Join(o.Out, rep.Packs[0].id))
	if err != nil {
		t.Fatal(err)
	}
	rdr, err := kvfile.BuildReader(bytes.NewReader(data), uint64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	err = rdr.ScanPrefix(nil, func(key, value []byte) error {
		_, stored, err := packfile.DecodeBlockValue(key, value)
		if err != nil {
			return err
		}
		f := want[string(key)]
		if stored.RefsKnown != f.known || len(stored.Refs) != len(f.refs) {
			t.Errorf("block %x: refs known %v refs %d, want %v %d", key, stored.RefsKnown, len(stored.Refs), f.known, len(f.refs))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestRepackFramesBadHash fails on a frame whose bytes do not match its key.
func TestRepackFramesBadHash(t *testing.T) {
	// The second frame names the first block's hash for its own bytes.
	rng := rand.New(rand.NewPCG(23, 24))
	root := t.TempDir()
	blks := newBlocks(t, rng, 2, 300)
	bad := frameBlock{testBlock: testBlock{ref: blks[0].ref, data: blks[1].data}, known: true}
	path := filepath.Join(root, "blocks.frames")
	writeFrames(t, path, []frameBlock{{testBlock: blks[0], known: true}, bad})

	// The run fails, naming the frame.
	_, err := run(frameOpts(nil, []string{path}, filepath.Join(root, "out")))
	if err == nil || !strings.Contains(err.Error(), "frame at offset") {
		t.Fatalf("bad hash taken: %v", err)
	}
}

// TestRepackFramesTruncated fails on a frame cut short, on a bad refs flag,
// and on a flag that disagrees with the object, naming the frame's offset.
func TestRepackFramesTruncated(t *testing.T) {
	// Write two frames, then cut the second.
	rng := rand.New(rand.NewPCG(25, 26))
	root := t.TempDir()
	blks := newBlocks(t, rng, 2, 300)
	first := appendFrame(t, nil, frameBlock{testBlock: blks[0], known: true})
	whole := appendFrame(t, bytes.Clone(first), frameBlock{testBlock: blks[1], known: false})
	offset := "frame at offset " + strconv.Itoa(len(first))
	cases := map[string][]byte{
		"cut object": whole[:len(whole)-5],
		"cut header": whole[:len(first)+3],
		"garbage":    append(bytes.Clone(first), 0xff),
	}

	// A refs flag of 2, and a flag of 1 for a refs-unknown object.
	flag := len(first) + 1 + maxKeyLen
	two := bytes.Clone(whole)
	two[flag] = 2
	cases["flag 2"] = two
	lie := bytes.Clone(whole)
	lie[flag] = 1
	cases["flag disagrees"] = lie

	// Each fails, naming the second frame.
	for name, data := range cases {
		path := filepath.Join(root, strings.ReplaceAll(name, " ", "-"))
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := run(frameOpts(nil, []string{path}, filepath.Join(root, "out-"+filepath.Base(path))))
		if err == nil || !strings.Contains(err.Error(), offset) {
			t.Errorf("%s: %v", name, err)
		}
		t.Logf("%s: %v", name, err)
	}
}

// TestRepackFramesDuplicate drops a frame's block an input pack also holds,
// in a run of a pack directory and a frame file.
func TestRepackFramesDuplicate(t *testing.T) {
	// A pack of three blocks, and frames of one of them and a new one.
	rng := rand.New(rand.NewPCG(27, 28))
	root := t.TempDir()
	in := filepath.Join(root, "in")
	blks := newBlocks(t, rng, 4, 500)
	writeInput(t, in, blks[:3])
	path := filepath.Join(root, "blocks.frames")
	writeFrames(t, path, []frameBlock{{testBlock: blks[1], known: true}, {testBlock: blks[3], known: false}})

	// The output holds the four distinct blocks.
	rep, err := run(frameOpts([]string{in}, []string{path}, filepath.Join(root, "out")))
	if err != nil {
		t.Fatal(err)
	}
	if rep.PacksIn != 1 || rep.FrameFiles != 1 || rep.BlocksIn != 5 || rep.Duplicates != 1 || rep.BlocksOut != 4 || rep.DataOut != 4*500 {
		t.Fatalf("report %+v", rep)
	}
}
