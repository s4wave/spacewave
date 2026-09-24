//go:build tinygo

package store

import (
	"github.com/s4wave/spacewave/db/block/bloom"
)

// bloomRef is an always-miss weak reference fallback.
//
// TinyGo's linker is missing weak.runtime_makeStrongFromWeak, so this build
// excludes weak behavior instead of retaining cached filters strongly in
// browser memory.
type bloomRef struct{}

// makeBloomRef drops bf from the weak cache fallback.
func makeBloomRef(_ *bloom.Filter) bloomRef {
	return bloomRef{}
}

// Value returns nil because TinyGo cannot use the weak cache.
func (r bloomRef) Value() *bloom.Filter {
	return nil
}
