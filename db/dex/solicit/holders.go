package dex_solicit

import (
	"maps"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/peer"
)

// SetHolders sets the peers other than this device that may hold the
// controller's blocks. A read then reports a block missing only when each
// holder answered that it lacks the block, and unavailable, naming the rest,
// otherwise. Nil holders means they are unknown, and a miss is what the
// connected peers answered. An empty set means no other peer holds the blocks.
func (c *Controller) SetHolders(holders []peer.ID) {
	// Index the holders, keeping nil for unknown holders.
	var next map[string]struct{}
	if holders != nil {
		next = make(map[string]struct{}, len(holders))
		for _, holder := range holders {
			next[holder.String()] = struct{}{}
		}
	}

	// Replace the set and wake waiting reads when it changed.
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if (c.holders == nil) == (next == nil) && maps.Equal(c.holders, next) {
			return
		}
		c.holders = next
		broadcast()
	})
}

// LinkHolder records that holder proved its identity over a link to the
// transport peer, or, when up is false, that one such link ended. A holder that
// never opens a link itself, such as a device's storage identity, answers
// through the DEX session with that transport peer. Each call with up set is
// paired with one later call without it.
func (c *Controller) LinkHolder(transport, holder peer.ID, up bool) {
	t, h := transport.String(), holder.String()
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Count the link up under its transport peer.
		bound := c.links[t]
		if up {
			if bound == nil {
				bound = make(map[string]int)
				if c.links == nil {
					c.links = make(map[string]map[string]int)
				}
				c.links[t] = bound
			}
			bound[h]++
			broadcast()
			return
		}

		// Count the link down, dropping the binding with its last link.
		if bound[h] == 0 {
			return
		}
		bound[h]--
		if bound[h] == 0 {
			delete(bound, h)
		}
		if len(bound) == 0 {
			delete(c.links, t)
		}
		broadcast()
	})
}

// verdict classifies a fanout's answers against the holders: the block when a
// session found it, nil when each holder answered that it lacks it, and an
// error naming the holders that did not answer otherwise. A holder answers
// through a session with itself or with a transport peer it is linked through.
// With unknown holders it returns what the connected peers answered.
func (c *Controller) verdict(ref *block.BlockRef, answers fanoutAnswers) (*DexMessage, error) {
	// A verified hit answers the read whoever served it.
	if answers.found != nil {
		return answers.found, nil
	}

	// List the holders no session answered for.
	var unanswered []string
	var known bool
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Leave unknown holders to the asked peers' answers.
		if c.holders == nil {
			return
		}

		// Mark each peer that missed and the holders linked through it.
		known = true
		answered := make(map[string]struct{}, len(answers.missed))
		for _, remote := range answers.missed {
			answered[remote] = struct{}{}
			for holder := range c.links[remote] {
				answered[holder] = struct{}{}
			}
		}

		// Collect the holders left unmarked.
		for holder := range c.holders {
			if _, ok := answered[holder]; !ok {
				unanswered = append(unanswered, holder)
			}
		}
	})

	// Report a miss each holder confirmed, or the holders that did not answer.
	if !known {
		return answers.result(ref)
	}
	if len(unanswered) == 0 {
		return nil, nil
	}
	slices.Sort(unanswered)
	return nil, &block.UnavailableError{Ref: ref, Holders: unanswered, Cause: answers.err}
}

// holdersNone reports that the holders are known and empty, so no peer can
// serve a block this device lacks. Call it under the broadcast lock.
func (c *Controller) holdersNone() bool {
	return c.holders != nil && len(c.holders) == 0
}
