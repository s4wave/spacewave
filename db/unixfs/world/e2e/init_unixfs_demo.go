package unixfs_world_e2e

import (
	"context"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// InitUnixFSDemo initializes a UnixFS filesystem with demo content in a world.
// Returns any error.
func InitUnixFSDemo(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	ts time.Time,
) (rev uint64, sysErr bool, err error) {
	op := NewInitUnixFSDemoOp(objKey, ts)
	return ws.ApplyWorldOp(ctx, op, sender)
}

// DefaultUnixFSObjectKey is the default object key for the UnixFS demo
// filesystem.
const DefaultUnixFSObjectKey = "unixfs"

// NewInitUnixFSDemoOp constructs a new InitUnixFSDemoOp block.
func NewInitUnixFSDemoOp(
	objKey string,
	ts time.Time,
) *InitUnixFSDemoOp {
	if objKey == "" {
		objKey = DefaultUnixFSObjectKey
	}
	return &InitUnixFSDemoOp{
		ObjectKey: objKey,
		Timestamp: timestamp.New(ts),
	}
}

// Validate performs cursory checks on the op.
func (o *InitUnixFSDemoOp) Validate() error {
	// Validate the effective demo object key and operation timestamp.
	objKey := o.GetObjectKey()
	if objKey == "" {
		objKey = DefaultUnixFSObjectKey
	}
	if len(objKey) == 0 {
		return world.ErrEmptyObjectKey
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *InitUnixFSDemoOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Default the object key.
	objKey := o.GetObjectKey()
	if objKey == "" {
		objKey = DefaultUnixFSObjectKey
	}

	// Initialize the UnixFS filesystem
	fsNodeType := unixfs_world.FSType_FSType_FS_NODE
	_, _, err = unixfs_world.FsInit(
		ctx,
		worldHandle,
		sender,
		objKey,
		fsNodeType,
		nil,
		false,
		o.GetTimestamp().AsTime(),
	)
	if err != nil {
		return false, err
	}

	// Create a cursor with a writer
	fsCursor, _ := unixfs_world.NewFSCursorWithWriter(
		ctx,
		le,
		worldHandle,
		objKey,
		fsNodeType,
		sender,
	)
	defer fsCursor.Release()

	// Create a filesystem handle
	fsh, err := unixfs.NewFSHandle(fsCursor)
	if err != nil {
		return false, err
	}
	defer fsh.Release()

	// Create the directory structure: /test/dir
	ts := o.GetTimestamp().AsTime()
	if err := fsh.MkdirAll(ctx, []string{"test", "dir"}, 0o700, ts); err != nil {
		return false, err
	}

	// Create files: hello.txt and world.md
	if err := fsh.Mknod(ctx, false, []string{"hello.txt", "world.md"}, unixfs.NewFSCursorNodeType_File(), 0o644, ts); err != nil {
		return false, err
	}

	// Write content to hello.txt
	helloTxtFsh, err := fsh.Lookup(ctx, "hello.txt")
	if err != nil {
		return false, err
	}
	defer helloTxtFsh.Release()
	if err := helloTxtFsh.WriteAt(ctx, 0, []byte("Hello world from Go!\n"), ts); err != nil {
		return false, err
	}
	return false, helloTxtFsh.Sync(ctx)
}

// _ is a type assertion
var _ world.Operation = (*InitUnixFSDemoOp)(nil)
