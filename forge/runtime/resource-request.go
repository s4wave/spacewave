package forge_runtime

import "github.com/pkg/errors"

// Validate validates the request.
func (r *ResourceRequest) Validate() error {
	switch {
	case r.GetMilliCpu() == 0:
		return errors.New("milli_cpu must be set")
	case r.GetMemoryBytes() == 0:
		return errors.New("memory_bytes must be set")
	case r.GetBackend() == "":
		return errors.New("backend must be set")
	}
	return errors.Wrap(r.GetCards().Validate(), "cards")
}
