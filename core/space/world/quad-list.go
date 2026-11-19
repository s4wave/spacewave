package space_world

import (
	"bufio"
	"cmp"
	"context"
	"encoding/binary"
	"io"

	"github.com/aperturerobotics/protobuf-go-lite/encoding/protowire"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	bquad "github.com/s4wave/spacewave/db/block/quad"
)

// A quad list is the set of graph quads a RegenerateWorldOp carries: one
// blob of block/quad.Quad messages, each behind its uvarint length, in
// CompareQuads order without repeats. The operation names the list by one
// block root, so every replica reads the same bytes and adds the same quads.

// maxQuadListEntry bounds one encoded quad in a list, so a damaged length
// prefix cannot make the reader allocate without limit.
const maxQuadListEntry = 1 << 20

// WriteQuadList writes the quads rangeQuads yields as the blob at bcs, which
// becomes the list's one root. rangeQuads must yield in CompareQuads order
// without repeats; a list out of order is refused.
//
// rangeQuads runs on its own goroutine, which only it uses, and feeds the
// blob builder through a pipe, so memory holds one quad and the builder's
// chunk; WriteQuadList returns only after rangeQuads has returned.
func WriteQuadList(ctx context.Context, bcs *block.Cursor, rangeQuads func(cb func(*bquad.Quad) error) error) error {
	// Encode each quad into the pipe as the range yields it.
	pr, pw := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		var prev *bquad.Quad
		var entry []byte
		pw.CloseWithError(rangeQuads(func(q *bquad.Quad) error {
			// Refuse a list a reader would merge out of order.
			if prev != nil && CompareQuads(prev, q) >= 0 {
				return errors.Errorf("quad list is not sorted at %s %s %s", q.GetSubject(), q.GetPredicate(), q.GetObj())
			}
			prev = q
			encoded, err := q.MarshalVT()
			if err != nil {
				return err
			}
			entry = append(protowire.AppendVarint(entry[:0], uint64(len(encoded))), encoded...)
			_, err = pw.Write(entry)
			return err
		}))
	}()

	// Build the blob from the stream, then stop and wait for the range.
	_, err := blob.BuildBlobWithReader(ctx, pr, bcs, nil)
	pr.CloseWithError(errors.New("quad list writer stopped"))
	<-done
	return err
}

// RangeQuadList calls cb with each quad of the list at bcs, in order, reading
// the blob one chunk at a time, and stops at the first error cb returns.
func RangeQuadList(ctx context.Context, bcs *block.Cursor, cb func(*bquad.Quad) error) error {
	// Read the blob as a stream of length-prefixed entries.
	rdr, err := blob.NewReader(ctx, bcs)
	if err != nil {
		return err
	}
	defer rdr.Close()
	br := bufio.NewReader(rdr)

	// Decode each entry until the blob ends at an entry boundary.
	for {
		size, err := binary.ReadUvarint(br)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.Wrap(err, "read quad list entry length")
		}
		if size > maxQuadListEntry {
			return errors.Errorf("quad list entry of %d bytes exceeds %d", size, maxQuadListEntry)
		}
		encoded := make([]byte, size)
		if _, err := io.ReadFull(br, encoded); err != nil {
			return errors.Wrap(err, "read quad list entry")
		}
		q := &bquad.Quad{}
		if err := q.UnmarshalVT(encoded); err != nil {
			return errors.Wrap(err, "decode quad list entry")
		}
		if err := cb(q); err != nil {
			return err
		}
	}
}

// CompareQuads orders quads by subject object key, predicate, target key and
// label, each compared as bytes without the IRI brackets the graph stores. Key
// order differs from stored-string order: key "a" sorts before "a-b", while
// "<a>" sorts after "<a-b>". The list needs only an order that is deterministic
// and that every writer agrees on; key order lets a writer emit it from an
// object walk while sorting one object's quads at a time.
func CompareQuads(a, b *bquad.Quad) int {
	return cmp.Or(
		cmp.Compare(unbracket(a.GetSubject()), unbracket(b.GetSubject())),
		cmp.Compare(unbracket(a.GetPredicate()), unbracket(b.GetPredicate())),
		cmp.Compare(unbracket(a.GetObj()), unbracket(b.GetObj())),
		cmp.Compare(a.GetLabel(), b.GetLabel()),
	)
}

// unbracket returns an IRI graph value's IRI, or any other value unchanged.
func unbracket(value string) string {
	if len(value) >= 2 && value[0] == '<' && value[len(value)-1] == '>' {
		return value[1 : len(value)-1]
	}
	return value
}
