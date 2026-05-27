//go:build !goscript

package kvtx_block_okra

import (
	"encoding/binary"
	"math"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/zeebo/blake3"
)

var okraHasherPool = sync.Pool{
	New: func() any {
		return blake3.New()
	},
}

func borrowOkraHasher() *blake3.Hasher {
	h := okraHasherPool.Get().(*blake3.Hasher)
	h.Reset()
	return h
}

func releaseOkraHasher(h *blake3.Hasher) {
	h.Reset()
	okraHasherPool.Put(h)
}

func finishOkraHash(h *blake3.Hasher) []byte {
	sum := h.Sum(nil)
	return sum[:HashSize]
}

func okraDigest(parts ...[]byte) ([]byte, error) {
	h := borrowOkraHasher()
	defer releaseOkraHasher(h)
	for _, part := range parts {
		if _, err := h.Write(part); err != nil {
			return nil, err
		}
	}
	return finishOkraHash(h), nil
}

func hashKeyValue(key, value []byte) ([]byte, error) {
	if uint64(len(key)) > math.MaxUint32 || uint64(len(value)) > math.MaxUint32 {
		return nil, errors.New("okra hash input exceeds uint32 length")
	}
	var size [4]byte
	h := borrowOkraHasher()
	defer releaseOkraHasher(h)
	binary.BigEndian.PutUint32(size[:], uint32(len(key)))
	if _, err := h.Write(size[:]); err != nil {
		return nil, err
	}
	if _, err := h.Write(key); err != nil {
		return nil, err
	}
	binary.BigEndian.PutUint32(size[:], uint32(len(value)))
	if _, err := h.Write(size[:]); err != nil {
		return nil, err
	}
	if _, err := h.Write(value); err != nil {
		return nil, err
	}
	return finishOkraHash(h), nil
}

func hashLeaf(key []byte, valueRef *block.BlockRef, valueIsBlob bool) ([]byte, error) {
	var refData []byte
	var err error
	if valueRef != nil {
		refData, err = valueRef.MarshalVT()
		if err != nil {
			return nil, err
		}
	}
	valueData := make([]byte, 1+len(refData))
	if valueIsBlob {
		valueData[0] = 1
	}
	copy(valueData[1:], refData)
	return hashKeyValue(key, valueData)
}

func hashNodeRange(nodes []*buildNode) ([]byte, error) {
	h := borrowOkraHasher()
	defer releaseOkraHasher(h)
	for _, node := range nodes {
		if _, err := h.Write(node.entry.GetHash()); err != nil {
			return nil, err
		}
	}
	return finishOkraHash(h), nil
}

func hashEntryRange(entries []*Entry) ([]byte, error) {
	h := borrowOkraHasher()
	defer releaseOkraHasher(h)
	for _, ent := range entries {
		if _, err := h.Write(ent.GetHash()); err != nil {
			return nil, err
		}
	}
	return finishOkraHash(h), nil
}

func hashPage(page *Page) ([]byte, error) {
	h := borrowOkraHasher()
	defer releaseOkraHasher(h)
	var buf [8]byte
	binary.BigEndian.PutUint32(buf[:4], page.GetLevel())
	if _, err := h.Write(buf[:4]); err != nil {
		return nil, err
	}
	if page.GetStartsAtAnchor() {
		buf[0] = 1
	} else {
		buf[0] = 0
	}
	if _, err := h.Write(buf[:1]); err != nil {
		return nil, err
	}
	for _, part := range [][]byte{page.GetLowerBound(), page.GetUpperBound()} {
		if uint64(len(part)) > math.MaxUint32 {
			return nil, errors.New("okra page bound exceeds uint32 length")
		}
		binary.BigEndian.PutUint32(buf[:4], uint32(len(part)))
		if _, err := h.Write(buf[:4]); err != nil {
			return nil, err
		}
		if _, err := h.Write(part); err != nil {
			return nil, err
		}
	}
	binary.BigEndian.PutUint64(buf[:], page.GetSize())
	if _, err := h.Write(buf[:]); err != nil {
		return nil, err
	}
	for _, ent := range page.GetEntries() {
		if ent.GetAnchor() {
			buf[0] = 1
		} else {
			buf[0] = 0
		}
		if _, err := h.Write(buf[:1]); err != nil {
			return nil, err
		}
		if uint64(len(ent.GetKey())) > math.MaxUint32 {
			return nil, errors.New("okra entry key exceeds uint32 length")
		}
		binary.BigEndian.PutUint32(buf[:4], uint32(len(ent.GetKey())))
		if _, err := h.Write(buf[:4]); err != nil {
			return nil, err
		}
		if _, err := h.Write(ent.GetKey()); err != nil {
			return nil, err
		}
		if _, err := h.Write(ent.GetHash()); err != nil {
			return nil, err
		}
		binary.BigEndian.PutUint64(buf[:], ent.GetSize())
		if _, err := h.Write(buf[:]); err != nil {
			return nil, err
		}
	}
	return finishOkraHash(h), nil
}

func isBoundary(nodeHash []byte) bool {
	if len(nodeHash) < 4 {
		return false
	}
	limit := (uint64(1) << 32) / FanoutDegree
	return uint64(binary.BigEndian.Uint32(nodeHash[:4])) < limit
}
