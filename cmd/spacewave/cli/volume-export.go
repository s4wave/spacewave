//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
	"github.com/s4wave/spacewave/db/world/refwalk"
)

// volumeExportTempPrefix reserves temporary filenames for incomplete exports.
const volumeExportTempPrefix = ".volume-export-"

// volumeExportResult counts the streamed refs and their export outcomes.
type volumeExportResult struct {
	// refs is the number of parsed refs.
	refs uint64
	// exported is the number of verified blocks written.
	exported uint64
	// bytes is the total size of the blocks written.
	bytes uint64
	// already is the number of verified blocks left in the output directory.
	already uint64
	// missing is the number of blocks the source lacks.
	missing uint64
	// mismatch is the number of source blocks that fail their hash.
	mismatch uint64
	// followed is the number of children placed or verified in the output.
	followed uint64
	// receiverHolds is the number of child hits where the receiver stops the walk.
	receiverHolds uint64
	// neitherHolds is the number of child hits absent from both volumes.
	neitherHolds uint64
}

// volumeBlockPresence checks the receiver without loading its block bytes.
type volumeBlockPresence interface {
	// HasBlock reports whether the receiver physically holds the ref.
	HasBlock(ctx context.Context, ref *block.BlockRef) (bool, error)
}

// volumeExport streams verified blocks into a private directory.
type volumeExport struct {
	// source reads the stopped source's raw blocks.
	source packcopy.Source
	// receiver stops the child walk at present blocks; nil disables following.
	receiver volumeBlockPresence
	// seen contains only refs verified in the output during this run.
	seen map[string]struct{}
	// out is the private output directory.
	out string
	// lacks receives missing named refs and corrupt source refs in traversal order.
	lacks io.Writer
	// result accumulates the input and output counts.
	result volumeExportResult
}

// run streams requested refs and optionally follows receiver-absent descendants.
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

	// Track only exported refs when following is enabled.
	if r.receiver != nil {
		r.seen = make(map[string]struct{})
	}

	// Keep only one block's bytes while consuming the requested refs.
	return streamVolumeBlockRefs(ctx, input, 1, func(refs []*block.BlockRef) error {
		return r.exportBlock(ctx, refs[0])
	})
}

// exportBlock preserves a verified existing file or exports verified source bytes.
func (r *volumeExport) exportBlock(ctx context.Context, ref *block.BlockRef) error {
	// Reuse only files whose contents hash to their filename's ref.
	r.result.refs++
	present, err := r.reuseBlock(ctx, ref)
	if err != nil {
		return err
	}
	if present {
		r.result.already++
		return r.followChildren(ctx, ref)
	}

	// Verify source bytes here so a corrupt source never reaches the write path.
	stored, err := r.readSourceBlock(ctx, ref, false)
	if err != nil || stored == nil {
		return err
	}

	// Publish one completed file before reading another block.
	if err := r.writeBlock(ref, stored.Data); err != nil {
		return err
	}
	r.result.exported++
	r.result.bytes += uint64(len(stored.Data))
	return r.followChildren(ctx, ref)
}

// readSourceBlock returns verified source bytes and accounts for unusable blocks.
// A missing child may be a hash that is not a ref, so it stays out of the report.
func (r *volumeExport) readSourceBlock(ctx context.Context, ref *block.BlockRef, child bool) (*block.StoredBlock, error) {
	// Read the raw source before allowing its bytes to reach writeBlock.
	stored, err := r.source.GetStoredBlock(ctx, ref)
	if err != nil {
		return nil, errors.Wrapf(err, "read source block %s", ref.MarshalString())
	}
	if stored == nil && child {
		r.result.neitherHolds++
		return nil, nil
	}

	// Record corrupt children and every unusable explicitly requested block.
	why := ""
	switch {
	case stored == nil:
		r.result.missing++
		why = volumeBlockMissingReason
	case ref.VerifyData(stored.Data, false) != nil:
		r.result.mismatch++
		why = volumeBlockMismatchReason
	}
	if why != "" {
		_, err := io.WriteString(r.lacks, ref.MarshalString()+"\t"+why+"\n")
		return nil, errors.Wrap(err, "write lacks output")
	}
	return stored, nil
}

// followChildren walks exported blocks with a ref stack instead of retaining their bytes.
// Only verified output refs enter seen; receiver stops and absent hits are not retained.
func (r *volumeExport) followChildren(ctx context.Context, ref *block.BlockRef) error {
	// Start each verified output block at most once in this run.
	if r.receiver == nil {
		return nil
	}
	if _, ok := r.seen[ref.MarshalString()]; ok {
		return nil
	}
	r.seen[ref.MarshalString()] = struct{}{}
	stack := []*block.BlockRef{ref}

	// Re-read output bytes when scanning instead of retaining pending blocks' bytes.
	for len(stack) != 0 {
		// Release the scanned block's bytes before reading any child from the source.
		if err := ctx.Err(); err != nil {
			return err
		}
		last := len(stack) - 1
		hits, err := r.scanExportedBlock(ctx, stack[last])
		stack = stack[:last]
		if err != nil {
			return err
		}

		// Inspect hits in wire order and stop wherever the receiver already holds one.
		for _, hit := range hits {
			// Keep the per-run set bounded by verified output refs.
			child := hit.Ref
			key := child.MarshalString()
			if _, ok := r.seen[key]; ok {
				continue
			}
			present, err := r.receiver.HasBlock(ctx, child)
			if err != nil {
				return errors.Wrapf(err, "check receiver block %s", key)
			}
			if present {
				r.result.receiverHolds++
				continue
			}

			// Require verified source bytes even if a previous run left the child on disk.
			stored, err := r.readSourceBlock(ctx, child, true)
			if err != nil {
				return err
			}
			if stored == nil {
				continue
			}
			present, err = r.reuseBlock(ctx, child)
			if err != nil {
				return err
			}
			if present {
				r.result.already++
			} else {
				if err := r.writeBlock(child, stored.Data); err != nil {
					return err
				}
				r.result.exported++
				r.result.bytes += uint64(len(stored.Data))
			}

			// Mark only completed output and queue its ref for the next scan.
			r.result.followed++
			r.seen[key] = struct{}{}
			stack = append(stack, child)
		}
	}
	return nil
}

// scanExportedBlock releases its bytes after extracting the child refs.
func (r *volumeExport) scanExportedBlock(ctx context.Context, ref *block.BlockRef) ([]refwalk.ScanHit, error) {
	// Read a published file through the carrier's regular-file checks.
	stored, err := dirBlocks(r.out).GetStoredBlock(ctx, ref)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, errors.Errorf("exported block %s disappeared before scanning", ref.MarshalString())
	}
	hits, _ := refwalk.ScanHits(stored.Data)
	return hits, nil
}

// reuseBlock keeps a verified existing file and removes corrupt output bytes.
func (r *volumeExport) reuseBlock(ctx context.Context, ref *block.BlockRef) (bool, error) {
	// Read through the carrier's regular-file checks and retain no block bytes.
	stored, err := dirBlocks(r.out).GetStoredBlock(ctx, ref)
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

// volumeExportFields formats the source and export counts in a field table.
func volumeExportFields(path string, r volumeExportResult) [][2]string {
	return [][2]string{
		{"Volume", path},
		{"Refs read", strconv.FormatUint(r.refs, 10)},
		{"Blocks exported", strconv.FormatUint(r.exported, 10)},
		{"Bytes exported", strconv.FormatUint(r.bytes, 10)},
		{"Already in directory", strconv.FormatUint(r.already, 10)},
		{"Source lacks", strconv.FormatUint(r.missing, 10)},
		{"Hash mismatch", strconv.FormatUint(r.mismatch, 10)},
		{"Children followed", strconv.FormatUint(r.followed, 10)},
		{"Children the receiver holds", strconv.FormatUint(r.receiverHolds, 10)},
		{"Children neither holds", strconv.FormatUint(r.neitherHolds, 10)},
	}
}
