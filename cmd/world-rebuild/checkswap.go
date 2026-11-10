package main

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// fileStamp is what a source file looked like when copy opened it: its
// header's file change counter (offset 24), its size and mtime, and its WAL's
// size (0 when there is none: a reader may create an empty one).
type fileStamp struct {
	Path    string    `json:"path"`
	Counter uint32    `json:"counter"`
	Size    int64     `json:"size"`
	MTime   time.Time `json:"mtime"`
	WALSize int64     `json:"walSize"`
}

// sourceStamps are the stamps of a copy's source World and pack index.
type sourceStamps struct {
	World fileStamp `json:"world"`
	Packs fileStamp `json:"packs"`
}

// stampPath is where copy writes the stamps of the copy at out.
func stampPath(out string) string { return out + ".source-stamp" }

// stampFile reads a SQLite file's stamp.
func stampFile(path string) (fileStamp, error) {
	// Read the size and mtime, then the header's change counter.
	st := fileStamp{Path: path}
	fi, err := os.Stat(path)
	if err != nil {
		return st, err
	}
	st.Size, st.MTime = fi.Size(), fi.ModTime().UTC()
	f, err := os.Open(path)
	if err != nil {
		return st, err
	}
	defer f.Close()
	var hdr [28]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return st, fmt.Errorf("%s: read header: %w", path, err)
	}
	st.Counter = binary.BigEndian.Uint32(hdr[24:28])

	// Read the WAL's size, if it has one.
	if wi, err := os.Stat(path + "-wal"); err == nil {
		st.WALSize = wi.Size()
	}
	return st, nil
}

// stampSources stamps the copy's sources and writes the stamps beside out.
func stampSources(o opts) error {
	// Stamp both files.
	var s sourceStamps
	var err error
	if s.World, err = stampFile(o.world); err != nil {
		return err
	}
	if s.Packs, err = stampFile(o.packs); err != nil {
		return err
	}

	// Write them beside the new file.
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stampPath(o.out), append(b, '\n'), 0o644)
}

// checkStamps compares the copy's sources with the stamps copy wrote, and
// refuses if any moved: a change counter, size, mtime or WAL size.
func checkStamps(o opts) error {
	// Read the stamps copy wrote.
	b, err := os.ReadFile(stampPath(o.out))
	if err != nil {
		return fmt.Errorf("source stamps of the copy: %w", err)
	}
	var was sourceStamps
	if err := json.Unmarshal(b, &was); err != nil {
		return fmt.Errorf("%s: %w", stampPath(o.out), err)
	}

	// Stamp the sources now and compare.
	var moved []string
	for _, w := range []fileStamp{was.World, was.Packs} {
		now, err := stampFile(w.Path)
		if err != nil {
			return err
		}
		if now.Counter != w.Counter {
			moved = append(moved, fmt.Sprintf("%s change counter %d, was %d", w.Path, now.Counter, w.Counter))
		}
		if now.Size != w.Size {
			moved = append(moved, fmt.Sprintf("%s size %d, was %d", w.Path, now.Size, w.Size))
		}
		if !now.MTime.Equal(w.MTime) {
			moved = append(moved, fmt.Sprintf("%s mtime %s, was %s", w.Path, now.MTime.Format(time.RFC3339Nano), w.MTime.Format(time.RFC3339Nano)))
		}
		if now.WALSize != w.WALSize {
			moved = append(moved, fmt.Sprintf("%s WAL size %d, was %d", w.Path, now.WALSize, w.WALSize))
		}
	}
	if len(moved) != 0 {
		return fmt.Errorf("the copy's sources moved since the copy opened them, do not swap: %v", moved)
	}
	return nil
}

// runCheckSwap is the last check before the swap: the source World and pack
// index are as copy found them.
func runCheckSwap(o opts) error {
	if o.out == "" {
		return fmt.Errorf("checkswap needs -out, the new World file copy wrote")
	}
	if err := checkStamps(o); err != nil {
		return err
	}
	fmt.Println("checkswap: ok, the sources are as the copy found them")
	return nil
}

// schemaRows lists sqlite_master as "type name" lines.
func schemaRows(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SELECT type, name FROM sqlite_master ORDER BY rowid")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var ty, name string
		if err := rows.Scan(&ty, &name); err != nil {
			return nil, err
		}
		out = append(out, ty+" "+name)
	}
	return out, rows.Err()
}

// onlyKVTable refuses a source with a table other than its kv table, or an
// index other than the kv table's autoindex: copy carries the kv table alone.
func onlyKVTable(db *sql.DB, table string) error {
	rows, err := schemaRows(db)
	if err != nil {
		return fmt.Errorf("read sqlite_master: %w", err)
	}
	var extra []string
	for _, r := range rows {
		if r != "table "+table && r != "index sqlite_autoindex_"+table+"_1" {
			extra = append(extra, r)
		}
	}
	if len(extra) != 0 {
		return fmt.Errorf("source World has schema objects copy does not carry: %v; refusing", extra)
	}
	return nil
}
