package world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// ObjectBodiesBatchByteBudget is the encoded-size budget shared by body batch requests and responses.
const ObjectBodiesBatchByteBudget = block.MaxBlockSize - 64*1024

// ObjectBody contains the serialized root body for one object key.
type ObjectBody struct {
	// ObjectKey is the requested object key.
	ObjectKey string
	// Body is the transformed root block data when the object exists.
	Body []byte
	// Rev is the object revision observed with Body.
	Rev uint64
	// Exists reports whether the object key exists.
	Exists bool
}

// ObjectBodyBatcher returns serialized object bodies for object keys.
type ObjectBodyBatcher interface {
	// GetObjectBodiesBatch returns object bodies for object keys.
	GetObjectBodiesBatch(ctx context.Context, keys []string) ([]*ObjectBody, error)
}

// ObjectBodyPageBatcher returns one budgeted page of serialized object bodies.
type ObjectBodyPageBatcher interface {
	// GetObjectBodiesBatchPage returns bodies and the number of keys consumed.
	GetObjectBodiesBatchPage(ctx context.Context, keys []string, byteBudget int) ([]*ObjectBody, uint32, error)
}

// ObjectBodyPageSeqnoBatcher returns one budgeted page and its World sequence number.
type ObjectBodyPageSeqnoBatcher interface {
	// GetObjectBodiesBatchPageWithSeqno returns bodies, consumed keys, and the
	// World sequence number observed by the read transaction.
	GetObjectBodiesBatchPageWithSeqno(ctx context.Context, keys []string, byteBudget int) ([]*ObjectBody, uint32, uint64, error)
}

// ObjectBodyTooLargeError reports a body that cannot fit in one response page.
type ObjectBodyTooLargeError struct {
	// ObjectKey is the key whose encoded entry exceeds the page budget.
	ObjectKey string
	// EncodedSize is the serialized size of the object body response entry.
	EncodedSize int
	// ByteBudget is the maximum encoded size allowed for one page.
	ByteBudget int
}

// Error returns the oversized object body details.
func (e *ObjectBodyTooLargeError) Error() string {
	return errors.Errorf(
		"object body %q encoded size %d exceeds page budget %d",
		e.ObjectKey,
		e.EncodedSize,
		e.ByteBudget,
	).Error()
}

// GetObjectBodiesBatch returns serialized object bodies for object keys.
// Results preserve the requested key order and include one missing marker per
// key that does not exist.
func GetObjectBodiesBatch(ctx context.Context, ws WorldState, keys []string) ([]*ObjectBody, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	if batcher, ok := ws.(ObjectBodyBatcher); ok {
		return batcher.GetObjectBodiesBatch(ctx, keys)
	}

	out := make([]*ObjectBody, len(keys))
	for i, key := range keys {
		body, rev, exists, err := LookupObjectBodyBytesWithRev(ctx, ws, key)
		if err != nil {
			return nil, err
		}
		out[i] = &ObjectBody{
			ObjectKey: key,
			Body:      body,
			Rev:       rev,
			Exists:    exists,
		}
	}
	return out, nil
}

// GetObjectBodiesBatchPage returns one encoded-size-bounded page of bodies.
func GetObjectBodiesBatchPage(
	ctx context.Context,
	ws WorldState,
	keys []string,
	byteBudget int,
) ([]*ObjectBody, uint32, error) {
	if len(keys) == 0 {
		return nil, 0, nil
	}
	if pager, ok := ws.(ObjectBodyPageBatcher); ok {
		return pager.GetObjectBodiesBatchPage(ctx, keys, byteBudget)
	}

	return getObjectBodiesBatchPage(ctx, keys, byteBudget, func(ctx context.Context, key string) ([]byte, uint64, bool, error) {
		return LookupObjectBodyBytesWithRev(ctx, ws, key)
	})
}

func getObjectBodiesBatchPage(
	ctx context.Context,
	keys []string,
	byteBudget int,
	lookup func(context.Context, string) ([]byte, uint64, bool, error),
) ([]*ObjectBody, uint32, error) {
	out := make([]*ObjectBody, 0, len(keys))
	encodedSize := 0
	for i, key := range keys {
		if len(out) > 0 && encodedSize >= byteBudget {
			return out, uint32(i), nil
		}
		body, rev, exists, err := lookup(ctx, key)
		if err != nil {
			return nil, 0, err
		}
		result := &ObjectBody{
			ObjectKey: key,
			Body:      body,
			Rev:       rev,
			Exists:    exists,
		}
		resultSize := objectBodyEncodedSize(result)
		if resultSize > byteBudget {
			return nil, 0, &ObjectBodyTooLargeError{
				ObjectKey:   key,
				EncodedSize: resultSize,
				ByteBudget:  byteBudget,
			}
		}
		if len(out) > 0 && encodedSize+resultSize > byteBudget {
			return out, uint32(i), nil
		}
		out = append(out, result)
		encodedSize += resultSize
	}
	return out, 0, nil
}

// GetObjectBodiesBatchPageWithSeqno returns one page and its World sequence number.
func GetObjectBodiesBatchPageWithSeqno(
	ctx context.Context,
	ws WorldState,
	keys []string,
	byteBudget int,
) ([]*ObjectBody, uint32, uint64, error) {
	if pager, ok := ws.(ObjectBodyPageSeqnoBatcher); ok {
		return pager.GetObjectBodiesBatchPageWithSeqno(ctx, keys, byteBudget)
	}
	bodies, consumed, err := GetObjectBodiesBatchPage(ctx, ws, keys, byteBudget)
	return bodies, consumed, 0, err
}

// objectBodyEncodedSize matches the generated protobuf size of one repeated
// ObjectBody response entry, including its key, body, exists flag, revision,
// and the enclosing repeated-message tag and length.
func objectBodyEncodedSize(body *ObjectBody) int {
	if body == nil {
		return 0
	}
	bodySize := 0
	if len(body.ObjectKey) > 0 {
		bodySize += 1 + protoVarintSize(uint64(len(body.ObjectKey))) + len(body.ObjectKey)
	}
	if len(body.Body) > 0 {
		bodySize += 1 + protoVarintSize(uint64(len(body.Body))) + len(body.Body)
	}
	if body.Exists {
		bodySize += 2
	}
	if body.Rev != 0 {
		bodySize += 1 + protoVarintSize(body.Rev)
	}
	return 1 + protoVarintSize(uint64(bodySize)) + bodySize
}

func protoVarintSize(value uint64) int {
	size := 1
	for value >= 128 {
		value >>= 7
		size++
	}
	return size
}

// ObjectBodyResult contains a typed body or a missing marker for one object
// key.
type ObjectBodyResult[T block.Block] struct {
	// ObjectKey is the requested object key.
	ObjectKey string
	// Body is the decoded object body when the object exists.
	Body T
	// Exists reports whether the object key exists.
	Exists bool
}

// LookupObjectBodies looks up and unmarshals object bodies for the given keys.
// Results preserve the requested key order and include a zero Body with
// Exists=false for missing objects.
func LookupObjectBodies[T block.Block](
	ctx context.Context,
	ws WorldState,
	objKeys []string,
	ctor func() block.Block,
) ([]*ObjectBodyResult[T], error) {
	bodies, err := GetObjectBodiesBatch(ctx, ws, objKeys)
	if err != nil {
		return nil, err
	}

	results := make([]*ObjectBodyResult[T], len(bodies))
	for i, body := range bodies {
		result := &ObjectBodyResult[T]{
			ObjectKey: body.ObjectKey,
			Exists:    body.Exists,
		}
		if body.Exists {
			result.Body, err = decodeObjectBody[T](body.Body, ctor)
			if err != nil {
				return nil, err
			}
		}
		results[i] = result
	}
	return results, nil
}

// _ is a type assertion.
var _ ObjectBodyBatcher = ((*engineWorldState)(nil))
var _ ObjectBodyPageBatcher = ((*engineWorldState)(nil))
var _ ObjectBodyPageSeqnoBatcher = ((*engineWorldState)(nil))
