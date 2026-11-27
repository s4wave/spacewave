//go:build !js

// Package restore exports and refills verified blocks for offline volume recovery.
package restore

import (
	"context"
	stderrors "errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
)

// volumeExportTempPrefix reserves temporary filenames for incomplete exports.
const volumeExportTempPrefix = ".volume-export-"

// ExportResult counts the streamed refs and their export outcomes.
type ExportResult struct {
	// Refs is the number of parsed refs.
	Refs uint64
	// Exported is the number of verified blocks written.
	Exported uint64
	// Bytes is the total size of the blocks written.
	Bytes uint64
	// Already is the number of verified blocks left in the output directory.
	Already uint64
	// Missing is the number of blocks the source lacks.
	Missing uint64
	// Mismatch is the number of source blocks that fail their hash.
	Mismatch uint64
}

// Export streams verified source blocks into out and reports missing or corrupt refs to lacks.
// out must be a private directory. The result includes completed work if an error occurs.
func Export(ctx context.Context, source packcopy.Source, input io.Reader, out string, lacks io.Writer) (ExportResult, error) {
	r := volumeExport{source: source, out: out, lacks: lacks}
	err := r.run(ctx, input)
	return r.result, err
}

// volumeExport streams verified blocks into a private directory.
type volumeExport struct {
	// source reads the stopped source's raw blocks.
	source packcopy.Source
	// out is the private output directory.
	out string
	// lacks receives missing or corrupt refs in input order.
	lacks io.Writer
	// result accumulates the input and output counts.
	result ExportResult
}

// run exports each parsed ref while retaining at most one block's bytes.
func (r *volumeExport) run(ctx context.Context, input io.Reader) (err error) {
	// Make every successful publication and removal durable before the run returns.
	out, err := os.Open(r.out)
	if err != nil {
		return errors.Wrap(err, "open export directory")
	}
	defer func() {
		// Windows cannot sync a directory and orders the rename itself.
		var syncErr error
		if runtime.GOOS != "windows" {
			syncErr = errors.Wrap(out.Sync(), "sync export directory")
		}
		err = stderrors.Join(err, syncErr, errors.Wrap(out.Close(), "close export directory"))
	}()

	// Remove only this export's leftover temporary names, in bounded directory groups.
	for {
		files, readErr := out.ReadDir(64)
		for _, file := range files {
			if strings.HasPrefix(file.Name(), volumeExportTempPrefix) {
				if err := os.Remove(filepath.Join(r.out, file.Name())); err != nil {
					return errors.Wrap(err, "remove leftover temporary block")
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return errors.Wrap(readErr, "read export directory")
		}
	}

	// Keep only one block's bytes while consuming the requested refs.
	return StreamRefs(ctx, input, 1, func(refs []*block.BlockRef) error {
		return r.exportBlock(ctx, refs[0])
	})
}

// exportBlock preserves a verified existing file or exports verified source bytes.
func (r *volumeExport) exportBlock(ctx context.Context, ref *block.BlockRef) error {
	// Reuse only files whose contents hash to their filename's ref.
	r.result.Refs++
	present, err := r.reuseBlock(ctx, ref)
	if err != nil {
		return err
	}
	if present {
		r.result.Already++
		return nil
	}

	// Verify source bytes here so a corrupt source never reaches the write path.
	stored, err := r.source.GetStoredBlock(ctx, ref)
	if err != nil {
		return errors.Wrapf(err, "read source block %s", ref.MarshalString())
	}
	why := ""
	switch {
	case stored == nil:
		r.result.Missing++
		why = MissingReason
	case ref.VerifyData(stored.Data, false) != nil:
		r.result.Mismatch++
		why = MismatchReason
	}
	if why != "" {
		_, err := io.WriteString(r.lacks, ref.MarshalString()+"\t"+why+"\n")
		return errors.Wrap(err, "write lacks output")
	}

	// Publish one completed file before reading another block.
	if err := r.writeBlock(ref, stored.Data); err != nil {
		return err
	}
	r.result.Exported++
	r.result.Bytes += uint64(len(stored.Data))
	return nil
}

// reuseBlock keeps a verified existing file and removes corrupt output bytes.
func (r *volumeExport) reuseBlock(ctx context.Context, ref *block.BlockRef) (bool, error) {
	// Read through the carrier's regular-file checks and retain no block bytes.
	stored, err := DirectorySource(r.out).GetStoredBlock(ctx, ref)
	if err != nil {
		return false, errors.Wrapf(err, "read exported block %s", ref.MarshalString())
	}
	if stored == nil {
		return false, nil
	}
	if ref.VerifyData(stored.Data, false) == nil {
		return true, nil
	}

	// Remove proven corrupt bytes even when the source cannot replace them.
	// #nosec G703 -- the name is the operator-named directory joined with a B58 ref, which holds no separator.
	if err := os.Remove(filepath.Join(r.out, ref.MarshalString())); err != nil {
		return false, errors.Wrap(err, "remove corrupt exported block")
	}
	return false, nil
}

// writeBlock publishes verified bytes with private permissions and an atomic rename.
func (r *volumeExport) writeBlock(ref *block.BlockRef, data []byte) (err error) {
	// Only verified source bytes reach this path. A temporary file has mode 0600,
	// and its bytes are synced before the file closes and its ref is published.
	// After a crash a ref's name holds either the whole verified block or nothing.
	// Replacing an invalid file uses rename, so a hard link never rewrites an input.
	out, err := os.CreateTemp(r.out, volumeExportTempPrefix+"*")
	if err != nil {
		return errors.Wrap(err, "create temporary block")
	}
	defer func() {
		if err != nil {
			err = stderrors.Join(err, errors.Wrap(os.Remove(out.Name()), "remove temporary block"))
		}
	}()

	// Sync the bytes before closing, and retain any write, sync and close failures.
	_, writeErr := out.Write(data)
	err = stderrors.Join(errors.Wrap(writeErr, "write temporary block"), errors.Wrap(out.Sync(), "sync temporary block"),
		errors.Wrap(out.Close(), "close temporary block"))
	if err != nil {
		return err
	}

	// Publish the block under its ref.
	// #nosec G703 -- both names are in the operator-named directory, and a B58 ref holds no separator.
	return errors.Wrap(os.Rename(out.Name(), filepath.Join(r.out, ref.MarshalString())), "publish exported block")
}
