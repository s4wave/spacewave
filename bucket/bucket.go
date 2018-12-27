package bucket

// Bucket is a bucket API handle.
type Bucket interface {
	// GetID returns the bucket ID.
	GetID() string
}
