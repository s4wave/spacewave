import type { Client as SRPCClient } from 'starpc'

// AttachedResource is a client-provided resource accessible by server-side RPC
// handlers as either a raw attached client or an attached resource-tree ref.
interface AttachedResource {
  label: string
  client: SRPCClient
  signal: AbortSignal
  controller: AbortController
  release?: () => void
}

export type { AttachedResource }
