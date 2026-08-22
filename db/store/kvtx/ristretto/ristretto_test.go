package store_kvtx_ristretto

import (
	"testing"
	"time"
)

// TestNewStoreWithCacheKeepsTTL tests that the configured TTL is kept.
func TestNewStoreWithCacheKeepsTTL(t *testing.T) {
	s := NewStoreWithCache(nil, time.Second)
	if s.ttl != time.Second {
		t.Fatalf("expected ttl 1s, got: %v", s.ttl)
	}
}
