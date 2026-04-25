package provider

import "errors"

var (
	// ErrEmptyResourceID is returned if the resource id was empty.
	ErrEmptyResourceID = errors.New("resource id cannot be empty")
	// ErrEmptyProviderID is returned if the provider id was empty.
	ErrEmptyProviderID = errors.New("provider id cannot be empty")
	// ErrEmptyProviderAccountID is returned if the provider id was empty.
	ErrEmptyProviderAccountID = errors.New("provider account id cannot be empty")
	// ErrUnimplementedProviderFeature is returned when a provider feature is not implemented.
	ErrUnimplementedProviderFeature = errors.New("provider feature unimplemented")
	// ErrProviderFeatureMetaSizeExceeded is returned if a size limit is exceeded.
	ErrProviderFeatureMetaSizeExceeded = errors.New("maximum provider feature metadata size exceeded")
)
