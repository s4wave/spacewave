package s4wave_world

import (
	"context"
	"io"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
)

const maxObjectBodiesBatchRevisionRetries = 3

// ObjectBodiesBatchService streams pages of object bodies from a WorldState
// resource.
type ObjectBodiesBatchService interface {
	GetObjectBodiesBatch(
		context.Context,
		*GetObjectBodiesBatchRequest,
	) (SRPCWorldStateResourceService_GetObjectBodiesBatchClient, error)
}

// ForEachObjectBodyPage calls cb with each page of serialized object bodies.
// The callback must finish with the page before the next page is read.
// Returns ObjectBodiesBatchRevisionError when pages come from different World
// revisions.
func ForEachObjectBodyPage(
	ctx context.Context,
	service ObjectBodiesBatchService,
	keys []string,
	cb func([]*world.ObjectBody) error,
) error {
	// Require object keys and a callback before opening page streams.
	if len(keys) == 0 {
		return nil
	}
	if cb == nil {
		return errors.New("object body page callback is nil")
	}

	// Split the object keys into requests that fit the byte budget.
	chunks, err := chunkObjectBodyKeys(keys)
	if err != nil {
		return err
	}

	// Read all object body chunks from one World revision.
	var worldSeqno uint64
	var haveWorldSeqno bool
	for _, chunk := range chunks {
		// Open the object body stream for this key chunk.
		strm, err := service.GetObjectBodiesBatch(ctx, &GetObjectBodiesBatchRequest{ObjectKeys: chunk})
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
					return &ObjectBodiesBatchRevisionError{
						Expected: worldSeqno,
						Got:      pageSeqno,
					}
				}

				// Copy the page's object bodies before invoking the callback.
				page := make([]*world.ObjectBody, len(resp.GetBodies()))
				for i, body := range resp.GetBodies() {
					page[i] = &world.ObjectBody{
						ObjectKey: body.GetObjectKey(),
						Body:      append([]byte(nil), body.GetBody()...),
						Exists:    body.GetExists(),
						Rev:       body.GetRev(),
					}
				}
				if err := cb(page); err != nil {
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

// GetObjectBodiesBatch collects all pages from a WorldState resource.
func GetObjectBodiesBatch(ctx context.Context, service ObjectBodiesBatchService, keys []string) ([]*world.ObjectBody, error) {
	for retry := 0; retry <= maxObjectBodiesBatchRevisionRetries; retry++ {
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
		var revisionErr *ObjectBodiesBatchRevisionError
		if !errors.As(err, &revisionErr) {
			return nil, err
		}
		revisionErr.Retries = retry
		if retry == maxObjectBodiesBatchRevisionRetries {
			return nil, revisionErr
		}
	}
	return nil, &ObjectBodiesBatchRevisionError{Retries: maxObjectBodiesBatchRevisionRetries}
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
