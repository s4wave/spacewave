package plugin_space_runtime

import (
	"slices"
	"sync/atomic"

	"github.com/pkg/errors"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
)

// errPluginHostSetChanged ends a generation when the daemon plugin host set
// changes. The runtime restarts without backoff.
var errPluginHostSetChanged = errors.New("daemon plugin host set changed")

// hostWatch accepts LookupPluginHost snapshots for one generation and decides
// whether the generation must end.
type hostWatch struct {
	// hostReady receives the startup result of the first delivery.
	hostReady chan<- error
	// reportTerminal ends the generation with an error.
	reportTerminal func(error)
	// initial records that the first snapshot was accepted.
	initial atomic.Bool
	// hosts is the first accepted snapshot. It is written before hostReady
	// receives nil and never changes afterwards.
	hosts []plugin_host.PluginHost
	// accepted is the sorted platform ID multiset of hosts.
	accepted []string
}

// canonicalizeHostSet returns the sorted platform ID multiset of the hosts.
// Duplicates are retained: an unsupported duplicated member is a genuine
// membership change.
func canonicalizeHostSet(hosts []plugin_host.PluginHost) []string {
	ids := make([]string, 0, len(hosts))
	for _, host := range hosts {
		ids = append(ids, host.GetPlatformId())
	}
	slices.Sort(ids)
	return ids
}

// deliver applies one snapshot and returns errPluginHostSetChanged when the
// accepted set changed. Resolver errors fail startup before the first snapshot
// and end the generation afterwards; they never read as an empty host set.
func (w *hostWatch) deliver(resErr []error, hosts []plugin_host.PluginHost) error {
	if len(resErr) != 0 {
		w.fail(resErr[0])
		return nil
	}
	if w.initial.Load() {
		if !slices.Equal(w.accepted, canonicalizeHostSet(hosts)) {
			return errPluginHostSetChanged
		}
		return nil
	}
	w.hosts = slices.Clone(hosts)
	w.accepted = canonicalizeHostSet(hosts)
	w.initial.Store(true)
	w.signalReady(nil)
	return nil
}

// fail fails startup before the first snapshot and ends the generation
// afterwards.
func (w *hostWatch) fail(err error) {
	if !w.initial.Load() {
		w.signalReady(err)
		return
	}
	w.reportTerminal(errors.Wrap(err, "watch daemon plugin hosts"))
}

// signalReady reports the startup result. Only the first result is kept.
func (w *hostWatch) signalReady(err error) {
	select {
	case w.hostReady <- err:
	default:
	}
}
