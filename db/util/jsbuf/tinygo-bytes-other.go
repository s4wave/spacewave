//go:build js && !tinygo

package jsbuf

// WithTinyGoBytes runs call without installing a TinyGo byte handle.
func WithTinyGoBytes(_ []byte, call func(id uint32)) {
	call(0)
}

// HoldTinyGoBytes returns a zero TinyGo byte handle and a no-op release.
func HoldTinyGoBytes(_ []byte) (uint32, func()) {
	return 0, func() {}
}
