package bucket_store_rpc

import (
	"errors"
	"regexp"

	"github.com/s4wave/spacewave/net/util/confparse"
)

// ErrReconcilerUnavailable is returned if the reconcile queues are not available.
var ErrReconcilerUnavailable = errors.New("reconciler queues are unavailable")

// ParseBucketIdRe parses the BucketIdRe field.
func (r *ListBucketInfoRequest) ParseBucketIdRe() (*regexp.Regexp, error) {
	return confparse.ParseRegexp(r.GetBucketIdRe())
}
