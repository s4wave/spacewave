// Package store_kvkey provides common key patterns for key/value stores.
package store_kvkey

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Prefix:                       []byte("hydra/"),
		BucketConfigPrefix:           []byte("bucket/config/"),
		BucketReconcilerMqueuePrefix: []byte("bucket/reconciler-mqueue/"),
		PeerPrivKey:                  []byte("peer-priv"),
	}
}

// Validate performs cursory validation.
func (c *Config) Validate() error {
	// TODO
	// note: c == nil is valid
	return nil
}
