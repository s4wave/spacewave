// Package store_kvkey provides common key patterns for key/value stores.
package store_kvkey

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Prefix:             []byte("hydra/"),
		BucketConfigPrefix: []byte("bucket-config/"),
		PeerPrivKey:        []byte("peer-priv"),
	}
}

// Validate performs cursory validation.
func (c *Config) Validate() error {
	return nil
}
