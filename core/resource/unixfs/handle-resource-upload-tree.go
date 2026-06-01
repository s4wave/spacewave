package resource_unixfs

import (
	"context"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
)

type uploadTreeFile struct {
	strm      s4wave_unixfs.SRPCFSHandleResourceService_UploadTreeStream
	state     *uploadTreeState
	name      string
	remaining int64
	buf       []byte
}

type uploadTreeState struct {
	b    *unixfs_world.BatchFSWriter
	dirs map[string]struct{}
	resp s4wave_unixfs.HandleUploadTreeResponse
}

// UploadTree uploads a directory tree relative to this handle in one batch.
func (r *FSHandleResource) UploadTree(
	strm s4wave_unixfs.SRPCFSHandleResourceService_UploadTreeStream,
) (_ *s4wave_unixfs.HandleUploadTreeResponse, rerr error) {
	ctx := strm.Context()
	if r.ws == nil || r.objKey == "" {
		return nil, errors.New("batch tree upload unavailable for detached handle resource")
	}

	nt, err := r.handle.GetNodeType(ctx)
	if err != nil {
		return nil, err
	}
	if !nt.GetIsDirectory() {
		return nil, errors.New("tree upload requires a directory handle")
	}

	state := &uploadTreeState{
		b:    unixfs_world.NewBatchFSWriter(r.ws, r.objKey, r.fsType, ""),
		dirs: make(map[string]struct{}),
	}
	committed := false
	defer func() {
		if !committed {
			recordUploadMetric(ctx, UploadMetric{Stage: "abort-cleanup"})
		}
	}()
	defer state.b.Release()

	for {
		msg, err := strm.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if err := r.handleUploadTreeMessage(ctx, strm, state, msg); err != nil {
			return nil, err
		}
	}
	recordUploadMetric(ctx, UploadMetric{Stage: "commit-start"})
	if err := state.b.Commit(ctx); err != nil {
		return nil, err
	}
	recordUploadMetric(ctx, UploadMetric{Stage: "commit-complete"})
	recordUploadMetric(ctx, UploadMetric{Stage: "reload-start"})
	if err := r.reloadHandle(ctx); err != nil {
		return nil, err
	}
	recordUploadMetric(ctx, UploadMetric{Stage: "reload-complete"})

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) { broadcast() })
	recordUploadMetric(ctx, UploadMetric{Stage: "broadcast"})
	committed = true
	return &state.resp, nil
}

// handleUploadTreeMessage handles one UploadTree stream message.
func (r *FSHandleResource) handleUploadTreeMessage(
	ctx context.Context,
	strm s4wave_unixfs.SRPCFSHandleResourceService_UploadTreeStream,
	state *uploadTreeState,
	msg *s4wave_unixfs.HandleUploadTreeRequest,
) error {
	if dir := msg.GetDirectory(); dir != nil {
		recordUploadMetric(ctx, UploadMetric{Stage: "receive-directory"})
		parts, err := parseUploadTreePath(dir.GetPath())
		if err != nil {
			return err
		}
		if len(parts) == 0 {
			return errors.New("directory path is required")
		}
		if err := r.ensureUploadTreeParents(ctx, state, parts[:len(parts)-1]); err != nil {
			return err
		}
		if err := r.addUploadTreeDir(
			ctx,
			state,
			append(append([]string(nil), r.path...), parts...),
			fs.FileMode(dir.GetMode()),
		); err != nil {
			return err
		}
		state.resp.DirectoriesWritten++
		return nil
	}

	if fileStart := msg.GetFileStart(); fileStart != nil {
		recordUploadMetric(ctx, UploadMetric{Stage: "receive-file-start"})
		parts, err := parseUploadTreePath(fileStart.GetPath())
		if err != nil {
			return err
		}
		if len(parts) == 0 {
			return errors.New("file path is required")
		}
		if fileStart.GetTotalSize() < 0 {
			return errors.New("file total_size must be non-negative")
		}

		parentPath := append(append([]string(nil), r.path...), parts[:len(parts)-1]...)
		if err := r.ensureUploadTreeParents(ctx, state, parts[:len(parts)-1]); err != nil {
			return err
		}
		rdr := &uploadTreeFile{
			strm:      strm,
			state:     state,
			name:      parts[len(parts)-1],
			remaining: fileStart.GetTotalSize(),
		}
		if err := state.b.AddFile(
			ctx,
			parentPath,
			parts[len(parts)-1],
			unixfs.NewFSCursorNodeType_File(),
			fileStart.GetTotalSize(),
			rdr,
			fs.FileMode(fileStart.GetMode()),
			time.Now(),
		); err != nil {
			return err
		}
		if rdr.remaining != 0 {
			return errors.Errorf(
				"tree upload file %q wrote %d bytes, expected %d",
				rdr.name,
				fileStart.GetTotalSize()-rdr.remaining,
				fileStart.GetTotalSize(),
			)
		}
		state.resp.FilesWritten++
		return nil
	}

	return errors.New("tree upload data received before file_start")
}

// ensureUploadTreeParents creates any missing parent directories for relParts.
func (r *FSHandleResource) ensureUploadTreeParents(
	ctx context.Context,
	state *uploadTreeState,
	relParts []string,
) error {
	if len(relParts) == 0 {
		return nil
	}
	fullPath := append(append([]string(nil), r.path...), relParts...)
	for i := range fullPath {
		if len(fullPath[:i+1]) <= len(r.path) {
			continue
		}
		if err := r.addUploadTreeDir(ctx, state, fullPath[:i+1], 0o755); err != nil {
			return err
		}
	}
	return nil
}

// addUploadTreeDir records one directory if it has not already been added.
func (r *FSHandleResource) addUploadTreeDir(
	ctx context.Context,
	state *uploadTreeState,
	fullPath []string,
	mode fs.FileMode,
) error {
	key := strings.Join(fullPath, "\x00")
	if _, ok := state.dirs[key]; ok {
		return nil
	}
	state.dirs[key] = struct{}{}
	return state.b.AddDir(
		ctx,
		fullPath[:len(fullPath)-1],
		fullPath[len(fullPath)-1],
		mode,
		time.Now(),
	)
}

// parseUploadTreePath validates a slash-separated relative upload path.
func parseUploadTreePath(path string) ([]string, error) {
	if path == "" {
		return nil, errors.New("empty upload path")
	}
	if strings.HasPrefix(path, "/") {
		return nil, errors.New("upload path must be relative")
	}
	parts := strings.Split(path, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return nil, errors.New("upload path cannot contain ..")
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil, errors.New("empty upload path")
	}
	return out, nil
}

func (f *uploadTreeFile) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(f.buf) != 0 {
		n := copy(p, f.buf)
		f.buf = f.buf[n:]
		return n, nil
	}
	if f.remaining == 0 {
		return 0, io.EOF
	}
	msg, err := f.strm.Recv()
	if err == io.EOF {
		return 0, errors.Errorf(
			"tree upload file %q ended with %d bytes remaining",
			f.name,
			f.remaining,
		)
	}
	if err != nil {
		return 0, err
	}
	data := msg.GetData()
	if len(data) == 0 {
		return 0, errors.Errorf("tree upload file %q expected data before declared size", f.name)
	}
	if err := validateUploadDataFrame(data); err != nil {
		return 0, err
	}
	recordUploadMetric(f.strm.Context(), UploadMetric{
		Stage: "receive-data",
		Bytes: len(data),
	})
	if int64(len(data)) > f.remaining {
		return 0, errors.Errorf("tree upload data exceeds declared size for %q", f.name)
	}
	f.state.resp.BytesWritten += int64(len(data))
	f.remaining -= int64(len(data))
	n := copy(p, data)
	if n < len(data) {
		f.buf = data[n:]
	}
	return n, nil
}
