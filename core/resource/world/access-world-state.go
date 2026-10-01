package resource_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_bucket_lookup "github.com/s4wave/spacewave/core/resource/bucket/lookup"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/sirupsen/logrus"
)

// accessWorldStateFunc calls AccessWorldState on a World storage handle.
type accessWorldStateFunc func(ctx context.Context, cb func(*bucket_lookup.Cursor) error) error

// addAccessWorldStateResource registers the cursor passed to an
// AccessWorldState callback as a resource.
//
// AccessWorldState releases its cursor and root pin when the callback returns,
// so the callback stays open in a background routine until the resource is
// released or the resource client ends.
func addAccessWorldStateResource(
	ctx context.Context,
	resourceCtx resource_server.ResourceClientContext,
	le *logrus.Entry,
	b bus.Bus,
	access accessWorldStateFunc,
) (uint32, error) {
	// Start a hold context and forward the cursor once the callback receives it.
	holdCtx, holdCancel := context.WithCancel(resourceCtx.Context())
	cursorCh := make(chan *bucket_lookup.Cursor, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- access(holdCtx, func(c *bucket_lookup.Cursor) error {
			cursorCh <- c
			<-holdCtx.Done()
			return nil
		})
	}()

	// Wait for the callback to receive its cursor.
	var cursor *bucket_lookup.Cursor
	select {
	case <-ctx.Done():
		holdCancel()
		return 0, context.Cause(ctx)
	case err := <-errCh:
		holdCancel()
		if err == nil {
			err = errors.New("world state access returned without a cursor")
		}
		return 0, err
	case cursor = <-cursorCh:
	}

	// Releasing the resource returns the callback and releases the cursor.
	cursorResource := resource_bucket_lookup.NewBucketLookupCursorResource(le, b, cursor)
	id, err := resourceCtx.AddResource(cursorResource.GetMux(), holdCancel)
	if err != nil {
		holdCancel()
		return 0, err
	}
	return id, nil
}
