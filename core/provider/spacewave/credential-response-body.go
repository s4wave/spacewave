package provider_spacewave

import "io"

// credentialResponseBody retains a request's revocation binding through body reads.
// It leaves concurrent Read and Close behavior with the HTTP response body.
type credentialResponseBody struct {
	// ReadCloser supplies the response bytes and their network lifetime.
	io.ReadCloser
	// release detaches the completed request from its credential generation.
	release func()
}

// Close releases request cancellation before closing the network response body.
func (b *credentialResponseBody) Close() error {
	// Cancel outstanding reads before asking the HTTP transport to close them.
	b.release()
	return b.ReadCloser.Close()
}

var _ io.ReadCloser = (*credentialResponseBody)(nil)
