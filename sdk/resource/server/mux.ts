import { createMux } from 'starpc'
import type { Mux, Handler } from 'starpc'

// newResourceMux creates a Mux with the given service handlers
// registered.
function newResourceMux(...handlers: Handler[]): Mux {
  const mux = createMux()
  for (const handler of handlers) {
    mux.register(handler)
  }
  return mux
}

export { newResourceMux }
