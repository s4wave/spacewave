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
	parentPath []string
	name       string
	pw         *io.PipeWriter
	done       chan error
}

type uploadTreeState struct {
	b       *unixfs_world.BatchFSWriter
	dirs    map[string]struct{}
	current *uploadTreeFile
	resp    s4wave_unixfs.HandleUploadTreeResponse
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
	defer state.b.Release()
	defer func() {
		if rerr == nil {
			return
		}
		if err := abortUploadTreeFile(state, rerr); err != nil {
			rerr = errors.Wrap(rerr, err.Error())
		}
	}()

	for {
		msg, err := strm.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if err := r.handleUploadTreeMessage(ctx, state, msg); err != nil {
			return nil, err
		}
	}
	if err := finishUploadTreeFile(state); err != nil {
		return nil, err
	}
	if err := state.b.Commit(ctx); err != nil {
		return nil, err
	}
	if err := r.reloadHandle(ctx); err != nil {
		return nil, err
	}

	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) { broadcast() })
	return &state.resp, nil
}

// handleUploadTreeMessage handles one UploadTree stream message.
func (r *FSHandleResource) handleUploadTreeMessage(
	ctx context.Context,
	state *uploadTreeState,
	msg *s4wave_unixfs.HandleUploadTreeRequest,
) error {
	if dir := msg.GetDirectory(); dir != nil {
		if err := finishUploadTreeFile(state); err != nil {
			return err
		}
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
		if err := finishUploadTreeFile(state); err != nil {
			return err
		}
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
		pr, pw := io.Pipe()
		done := make(chan error, 1)
		go func() {
			done <- state.b.AddFile(
				ctx,
				parentPath,
				parts[len(parts)-1],
				unixfs.NewFSCursorNodeType_File(),
				fileStart.GetTotalSize(),
				pr,
				fs.FileMode(fileStart.GetMode()),
				time.Now(),
			)
		}()
		state.current = &uploadTreeFile{
			parentPath: parentPath,
			name:       parts[len(parts)-1],
			pw:         pw,
			done:       done,
		}
		state.resp.FilesWritten++
		return nil
	}

	data := msg.GetData()
	if len(data) == 0 {
		return errors.New("tree upload message missing body")
	}
	if state.current == nil {
		return errors.New("tree upload data received before file_start")
	}
	n, err := state.current.pw.Write(data)
	state.resp.BytesWritten += int64(n)
	if err != nil {
		return err
	}
	return nil
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

// finishUploadTreeFile closes and waits for the current file upload, if any.
func finishUploadTreeFile(state *uploadTreeState) error {
	if state.current == nil {
		return nil
	}
	curr := state.current
	state.current = nil
	if err := curr.pw.Close(); err != nil {
		return err
	}
	return <-curr.done
}

// abortUploadTreeFile aborts and waits for the current file upload, if any.
func abortUploadTreeFile(state *uploadTreeState, cause error) error {
	if state.current == nil {
		return nil
	}
	curr := state.current
	state.current = nil
	var ret error
	if err := curr.pw.CloseWithError(cause); err != nil {
		ret = err
	}
	if err := <-curr.done; err != nil && ret == nil {
		ret = err
	}
	return ret
}

// parseUploadTreePath validates a slash-separated relative upload path.
func parseUploadTreePath(path string) ([]string, error) {
	if path == "" {
		return nil, errors.New("empty upload path")
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
