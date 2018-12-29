package bucket

// Bucket is a bucket API handle.
type Bucket interface {
	// GetID returns the bucket ID.
	GetID() string
}

// NewBucketInfo constructs a new bucket info with required fields.
func NewBucketInfo(conf *Config) *BucketInfo {
	if conf == nil {
		return nil
	}

	return &BucketInfo{
		Id:     conf.GetId(),
		Config: conf,
	}
}
