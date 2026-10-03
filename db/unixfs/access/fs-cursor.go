package unixfs_access

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/unixfs"
)

// FSCursor attaches a cursor to a UnixFS access function.
//
// When the cursor is resolved with GetProxyCursor we call and wait for the access func.
type FSCursor = unixfs.FSCursorGetter

// NewFSCursor constructs a new FSCursor with a world object ref.
func NewFSCursor(
	access AccessUnixFSFunc,
) *FSCursor {
	return unixfs.NewFSCursorGetter(NewAccessUnixFSFuncFSCursorGetter(access))
}

// NewAccessUnixFSFuncFSCursorGetter returns a FSCursorGetter bound to a AccessUnixFSFunc.
func NewAccessUnixFSFuncFSCursorGetter(access AccessUnixFSFunc) func(ctx context.Context) (unixfs.FSCursor, error) {
	return func(rctx context.Context) (unixfs.FSCursor, error) {
		// Require a UnixFS provider before resolving its cursor.
		if access == nil {
			return nil, errors.New("unixfs access func is nil")
		}

		// Tie the resolved cursor to the access context and release callback.
		ctx, ctxCancel := context.WithCancel(rctx)
		var subCursor atomic.Pointer[unixfs.FSHandleCursor]
		released := func() {
			ctxCancel()
			if cursor := subCursor.Load(); cursor != nil {
				cursor.Release()
				subCursor.Store(nil)
			}
		}

		// Acquire the UnixFS handle from the provider.
		fsh, relFsh, err := access(ctx, released)
		if err != nil {
			ctxCancel()
			return nil, err
		}

		// Publish the handle cursor unless access was already canceled.
		retCursor := unixfs.NewFSHandleCursor(fsh, true, relFsh)
		subCursor.Store(retCursor)
		if ctx.Err() != nil {
			retCursor.Release()
			subCursor.Store(nil)
			return nil, context.Canceled
		}

		return retCursor, nil
	}
}

// _ is a type assertion
var _ unixfs.FSCursor = (*FSCursor)(nil)
