package bldr_manifest

import (
	"context"
	stderrors "errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	pkgerrors "github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
)

const nativeViewerArtifactMaxBytes uint64 = 512 << 20
const nativeViewerArtifactMode os.FileMode = 0o700

// NativeViewerArtifact is one materialized native viewer executable.
type NativeViewerArtifact struct {
	// Path is the absolute private executable path.
	Path string

	cleanupOnce sync.Once
	cleanupErr  error
}

// Close removes the private executable and is safe to call more than once.
func (a *NativeViewerArtifact) Close() error {
	if a == nil {
		return nil
	}
	a.cleanupOnce.Do(func() {
		a.cleanupErr = os.Remove(a.Path)
		if os.IsNotExist(a.cleanupErr) {
			a.cleanupErr = nil
		}
	})
	return a.cleanupErr
}

type nativeViewerArtifactFile interface {
	GetNodeType(context.Context) (unixfs.FSCursorNodeType, error)
	GetSize(context.Context) (uint64, error)
	ReadAt(context.Context, int64, []byte) (int64, error)
	Release()
}

type nativeViewerArtifactLookup func(context.Context, string) (nativeViewerArtifactFile, error)

// MaterializeNativeViewerArtifact looks up the validated viewer entrypoint in
// the selected distribution and writes its bytes to a new private executable.
// The destination directory must already exist and is never replaced with a
// fallback path.
func MaterializeNativeViewerArtifact(
	ctx context.Context,
	resolution *NativeViewerResolution,
	dist *unixfs.FSHandle,
	destinationDir string,
) (*NativeViewerArtifact, error) {
	if dist == nil {
		return nil, pkgerrors.New("selected distribution is required")
	}
	return materializeNativeViewerArtifactWithLookup(
		ctx,
		resolution,
		destinationDir,
		func(ctx context.Context, entrypoint string) (nativeViewerArtifactFile, error) {
			handle, _, err := dist.LookupPath(ctx, entrypoint)
			if handle == nil {
				return nil, err
			}
			return handle, err
		},
	)
}

func materializeNativeViewerArtifactWithLookup(
	ctx context.Context,
	resolution *NativeViewerResolution,
	destinationDir string,
	lookup nativeViewerArtifactLookup,
) (*NativeViewerArtifact, error) {
	if ctx == nil {
		return nil, pkgerrors.New("context is required")
	}
	if resolution == nil {
		return nil, pkgerrors.New("native viewer resolution is required")
	}
	if lookup == nil {
		return nil, pkgerrors.New("native viewer lookup is required")
	}
	if err := validateNativeEntrypoint(resolution.Entrypoint); err != nil {
		return nil, pkgerrors.Wrap(err, "entrypoint")
	}

	destinationDir, err := filepath.Abs(destinationDir)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "destination directory")
	}
	destinationInfo, err := os.Lstat(destinationDir)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "destination directory")
	}
	if destinationInfo.Mode()&os.ModeSymlink != 0 || !destinationInfo.IsDir() {
		return nil, pkgerrors.New("destination must be an existing directory")
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entrypoint, err := lookup(ctx, resolution.Entrypoint)
	if entrypoint != nil {
		defer entrypoint.Release()
	}
	if err != nil {
		return nil, pkgerrors.Wrap(err, "entrypoint lookup")
	}
	if entrypoint == nil {
		return nil, pkgerrors.New("entrypoint lookup returned no handle")
	}

	nodeType, err := entrypoint.GetNodeType(ctx)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "entrypoint type")
	}
	if nodeType == nil || !nodeType.GetIsFile() || nodeType.GetIsDirectory() || nodeType.GetIsSymlink() {
		return nil, pkgerrors.New("entrypoint must be a regular file")
	}
	size, err := entrypoint.GetSize(ctx)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "entrypoint size")
	}
	if size == 0 {
		return nil, pkgerrors.New("entrypoint is empty")
	}
	if size > nativeViewerArtifactMaxBytes {
		return nil, pkgerrors.Errorf("entrypoint exceeds %d-byte limit", nativeViewerArtifactMaxBytes)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tmp, err := os.CreateTemp(destinationDir, ".native-viewer-tmp-*")
	if err != nil {
		return nil, pkgerrors.Wrap(err, "create native viewer artifact")
	}
	tmpPath := tmp.Name()
	removePartial := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if err := writeNativeViewerArtifact(ctx, tmp, entrypoint, size); err != nil {
		removePartial()
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		removePartial()
		return nil, pkgerrors.Wrap(err, "sync native viewer artifact")
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, pkgerrors.Wrap(err, "close native viewer artifact")
	}
	if err := os.Chmod(tmpPath, nativeViewerArtifactMode); err != nil {
		_ = os.Remove(tmpPath)
		return nil, pkgerrors.Wrap(err, "make native viewer artifact executable")
	}

	finalPath := tmpPath + ".ready"
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, pkgerrors.Wrap(err, "publish native viewer artifact")
	}
	finalPath, err = filepath.Abs(finalPath)
	if err != nil {
		_ = os.Remove(finalPath)
		return nil, pkgerrors.Wrap(err, "native viewer artifact path")
	}
	finalInfo, err := os.Lstat(finalPath)
	if err != nil {
		_ = os.Remove(finalPath)
		return nil, pkgerrors.Wrap(err, "stat native viewer artifact")
	}
	if finalInfo.Mode()&os.ModeSymlink != 0 || !finalInfo.Mode().IsRegular() || finalInfo.Mode().Perm() != nativeViewerArtifactMode {
		_ = os.Remove(finalPath)
		return nil, pkgerrors.New("native viewer artifact is not a private regular executable")
	}

	return &NativeViewerArtifact{Path: finalPath}, nil
}

func writeNativeViewerArtifact(
	ctx context.Context,
	dst io.Writer,
	src nativeViewerArtifactFile,
	size uint64,
) error {
	const chunkSize = 1 << 20
	var offset uint64
	for offset < size {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunkLen := min(uint64(chunkSize), size-offset)
		chunk := make([]byte, chunkLen)
		n, err := src.ReadAt(ctx, int64(offset), chunk) // #nosec G115 -- size is bounded above.
		if n < 0 || n > int64(len(chunk)) {
			return pkgerrors.Errorf("entrypoint read returned invalid byte count %d", n)
		}
		if n != int64(len(chunk)) {
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			return pkgerrors.Wrap(err, "read native viewer artifact")
		}
		if err != nil && !stderrors.Is(err, io.EOF) {
			return pkgerrors.Wrap(err, "read native viewer artifact")
		}
		written, writeErr := dst.Write(chunk)
		if written < 0 || written > len(chunk) {
			return pkgerrors.Errorf("native viewer artifact write returned invalid byte count %d", written)
		}
		if written != len(chunk) {
			if writeErr == nil {
				writeErr = io.ErrShortWrite
			}
			return pkgerrors.Wrap(writeErr, "write native viewer artifact")
		}
		if writeErr != nil {
			return pkgerrors.Wrap(writeErr, "write native viewer artifact")
		}
		offset += uint64(len(chunk))
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
