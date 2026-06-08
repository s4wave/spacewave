//go:build !goscript

package store_kvtx

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// ResolveHashType resolves the store config hash choice.
func (c *Config) ResolveHashType() hash.HashType {
	if c != nil {
		if hashType := c.GetHashType(); hashType != 0 {
			return hashType
		}
	}
	return block.DefaultHashType
}
