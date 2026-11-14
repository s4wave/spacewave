package provider_transfer

import "time"

// SetSendTimeout sets how long cleanup waits for the target to receive the
// source's operations on a shared Space, and returns a restore.
func SetSendTimeout(d time.Duration) (restore func()) {
	prev := sendTimeout
	sendTimeout = d
	return func() { sendTimeout = prev }
}
