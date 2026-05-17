//go:build tinygo

package store

import (
	"testing"

	"github.com/s4wave/spacewave/db/block/bloom"
)

func TestBloomRefTinyGoAlwaysMisses(t *testing.T) {
	if bf := makeBloomRef(bloom.NewFilter(1, 0.1)).Value(); bf != nil {
		t.Fatal("tinygo bloomRef retained a filter")
	}
}
