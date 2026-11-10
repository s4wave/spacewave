package kvtx_cayley_nodegraph

import (
	"encoding/binary"
)

// Key layout. Every node has an 8-byte big-endian ID, allocated from the
// tree's next ID, so a new node's keys sort together at the end of the tree.
// The next ID only grows: a deleted node's ID is never allocated again, so a
// key held by two trees names the same node and quad in both.
//
//	n/next                               -> next node ID
//	v/<value text>                       -> node ID
//	g/<node> 0x00                        -> NodeMeta
//	g/<node> 0x01 <pred> <object> [label] -> empty, one per quad
//	g/<node> 0x02 <pred> <subject> [label] -> empty, one per quad
//
// An edge key's suffix is 16 bytes, or 24 when the quad has a label.
const (
	// tagMeta tags a node's meta key.
	tagMeta byte = 0x00
	// tagOut tags the edges whose subject is the node.
	tagOut byte = 0x01
	// tagIn tags the edges whose object is the node.
	tagIn byte = 0x02

	// idLen is the length of a node ID.
	idLen = 8
	// nodeKeyLen is the length of g/<node>.
	nodeKeyLen = 2 + idLen
	// edgeKeyLen is the length of an edge key without a label.
	edgeKeyLen = nodeKeyLen + 1 + 2*idLen
)

var (
	// nextIDKey holds the next node ID, absent until the first node.
	nextIDKey = []byte("n/next")
	// valuePrefix prefixes the value to ID map.
	valuePrefix = []byte("v/")
	// nodePrefix prefixes the node ranges.
	nodePrefix = []byte("g/")
)

// valueKey returns the ID map key of a value's text.
func valueKey(text string) []byte {
	k := make([]byte, 0, len(valuePrefix)+len(text))
	k = append(k, valuePrefix...)
	return append(k, text...)
}

// nodeKey appends g/<id> to dst.
func nodeKey(dst []byte, id uint64) []byte {
	dst = append(dst, nodePrefix...)
	return binary.BigEndian.AppendUint64(dst, id)
}

// metaKey returns the meta key of a node.
func metaKey(id uint64) []byte {
	return append(nodeKey(make([]byte, 0, nodeKeyLen+1), id), tagMeta)
}

// rangeKey returns the prefix of a node's edges with tag, narrowed by the
// IDs that follow it in the key, such as a predicate and the other endpoint.
func rangeKey(id uint64, tag byte, ids ...uint64) []byte {
	k := append(nodeKey(make([]byte, 0, nodeKeyLen+1+len(ids)*idLen), id), tag)
	for _, v := range ids {
		k = binary.BigEndian.AppendUint64(k, v)
	}
	return k
}

// edgeKeys returns the out and in keys of a quad. A zero label is no label.
func edgeKeys(q quadRef) (out, in []byte) {
	if q.l == 0 {
		return rangeKey(q.s, tagOut, q.p, q.o), rangeKey(q.o, tagIn, q.p, q.s)
	}
	return rangeKey(q.s, tagOut, q.p, q.o, q.l), rangeKey(q.o, tagIn, q.p, q.s, q.l)
}

// parseNodeKey returns the node ID and tag of a key under g/.
func parseNodeKey(k []byte) (id uint64, tag byte, ok bool) {
	if len(k) < nodeKeyLen+1 || string(k[:2]) != string(nodePrefix) {
		return 0, 0, false
	}
	return binary.BigEndian.Uint64(k[2:nodeKeyLen]), k[nodeKeyLen], true
}

// parseEdgeKey returns the quad of an out or in key.
func parseEdgeKey(k []byte) (quadRef, bool) {
	// An edge key is a node key with two or three IDs after its tag.
	id, tag, ok := parseNodeKey(k)
	if !ok || (len(k) != edgeKeyLen && len(k) != edgeKeyLen+idLen) {
		return quadRef{}, false
	}

	// Read the predicate, the other endpoint and the label, if any.
	rest := k[nodeKeyLen+1:]
	pred := binary.BigEndian.Uint64(rest)
	other := binary.BigEndian.Uint64(rest[idLen:])
	var label uint64
	if len(rest) == 3*idLen {
		label = binary.BigEndian.Uint64(rest[2*idLen:])
	}

	// The tag says which endpoint the key's node is.
	switch tag {
	case tagOut:
		return quadRef{s: id, p: pred, o: other, l: label}, true
	case tagIn:
		return quadRef{s: other, p: pred, o: id, l: label}, true
	}
	return quadRef{}, false
}

// encodeID encodes a node ID as the ID map's value.
func encodeID(id uint64) []byte {
	return binary.BigEndian.AppendUint64(make([]byte, 0, idLen), id)
}

// decodeID decodes an ID map value.
func decodeID(v []byte) (uint64, bool) {
	if len(v) != idLen {
		return 0, false
	}
	return binary.BigEndian.Uint64(v), true
}
