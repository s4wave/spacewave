package forge_runtime

import "github.com/pkg/errors"

// ResourceRequest declares the host capacity and backend required by one execution attempt.
type ResourceRequest struct {
	// MilliCPU is the requested CPU in milli-cores.
	MilliCPU uint64
	// MemoryBytes is the requested memory in bytes.
	MemoryBytes uint64
	// Backend names the runtime backend required by the attempt.
	Backend string
}

// Validate validates the request.
func (r ResourceRequest) Validate() error {
	switch {
	case r.MilliCPU == 0:
		return errors.New("milli_cpu must be set")
	case r.MemoryBytes == 0:
		return errors.New("memory_bytes must be set")
	case r.Backend == "":
		return errors.New("backend must be set")
	}
	return nil
}
