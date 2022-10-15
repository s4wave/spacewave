package rpc_bucket

import (
	"errors"
	"regexp"

	"github.com/aperturerobotics/bifrost/util/confparse"
)

// ErrReconcilerUnavailable is returned if the reconcile queues are not available.
var ErrReconcilerUnavailable = errors.New("reconciler queues are unavailable")

// ParseBucketIdRe parses the BucketIdRe field.
func (r *ListBucketInfoRequest) ParseBucketIdRe() (*regexp.Regexp, error) {
	return confparse.ParseRegexp(r.GetBucketIdRe())
}
