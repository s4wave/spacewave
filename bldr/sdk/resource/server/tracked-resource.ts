import type { Mux } from 'starpc'

// TrackedResource tracks a resource registered with a client.
interface TrackedResource {
  mux: Mux
  ownerClientID: number
  releaseFn: (() => void) | undefined
}

export type { TrackedResource }
