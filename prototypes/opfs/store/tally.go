//go:build js && wasm

package store

import (
	"strconv"
)

const tallyFileName = "__storage_tally__"

// loadTally reads the storage tally from the metadata file.
// Falls back to recomputing from file sizes if the tally file is missing.
func (s *Store) loadTally() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	fh, err := s.root.GetFileHandle(tallyFileName, false)
	if err != nil {
		// Tally file missing, recompute.
		tally, err := s.recomputeTally()
		if err != nil {
			return err
		}
		s.tally = tally
		return nil
	}

	data, err := fh.ReadFile()
	if err != nil || len(data) == 0 {
		tally, err := s.recomputeTally()
		if err != nil {
			return err
		}
		s.tally = tally
		return nil
	}

	val, err := strconv.ParseUint(string(data), 10, 64)
	if err != nil {
		tally, err := s.recomputeTally()
		if err != nil {
			return err
		}
		s.tally = tally
		return nil
	}

	s.tally = val
	return nil
}

// saveTally writes the current tally to the metadata file.
func (s *Store) saveTally() error {
	s.mu.Lock()
	val := s.tally
	s.mu.Unlock()

	fh, err := s.root.GetFileHandle(tallyFileName, true)
	if err != nil {
		return err
	}

	ops, err := fh.OpenFileOps()
	if err != nil {
		return err
	}

	data := []byte(strconv.FormatUint(val, 10))
	if err := ops.Truncate(0); err != nil {
		_ = ops.Close()
		return err
	}
	_, err = ops.Write(data)
	if err != nil {
		_ = ops.Close()
		return err
	}
	if err := ops.Flush(); err != nil {
		_ = ops.Close()
		return err
	}
	return ops.Close()
}

// recomputeTally walks all shard directories and sums file sizes.
func (s *Store) recomputeTally() (uint64, error) {
	shards, err := s.data.Entries()
	if err != nil {
		return 0, err
	}
	var total uint64
	for _, shard := range shards {
		if shard.Kind != "directory" {
			continue
		}
		dir := shard.AsDirectoryHandle()
		files, err := dir.Entries()
		if err != nil {
			return 0, err
		}
		for _, f := range files {
			if f.Kind != "file" {
				continue
			}
			fh := f.AsFileHandle()
			data, err := fh.ReadFile()
			if err != nil {
				continue
			}
			total += uint64(len(data))
		}
	}
	return total, nil
}

// initTally loads or recomputes the storage tally on store open.
func initTally(s *Store) error {
	return s.loadTally()
}
