package world

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/block"
)

// ObjectBodiesBatchByteBudget is the encoded-size budget shared by body batch
// requests and responses. One page must fit in a single starpc frame, which is
// capped at 10,000,000 bytes, with room for the packet envelope. A body whose
// entry alone exceeds the budget is withheld; see ObjectBody.WithheldSize.
const ObjectBodiesBatchByteBudget = 8 << 20

// ObjectBody contains the serialized root body for one object key.
type ObjectBody struct {
	// ObjectKey is the requested object key.
	ObjectKey string
	// Body is the transformed root block data when the object exists.
	Body []byte
	// Rev is the object revision observed with Body.
	Rev uint64
	// ModSeqno is the World seqno after the last commit that changed the
	// object. Zero when the object state does not report it.
	ModSeqno uint64
	// Exists reports whether the object key exists.
	Exists bool
	// WithheldSize is the length of a body left out of its page because the
	// entry alone exceeds the page budget; Body is then empty. Zero for a body
	// the page carries. LookupObjectBodyEntry reads a withheld body alone.
	WithheldSize uint64

	// keep stores the body's block once its decoder knows the block's refs,
	// set when the store served the block without them; see
	// block.Cursor.FetchWithKeep. It never crosses a process boundary.
	keep func(context.Context, []*block.BlockRef)
}

// keepDecoded hands the refs of blk, the block decoded from b.Body, to the
// store that served it without them, so the store keeps the block instead of
// reading it again on the next lookup.
func (b *ObjectBody) keepDecoded(ctx context.Context, blk any) {
	if b.keep == nil {
		return
	}
	if refs, err := block.ExtractBlockRefs(blk); err == nil {
		b.keep(context.WithoutCancel(ctx), refs)
	}
}

// GetObjectBodiesBatch returns serialized object bodies for object keys, all
// read at one World seqno. Results preserve the requested key order and
// include one missing marker per key that does not exist. Every existing body
// is whole: a body its page withholds is read alone through the object.
// Returns ObjectBodiesBatchRevisionError when writes keep landing between
// pages.
func GetObjectBodiesBatch(ctx context.Context, ws WorldState, keys []string) ([]*ObjectBody, error) {
	// Collect pages from one World revision, restarting when it moves.
	if len(keys) == 0 {
		return nil, nil
	}
	var bodies []*ObjectBody
	for retry := 0; ; retry++ {
		var err error
		bodies, err = collectObjectBodyPages(ctx, ws, keys)
		if err == nil {
			break
		}
		var revisionErr *ObjectBodiesBatchRevisionError
		if !errors.As(err, &revisionErr) {
			return nil, err
		}
		revisionErr.Retries = retry
		if retry == MaxObjectBodiesBatchRevisionRetries {
			return nil, revisionErr
		}
	}

	// Read each withheld body alone.
	for i, body := range bodies {
		if body.WithheldSize == 0 {
			continue
		}
		whole, err := LookupObjectBodyEntry(ctx, ws, body.ObjectKey)
		if err != nil {
			return nil, err
		}
		bodies[i] = whole
	}
	return bodies, nil
}

// collectObjectBodyPages reads every page of keys and requires one seqno.
func collectObjectBodyPages(ctx context.Context, ws WorldState, keys []string) ([]*ObjectBody, error) {
	out := make([]*ObjectBody, 0, len(keys))
	var worldSeqno uint64
	for start := 0; start < len(keys); {
		page, consumed, seqno, err := ws.GetObjectBodiesBatchPage(ctx, keys[start:], ObjectBodiesBatchByteBudget)
		if err != nil {
			return nil, err
		}
		if start == 0 {
			worldSeqno = seqno
		} else if seqno != worldSeqno {
			return nil, &ObjectBodiesBatchRevisionError{Expected: worldSeqno, Got: seqno}
		}
		out = append(out, page...)
		if consumed == 0 {
			break
		}
		start += int(consumed)
	}
	return out, nil
}

// LookupObjectBodiesPage implements WorldStateObject.GetObjectBodiesBatchPage
// for a store without a batch read: it reads the seqno, then each body
// through LookupObjectBodyEntry. Call it only on a store that reads one
// revision, such as a transaction.
func LookupObjectBodiesPage(
	ctx context.Context,
	ws WorldState,
	keys []string,
	byteBudget int,
) ([]*ObjectBody, uint32, uint64, error) {
	// Read the seqno, then each body until the page is full.
	if len(keys) == 0 {
		return nil, 0, 0, nil
	}
	seqno, err := ws.GetSeqno(ctx)
	if err != nil {
		return nil, 0, 0, err
	}
	bodies, consumed, err := getObjectBodiesBatchPage(ctx, keys, byteBudget, func(ctx context.Context, key string) (*ObjectBody, error) {
		return LookupObjectBodyEntry(ctx, ws, key)
	})
	return bodies, consumed, seqno, err
}

func getObjectBodiesBatchPage(
	ctx context.Context,
	keys []string,
	byteBudget int,
	lookup func(context.Context, string) (*ObjectBody, error),
) ([]*ObjectBody, uint32, error) {
	out := make([]*ObjectBody, 0, len(keys))
	encodedSize := 0
	for i, key := range keys {
		if len(out) > 0 && encodedSize >= byteBudget {
			return out, uint32(i), nil
		}
		result, err := lookup(ctx, key)
		if err != nil {
			return nil, 0, err
		}
		resultSize := objectBodyEncodedSize(result)
		if resultSize > byteBudget {
			result.WithheldSize, result.Body, result.keep = uint64(len(result.Body)), nil, nil
			resultSize = objectBodyEncodedSize(result)
		}
		if len(out) > 0 && encodedSize+resultSize > byteBudget {
			return out, uint32(i), nil
		}
		out = append(out, result)
		encodedSize += resultSize
	}
	return out, 0, nil
}

// objectBodyEncodedSize matches the generated protobuf size of one repeated
// ObjectBody response entry, including its key, body, exists flag, revision,
// ModSeqno, withheld size, and the enclosing repeated-message tag and length.
func objectBodyEncodedSize(body *ObjectBody) int {
	// Measure the length-delimited key and body fields.
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

	// Measure the scalar fields, then the enclosing protobuf entry.
	if body.Exists {
		bodySize += 2
	}
	if body.Rev != 0 {
		bodySize += 1 + protoVarintSize(body.Rev)
	}
	if body.ModSeqno != 0 {
		bodySize += 1 + protoVarintSize(body.ModSeqno)
	}
	if body.WithheldSize != 0 {
		bodySize += 1 + protoVarintSize(body.WithheldSize)
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
func LookupObjectBodies[T any, PT block.Ptr[T]](
	ctx context.Context,
	ws WorldState,
	objKeys []string,
) ([]*ObjectBodyResult[PT], error) {
	// Fetch serialized object bodies in the requested key order.
	bodies, err := GetObjectBodiesBatch(ctx, ws, objKeys)
	if err != nil {
		return nil, err
	}

	// Decode existing object bodies while retaining missing markers.
	results := make([]*ObjectBodyResult[PT], len(bodies))
	for i, body := range bodies {
		result := &ObjectBodyResult[PT]{
			ObjectKey: body.ObjectKey,
			Exists:    body.Exists,
		}
		if body.Exists {
			result.Body = PT(new(T))
		}
		if body.Exists && body.Body != nil {
			if err := result.Body.UnmarshalVT(body.Body); err != nil {
				return nil, err
			}
			body.keepDecoded(ctx, result.Body)
		}
		results[i] = result
	}
	return results, nil
}
