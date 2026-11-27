//go:build !js

package spacewave_cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/net/hash"
)

// TestVolumeExportFollowAbsent follows a lost chain and stops at receiver-held siblings.
func TestVolumeExportFollowAbsent(t *testing.T) {
	// Build actual protobuf blocks with a three-level chain and a receiver-held branch.
	source := make(volumeExportBlocks)
	makeBlock := func(children ...*block.BlockRef) *block.BlockRef {
		// Encode child refs in a chunked blob message.
		t.Helper()
		msg := &blob.Blob{BlobType: blob.BlobType_BlobType_CHUNKED, TotalSize: uint64(len(source) + 1), ChunkIndex: &blob.ChunkIndex{}}
		for _, child := range children {
			msg.ChunkIndex.Chunks = append(msg.ChunkIndex.Chunks, &blob.Chunk{DataRef: child, Size: 1})
		}
		data, err := msg.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}

		// Store the encoded bytes under their content hash.
		h, err := hash.Sum(hash.HashType_HashType_BLAKE3, data)
		if err != nil {
			t.Fatal(err)
		}
		ref := block.NewBlockRef(h)
		source[ref.MarshalString()] = data
		return ref
	}

	// Connect the lost chain and a held sibling whose own child must stay unvisited.
	c := makeBlock()
	d := makeBlock(makeBlock())
	b := makeBlock(c)
	absent := makeBlock()
	delete(source, absent.MarshalString())
	a := makeBlock(b, d, absent)
	receiver := volumeExportBlocks{d.MarshalString(): source[d.MarshalString()]}

	// Compare fresh, resumed, explicitly repeated and disabled walks with the same graph.
	for _, mode := range []string{"fresh", "resumed", "named child", "disabled", "corrupt child"} {
		t.Run(mode, func(t *testing.T) {
			// Give each run its own output while retaining the source's original block bytes.
			out := t.TempDir()
			var lacks bytes.Buffer
			r := volumeExport{source: source, receiver: receiver, out: out, lacks: &lacks}
			input := a.MarshalString()
			want := volumeExportResult{refs: 1, exported: 3, followed: 2, receiverHolds: 1, neitherHolds: 1,
				bytes: uint64(len(source[a.MarshalString()]) + len(source[b.MarshalString()]) + len(source[c.MarshalString()]))}
			wantLacks := ""
			written := []*block.BlockRef{a, b, c}
			switch mode {
			case "resumed":
				// Existing roots and children still need their descendants scanned.
				for _, ref := range []*block.BlockRef{a, b} {
					if err := os.WriteFile(filepath.Join(out, ref.MarshalString()), source[ref.MarshalString()], 0o600); err != nil {
						t.Fatal(err)
					}
					want.bytes -= uint64(len(source[ref.MarshalString()]))
				}
				want.exported, want.already = 1, 2
			case "named child":
				input += "\n" + b.MarshalString()
				want.refs, want.already = 2, 1
			case "disabled":
				r.receiver = nil
				written = []*block.BlockRef{a}
				want = volumeExportResult{refs: 1, exported: 1, bytes: uint64(len(source[a.MarshalString()]))}
			case "corrupt child":
				// Corrupt bytes under a real child ref must never reach the directory.
				corrupt := make(volumeExportBlocks, len(source))
				for key, data := range source {
					corrupt[key] = data
				}
				corrupt[c.MarshalString()] = []byte("wrong source bytes")
				r.source = corrupt
				written = []*block.BlockRef{a, b}
				want.exported, want.followed, want.mismatch = 2, 1, 1
				want.bytes -= uint64(len(source[c.MarshalString()]))
				wantLacks = c.MarshalString() + "\t" + volumeBlockMismatchReason + "\n"
			}

			// Run the production exporter and compare every outcome counter.
			if err := r.run(t.Context(), strings.NewReader(input)); err != nil {
				t.Fatal(err)
			}
			if r.result != want {
				t.Fatalf("counts: %+v, want %+v", r.result, want)
			}
			if lacks.String() != wantLacks {
				t.Fatalf("lacks report: %q, want %q", lacks.String(), wantLacks)
			}

			// Inspect the whole directory so a receiver-held, corrupt or absent file fails.
			files, err := os.ReadDir(out)
			if err != nil || len(files) != len(written) {
				t.Fatalf("output files: %d, want %d: %v", len(files), len(written), err)
			}
			for _, ref := range written {
				data, err := os.ReadFile(filepath.Join(out, ref.MarshalString()))
				if err != nil || !bytes.Equal(data, source[ref.MarshalString()]) {
					t.Fatalf("output block %s differs from source: %v", ref.MarshalString(), err)
				}
			}
		})
	}
}

// TestVolumeExportReceiverInputs checks flag wiring and protects the receiver from reports.
func TestVolumeExportReceiverInputs(t *testing.T) {
	// Keep the flag and its example visible in the command's generated help.
	help, err := runVolumeBlockCLI(t, "volume-export", "--help")
	if err != nil || !strings.Contains(help, "--follow-absent-from") || !strings.Contains(help, "receiver.s4db") {
		t.Fatalf("export help omitted receiver following: %v\n%s", err, help)
	}

	// Prepare stopped files through the same raw-store fixture as the refill tests.
	dir := t.TempDir()
	refs := make([]*block.BlockRef, 6)
	data := make([][]byte, len(refs))
	for i := range refs {
		data[i] = []byte{byte(i + 1)}
		h, err := hash.Sum(hash.HashType_HashType_BLAKE3, data[i])
		if err != nil {
			t.Fatal(err)
		}
		refs[i] = block.NewBlockRef(h)
	}

	// Persist separate source and receiver fixtures and the requested root.
	source, receiver := filepath.Join(dir, "source.s4db"), filepath.Join(dir, "receiver.s4db")
	makeVolumeRefillSource(t, source, refs, data)
	makeVolumeRefillSource(t, receiver, refs, data)
	input := filepath.Join(dir, "refs.tsv")
	if err := os.WriteFile(input, []byte(refs[0].MarshalString()), 0o600); err != nil {
		t.Fatal(err)
	}

	// Follow through the real CLI using read-only snapshots of both inputs.
	out := filepath.Join(dir, "out")
	report, err := runVolumeBlockCLI(t, "volume-export", "--refs", input, "--out", out, "--follow-absent-from", receiver, source)
	if err != nil || !strings.Contains(report, "Children followed:") {
		t.Fatalf("receiver export: %v\n%s", err, report)
	}

	// Refuse a missing receiver before creating an output directory.
	unopened := filepath.Join(dir, "unopened")
	_, err = runVolumeBlockCLI(t, "volume-export", "--refs", input, "--out", unopened, "--follow-absent-from", filepath.Join(dir, "absent.s4db"), source)
	if err == nil {
		t.Fatal("accepted a missing receiver")
	}
	if _, err := os.Stat(unopened); !os.IsNotExist(err) {
		t.Fatalf("created output before opening receiver: %v", err)
	}

	// A hard link to the receiver must be rejected before the report truncates it.
	alias := filepath.Join(dir, "receiver-link")
	if err := os.Link(receiver, alias); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(receiver)
	if err != nil {
		t.Fatal(err)
	}
	_, err = runVolumeBlockCLI(t, "volume-export", "--refs", input, "--out", out, "--follow-absent-from", receiver, "--lacks-out", alias, source)
	if err == nil || !strings.Contains(err.Error(), "lacks output names an input file") {
		t.Fatalf("receiver report alias: %v", err)
	}
	after, err := os.Stat(receiver)
	if err != nil || after.Size() != before.Size() {
		t.Fatalf("receiver was truncated: %v", err)
	}
}
