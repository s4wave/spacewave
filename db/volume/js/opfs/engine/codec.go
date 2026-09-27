package engine

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
)

const (
	// formatVersion identifies the clean immutable volume format.
	formatVersion = 4
	// maxKeyBytes bounds comparison, routing, and catalogue page sizes.
	maxKeyBytes = 4096
	// MaxValueBytes preserves the supported maximum block and metadata value.
	MaxValueBytes = 2 << 20
	// runTargetBytes bounds ordinary sorted runs; one full record may exceed it.
	runTargetBytes = 256 << 10
	// maxRunBytes includes a maximum-sized record and its key and framing.
	maxRunBytes = MaxValueBytes + maxKeyBytes + 128
	// partitionRunLimit bounds every lookup and complete-overlap merge.
	partitionRunLimit = 4
	// pageFanout bounds one immutable catalogue page.
	pageFanout = 32
	// maxBatchBytes bounds retained transaction mutations.
	maxBatchBytes = 4 << 20
	// maxBatchRecords bounds catalogue paths changed by one transaction.
	maxBatchRecords = 4096
)

var (
	// ErrCorrupt reports an invalid committed file without resetting data.
	ErrCorrupt = errors.New("invalid immutable volume file")
	// ErrLimit reports an input exceeding a documented engine bound.
	ErrLimit = errors.New("immutable volume input exceeds limit")
	// ErrClosed reports work attempted after engine shutdown.
	ErrClosed = errors.New("immutable volume is closed")
)

// message is the generated protobuf contract for persistent records.
type message interface {
	// MarshalVT encodes the generated durable record.
	MarshalVT() ([]byte, error)
	// UnmarshalVT decodes bytes after the framing checksum has been validated.
	UnmarshalVT([]byte) error
}

// encode checksums a generated persistent record.
func encode(value message) ([]byte, error) {
	data, err := value.MarshalVT()
	if err != nil {
		return nil, err
	}
	return binary.LittleEndian.AppendUint32(data, crc32.ChecksumIEEE(data)), nil
}

// decode validates framing before decoding a persistent record.
func decode(data []byte, value message) error {
	if len(data) < 4 {
		return ErrCorrupt
	}
	body := data[:len(data)-4]
	if crc32.ChecksumIEEE(body) != binary.LittleEndian.Uint32(data[len(body):]) {
		return ErrCorrupt
	}
	if err := value.UnmarshalVT(body); err != nil {
		return errors.Join(ErrCorrupt, err)
	}
	return nil
}
