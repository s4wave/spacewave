package transport

import (
	"net/http"
	"sync/atomic"
	"time"
)

// clockTolerance is how far the local clock may disagree with the server's
// before signed requests claim the server's time instead. It sits well inside
// the server's five-minute window and far above the one-second resolution of
// the Date header that measures the disagreement.
const clockTolerance = time.Minute

// serverClock follows how far the server's clock runs from the local one, as
// the Date header on a signed request's answer shows, so a device whose clock
// is set wrong still signs at a time the server accepts. The server keeps its
// own replay window: the signer only claims the server's time. The zero value
// trusts the local clock until an answer shows otherwise, and is safe for
// concurrent use.
type serverClock struct {
	// local reads the local clock; nil reads time.Now.
	local func() time.Time
	// offset is the server's clock minus the local one in nanoseconds, or
	// zero while they agree within clockTolerance.
	offset atomic.Int64
}

// now returns the local time corrected to the server's clock.
func (c *serverClock) now() time.Time {
	return c.localNow().Add(time.Duration(c.offset.Load()))
}

// observe records the server's time from resp's Date header, and reports
// whether that changed the offset. An answer without a readable Date header
// changes nothing.
func (c *serverClock) observe(resp *http.Response) bool {
	// Measure the server's lead, treating a small one as agreement.
	date, err := http.ParseTime(resp.Header.Get("Date"))
	if err != nil {
		return false
	}
	offset := date.Sub(c.localNow())
	if offset.Abs() < clockTolerance {
		offset = 0
	}
	return c.offset.Swap(int64(offset)) != int64(offset)
}

// localNow reads the local clock.
func (c *serverClock) localNow() time.Time {
	if c.local != nil {
		return c.local()
	}
	return time.Now()
}
