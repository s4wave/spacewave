package floodsub

import (
	"time"
)

const (
	// seenMessageTTL is how long a seen message ID is remembered.
	seenMessageTTL = 120 * time.Second
)

// Validate validates the configuration.
func (c *Config) Validate() error { return nil }
