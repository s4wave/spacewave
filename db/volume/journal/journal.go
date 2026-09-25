// Package journal encodes the garbage collection journal entries volume
// engines store.
package journal

import (
	"encoding/binary"

	"github.com/pkg/errors"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// Marshal encodes one journal entry of reference graph adds and removes.
func Marshal(adds, removes []block_gc.RefEdge) []byte {
	return appendEdges(appendEdges(nil, adds), removes)
}

// Unmarshal decodes a journal entry written by Marshal.
func Unmarshal(b []byte) (adds, removes []block_gc.RefEdge, err error) {
	adds, b, err = readEdges(b)
	if err != nil {
		return nil, nil, err
	}
	removes, _, err = readEdges(b)
	if err != nil {
		return nil, nil, err
	}
	return adds, removes, nil
}

// appendEdges appends a count and each edge's subject and object, every
// value length prefixed with an unsigned varint.
func appendEdges(b []byte, edges []block_gc.RefEdge) []byte {
	b = binary.AppendUvarint(b, uint64(len(edges)))
	for _, e := range edges {
		b = binary.AppendUvarint(b, uint64(len(e.Subject)))
		b = append(b, e.Subject...)
		b = binary.AppendUvarint(b, uint64(len(e.Object)))
		b = append(b, e.Object...)
	}
	return b
}

// readEdges decodes edges written by appendEdges and returns the rest of b.
func readEdges(b []byte) ([]block_gc.RefEdge, []byte, error) {
	n, size := binary.Uvarint(b)
	if size <= 0 || n > uint64(len(b)) {
		return nil, nil, errors.New("invalid journal entry")
	}
	b = b[size:]
	edges := make([]block_gc.RefEdge, n)
	for i := range edges {
		var fields [2]string
		for j := range fields {
			l, size := binary.Uvarint(b)
			if size <= 0 || l > uint64(len(b)-size) { //nolint:gosec
				return nil, nil, errors.New("invalid journal entry")
			}
			end := size + int(l) //nolint:gosec
			fields[j] = string(b[size:end])
			b = b[end:]
		}
		edges[i] = block_gc.RefEdge{Subject: fields[0], Object: fields[1]}
	}
	return edges, b, nil
}
