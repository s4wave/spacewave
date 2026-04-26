//go:build !js

package resource_listener

import (
	"sync"

	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
)

// processBrokerOnce guards lazy construction of the process-wide
// yield broker. The listener controller and the Root resource server
// share a single broker so takeover prompts and reclaim signals
// synchronize across the two packages.
var (
	processBrokerOnce sync.Once
	processBroker     *yield_policy.Broker
)

// GetProcessYieldBroker returns the process-wide yield broker used by
// the listener controller and the Root resource server. On the first
// call it lazily constructs a broker with the default prompt timeout.
func GetProcessYieldBroker() *yield_policy.Broker {
	processBrokerOnce.Do(func() {
		processBroker = yield_policy.NewBroker()
	})
	return processBroker
}
