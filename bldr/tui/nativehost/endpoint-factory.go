//go:build !js && !windows

package nativehost

import "context"

// EndpointFactory creates fresh endpoint handles for each child attempt.
type EndpointFactory func(context.Context) (*EndpointSet, error)
