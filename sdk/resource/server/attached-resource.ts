import type { Client as SRPCClient } from 'starpc'

// AttachedResource is a client-provided resource accessible by
// server-side RPC handlers via getAttachedRef(id).
interface AttachedResource {
  label: string
  client: SRPCClient
  signal: AbortSignal
  controller: AbortController
}

export type { AttachedResource }
