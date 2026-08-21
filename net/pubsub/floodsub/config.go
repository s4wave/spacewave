package floodsub

import (
	"time"
)

const (
	// HeartbeatInitialDelay is the delay before the first heartbeat.
	HeartbeatInitialDelay = 100 * time.Millisecond
	// HeartbeatInterval is the interval between heartbeats.
	HeartbeatInterval = 1 * time.Second
	// SubFanoutTTL is how long subscription fanout state is retained.
	SubFanoutTTL = 60 * time.Second
)

// Validate validates the configuration.
func (c *Config) Validate() error { return nil }
