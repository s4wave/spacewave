package store

// setTransportFetchMaxBytes caps every transport fetch at the platform limit.
func (e *PackReader) setTransportFetchMaxBytes(maxBytes int) {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if maxBytes > 0 {
			e.transportFetchMaxBytes = maxBytes
			e.normalizeTransportLocked()
		}
	})
}

func (e *PackReader) normalizeTransportLocked() {
	if e.minWindow <= 0 {
		e.minWindow = defaultTransportMinWindow
	}
	if e.transportQuantum <= 0 {
		e.transportQuantum = e.minWindow
	}
	if e.transportFetchMaxBytes > 0 {
		if e.minWindow > e.transportFetchMaxBytes {
			e.minWindow = e.transportFetchMaxBytes
		}
		if e.transportQuantum > e.transportFetchMaxBytes {
			e.transportQuantum = e.transportFetchMaxBytes
		}
		if e.maxWindow <= 0 || e.maxWindow > e.transportFetchMaxBytes {
			e.maxWindow = e.transportFetchMaxBytes
		}
	}
	minMaxWindow := max(e.minWindow, e.transportQuantum)
	if e.maxWindow > 0 && e.maxWindow < minMaxWindow {
		e.maxWindow = minMaxWindow
	}
	e.currentWindow = e.clampWindow(e.currentWindow)
}
