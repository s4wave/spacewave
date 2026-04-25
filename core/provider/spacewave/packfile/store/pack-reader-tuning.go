package store

import "time"

// PackReaderTuning describes the active engine tuning values.
type PackReaderTuning struct {
	PageSize               int
	MinWindow              int
	TransportQuantum       int
	MaxWindow              int
	CurrentWindow          int
	TargetRequestHz        float64
	Smoothing              float64
	SparseReads            bool
	SparseColdWindow       int
	SparseLocalityDistance int64
	ResidentBudget         int64
	WritebackWindow        int64
	IndexPromotion         bool
}

// SetTransportPageSize sets the resident span page size.
func (e *PackReader) SetTransportPageSize(pageSize int) {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if pageSize > 0 {
			e.pageSize = pageSize
		}
	})
}

// SetTransportMinWindow sets the minimum transport fetch size.
func (e *PackReader) SetTransportMinWindow(minWindow int) {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if minWindow > 0 {
			e.minWindow = minWindow
			e.normalizeTransportLocked()
		}
	})
}

// SetTransportQuantum sets the transport alignment quantum.
func (e *PackReader) SetTransportQuantum(quantum int) {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if quantum > 0 {
			e.transportQuantum = quantum
			e.normalizeTransportLocked()
		}
	})
}

// SetTransportMaxWindow sets the maximum transport fetch size.
func (e *PackReader) SetTransportMaxWindow(maxWindow int) {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if maxWindow > 0 {
			e.maxWindow = maxWindow
			e.normalizeTransportLocked()
		}
	})
}

// SetTransportTargetRequestHz sets the steady-state request-rate target.
func (e *PackReader) SetTransportTargetRequestHz(targetHz float64) {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if targetHz > 0 {
			e.targetInterval = time.Duration(float64(time.Second) / targetHz)
		}
	})
}

// SetTransportWindowSmoothing sets the upward window growth smoothing factor.
func (e *PackReader) SetTransportWindowSmoothing(smoothing float64) {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		e.smoothing = min(max(smoothing, 0), 1)
	})
}

// SetSparseReadTuning configures first-touch sparse payload range planning.
func (e *PackReader) SetSparseReadTuning(enabled bool, coldWindow int, localityDistance int64) {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		e.sparseReads = enabled
		if coldWindow > 0 {
			e.sparseColdWindow = coldWindow
		}
		if localityDistance > 0 {
			e.sparseLocalityDistance = localityDistance
		}
	})
}

// SetIndexPromotionEnabled sets whether resident spans auto-promote covered blocks.
func (e *PackReader) SetIndexPromotionEnabled(enabled bool) {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		e.indexPromotion = enabled
	})
}

// SnapshotTuning returns the active engine tuning values.
func (e *PackReader) SnapshotTuning() PackReaderTuning {
	var snap PackReaderTuning
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		snap = PackReaderTuning{
			PageSize:               e.pageSize,
			MinWindow:              e.minWindow,
			TransportQuantum:       e.transportQuantum,
			MaxWindow:              e.maxWindow,
			CurrentWindow:          e.currentWindow,
			TargetRequestHz:        e.targetRequestHzLocked(),
			Smoothing:              e.smoothing,
			SparseReads:            e.sparseReads,
			SparseColdWindow:       e.sparseColdWindow,
			SparseLocalityDistance: e.sparseLocalityDistance,
			ResidentBudget:         e.maxBytes,
			WritebackWindow:        e.writebackWindow,
			IndexPromotion:         e.indexPromotion,
		}
	})
	return snap
}

func (e *PackReader) normalizeTransportLocked() {
	if e.minWindow <= 0 {
		e.minWindow = defaultTransportMinWindow
	}
	if e.transportQuantum <= 0 {
		e.transportQuantum = e.minWindow
	}
	minMaxWindow := max(e.minWindow, e.transportQuantum)
	if e.maxWindow > 0 && e.maxWindow < minMaxWindow {
		e.maxWindow = minMaxWindow
	}
	e.currentWindow = e.clampWindow(e.currentWindow)
}

func (e *PackReader) targetRequestHzLocked() float64 {
	if e.targetInterval <= 0 {
		return 0
	}
	return float64(time.Second) / float64(e.targetInterval)
}
