package psecho

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/hydra/block"
	"github.com/pkg/errors"
)

// ConfigID is the identifier for the config type.
const ConfigID = ControllerID

// defaultMaxConcurrentStreams is the default max outgoing streams per peer.
const defaultMaxConcurrentStreams = 4

// defaultPublishDebounceMs is the default debounce window in milliseconds.
const defaultPublishDebounceMs = 100

// defaultMaxBlockSize is the default maximum block size in bytes (10MB).
const defaultMaxBlockSize = block.MaxBlockSize

// defaultChunkSize is the default chunk size in bytes (1KB).
const defaultChunkSize = 1024

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.GetBucketId() == "" {
		return errors.New("bucket_id is required")
	}
	if c.GetPubsubChannelId() == "" {
		return errors.New("pubsub_channel_id is required")
	}
	if pid := c.GetPeerId(); pid != "" {
		if _, err := confparse.ParsePeerID(pid); err != nil {
			return errors.Wrap(err, "peer_id")
		}
	}
	return nil
}

// ParsePeerID parses the peer ID from config.
func (c *Config) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(c.GetPeerId())
}

// GetConfigID returns the unique string for this configuration type.
func (c *Config) GetConfigID() string {
	return ConfigID
}

// EqualsConfig checks if the config is equal to another.
func (c *Config) EqualsConfig(other config.Config) bool {
	return config.EqualsConfig[*Config](c, other)
}

// GetMaxConcurrentStreamsOrDefault returns the max concurrent streams or default.
func (c *Config) GetMaxConcurrentStreamsOrDefault() uint32 {
	if v := c.GetMaxConcurrentStreams(); v != 0 {
		return v
	}
	return defaultMaxConcurrentStreams
}

// GetPublishDebounceMsOrDefault returns the publish debounce ms or default.
func (c *Config) GetPublishDebounceMsOrDefault() uint32 {
	if v := c.GetPublishDebounceMs(); v != 0 {
		return v
	}
	return defaultPublishDebounceMs
}

// GetMaxBlockSizeOrDefault returns the max block size or default.
func (c *Config) GetMaxBlockSizeOrDefault() uint64 {
	if v := c.GetMaxBlockSize(); v != 0 {
		return v
	}
	return defaultMaxBlockSize
}

// GetChunkSizeOrDefault returns the chunk size or default.
func (c *Config) GetChunkSizeOrDefault() uint32 {
	if v := c.GetChunkSize(); v != 0 {
		return v
	}
	return defaultChunkSize
}

// _ is a type assertion
var _ config.Config = ((*Config)(nil))
