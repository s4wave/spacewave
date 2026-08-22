package forge_runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strconv"

	"github.com/aperturerobotics/fastjson"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

const workerCapacityObjectKeyPrefix = "forge/runtime/capacity/"

// BuildWorkerCapacityObjectKey builds the deterministic capacity object key
// for one Forge Worker. The Worker daemon owns this record; Forge validates
// and debits it, callers never keep a parallel ledger.
func BuildWorkerCapacityObjectKey(workerObjectKey string) string {
	hash := sha256.Sum256([]byte(workerObjectKey))
	return workerCapacityObjectKeyPrefix + hex.EncodeToString(hash[:12])
}

// NewWorkerCapacityBlock constructs an empty WorkerCapacity block.
func NewWorkerCapacityBlock() block.Block {
	return &WorkerCapacity{}
}

// WorkerCapacity is the observed and reserved capacity for one Forge Worker.
type WorkerCapacity struct {
	// MilliCPUTotal is the observed total CPU in milli-cores.
	MilliCPUTotal uint64
	// MilliCPUReserved is the CPU currently debited by live reservations.
	MilliCPUReserved uint64
	// MemoryBytesTotal is the observed total memory in bytes.
	MemoryBytesTotal uint64
	// MemoryBytesReserved is the memory currently debited by live reservations.
	MemoryBytesReserved uint64
	// Backends lists the runtime backends the Worker supports.
	Backends []string
	// ObservedAt is when the Worker reported the totals.
	ObservedAt *timestamp.Timestamp
	// Generation increments on every mutation of this record so observers can
	// fence stale capacity views.
	Generation uint64
}

// SupportsBackend reports whether the Worker declares the backend.
func (w *WorkerCapacity) SupportsBackend(backend string) bool {
	return slices.Contains(w.Backends, backend)
}

// Validate validates the capacity record.
func (w *WorkerCapacity) Validate() error {
	switch {
	case w.MilliCPUReserved > w.MilliCPUTotal:
		return errors.New("reserved milli_cpu exceeds total")
	case w.MemoryBytesReserved > w.MemoryBytesTotal:
		return errors.New("reserved memory_bytes exceeds total")
	case w.Generation == 0:
		return errors.New("generation must be set")
	}
	if err := w.ObservedAt.Validate(false); err != nil {
		return errors.Wrap(err, "observed_at")
	}
	return nil
}

// Reset resets the block.
func (w *WorkerCapacity) Reset() {
	*w = WorkerCapacity{}
}

// MarshalBlock marshals the block to binary.
func (w *WorkerCapacity) MarshalBlock() ([]byte, error) {
	return w.MarshalJSON()
}

// UnmarshalBlock unmarshals the block from binary.
func (w *WorkerCapacity) UnmarshalBlock(data []byte) error {
	return w.UnmarshalJSON(data)
}

// MarshalJSON marshals the WorkerCapacity to JSON without reflection.
func (w *WorkerCapacity) MarshalJSON() ([]byte, error) {
	var arena fastjson.Arena
	obj := arena.NewObject()
	obj.Set("milliCpuTotal", arena.NewNumberString(strconv.FormatUint(w.MilliCPUTotal, 10)))
	obj.Set("milliCpuReserved", arena.NewNumberString(strconv.FormatUint(w.MilliCPUReserved, 10)))
	obj.Set("memoryBytesTotal", arena.NewNumberString(strconv.FormatUint(w.MemoryBytesTotal, 10)))
	obj.Set("memoryBytesReserved", arena.NewNumberString(strconv.FormatUint(w.MemoryBytesReserved, 10)))
	backends := arena.NewArray()
	for i, b := range w.Backends {
		backends.SetArrayItem(i, arena.NewString(b))
	}
	obj.Set("backends", backends)
	tsValue, err := marshalTimestampField(&arena, w.ObservedAt)
	if err != nil {
		return nil, err
	}
	obj.Set("observedAt", tsValue)
	obj.Set("generation", arena.NewNumberString(strconv.FormatUint(w.Generation, 10)))
	return obj.MarshalTo(nil), nil
}

// UnmarshalJSON unmarshals the WorkerCapacity from JSON without reflection.
func (w *WorkerCapacity) UnmarshalJSON(data []byte) error {
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		return err
	}
	if value.Type() == fastjson.TypeNull {
		*w = WorkerCapacity{}
		return nil
	}
	if value.Type() != fastjson.TypeObject {
		return errors.New("worker capacity must be object")
	}
	w.MilliCPUTotal = value.GetUint64("milliCpuTotal")
	w.MilliCPUReserved = value.GetUint64("milliCpuReserved")
	w.MemoryBytesTotal = value.GetUint64("memoryBytesTotal")
	w.MemoryBytesReserved = value.GetUint64("memoryBytesReserved")
	w.Backends = nil
	for _, b := range value.GetArray("backends") {
		w.Backends = append(w.Backends, string(b.GetStringBytes()))
	}
	w.ObservedAt, err = unmarshalTimestampField(value, "observedAt")
	if err != nil {
		return err
	}
	w.Generation = value.GetUint64("generation")
	return nil
}

// LookupWorkerCapacity loads one Worker capacity record or ErrWorkerNotObserved.
func LookupWorkerCapacity(ctx context.Context, ws world.WorldState, workerObjectKey string) (*WorkerCapacity, error) {
	capacity, err := world.LookupObjectBody[*WorkerCapacity](
		ctx,
		ws,
		BuildWorkerCapacityObjectKey(workerObjectKey),
		NewWorkerCapacityBlock,
	)
	if errors.Is(err, world.ErrObjectNotFound) {
		return nil, ErrWorkerNotObserved
	}
	if err != nil {
		return nil, err
	}
	return capacity, nil
}

var _ block.Block = (*WorkerCapacity)(nil)
