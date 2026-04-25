package resource_root

import "sync"

// WebListenerKeepaliveFunc acquires daemon lifetime for a background listener.
type WebListenerKeepaliveFunc func(listenerID string) func()

var webListenerKeepalive struct {
	mu sync.Mutex
	fn WebListenerKeepaliveFunc
}

// SetWebListenerKeepaliveFunc installs the process-local web listener keepalive hook.
func SetWebListenerKeepaliveFunc(fn WebListenerKeepaliveFunc) func() {
	webListenerKeepalive.mu.Lock()
	prev := webListenerKeepalive.fn
	webListenerKeepalive.fn = fn
	webListenerKeepalive.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			webListenerKeepalive.mu.Lock()
			webListenerKeepalive.fn = prev
			webListenerKeepalive.mu.Unlock()
		})
	}
}

func acquireWebListenerKeepalive(listenerID string) func() {
	webListenerKeepalive.mu.Lock()
	fn := webListenerKeepalive.fn
	webListenerKeepalive.mu.Unlock()
	if fn == nil {
		return nil
	}
	return fn(listenerID)
}
