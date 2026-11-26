package forge_runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

const workerCapacityObjectKeyPrefix = "forge/runtime/capacity/"

// BuildWorkerCapacityObjectKey builds the deterministic capacity object key
// for one Forge Worker. The Worker process owns this record; Forge validates
// and debits it, callers never keep a parallel ledger.
func BuildWorkerCapacityObjectKey(workerObjectKey string) string {
	hash := sha256.Sum256([]byte(workerObjectKey))
	return workerCapacityObjectKeyPrefix + hex.EncodeToString(hash[:12])
}

// Valid reports whether the state is a defined value.
func (s CapacityOwnerState) Valid() bool {
	_, ok := CapacityOwnerState_name[int32(s)]
	return ok
}

// SupportsBackend reports whether the Worker declares the backend.
func (w *WorkerCapacity) SupportsBackend(backend string) bool {
	return slices.Contains(w.GetBackends(), backend)
}

// Admits reports whether the Worker can take request now and chooses its
// cards: the first free matching cards in id order. It returns
// ErrBackendUnsupported; ErrCapacityNeverFits, naming the shortfall, when the
// declared totals could never hold the request; or ErrCapacityExhausted when
// the request fits the totals but live reservations hold what it needs. The
// owner claim is checked separately.
func (w *WorkerCapacity) Admits(request *ResourceRequest) ([]string, error) {
	// Refuse a backend the Worker does not run.
	if !w.SupportsBackend(request.GetBackend()) {
		return nil, errors.Wrapf(ErrBackendUnsupported, "backend %q", request.GetBackend())
	}

	// Refuse a request the declared totals could never hold.
	cards := request.GetCards()
	count := int(cards.GetCount())
	matching := w.matchingCardIDs(cards)
	switch {
	case request.GetMilliCpu() > w.GetMilliCpuTotal():
		return nil, errors.Wrapf(ErrCapacityNeverFits, "requests %d milli-CPU, Worker offers %d", request.GetMilliCpu(), w.GetMilliCpuTotal())
	case request.GetMemoryBytes() > w.GetMemoryBytesTotal():
		return nil, errors.Wrapf(ErrCapacityNeverFits, "requests %d memory bytes, Worker offers %d", request.GetMemoryBytes(), w.GetMemoryBytesTotal())
	case len(matching) < count:
		return nil, errors.Wrapf(ErrCapacityNeverFits, "requests %s, Worker offers %d", describeCardRequest(cards), len(matching))
	}

	// Wait while live reservations hold the CPU, memory or cards it needs.
	if remainingMilliCPU(w) < request.GetMilliCpu() || remainingMemoryBytes(w) < request.GetMemoryBytes() {
		return nil, ErrCapacityExhausted
	}
	reserved := idSet(w.GetReservedCardIds())
	matching = slices.DeleteFunc(matching, func(id string) bool {
		_, held := reserved[id]
		return held
	})
	if len(matching) < count {
		return nil, errors.Wrapf(ErrCapacityExhausted, "%d of %s free", len(matching), describeCardRequest(cards))
	}
	return matching[:count], nil
}

// matchingCardIDs returns the ids of the offered cards that meet the card
// request's vendor and memory, sorted.
func (w *WorkerCapacity) matchingCardIDs(request *s4wave_device.GraphicsCardRequest) []string {
	// A request for no cards matches none.
	var ids []string
	if request.GetCount() == 0 {
		return ids
	}

	// Keep each offered card of the requested vendor and memory.
	for _, card := range w.GetCards() {
		vendor := request.GetVendor()
		if vendor != s4wave_device.GraphicsCardVendor_GRAPHICS_CARD_VENDOR_UNKNOWN && card.GetVendor() != vendor {
			continue
		}
		if card.GetMemoryBytes() < request.GetMinMemoryBytes() {
			continue
		}
		ids = append(ids, card.GetId())
	}
	slices.Sort(ids)
	return ids
}

// describeCardRequest names a card request for a refusal or a wait, such as
// "1 AMD card with at least 8589934592 bytes".
func describeCardRequest(request *s4wave_device.GraphicsCardRequest) string {
	// Name the count and vendor, then the memory floor when there is one.
	vendor := "graphics"
	if v := request.GetVendor(); v != s4wave_device.GraphicsCardVendor_GRAPHICS_CARD_VENDOR_UNKNOWN {
		vendor = strings.TrimPrefix(v.String(), "GRAPHICS_CARD_VENDOR_")
	}
	desc := strconv.FormatUint(uint64(request.GetCount()), 10) + " " + vendor + " card"
	if request.GetCount() != 1 {
		desc += "s"
	}
	if minMemory := request.GetMinMemoryBytes(); minMemory != 0 {
		desc += " with at least " + strconv.FormatUint(minMemory, 10) + " bytes"
	}
	return desc
}

// Validate validates the capacity record. Every owner field must be set, so
// a half-written claim can never decode as available.
func (w *WorkerCapacity) Validate() error {
	// An ACTIVE record never over-commits; a DRAINING record may hold debits
	// above shrunk declared totals until credits land or the claim deletes it.
	if w.OwnerState == CapacityOwnerState_CAPACITY_OWNER_STATE_ACTIVE {
		switch {
		case w.MilliCpuReserved > w.MilliCpuTotal:
			return errors.New("reserved milli_cpu exceeds total")
		case w.MemoryBytesReserved > w.MemoryBytesTotal:
			return errors.New("reserved memory_bytes exceeds total")
		}
	}
	if err := w.validateCards(); err != nil {
		return err
	}
	if w.Generation == 0 {
		return errors.New("generation must be set")
	}
	if err := w.ObservedAt.Validate(false); err != nil {
		return errors.Wrap(err, "observed_at")
	}
	switch {
	case w.WorkerObjectKey == "":
		return errors.New("worker_object_key must be set")
	case w.OwnerDeviceObjectKey == "":
		return errors.New("owner_device_object_key must be set")
	case w.ClaimId == "":
		return errors.New("claim_id must be set")
	case w.OwnerEpoch == 0:
		return errors.New("owner_epoch must be set")
	case w.OwnerState == CapacityOwnerState_CAPACITY_OWNER_STATE_UNKNOWN || !w.OwnerState.Valid():
		return errors.New("invalid owner_state")
	}
	if err := w.OwnerLeaseExpiresAt.Validate(false); err != nil {
		return errors.Wrap(err, "owner_lease_expires_at")
	}
	return nil
}

// HoldsDebit reports whether the record's reserved totals include the
// reservation's debit: its CPU, its memory and each of its cards.
func (w *WorkerCapacity) HoldsDebit(res *Reservation) bool {
	request := res.GetRequest()
	if w.GetMilliCpuReserved() < request.GetMilliCpu() || w.GetMemoryBytesReserved() < request.GetMemoryBytes() {
		return false
	}
	return containsAll(idSet(w.GetReservedCardIds()), res.GetCardIds())
}

// fitsTotals reports whether the reserved debits fit the declared totals: CPU,
// memory, and every reserved card still offered.
func (w *WorkerCapacity) fitsTotals() bool {
	if w.MilliCpuReserved > w.MilliCpuTotal || w.MemoryBytesReserved > w.MemoryBytesTotal {
		return false
	}
	return containsAll(w.offeredCardIDs(), w.GetReservedCardIds())
}

// offeredCardIDs returns the set of declared card ids.
func (w *WorkerCapacity) offeredCardIDs() map[string]struct{} {
	offered := make(map[string]struct{}, len(w.GetCards()))
	for _, card := range w.GetCards() {
		offered[card.GetId()] = struct{}{}
	}
	return offered
}

// idSet returns the set of ids.
func idSet(ids []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

// containsAll reports whether set holds every id.
func containsAll(set map[string]struct{}, ids []string) bool {
	for _, id := range ids {
		if _, ok := set[id]; !ok {
			return false
		}
	}
	return true
}

// validateCards checks the declared cards and the reserved card ids. Every
// card has a unique id, a vendor and its device nodes, and no id is reserved
// twice. An ACTIVE record reserves only cards it offers; a DRAINING record may
// hold the id of a card that left the host until its reservation credits it.
func (w *WorkerCapacity) validateCards() error {
	// Require each declared card to be complete and declared once.
	offered := make(map[string]struct{}, len(w.GetCards()))
	for _, card := range w.GetCards() {
		if err := card.Validate(); err != nil {
			return errors.Wrapf(err, "card %q", card.GetId())
		}
		if _, dup := offered[card.GetId()]; dup {
			return errors.Errorf("card %q declared twice", card.GetId())
		}
		offered[card.GetId()] = struct{}{}
	}

	// Require each reserved card to be reserved once, and offered while ACTIVE.
	reserved := make(map[string]struct{}, len(w.GetReservedCardIds()))
	for _, id := range w.GetReservedCardIds() {
		if _, dup := reserved[id]; dup {
			return errors.Errorf("card %q reserved twice", id)
		}
		reserved[id] = struct{}{}
		if _, ok := offered[id]; !ok && w.OwnerState == CapacityOwnerState_CAPACITY_OWNER_STATE_ACTIVE {
			return errors.Errorf("reserved card %q is not offered", id)
		}
	}
	return nil
}

// OwnerClaimActive reports whether the record carries a live owner claim at
// the given time: lease unexpired and state ACTIVE. It returns typed sentinel
// errors so callers can distinguish expired and draining records without
// string matching.
func (w *WorkerCapacity) OwnerClaimActive(now time.Time) error {
	switch {
	case w.OwnerLeaseExpiresAt == nil || !now.Before(w.OwnerLeaseExpiresAt.AsTime()):
		return ErrCapacityOwnerExpired
	case w.OwnerState != CapacityOwnerState_CAPACITY_OWNER_STATE_ACTIVE:
		return ErrCapacityDraining
	}
	return nil
}

// LookupWorkerCapacity loads one Worker capacity record or ErrWorkerNotObserved.
func LookupWorkerCapacity(ctx context.Context, ws world.WorldState, workerObjectKey string) (*WorkerCapacity, error) {
	capacity, err := lookupWorkerCapacityBody(ctx, ws, BuildWorkerCapacityObjectKey(workerObjectKey))
	if errors.Is(err, world.ErrObjectNotFound) {
		return nil, ErrWorkerNotObserved
	}
	if err != nil {
		return nil, err
	}
	return capacity, nil
}

// lookupWorkerCapacityBody loads one capacity record body by object key.
func lookupWorkerCapacityBody(ctx context.Context, ws world.WorldState, objKey string) (*WorkerCapacity, error) {
	return lookupRecord(ctx, ws, objKey, unmarshalLegacyWorkerCapacity)
}

// _ is a type assertion
var _ block.Block = (*WorkerCapacity)(nil)
