//go:build !js

package harness

// Generation pairs a store-owned immutable publication token with its artifact.
type Generation[A any] struct {
	Token    string
	Artifact A
}
