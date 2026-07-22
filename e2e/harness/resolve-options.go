//go:build !js

package harness

// ResolveOptions configures the shared artifact resolution policy.
type ResolveOptions struct {
	LockDir      string
	LockName     string
	RequireFresh bool
}
