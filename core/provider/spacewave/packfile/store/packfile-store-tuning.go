package store

type engineTuningOverrides struct {
	pageSizeSet            bool
	pageSize               int
	minWindowSet           bool
	minWindow              int
	transportQuantumSet    bool
	transportQuantum       int
	maxWindowSet           bool
	maxWindow              int
	targetHzSet            bool
	targetHz               float64
	smoothingSet           bool
	smoothing              float64
	sparseReadsSet         bool
	sparseReads            bool
	sparseColdWindow       int
	sparseLocalityDistance int64
	indexPromotionSet      bool
	indexPromotion         bool
}

func (o engineTuningOverrides) apply(e *PackReader) {
	if o.pageSizeSet {
		e.SetTransportPageSize(o.pageSize)
	}
	if o.minWindowSet {
		e.SetTransportMinWindow(o.minWindow)
	}
	if o.transportQuantumSet {
		e.SetTransportQuantum(o.transportQuantum)
	}
	if o.maxWindowSet {
		e.SetTransportMaxWindow(o.maxWindow)
	}
	if o.targetHzSet {
		e.SetTransportTargetRequestHz(o.targetHz)
	}
	if o.smoothingSet {
		e.SetTransportWindowSmoothing(o.smoothing)
	}
	if o.sparseReadsSet {
		e.SetSparseReadTuning(o.sparseReads, o.sparseColdWindow, o.sparseLocalityDistance)
	}
	if o.indexPromotionSet {
		e.SetIndexPromotionEnabled(o.indexPromotion)
	}
}

// SetTransportPageSize sets the resident span page size on all engines.
func (s *PackfileStore) SetTransportPageSize(pageSize int) {
	if pageSize <= 0 {
		return
	}
	s.mu.Lock()
	s.tuningOverrides.pageSizeSet = true
	s.tuningOverrides.pageSize = pageSize
	engines := s.snapshotEnginesLocked()
	s.mu.Unlock()
	for _, e := range engines {
		e.SetTransportPageSize(pageSize)
	}
}

// SetTransportMinWindow sets the minimum transport fetch size on all engines.
func (s *PackfileStore) SetTransportMinWindow(minWindow int) {
	if minWindow <= 0 {
		return
	}
	s.mu.Lock()
	s.tuningOverrides.minWindowSet = true
	s.tuningOverrides.minWindow = minWindow
	engines := s.snapshotEnginesLocked()
	s.mu.Unlock()
	for _, e := range engines {
		e.SetTransportMinWindow(minWindow)
	}
}

// SetTransportQuantum sets the transport alignment quantum on all engines.
func (s *PackfileStore) SetTransportQuantum(quantum int) {
	if quantum <= 0 {
		return
	}
	s.mu.Lock()
	s.tuningOverrides.transportQuantumSet = true
	s.tuningOverrides.transportQuantum = quantum
	engines := s.snapshotEnginesLocked()
	s.mu.Unlock()
	for _, e := range engines {
		e.SetTransportQuantum(quantum)
	}
}

// SetTransportMaxWindow sets the maximum transport fetch size on all engines.
func (s *PackfileStore) SetTransportMaxWindow(maxWindow int) {
	if maxWindow <= 0 {
		return
	}
	s.mu.Lock()
	s.tuningOverrides.maxWindowSet = true
	s.tuningOverrides.maxWindow = maxWindow
	engines := s.snapshotEnginesLocked()
	s.mu.Unlock()
	for _, e := range engines {
		e.SetTransportMaxWindow(maxWindow)
	}
}

// SetTransportTargetRequestHz sets the request-rate target on all engines.
func (s *PackfileStore) SetTransportTargetRequestHz(targetHz float64) {
	if targetHz <= 0 {
		return
	}
	s.mu.Lock()
	s.tuningOverrides.targetHzSet = true
	s.tuningOverrides.targetHz = targetHz
	engines := s.snapshotEnginesLocked()
	s.mu.Unlock()
	for _, e := range engines {
		e.SetTransportTargetRequestHz(targetHz)
	}
}

// SetTransportWindowSmoothing sets the upward growth smoothing factor on all engines.
func (s *PackfileStore) SetTransportWindowSmoothing(smoothing float64) {
	s.mu.Lock()
	s.tuningOverrides.smoothingSet = true
	s.tuningOverrides.smoothing = smoothing
	engines := s.snapshotEnginesLocked()
	s.mu.Unlock()
	for _, e := range engines {
		e.SetTransportWindowSmoothing(smoothing)
	}
}

// SetSparseReadTuning configures first-touch sparse range planning on all engines.
func (s *PackfileStore) SetSparseReadTuning(enabled bool, coldWindow int, localityDistance int64) {
	s.mu.Lock()
	s.tuningOverrides.sparseReadsSet = true
	s.tuningOverrides.sparseReads = enabled
	s.tuningOverrides.sparseColdWindow = coldWindow
	s.tuningOverrides.sparseLocalityDistance = localityDistance
	engines := s.snapshotEnginesLocked()
	s.mu.Unlock()
	for _, e := range engines {
		e.SetSparseReadTuning(enabled, coldWindow, localityDistance)
	}
}

// SetIndexPromotionEnabled sets whether resident spans auto-promote covered blocks.
func (s *PackfileStore) SetIndexPromotionEnabled(enabled bool) {
	s.mu.Lock()
	s.tuningOverrides.indexPromotionSet = true
	s.tuningOverrides.indexPromotion = enabled
	engines := s.snapshotEnginesLocked()
	s.mu.Unlock()
	for _, e := range engines {
		e.SetIndexPromotionEnabled(enabled)
	}
}

func (s *PackfileStore) snapshotEnginesLocked() []*PackReader {
	engines := make([]*PackReader, 0, len(s.engines))
	for _, e := range s.engines {
		engines = append(engines, e)
	}
	return engines
}
