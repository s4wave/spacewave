//go:build !js

package spacewave_cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/aperturerobotics/cli"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/s4db"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// TestDebugVolumeRefill restores verified bytes while keeping the target's child list intact.
func TestDebugVolumeRefill(t *testing.T) {
	// Give the target a parked block with bytes and four listed blocks without bytes.
	ctx := t.Context()
	dir := t.TempDir()
	target := filepath.Join(dir, "target.s4db")
	from := filepath.Join(dir, "source.s4db")
	input := filepath.Join(dir, "refs.tsv")
	lacks := filepath.Join(dir, "lacks.tsv")
	data := [][]byte{
		[]byte("restore me"), []byte("check my hash"), []byte("missing from source"),
		[]byte("unlisted"), []byte("already stored"), bytes.Repeat([]byte{1}, volume_kvtx.MaxFillBatchBytes+1),
	}
	refs := make([]*block.BlockRef, len(data))
	for i, dat := range data {
		ref, err := block.BuildBlockRef(dat, nil)
		if err != nil {
			t.Fatal(err)
		}
		refs[i] = ref
	}
	list := makeVolumeRefillTarget(t, target, refs, data)

	// Give the older raw source good bytes for an ordinary and an oversized
	// parked fill, a corrupt copy, and good bytes for an unlisted ref.
	makeVolumeRefillSource(t, from, refs, data)
	fromSum := sumVolumeRefillFile(t, from)
	targetSum := sumVolumeRefillFile(t, target)
	lines := []string{"", refs[0].MarshalString() + "\tfront entry\textra", refs[3].MarshalString(),
		" \t ", refs[1].MarshalString(), refs[2].MarshalString(), refs[4].MarshalString(), refs[5].MarshalString()}
	if err := os.WriteFile(input, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	// Exercise the registered help and preview, then compare the target file byte for byte.
	help, err := runVolumeBlockCLI(t, "volume-refill", "--help")
	if err != nil || !strings.Contains(help, "--upgrade-format") || !strings.Contains(help, "--lacks-out") {
		t.Fatalf("refill help: %v\n%s", err, help)
	}
	args := []string{"--from", from, "--refs", input, "--lacks-out", lacks, target}
	dry, err := runVolumeBlockCLI(t, "volume-refill", append([]string{"--dry-run"}, args...)...)
	if err != nil {
		t.Fatal(err)
	}
	checkVolumeRefillReport(t, dry, []string{"Refs read", "Would skip (already stored)", "Would lack from source",
		"Would reject (hash mismatch)", "Would restore", "Would restore under a hold, parked",
		"Would drop", "Bytes that would be restored"}, len(data[0])+len(data[5]), 1, 1)
	if got := sumVolumeRefillFile(t, target); got != targetSum {
		t.Fatal("dry run changed the target file")
	}

	// Report a malformed ref by its physical line number, including skipped blanks.
	badRefs := filepath.Join(dir, "bad-refs.tsv")
	if err := os.WriteFile(badRefs, []byte("\nnot-a-ref\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = runVolumeBlockCLI(t, "volume-refill", "--dry-run", "--from", from, "--refs", badRefs, target)
	if err == nil || !strings.Contains(err.Error(), "refs line 2 has an invalid B58 block ref") {
		t.Fatalf("parse malformed refs file: %v", err)
	}

	// Reuse the dry run's output path and check the real run's counts.
	if err := os.Chmod(lacks, 0o644); err != nil {
		t.Fatal(err)
	}
	real, err := runVolumeBlockCLI(t, "volume-refill", args...)
	if err != nil {
		t.Fatal(err)
	}
	checkVolumeRefillReport(t, real, []string{"Refs read", "Already stored", "Source lacks", "Hash mismatch",
		"Restored", "Restored under a hold, parked", "Dropped", "Bytes restored"}, len(data[0])+len(data[5]), 1, 1)
	if got := sumVolumeRefillFile(t, from); got != fromSum {
		t.Fatal("refill changed the source file")
	}

	// Read the exception file as a caller would, in input order and with private permissions.
	out, err := os.ReadFile(lacks)
	if err != nil {
		t.Fatal(err)
	}
	want := refs[3].MarshalString() + "\trecords do not accept a copied fill\n" +
		refs[1].MarshalString() + "\thash mismatch\n" +
		refs[2].MarshalString() + "\tsource lacks block\n"
	if string(out) != want {
		t.Fatalf("lacks output %q, want %q", out, want)
	}
	info, err := os.Stat(lacks)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("lacks mode %o, want 600", mode)
	}

	// Export next to the stopped source, then refill a fresh target from its carrier.
	t.Run("export and directory refill", func(t *testing.T) {
		// Check help through the same registered command tree as a real invocation.
		help, err := runVolumeBlockCLI(t, "volume-export", "--help")
		if err != nil || !strings.Contains(help, "--refs") || !strings.Contains(help, "--out") ||
			!strings.Contains(help, "--lacks-out") || !strings.Contains(help, "volume-refill --from") {
			t.Fatalf("export help: %v\n%s", err, help)
		}

		// Export only verified source blocks, including the unlisted and oversized blocks.
		blocksDir := filepath.Join(dir, "exported-blocks")
		exportLacks := filepath.Join(dir, "export-lacks.tsv")
		exportArgs := []string{"--refs", input, "--out", blocksDir, "--lacks-out", exportLacks, from}
		report, err := runVolumeBlockCLI(t, "volume-export", exportArgs...)
		if err != nil {
			t.Fatal(err)
		}
		checkVolumeExportReport(t, report, from, 3, len(data[0])+len(data[3])+len(data[5]), 0)
		if got := sumVolumeRefillFile(t, from); got != fromSum {
			t.Fatal("export changed the source file")
		}

		// Inspect the actual directory so an extra, corrupt or temporary file fails the check.
		info, err := os.Stat(blocksDir)
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("export directory mode: %v, %v", info, err)
		}
		files, err := os.ReadDir(blocksDir)
		if err != nil || len(files) != 3 {
			t.Fatalf("exported files: %v, %v", files, err)
		}
		for _, i := range []int{0, 3, 5} {
			// Require the correct bytes and private permissions under each requested ref.
			path := filepath.Join(blocksDir, refs[i].MarshalString())
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, data[i]) {
				t.Fatalf("exported block %d: size=%d, %v", i, len(got), err)
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("exported block %d mode: %v, %v", i, info, err)
			}
		}

		// Preserve the parser's order, tab notes and final line without a newline.
		got, err := os.ReadFile(exportLacks)
		want := refs[1].MarshalString() + "\thash mismatch\n" +
			refs[2].MarshalString() + "\tsource lacks block\n" +
			refs[4].MarshalString() + "\tsource lacks block\n"
		if err != nil || string(got) != want {
			t.Fatalf("export lacks %q, want %q: %v", got, want, err)
		}
		info, err = os.Stat(exportLacks)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("export lacks mode: %v, %v", info, err)
		}

		// A corrupt source was omitted, so a clean carrier reports that ref as missing.
		freshTarget := filepath.Join(dir, "directory-target.s4db")
		makeVolumeRefillTarget(t, freshTarget, refs, data)
		directoryReport, err := runVolumeBlockCLI(t, "volume-refill", "--from", blocksDir, "--refs", input, freshTarget)
		if err != nil {
			t.Fatal(err)
		}
		checkVolumeRefillReport(t, directoryReport, []string{"Refs read", "Already stored", "Source lacks", "Hash mismatch",
			"Restored", "Restored under a hold, parked", "Dropped", "Bytes restored"}, len(data[0])+len(data[5]), 2, 0)
		checkVolumeRefillTarget(t, freshTarget, refs, data, list)

		// Replace an invalid existing file, but leave a verified existing inode alone.
		goodPath := filepath.Join(blocksDir, refs[3].MarshalString())
		before, err := os.Stat(goodPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(blocksDir, refs[0].MarshalString()), []byte("bad existing export"), 0o600); err != nil {
			t.Fatal(err)
		}
		// A resumed export must remove its abandoned temporary and unreplaceable corrupt files.
		leftover := filepath.Join(blocksDir, ".volume-export-leftover")
		if err := os.WriteFile(leftover, []byte("incomplete export"), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, i := range []int{1, 2} {
			if err := os.WriteFile(filepath.Join(blocksDir, refs[i].MarshalString()), []byte("bad output without a good source"), 0o600); err != nil {
				t.Fatal(err)
			}
		}

		report, err = runVolumeBlockCLI(t, "volume-export", exportArgs...)
		if err != nil {
			t.Fatal(err)
		}
		checkVolumeExportReport(t, report, from, 1, len(data[0]), 2)
		after, err := os.Stat(goodPath)
		if err != nil || !os.SameFile(before, after) {
			t.Fatalf("export rewrote a verified file: %v", err)
		}
		got, err = os.ReadFile(filepath.Join(blocksDir, refs[0].MarshalString()))
		if err != nil || !bytes.Equal(got, data[0]) {
			t.Fatalf("export failed to replace corrupt output: %q, %v", got, err)
		}

		// Missing and corrupt sources retain their report reasons while bad output files disappear.
		for _, path := range []string{leftover, filepath.Join(blocksDir, refs[1].MarshalString()), filepath.Join(blocksDir, refs[2].MarshalString())} {
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				t.Fatalf("export retained invalid output %s: %v", path, err)
			}
		}
		got, err = os.ReadFile(exportLacks)
		if err != nil || string(got) != want {
			t.Fatalf("resumed export lacks %q, want %q: %v", got, want, err)
		}

		// A planted corrupt block must be rejected, and the good unlisted block must be dropped.
		if err := os.WriteFile(filepath.Join(blocksDir, refs[1].MarshalString()), []byte("planted wrong bytes"), 0o600); err != nil {
			t.Fatal(err)
		}
		plantedTarget := filepath.Join(dir, "planted-target.s4db")
		makeVolumeRefillTarget(t, plantedTarget, refs, data)
		directoryLacks := filepath.Join(dir, "directory-lacks.tsv")
		directoryReport, err = runVolumeBlockCLI(t, "volume-refill", "--from", blocksDir, "--refs", input, "--lacks-out", directoryLacks, plantedTarget)
		if err != nil || directoryReport != real {
			t.Fatalf("planted directory refill report: %v\n%s\nwant:\n%s", err, directoryReport, real)
		}
		got, err = os.ReadFile(directoryLacks)
		if err != nil || !bytes.Equal(got, out) {
			t.Fatalf("directory lacks %q, want %q: %v", got, out, err)
		}
		checkVolumeRefillTarget(t, plantedTarget, refs, data, list)
	})

	// A symlink's correct target bytes cannot substitute for bytes in the carrier.
	t.Run("directory symlinks", func(t *testing.T) {
		// Put the verified bytes outside the carrier and only a link under the ref's name.
		blocksDir := filepath.Join(t.TempDir(), "blocks")
		if err := os.Mkdir(blocksDir, 0o700); err != nil {
			t.Fatal(err)
		}
		linkedFile := filepath.Join(t.TempDir(), "good-block")
		if err := os.WriteFile(linkedFile, data[0], 0o600); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(blocksDir, refs[0].MarshalString())
		if err := os.Symlink(linkedFile, link); err != nil {
			t.Fatal(err)
		}

		// A refill must refuse the link before the target can receive its bytes.
		freshTarget := filepath.Join(dir, "symlink-target.s4db")
		makeVolumeRefillTarget(t, freshTarget, refs, data)
		_, err := runVolumeBlockCLI(t, "volume-refill", "--from", blocksDir, "--refs", input, freshTarget)
		if err == nil || !strings.Contains(err.Error(), link) {
			t.Fatalf("refill accepted a symlink or omitted its path: %v", err)
		}
		vol, err := openStoppedVolume(t.Context(), logrus.NewEntry(logrus.New()), freshTarget, volumeReadOnly)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := vol.Close(); err != nil {
				t.Error(err)
			}
		})
		found, err := vol.Store.GetBlockExists(t.Context(), refs[0])
		if err != nil || found {
			t.Fatalf("symlink refill stored bytes: found=%t, %v", found, err)
		}

		// Export must reject the same link without changing the file it points at.
		before := sumVolumeRefillFile(t, linkedFile)
		_, err = runVolumeBlockCLI(t, "volume-export", "--refs", input, "--out", blocksDir, from)
		if err == nil || !strings.Contains(err.Error(), link) {
			t.Fatalf("export accepted a symlink or omitted its path: %v", err)
		}
		if got := sumVolumeRefillFile(t, linkedFile); got != before {
			t.Fatal("export changed the symlink's target file")
		}
	})

	// A directory readable by anyone else must be refused before exporting blocks.
	t.Run("export refuses public directory", func(t *testing.T) {
		for _, mode := range []os.FileMode{0o750, 0o707} {
			// Chmod explicitly so the process umask cannot remove the permission under test.
			publicDir := t.TempDir()
			if err := os.Chmod(publicDir, mode); err != nil {
				t.Fatal(err)
			}
			_, err := runVolumeBlockCLI(t, "volume-export", "--refs", input, "--out", publicDir, from)
			if err == nil || !strings.Contains(err.Error(), "group or other permission bits") {
				t.Fatalf("export with directory mode %o: %v", mode, err)
			}
			files, err := os.ReadDir(publicDir)
			if err != nil || len(files) != 0 {
				t.Fatalf("refused directory changed: %v, %v", files, err)
			}
		}
	})

	// Export uses the same physical line numbers and protects both inputs from report truncation.
	t.Run("export input checks", func(t *testing.T) {
		// The export parser must reject the same malformed line as refill.
		_, err := runVolumeBlockCLI(t, "volume-export", "--refs", badRefs, "--out", filepath.Join(t.TempDir(), "out"), from)
		if err == nil || !strings.Contains(err.Error(), "refs line 2 has an invalid B58 block ref") {
			t.Fatalf("export malformed refs: %v", err)
		}

		// Bound even a notes column before parsing, including a final line without a newline.
		longRefs := filepath.Join(dir, "long-refs.tsv")
		for _, ending := range []string{"\n", ""} {
			if err := os.WriteFile(longRefs, []byte("\n"+refs[0].MarshalString()+"\t"+strings.Repeat("x", 64*1024+1)+ending), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, command := range []string{"volume-refill", "volume-export"} {
				args := []string{"--refs", longRefs, "--out", filepath.Join(t.TempDir(), "out"), from}
				if command == "volume-refill" {
					args = []string{"--refs", longRefs, "--from", from, target}
				}
				_, err := runVolumeBlockCLI(t, command, args...)
				if err == nil || !strings.Contains(err.Error(), "refs line 2 is longer than 65536 bytes") {
					t.Fatalf("%s accepted an overlong line or omitted its line number: %v", command, err)
				}
			}
		}

		// Check file identity through a hard link to either input.
		for _, path := range []string{from, input} {
			alias := filepath.Join(t.TempDir(), "input-link")
			if err := os.Link(path, alias); err != nil {
				t.Fatal(err)
			}
			before := sumVolumeRefillFile(t, path)
			_, err := runVolumeBlockCLI(t, "volume-export", "--refs", input, "--out", filepath.Join(t.TempDir(), "out"), "--lacks-out", alias, from)
			if err == nil || !strings.Contains(err.Error(), "lacks output names an input file") {
				t.Fatalf("export report aliases %s: %v", path, err)
			}
			if got := sumVolumeRefillFile(t, path); got != before {
				t.Fatalf("export truncated %s", path)
			}
		}
	})

	// Check physical presence, restored bytes and the original serialized child list.
	vol, err := openStoppedVolume(ctx, logrus.NewEntry(logrus.New()), target, volumeReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()
	found, err := vol.Store.GetBlockExistsBatch(ctx, refs)
	if err != nil || !slices.Equal(found, []bool{true, false, false, false, true, true}) {
		t.Fatalf("stored blocks %v: %v", found, err)
	}
	if visible, err := vol.GetBlockExists(ctx, refs[4]); err != nil || visible {
		t.Fatalf("the skipped block should stay parked: visible=%t, err=%v", visible, err)
	}
	got, foundBlock, err := vol.GetBlock(ctx, refs[0])
	if err != nil || !foundBlock || !bytes.Equal(got, data[0]) {
		t.Fatalf("restored block found=%t, size=%d: %v", foundBlock, len(got), err)
	}
	if got := readVolumeRefillList(t, vol, refs[0]); !bytes.Equal(got, list) {
		t.Fatal("refill changed the original child list")
	}

	// Read back the oversized parked fill and the sole root its hold created.
	got, foundBlock, err = vol.GetBlock(ctx, refs[5])
	if err != nil || !foundBlock || !bytes.Equal(got, data[5]) {
		t.Fatalf("parked fill found=%t, size=%d: %v", foundBlock, len(got), err)
	}
	var roots []block.NamedRoot
	err = vol.ListRoots(ctx, "fill:volume-refill/", func(root block.NamedRoot) error {
		roots = append(roots, root)
		return nil
	})
	if err != nil || len(roots) != 1 || !roots[0].Ref.EqualVT(refs[5]) || roots[0].Lease == "" {
		t.Fatalf("parked fill roots: %+v, %v", roots, err)
	}
}

// checkVolumeExportReport checks the export counts independently of field padding.
func checkVolumeExportReport(t *testing.T, report, path string, exported, size, already int) {
	// Compare every reported row with the fixture's independently counted outcomes.
	t.Helper()
	want := [][2]string{
		{"Volume", path}, {"Refs read", "6"}, {"Blocks exported", strconv.Itoa(exported)},
		{"Bytes exported", strconv.Itoa(size)}, {"Already in directory", strconv.Itoa(already)},
		{"Source lacks", "2"}, {"Hash mismatch", "1"},
	}
	lines := strings.Split(strings.TrimSpace(report), "\n")
	if len(lines) != len(want) {
		t.Fatalf("export report has %d rows, want %d:\n%s", len(lines), len(want), report)
	}
	for i, line := range lines {
		label, value, found := strings.Cut(line, ":")
		if !found || label != want[i][0] || strings.TrimSpace(value) != want[i][1] {
			t.Fatalf("export row %q, want %s = %s", line, want[i][0], want[i][1])
		}
	}
}

// checkVolumeRefillTarget verifies the carrier restored only accepted, verified bytes.
func checkVolumeRefillTarget(t *testing.T, path string, refs []*block.BlockRef, data [][]byte, list []byte) {
	// Inspect the physical store so rejected or dropped bytes cannot hide behind visibility.
	t.Helper()
	vol, err := openStoppedVolume(t.Context(), logrus.NewEntry(logrus.New()), path, volumeReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := vol.Close(); err != nil {
			t.Error(err)
		}
	})
	found, err := vol.Store.GetBlockExistsBatch(t.Context(), refs)
	if err != nil || !slices.Equal(found, []bool{true, false, false, false, true, true}) {
		t.Fatalf("directory target stored blocks %v: %v", found, err)
	}

	// Compare the accepted bytes and the recorded list with the original target fixture.
	for _, i := range []int{0, 5} {
		got, found, err := vol.GetBlock(t.Context(), refs[i])
		if err != nil || !found || !bytes.Equal(got, data[i]) {
			t.Fatalf("directory fill %d found=%t, size=%d: %v", i, found, len(got), err)
		}
	}
	if got := readVolumeRefillList(t, vol, refs[0]); !bytes.Equal(got, list) {
		t.Fatal("directory refill changed the original child list")
	}
}

// makeVolumeRefillTarget leaves child lists intact while preparing ordinary and parked fills.
func makeVolumeRefillTarget(t *testing.T, path string, refs []*block.BlockRef, data [][]byte) []byte {
	// Set up a stopped target using the physical store and recorded child lists.
	t.Helper()
	ctx := t.Context()
	vol, err := volume_s4db.NewVolume(ctx, logrus.NewEntry(logrus.New()), &volume_s4db.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()
	for _, i := range []int{0, 1, 2, 4} {
		if ref, _, err := vol.Store.PutBlock(ctx, data[i], nil); err != nil || !ref.EqualVT(refs[i]) {
			t.Fatalf("store target fixture block %d: %v", i, err)
		}
	}

	// Record a nonempty list for the block the command will restore.
	writeRecords(t, vol, func(rec *block_gc.Records) error {
		if err := rec.SetState(ctx, &block_gc.LivenessState{Generation: block_gc.Generation_GENERATION_OPEN}); err != nil {
			return err
		}
		if err := rec.PutChildren(ctx, refs[0], []*block.BlockRef{refs[2]}); err != nil {
			return err
		}
		for _, i := range []int{1, 2, 4, 5} {
			if err := rec.PutChildren(ctx, refs[i], nil); err != nil {
				return err
			}
		}
		for _, ref := range refs[4:] {
			if err := rec.SetHolders(ctx, ref, 0, 1); err != nil {
				return err
			}
		}
		return nil
	})

	// Park both leaves as an Open generation does. Only the first has bytes.
	for _, ref := range refs[4:] {
		key := block_gc.BlockKey(ref)
		parked := binary.BigEndian.AppendUint64([]byte{2, 'X'}, 1)
		if err := putKey(ctx, vol, append(parked, key...), nil); err != nil {
			t.Fatal(err)
		}
		if err := putKey(ctx, vol, append([]byte{2, 'I'}, key...), nil); err != nil {
			t.Fatal(err)
		}
	}

	// Lose only the bytes, just as a volume whose child lists outlive the blocks does.
	for _, ref := range refs[:3] {
		if err := vol.Store.RmBlock(ctx, ref); err != nil {
			t.Fatal(err)
		}
	}
	list := readVolumeRefillList(t, vol, refs[0])
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}
	return list
}

// makeVolumeRefillSource writes raw block keys in a file with an older volume format.
func makeVolumeRefillSource(t *testing.T, path string, refs []*block.BlockRef, data [][]byte) {
	// Keep only the source hits, including one deliberately corrupt copy.
	t.Helper()
	makeVolumeBlockSource(t, path, []*block.BlockRef{refs[0], refs[1], refs[3], refs[5]},
		[][]byte{data[0], []byte("corrupt copy"), data[3], data[5]})
}

// makeVolumeBlockSource writes raw block fixtures into a stopped s4db file.
func makeVolumeBlockSource(t *testing.T, path string, refs []*block.BlockRef, data [][]byte) {
	// Open only the s4db engine so the fixture can put bytes under any key.
	t.Helper()
	ctx := t.Context()
	db, err := s4db.Open(path, s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Write the selected bytes and format in one commit, then stop the source.
	keys := kvkey.NewDefaultKVKey()
	for i, ref := range refs {
		key, err := ref.MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, keys.GetBlockKey(key), data[i]); err != nil {
			t.Fatal(err)
		}
	}

	if err := tx.Set(ctx, keys.GetFormatVersionKey(), kvkey.MarshalFormatVersion(kvkey.FormatVersion-1)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// readVolumeRefillList returns the serialized child list from the target fixture.
func readVolumeRefillList(t *testing.T, vol *volume_s4db.Volume, ref *block.BlockRef) []byte {
	// Read the child list's physical record, including its encoding marker.
	t.Helper()
	ctx := t.Context()
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	value, found, err := tx.Get(ctx, append([]byte{2, 'c'}, block_gc.BlockKey(ref)...))
	if err != nil || !found {
		t.Fatalf("read child list: found=%t, err=%v", found, err)
	}
	return value
}

// sumVolumeRefillFile returns the fixture file's checksum.
func sumVolumeRefillFile(t *testing.T, path string) [sha256.Size]byte {
	// Include every byte the engine changes on open or close.
	t.Helper()
	dat, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(dat)
}

// runVolumeBlockCLI invokes an offline block command and captures its report.
func runVolumeBlockCLI(t *testing.T, command string, args ...string) (string, error) {
	// Use a fresh command tree for each flag parse.
	t.Helper()
	var out bytes.Buffer
	app := &cli.App{Writer: &out, Commands: []*cli.Command{newDebugCommand(nil, &FormatUpgrade{})}}
	err := app.RunContext(t.Context(), append([]string{"spacewave", "debug", command}, args...))
	return out.String(), err
}

// checkVolumeRefillReport checks every row and its count, independently of padding.
func checkVolumeRefillReport(t *testing.T, report string, labels []string, size, missing, mismatch int) {
	// Decode the sibling command report's label and value columns.
	t.Helper()
	counts := []string{"6", "1", strconv.Itoa(missing), strconv.Itoa(mismatch), "1", "1", "1", strconv.Itoa(size)}
	lines := strings.Split(strings.TrimSpace(report), "\n")
	if len(lines) != len(labels) {
		t.Fatalf("refill report has %d rows, want %d:\n%s", len(lines), len(labels), report)
	}
	for i, line := range lines {
		label, count, found := strings.Cut(line, ":")
		if !found || label != labels[i] || strings.TrimSpace(count) != counts[i] {
			t.Fatalf("refill row %q, want %s = %s", line, labels[i], counts[i])
		}
	}
}
