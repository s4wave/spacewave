package kernel

// File seals restrict the changes to an in-memory file, as on Linux. They
// belong to the file, not the descriptor. A memfd created with
// MFD_ALLOW_SEALING starts unsealed. Every other in-memory file holds
// sealSeal, so it reports that seal and takes no more.
const (
	// sealSeal refuses further seals.
	sealSeal = 0x1
	// sealShrink refuses a truncate that shrinks the file.
	sealShrink = 0x2
	// sealGrow refuses a write or truncate that grows the file.
	sealGrow = 0x4
	// sealWrite refuses writes and writable shared mappings.
	sealWrite = 0x8
	// sealFutureWrite refuses writes and new writable shared mappings, and
	// keeps the existing ones.
	sealFutureWrite = 0x10
	// sealExec refuses a change to the execute bits.
	sealExec = 0x20
	// sealAll holds every seal.
	sealAll = 0x3f
)

// sealable reports whether node can hold seals: an in-memory regular file.
func (node *VNode) sealable() bool {
	return node.mode&sIFMT == sIFREG && node.hostPath == "" && node.gen == nil
}

// checkWrite returns EPERM when a seal forbids writing n bytes at off, and
// zero otherwise.
func (node *VNode) checkWrite(off int64, n int) uint64 {
	if node.seals&(sealWrite|sealFutureWrite) != 0 || node.seals&sealGrow != 0 && off+int64(n) > node.size {
		return negErrno(EPERM)
	}
	return 0
}

// checkTruncate returns EPERM when a seal forbids resizing the file to size,
// and zero otherwise.
func (node *VNode) checkTruncate(size int64) uint64 {
	if node.seals&sealShrink != 0 && size < node.size || node.seals&sealGrow != 0 && size > node.size {
		return negErrno(EPERM)
	}
	return 0
}

// getSeals returns the seals of the file of, as fcntl's F_GET_SEALS.
func getSeals(of *OpenFile) uint64 {
	if of.node == nil || !of.node.sealable() {
		return negErrno(EINVAL)
	}
	return uint64(of.node.seals)
}

// addSeals adds seals to the file of, as fcntl's F_ADD_SEALS. The
// descriptor must be writable, and sealWrite waits for no writable shared
// mapping of the file. The caller holds the kernel lock.
func (k *Kernel) addSeals(of *OpenFile, seals uint32) uint64 {
	node := of.node
	switch {
	case node == nil || !node.sealable() || seals&^sealAll != 0:
		return negErrno(EINVAL)
	case of.flags&0o3 == oRDONLY || node.seals&sealSeal != 0:
		return negErrno(EPERM)
	case seals&sealWrite != 0 && k.writablyMapped(node):
		return negErrno(EBUSY)
	}
	node.seals |= seals
	return 0
}

// writablyMapped reports whether a process holds a writable shared mapping
// of node. The caller holds the kernel lock.
func (k *Kernel) writablyMapped(node *VNode) bool {
	for _, p := range k.procs.filter(func(p *Process) bool { return len(p.mappings) != 0 }) {
		for _, m := range p.mappings {
			if m.of.node == node {
				return true
			}
		}
	}
	return false
}
