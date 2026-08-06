import type { Mux } from 'starpc'

// TrackedResource tracks a resource registered with a client.
interface TrackedResource {
  mux: Mux
  ownerClientID: number
  releaseFn: (() => void) | undefined
  parentResourceID: number | undefined
  pendingSince: number | undefined
  serviceID: string | undefined
  methodID: string | undefined
  adopted: boolean
}

export type { TrackedResource }
