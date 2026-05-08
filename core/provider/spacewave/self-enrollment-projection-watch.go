package provider_spacewave

// SelfEnrollmentProjectionWatch carries the backend wait channels used to keep
// the projection reactive.
type SelfEnrollmentProjectionWatch struct {
	AccountCh   <-chan struct{}
	RunCh       <-chan struct{}
	EntityKeyCh <-chan struct{}
}
