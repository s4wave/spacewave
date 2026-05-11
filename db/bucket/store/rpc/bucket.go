package bucket_store_rpc

import (
	"regexp"

	"github.com/s4wave/spacewave/net/util/confparse"
)

// ParseBucketIdRe parses the BucketIdRe field.
func (r *ListBucketInfoRequest) ParseBucketIdRe() (*regexp.Regexp, error) {
	return confparse.ParseRegexp(r.GetBucketIdRe())
}
