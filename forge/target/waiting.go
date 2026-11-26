package forge_target

import (
	"strconv"

	"github.com/pkg/errors"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
)

// NewPluginWaiting returns a wait for the named plugin's load.
func NewPluginWaiting(pluginID string) *Waiting {
	return &Waiting{Reason: &Waiting_PluginId{PluginId: pluginID}}
}

// NewCapacityWaiting returns a wait for capacity on the Worker.
func NewCapacityWaiting(workerObjectKey string, request *forge_runtime.ResourceRequest) *Waiting {
	return &Waiting{Reason: &Waiting_Capacity{Capacity: &CapacityWait{
		WorkerObjectKey: workerObjectKey,
		Request:         request,
	}}}
}

// Validate checks that the wait names its plugin or Worker. A nil wait or one
// with no reason is no wait and is valid.
func (w *Waiting) Validate() error {
	switch reason := w.GetReason().(type) {
	case *Waiting_PluginId:
		if reason.PluginId == "" {
			return errors.New("plugin_id cannot be empty")
		}
	case *Waiting_Capacity:
		if reason.Capacity.GetWorkerObjectKey() == "" {
			return errors.New("capacity worker_object_key cannot be empty")
		}
		return errors.Wrap(reason.Capacity.GetRequest().Validate(), "capacity request")
	}
	return nil
}

// Describe names what the wait needs and where, completing "Waiting for": "a
// graphics card on <Worker>", "2 graphics cards on <Worker>", or "CPU and
// memory on <Worker>" when the request asks for no cards.
func (c *CapacityWait) Describe() string {
	what := "CPU and memory"
	switch n := c.GetRequest().GetCards().GetCount(); {
	case n == 1:
		what = "a graphics card"
	case n > 1:
		what = strconv.FormatUint(uint64(n), 10) + " graphics cards"
	}
	return what + " on " + c.GetWorkerObjectKey()
}
