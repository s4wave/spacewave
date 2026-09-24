package git_lfs

import (
	"context"
	"io"

	"github.com/pkg/errors"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
)

const (
	// chunkSize is the payload of one upload frame or ReadAt call, the
	// largest the UnixFS handle resource accepts.
	chunkSize = 64 * 1024
	// readWindow is the number of ReadAt calls a download keeps in flight,
	// so throughput is not bound by one round trip per chunk.
	readWindow = 16
)

// UnixFSStore stores Git LFS objects as files in a UnixFS object at
// ObjectPath, through the object's root handle resource.
//
// Each Put is one UploadTree call, so the blob is ingested outside the
// object's write lock and the file and its shard directories land in one
// commit. Concurrent Puts from several agents overlap their ingest.
type UnixFSStore struct {
	// root is the root handle service of the UnixFS object.
	root s4wave_unixfs.SRPCFSHandleResourceServiceClient
	// res references and releases the handles that lookups open.
	res *resource_client.Client
}

// NewUnixFSStore constructs a UnixFSStore over the root handle of a UnixFS
// object and the resource client that owns it.
func NewUnixFSStore(
	root s4wave_unixfs.SRPCFSHandleResourceServiceClient,
	res *resource_client.Client,
) *UnixFSStore {
	return &UnixFSStore{root: root, res: res}
}

// Has reports whether a file of exactly size bytes is stored for oid.
//
// Errors cross the resource RPC as strings, so a missing path cannot be told
// apart from another lookup failure. Every failure reports the object as
// missing; the Put that follows surfaces a real failure.
func (s *UnixFSStore) Has(ctx context.Context, oid string, size int64) (bool, error) {
	resp, err := s.root.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{Path: ObjectPath(oid)})
	if err != nil {
		return false, nil
	}
	s.res.CreateResourceReference(resp.GetResourceId()).Release()
	info := resp.GetInfo()
	return !info.GetIsDir() && info.GetSize() == size, nil
}

// Put streams size bytes from rdr into the file for oid in one UploadTree
// call, replacing an existing file.
func (s *UnixFSStore) Put(ctx context.Context, oid string, size int64, rdr io.Reader) error {
	// Open the upload stream and start the file at its sharded path.
	strm, err := s.root.UploadTree(ctx)
	if err != nil {
		return err
	}
	defer strm.Close()
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{
			FileStart: &s4wave_unixfs.HandleUploadTreeFileStart{
				Path:      ObjectPath(oid),
				TotalSize: size,
				Mode:      0o644,
			},
		},
	}); err != nil {
		return err
	}

	// Stream the payload in frames. Send encodes the frame before returning,
	// so one buffer serves every frame.
	buf := make([]byte, chunkSize)
	for remaining := size; remaining > 0; {
		n := int(min(remaining, chunkSize))
		if _, err := io.ReadFull(rdr, buf[:n]); err != nil {
			return errors.Wrap(err, "read object")
		}
		if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
			Body: &s4wave_unixfs.HandleUploadTreeRequest_Data{Data: buf[:n]},
		}); err != nil {
			return err
		}
		remaining -= int64(n)
	}

	// Commit the file and confirm the daemon applied it.
	resp, err := strm.CloseAndRecv()
	if err != nil {
		return err
	}
	if resp.GetFilesWritten() != 1 || resp.GetBytesWritten() != size {
		return errors.Errorf(
			"upload wrote %d files and %d bytes, want 1 file and %d bytes",
			resp.GetFilesWritten(),
			resp.GetBytesWritten(),
			size,
		)
	}
	return nil
}

// Get writes the file for oid to w in order, keeping readWindow ReadAt calls
// in flight.
func (s *UnixFSStore) Get(ctx context.Context, oid string, size int64, w io.Writer) error {
	// Open the object's file handle.
	resp, err := s.root.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{Path: ObjectPath(oid)})
	if err != nil {
		return errors.Wrap(err, "lookup object")
	}
	ref := s.res.CreateResourceReference(resp.GetResourceId())
	defer ref.Release()
	if got := resp.GetInfo().GetSize(); got != size {
		return errors.Errorf("stored object has %d bytes, want %d", got, size)
	}
	client, err := ref.GetClient()
	if err != nil {
		return err
	}
	file := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(client)

	// Issue reads ahead of the writer and consume them in offset order. The
	// canceled context stops outstanding reads when Get returns early, and
	// each result channel is buffered so no reader blocks after that.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type chunk struct {
		data []byte
		err  error
	}
	read := func(off int64) <-chan chunk {
		ch := make(chan chunk, 1)
		go func() {
			resp, err := file.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{
				Offset: off,
				Length: chunkSize,
			})
			ch <- chunk{data: resp.GetData(), err: err}
		}()
		return ch
	}
	pending := make([]<-chan chunk, 0, readWindow)
	var next int64
	for off := int64(0); off < size; {
		for len(pending) < readWindow && next < size {
			pending = append(pending, read(next))
			next += chunkSize
		}
		var c chunk
		select {
		case <-ctx.Done():
			return ctx.Err()
		case c = <-pending[0]:
		}
		pending = pending[1:]
		if c.err != nil {
			return errors.Wrapf(c.err, "read object at %d", off)
		}
		if want := min(size-off, chunkSize); int64(len(c.data)) != want {
			return errors.Errorf("read %d bytes at %d, want %d", len(c.data), off, want)
		}
		if _, err := w.Write(c.data); err != nil {
			return err
		}
		off += int64(len(c.data))
	}
	return nil
}

// ObjectPath returns the slash path of oid within the UnixFS object:
// objects/<oid[0:2]>/<oid[2:4]>/<oid>, the layout git-lfs uses locally. The
// two-level shard keeps each directory small.
func ObjectPath(oid string) string {
	return "objects/" + oid[0:2] + "/" + oid[2:4] + "/" + oid
}

// _ is a type assertion
var _ Store = ((*UnixFSStore)(nil))
