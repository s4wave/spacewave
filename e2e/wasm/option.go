//go:build !js

package wasm

import (
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	e2e_wasm_session "github.com/s4wave/spacewave/e2e/wasm/session"
)

// Option configures the Harness.
type Option func(*options)

type options struct {
	repoRoot       string
	headless       *bool
	configMutators []func(*bldr_project.ProjectConfig) error
}

// WithRepoRoot overrides automatic repo root discovery.
func WithRepoRoot(root string) Option {
	return func(o *options) {
		o.repoRoot = root
	}
}

// WithHeadless controls whether the browser runs headless (default true).
func WithHeadless(headless bool) Option {
	return func(o *options) {
		o.headless = &headless
	}
}

// WithConfigMutator registers a function that mutates the loaded project
// config before the project controller starts. Use this to inject test-only
// controller wiring such as trace service entries.
func WithConfigMutator(fn func(*bldr_project.ProjectConfig) error) Option {
	return func(o *options) {
		o.configMutators = append(o.configMutators, fn)
	}
}

// WithSessionHarness injects the session harness controller into the
// plugin WASM processes for test orchestration (peer info, signaling
// relay, link establishment).
func WithSessionHarness() Option {
	return WithConfigMutator(e2e_wasm_session.InjectSessionHarnessConfig)
}
