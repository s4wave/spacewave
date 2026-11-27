package s4wave_world

import (
	"context"
	"io"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
)

// ObjectBodiesBatchService streams pages of object bodies from a WorldState
// resource.
type ObjectBodiesBatchService interface {
	// GetObjectBodiesBatch streams pages of the requested object bodies.
	GetObjectBodiesBatch(
		context.Context,
		*GetObjectBodiesBatchRequest,
	) (SRPCWorldStateResourceService_GetObjectBodiesBatchClient, error)
}

// ForEachObjectBodyPage calls cb with each page of serialized object bodies.
// A body over the page budget arrives withheld; see world.ObjectBody.
// The callback must finish with the page before the next page is read.
// Returns world.ObjectBodiesBatchRevisionError when pages come from different World
// revisions.
func ForEachObjectBodyPage(
	ctx context.Context,
	service ObjectBodiesBatchService,
	keys []string,
	cb func([]*world.ObjectBody) error,
) error {
	if cb == nil {
		return errors.New("object body page callback is nil")
	}
	return forEachObjectBodiesResponse(ctx, service, keys, false, func(resp *GetObjectBodiesBatchResponse) error {
		// Copy the page's object bodies before invoking the callback.
		page := make([]*world.ObjectBody, len(resp.GetBodies()))
		for i, body := range resp.GetBodies() {
			page[i] = &world.ObjectBody{
				ObjectKey:    body.GetObjectKey(),
				Body:         append([]byte(nil), body.GetBody()...),
				Exists:       body.GetExists(),
				Rev:          body.GetRev(),
				ModSeqno:     body.GetModSeqno(),
				WithheldSize: body.GetWithheldSize(),
			}
		}
		return cb(page)
	})
}

// ForEachObjectBodyJSONPage calls cb with each page of object bodies carrying
// their type IDs and, where the host or the type's plugin can render them,
// JSON. The page belongs to cb. Returns world.ObjectBodiesBatchRevisionError when
// pages come from different World revisions.
func ForEachObjectBodyJSONPage(
	ctx context.Context,
	service ObjectBodiesBatchService,
	keys []string,
	cb func([]*ObjectBody) error,
) error {
	if cb == nil {
		return errors.New("object body page callback is nil")
	}
	return forEachObjectBodiesResponse(ctx, service, keys, true, func(resp *GetObjectBodiesBatchResponse) error {
		return cb(resp.GetBodies())
	})
}

// forEachObjectBodiesResponse streams the GetObjectBodiesBatch pages for keys
// to cb, in key chunks that fit the request budget, and requires every page
// to come from the first page's World revision.
func forEachObjectBodiesResponse(
	ctx context.Context,
	service ObjectBodiesBatchService,
	keys []string,
	json bool,
	cb func(*GetObjectBodiesBatchResponse) error,
) error {
	// Split the object keys into requests that fit the byte budget.
	if len(keys) == 0 {
		return nil
	}
	chunks, err := chunkObjectBodyKeys(keys)
	if err != nil {
		return err
	}

	// Read all object body chunks from one World revision.
	var worldSeqno uint64
	var haveWorldSeqno bool
	for _, chunk := range chunks {
		// Open the object body stream for this key chunk.
		strm, err := service.GetObjectBodiesBatch(ctx, &GetObjectBodiesBatchRequest{ObjectKeys: chunk, Json: json})
		if err != nil {
			return err
		}

		// Consume this chunk's pages and close its stream on completion.
		err = func() error {
			// Keep the object body stream open until all its pages are consumed.
			defer strm.Close()
			for {
				// Receive the next object body page or finish at the end of the stream.
				resp, err := strm.Recv()
				if err == io.EOF {
					return nil
				}
				if err != nil {
					return err
				}

				// Require every page to belong to the first observed World revision.
				pageSeqno := resp.GetWorldSeqno()
				if !haveWorldSeqno {
					worldSeqno = pageSeqno
					haveWorldSeqno = true
				} else if pageSeqno != worldSeqno {
					return &world.ObjectBodiesBatchRevisionError{
						Expected: worldSeqno,
						Got:      pageSeqno,
					}
				}
				if err := cb(resp); err != nil {
					return err
				}
			}
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

// GetObjectBodiesPage reads one page of at most byteBudget encoded body bytes
// for a prefix of keys in one call, with the World seqno it read. It follows
// world.WorldStateObject.GetObjectBodiesBatchPage: consumed is the number of
// keys the page answers, or zero when it answers all, and a body over the
// budget arrives withheld. The budget is capped at world.ObjectBodiesBatchByteBudget.
func GetObjectBodiesPage(
	ctx context.Context,
	service ObjectBodiesBatchService,
	keys []string,
	byteBudget int,
) ([]*world.ObjectBody, uint32, uint64, error) {
	// Ask for one page of the keys that fit one request.
	if len(keys) == 0 {
		return nil, 0, 0, nil
	}
	chunks, err := chunkObjectBodyKeys(keys)
	if err != nil {
		return nil, 0, 0, err
	}
	pageBudget := uint32(max(1, min(byteBudget, world.ObjectBodiesBatchByteBudget)))
	strm, err := service.GetObjectBodiesBatch(ctx, &GetObjectBodiesBatchRequest{
		ObjectKeys: chunks[0],
		PageBudget: pageBudget,
	})
	if err != nil {
		return nil, 0, 0, err
	}
	defer strm.Close()

	// Copy the one page out of the stream buffer and count the keys it answers.
	resp, err := strm.Recv()
	if err != nil {
		return nil, 0, 0, err
	}
	page := make([]*world.ObjectBody, len(resp.GetBodies()))
	for i, body := range resp.GetBodies() {
		page[i] = &world.ObjectBody{
			ObjectKey:    body.GetObjectKey(),
			Body:         append([]byte(nil), body.GetBody()...),
			Exists:       body.GetExists(),
			Rev:          body.GetRev(),
			ModSeqno:     body.GetModSeqno(),
			WithheldSize: body.GetWithheldSize(),
		}
	}
	if len(page) == 0 {
		return nil, 0, 0, errors.New("object body page is empty")
	}
	var consumed uint32
	if len(page) < len(keys) {
		consumed = uint32(len(page)) //nolint:gosec // one request chunk of keys
	}
	return page, consumed, resp.GetWorldSeqno(), nil
}

// GetObjectBodiesBatch collects all pages from a WorldState resource. A body
// over the page budget arrives withheld. world.GetObjectBodiesBatch reads it.
func GetObjectBodiesBatch(ctx context.Context, service ObjectBodiesBatchService, keys []string) ([]*world.ObjectBody, error) {
	for retry := 0; retry <= world.MaxObjectBodiesBatchRevisionRetries; retry++ {
		// Collect all object body pages from a consistent World revision.
		bodies := make([]*world.ObjectBody, 0, len(keys))
		err := ForEachObjectBodyPage(ctx, service, keys, func(page []*world.ObjectBody) error {
			bodies = append(bodies, page...)
			return nil
		})
		if err == nil {
			return bodies, nil
		}

		// Retry collection only when pages span different World revisions.
		var revisionErr *world.ObjectBodiesBatchRevisionError
		if !errors.As(err, &revisionErr) {
			return nil, err
		}
		revisionErr.Retries = retry
		if retry == world.MaxObjectBodiesBatchRevisionRetries {
			return nil, revisionErr
		}
	}
	return nil, &world.ObjectBodiesBatchRevisionError{Retries: world.MaxObjectBodiesBatchRevisionRetries}
}

func chunkObjectBodyKeys(keys []string) ([][]string, error) {
	var chunks [][]string
	for start := 0; start < len(keys); {
		end := start
		encodedSize := 0
		for end < len(keys) {
			keySize := protobuf_go_lite.SizeStringValue(1, keys[end])
			if encodedSize+keySize > world.ObjectBodiesBatchByteBudget {
				if end == start {
					return nil, errors.Errorf("object body key %q exceeds request byte budget %d", keys[start], world.ObjectBodiesBatchByteBudget)
				}
				break
			}
			encodedSize += keySize
			end++
		}
		chunks = append(chunks, keys[start:end])
		start = end
	}
	return chunks, nil
}
