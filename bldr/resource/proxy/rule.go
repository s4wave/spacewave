package resource_proxy

// Rule is what the proxy does with a call when its upstream is replaced.
type Rule uint8

const (
	// End ends the call with a retryable error, so the client opens it
	// again. It is the rule of every method Rules does not name, so an
	// unclassified call behaves as it would without the proxy.
	End Rule = iota
	// Reopen keeps a server stream open to the client and opens it again
	// upstream with the same request. The first upstream message is dropped
	// when its bytes equal the last one forwarded. A method gets Reopen only
	// when the first message of a new stream is safe for a client holding
	// the old stream's state to apply: a whole state that replaces what the
	// client holds, or entries the client merges by key.
	Reopen
	// Retry holds a unary call until an upstream answers it or the caller's
	// deadline passes, sending it again on each new upstream. A method gets
	// Retry only when applying it twice has the effect of applying it once.
	Retry
	// Once holds a unary call that has not been sent, and ends one that was
	// sent but not answered with a retryable error, so the client's own retry
	// decides whether it applies again.
	Once
	// Open creates a child resource, whose ID the response carries in field
	// 1. The proxy hands the client an ID of its own for the child and, on
	// each new upstream, sends the call again after its parent's, before any
	// other call reaches the child. A refused replay releases the client's
	// handle. Until the call is answered it is held like Retry, so a method
	// gets Open only when a second open leaves nothing behind the first did
	// not.
	Open
)

// String returns the rule's name.
func (r Rule) String() string {
	switch r {
	case Reopen:
		return "REOPEN"
	case Retry:
		return "RETRY"
	case Once:
		return "ONCE"
	case Open:
		return "OPEN"
	default:
		return "END"
	}
}
