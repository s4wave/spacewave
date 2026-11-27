//go:build !js && !wasip1

package world_recovery

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	"github.com/s4wave/spacewave/db/packfile/writer"
)

// packDirBatch bounds the directory entries one read returns while the next
// pack name is chosen.
const packDirBatch = 256

// readOnlyFile is a captured pack opened O_RDONLY. It exposes only ReadAt and
// Close, so no write, truncate, rename or remove of a source pack is reachable
// through it, and the kernel refuses a write on the descriptor regardless.
type readOnlyFile struct {
	// f is the read-only descriptor.
	f *os.File
}

// openReadOnly opens path for reading only, refusing anything but a regular
// file.
func openReadOnly(path string) (*readOnlyFile, os.FileInfo, error) {
	// Open without write access and refuse links' targets other than files.
	f, err := os.OpenFile(path, os.O_RDONLY, 0) // #nosec G304 -- a declared capture's entry.
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, errors.Join(errors.New("captured pack is not a regular file"), err, f.Close())
	}
	return &readOnlyFile{f: f}, info, nil
}

// ReadAt reads len(p) bytes at off.
func (r *readOnlyFile) ReadAt(p []byte, off int64) (int, error) {
	return r.f.ReadAt(p, off)
}

// Close releases the descriptor.
func (r *readOnlyFile) Close() error {
	return r.f.Close()
}

// nextPackName returns the smallest pack file name in dir after after, or ""
// when none is left. It streams the directory in bounded batches, so memory
// does not grow with the number of packs. Each call costs one pass over the
// directory's entries. Entries that are not regular files, such as links, are
// reported through skipped and never followed.
func nextPackName(dir string, tier PackTier, after string, skipped func()) (string, error) {
	// Keep the least name past the cursor across bounded directory batches.
	f, err := os.Open(dir) // #nosec G304 -- a declared capture directory.
	if err != nil {
		return "", err
	}
	defer f.Close()
	var next string
	for {
		entries, err := f.ReadDir(packDirBatch)
		for _, entry := range entries {
			name := entry.Name()
			if name <= after || (next != "" && name >= next) {
				continue
			}
			if !entry.Type().IsRegular() {
				skipped()
				continue
			}
			if open, finalized := packahead.ClassifyName(name); tier == PackTier_PACK_TIER_LOCAL && !open && !finalized {
				continue
			}
			next = name
		}
		if errors.Is(err, io.EOF) || (err == nil && len(entries) == 0) {
			return next, nil
		}
		if err != nil {
			return "", err
		}
	}
}

// inventoryPack reads the next pack file of the current pack directory and
// commits its verified values, counts and the pack cursor in one scratch
// write. A pack is the unit of resume: an interrupted pack is read again.
func (s *Scanner) inventoryPack(ctx context.Context) error {
	// Choose the next pack in name order, or finish the directory.
	dirIndex := s.result.GetPackDir()
	dir := s.source.GetPacks()[dirIndex]
	if s.opts.BeforeRead != nil {
		if err := s.opts.BeforeRead(); err != nil {
			return err
		}
	}
	var skipped bool
	name, err := nextPackName(dir.GetPath(), dir.GetTier(), s.result.GetPackAfter(), func() { skipped = true })
	if err != nil {
		return err
	}
	if name == "" {
		return s.db.Update(func(tx *scratchTx) error {
			if skipped {
				s.addGap("pack directory entries that are not regular files are not read")
			}
			s.result.PackDir++
			s.result.PackAfter = ""
			return s.save(tx)
		})
	}

	// Read the pack inside the scratch write, so its values, counts and
	// cursor commit together or not at all.
	path := filepath.Join(dir.GetPath(), name)
	location := append(binary.BigEndian.AppendUint32(nil, uint32(len(s.source.GetTables()))+dirIndex), name...) //nolint:gosec // bounded declarations.
	location = append(location, 0)
	return s.db.Update(func(tx *scratchTx) error {
		// Open and read the pack, reporting any skipped entries.
		if skipped {
			s.addGap("pack directory entries that are not regular files are not read")
		}
		f, before, err := openReadOnly(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := s.readPack(ctx, tx, f, before.Size(), name, dir.GetTier(), location); err != nil {
			return err
		}

		// Refuse a pack that changed while it was read.
		after, err := os.Stat(path)
		if err != nil {
			return err
		}
		if after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || !os.SameFile(before, after) {
			return errors.New("captured pack changed while it was read")
		}
		s.result.Counts.PackFiles++
		s.result.PackAfter = name
		return s.save(tx)
	})
}

// readPack records every verified value of one pack. A finalized or remote
// pack reads through its index. An open log, or a local pack whose index does
// not read, reads through its valid batch records, and the bytes past the
// last valid record are counted as retained and unread, never truncated. A
// pack neither reader accepts is counted unreadable, keeping any value it
// already gave.
func (s *Scanner) readPack(ctx context.Context, tx *scratchTx, f *readOnlyFile, size int64, name string, tier PackTier, location []byte) error {
	// Record a value under the location of its pack.
	var fnErr error
	values := 0
	record := func(key, value []byte) error {
		// Count a corrupt value and go on. Record a verified value.
		values++
		ref, stored, err := packfile.DecodeBlockValue(key, value)
		if err != nil {
			s.result.Counts.CorruptValues++
			return nil
		}
		canonical, err := ref.MarshalKey()
		if err == nil {
			err = s.recordBlock(tx, canonical, stored.GetData(), append(location[:len(location):len(location)], canonical...))
		}
		fnErr = err
		return err
	}

	// Read a finalized pack through its index.
	open, _ := packahead.ClassifyName(name)
	if !open {
		err := packfile.ScanPack(ctx, f, uint64(size), uint64(writer.DefaultMaxPackBytes), record) //nolint:gosec // non-negative.
		if fnErr != nil || ctx.Err() != nil {
			return errors.Join(fnErr, ctx.Err())
		}
		if err == nil {
			return nil
		}
		if tier != PackTier_PACK_TIER_LOCAL || values != 0 {
			s.result.Counts.UnreadablePacks++
			s.addGap("some pack files have no readable index; their unread values are not counted")
			return nil
		}
	}

	// Read the valid batch records of an open log, or of a local pack whose
	// index failed, and retain the rest.
	end, err := packahead.ScanRecords(ctx, f, size, writer.DefaultMaxPackBytes, record)
	if fnErr != nil || ctx.Err() != nil {
		return errors.Join(fnErr, ctx.Err())
	}
	if err != nil {
		return err
	}
	if !open && values == 0 {
		s.result.Counts.UnreadablePacks++
		s.addGap("some pack files have no readable index; their unread values are not counted")
		return nil
	}
	if end < size {
		s.result.Counts.UnreadTailBytes += uint64(size - end) //nolint:gosec // end <= size.
		s.addGap("open pack logs hold bytes past their last valid record; they are retained, not truncated, and not interpreted")
	}
	return nil
}

// packGaps lists the value tiers the declared packs leave unread.
func packGaps(packs []*PackSource) []string {
	// List each undeclared tier, then the tiers no reader covers.
	declared := map[PackTier]bool{}
	for _, p := range packs {
		declared[p.GetTier()] = true
	}
	var gaps []string
	if !declared[PackTier_PACK_TIER_LOCAL] {
		gaps = append(gaps, "no local pack log was declared; packahead values are not covered")
	}
	if !declared[PackTier_PACK_TIER_REMOTE] {
		gaps = append(gaps, "no retained remote packs were declared; remote pack values are not covered")
	}
	return append(gaps,
		"remote catalogs are not read; packs outside the declared directories, including uncataloged ones, are not covered",
		"s4db stores are not read: no read-only s4db capture reader exists",
		"paylog BlockDomain values (browser Volumes) are not read: its index opener replays the whole index into memory and starts new segments, so no bounded read-only reader exists",
	)
}

// packDirIdentity binds a declared directory's identity and membership
// metadata: an added, removed or renamed pack changes its mtime and ctime.
func packDirIdentity(packs []*PackSource) ([]byte, error) {
	var identity []byte
	for i, p := range packs {
		id, err := statIdentity(p.GetPath(), true)
		if err != nil {
			return nil, err
		}
		identity = append(identity, strconv.Itoa(i)...)
		identity = append(identity, 0)
		identity = append(identity, id...)
	}
	return identity, nil
}
