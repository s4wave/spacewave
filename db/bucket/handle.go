package bucket

// bucketHandle implements a bucket handle with a block store and kvkey.
type bucketHandle struct {
	bucketID string
	bucket   Bucket
}

// NewBucketHandle constructs a new bucket handle.
func NewBucketHandle(bucketID string, bkt Bucket) BucketHandle {
	return &bucketHandle{
		bucketID: bucketID,
		bucket:   bkt,
	}
}

// GetID returns the bucket ID.
func (h *bucketHandle) GetID() string {
	return h.bucketID
}

// GetExists returns if the bucket exists. If false, the bucket does not
// exist in the store, and all block calls will not work.
func (h *bucketHandle) GetExists() bool {
	return h.bucket != nil
}

// GetBucketConfig returns the bucket configuration in use.
// May be nil if the bucket does not exist in the store.
func (h *bucketHandle) GetBucketConfig() *Config {
	return h.bucket.GetBucketConfig()
}

// GetBucket returns the bucket object.
// May be nil if the bucket does not exist in the store.
func (h *bucketHandle) GetBucket() Bucket {
	return h.bucket
}

// _ is a type assertion
var _ BucketHandle = ((*bucketHandle)(nil))
