package bifrost

import (
	"sync"

	"github.com/sirupsen/logrus"
)

// subscriptionCounter counts subscription packets received by real peer streams.
type subscriptionCounter struct {
	// mtx guards packets.
	mtx sync.Mutex
	// packets counts received packets containing subscription changes.
	packets int
}

// Levels selects debug messages emitted by the FloodSub peer reader.
func (c *subscriptionCounter) Levels() []logrus.Level {
	return []logrus.Level{logrus.DebugLevel}
}

// Fire counts peer packets containing subscription changes.
func (c *subscriptionCounter) Fire(entry *logrus.Entry) error {
	if entry.Message == "received message from peer" {
		if count, ok := entry.Data["subscription-count"].(int); ok && count != 0 {
			c.mtx.Lock()
			c.packets++
			c.mtx.Unlock()
		}
	}
	return nil
}

// Count returns the received subscription packet count.
func (c *subscriptionCounter) Count() int {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.packets
}

// _ is a type assertion.
var _ logrus.Hook = (*subscriptionCounter)(nil)
