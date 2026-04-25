import { useCallback, useEffect, useRef, useState } from 'react'
import { LuRadar } from 'react-icons/lu'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Root } from '@s4wave/sdk/root'
import {
  CacheSeedInspectorClient,
  type CacheSeedInspector,
} from '@s4wave/core/provider/spacewave/cacheseed/cacheseed_srpc.pb.js'
import {
  CacheSeedEntry,
  GetCacheSeedReasonsRequest,
} from '@s4wave/core/provider/spacewave/cacheseed/cacheseed.pb.js'

// MAX_ENTRIES caps the UI scrollback so long-running sessions do not grow
// unbounded. Matches the server-side default ring buffer capacity.
const MAX_ENTRIES = 1024

export interface CacheSeedTabProps {
  rootResource: Resource<Root>
}

// CacheSeedTab streams the provider's cache-seed inspector RPC entries and
// renders them as a rolling list. Only populates when the provider binary is
// built with the =alphadebug= build tag; otherwise the stream returns an
// unknown-service error which is surfaced inline.
export function CacheSeedTab({ rootResource }: CacheSeedTabProps) {
  const root = rootResource.value
  const [entries, setEntries] = useState<CacheSeedEntry[]>([])
  const [error, setError] = useState<string | null>(null)
  const abortRef = useRef<AbortController | null>(null)

  const start = useCallback(
    (signal: AbortSignal) => {
      if (!root) return
      const svc: CacheSeedInspector = new CacheSeedInspectorClient(root.client)
      ;(async () => {
        try {
          const stream = svc.GetCacheSeedReasons(
            GetCacheSeedReasonsRequest.create({}),
            signal,
          )
          for await (const entry of stream) {
            setEntries((prev) => {
              const next = prev.concat(entry)
              if (next.length > MAX_ENTRIES) {
                next.splice(0, next.length - MAX_ENTRIES)
              }
              return next
            })
          }
        } catch (err) {
          if (signal.aborted) return
          setError(String(err))
        }
      })()
    },
    [root],
  )

  useEffect(() => {
    const ctrl = new AbortController()
    abortRef.current = ctrl
    setEntries([])
    setError(null)
    start(ctrl.signal)
    return () => ctrl.abort()
  }, [start])

  if (error) {
    return (
      <div className="text-foreground-alt/60 p-3 text-xs">
        <div className="flex items-center gap-2">
          <LuRadar className="h-4 w-4" />
          <span>Cache-seed inspector unavailable.</span>
        </div>
        <div className="mt-1 opacity-60">{error}</div>
        <div className="mt-1 opacity-60">
          Rebuild the provider with the <code>alphadebug</code> build tag.
        </div>
      </div>
    )
  }

  if (entries.length === 0) {
    return (
      <div className="text-foreground-alt/50 flex flex-1 items-center justify-center p-4 text-xs">
        Waiting for cache-seed entries...
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col overflow-auto font-mono text-xs">
      {entries.map((entry, i) => (
        <div key={i} className="border-border/40 flex gap-2 border-b px-2 py-1">
          <span className="text-foreground-alt/60 w-28">
            {new Date(Number(entry.timestampMs)).toLocaleTimeString()}
          </span>
          <span className="text-accent w-40">
            {entry.reason || '(untagged)'}
          </span>
          <span className="flex-1 truncate">{entry.path}</span>
        </div>
      ))}
    </div>
  )
}
