package plugin_host_scheduler

import (
	"cmp"
	"context"
	"errors"
	"slices"

	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
)

// GetHostState returns host identities, a wait for this snapshot to change,
// and the host lookup failure. The scheduler owns the lookup and its lifetime.
func (c *Controller) GetHostState() ([]plugin_host.PluginHost, func(context.Context) error, error) {
	state := c.pluginHostsCtr.GetValue()
	wait := func(ctx context.Context) error {
		_, err := c.pluginHostsCtr.WaitValueChange(ctx, state, nil)
		return err
	}
	if state == nil {
		return nil, wait, nil
	}
	return slices.Clone(state.pluginHosts), wait, state.err
}

// publishHosts publishes one plugin host lookup result. Resolver errors are
// reported without withdrawing the live hosts they accompany.
func (c *Controller) publishHosts(resErr []error, hosts []plugin_host.PluginHost) {
	// Report resolver errors without dropping the host set.
	if len(resErr) != 0 {
		c.le.WithField("resolver-errs", resErr).Warn("one or more plugin hosts are erroring")
	}

	// Publish the hosts in the order the previous snapshot held them.
	hostSet := &pluginHostSet{
		pluginHosts: orderHosts(c.pluginHostsCtr.GetValue(), hosts),
		err:         errors.Join(resErr...),
	}
	ids := make([]string, 0, len(hostSet.pluginHosts))
	for _, host := range hostSet.pluginHosts {
		ids = append(ids, host.GetPlatformId())
	}
	slices.Sort(ids)
	unique := slices.Compact(slices.Clone(ids))
	c.le.WithField("plugin-hosts", unique).Infof("scheduling with %d plugin host(s)", len(unique))
	c.pluginHostsCtr.SetValue(hostSet)

	// Warn if several plugin hosts serve one platform; the first one runs it.
	if len(unique) < len(ids) {
		c.le.WithField("plugin-hosts", unique).Warn("detected multiple plugin hosts with the same platform id")
	}
}

// orderHosts returns hosts in the order prev held them, with new hosts last in
// lookup order. The lookup reports values in an order that changes when any
// host leaves, so a platform with several hosts keeps running on the host that
// has been present longest instead of whichever the lookup lists first.
func orderHosts(prev *pluginHostSet, hosts []plugin_host.PluginHost) []plugin_host.PluginHost {
	// Rank each host by its first position in the previous snapshot.
	ranks := make(map[plugin_host.PluginHost]int)
	var known int
	if prev != nil {
		known = len(prev.pluginHosts)
		for i := len(prev.pluginHosts) - 1; i >= 0; i-- {
			ranks[prev.pluginHosts[i]] = i
		}
	}

	// Place unranked hosts after every ranked one.
	rank := func(host plugin_host.PluginHost) int {
		if rank, ok := ranks[host]; ok {
			return rank
		}
		return known
	}
	ordered := slices.Clone(hosts)
	slices.SortStableFunc(ordered, func(a, b plugin_host.PluginHost) int {
		return cmp.Compare(rank(a), rank(b))
	})
	return ordered
}
