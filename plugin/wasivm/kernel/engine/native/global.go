package native

import (
	"github.com/tetratelabs/wazero/api"

	"github.com/s4wave/spacewave/plugin/wasivm/kernel/engine"
)

// Global is an exported wazero guest global.
type Global struct {
	// g is the wazero global.
	g api.Global
}

// Get returns the current value.
func (g *Global) Get() uint64 {
	return g.g.Get()
}

// Set changes the value. Set has no effect on an immutable global.
func (g *Global) Set(val uint64) {
	if mg, ok := g.g.(api.MutableGlobal); ok {
		mg.Set(val)
	}
}

// _ is a type assertion
var _ engine.Global = (*Global)(nil)
