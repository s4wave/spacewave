package floodsub

import (
	"time"
)

var (
	HeartbeatInitialDelay = 100 * time.Millisecond
	HeartbeatInterval     = 1 * time.Second
	SubFanoutTTL          = 60 * time.Second
)

// Validate validates the configuration.
func (c *Config) Validate() error { return nil }
