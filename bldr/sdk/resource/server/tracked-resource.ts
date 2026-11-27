import type { Mux } from 'starpc'
import type { ResourceOffers } from './resource-offers.js'

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
  // offer owns this handle until adoption or release consumes it.
  offer?: ResourceOffers
}

export type { TrackedResource }
