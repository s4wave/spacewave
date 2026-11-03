import { useCallback, useEffect, useRef, useState } from 'react'
import { LuRadar } from '@s4wave/web/images/icons.js'

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

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
  utilityState as themeUtilityState,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: { opacity: 0.6, marginTop: themeLayout['--spacing'] },
  rule5: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    flex: 1,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule6: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    flex: 1,
    overflow: 'auto',
    display: 'flex',
    flexDirection: 'column',
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule7: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-border']} 40%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule8: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    width: `calc(${themeLayout['--spacing']} * 28)`,
  },
  rule9: {
    color: themeColors['--color-accent'],
    width: `calc(${themeLayout['--spacing']} * 40)`,
  },
  rule10: {
    flex: 1,
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
})

// MAX_ENTRIES caps the UI scrollback so long-running sessions do not grow
// unbounded. Matches the server-side default ring buffer capacity.
const MAX_ENTRIES = 1024

function formatCacheSeedTime(
  timestampMs: number | string | bigint | undefined,
): string {
  if (timestampMs === undefined) return ''
  return new Date(Number(timestampMs)).toLocaleTimeString()
}

export interface CacheSeedTabProps {
  rootResource: Resource<Root>
}

// CacheSeedTab streams the provider's cache-seed inspector RPC entries and
// renders them as a rolling list. Only populates when the provider binary is
// built with the =alphadebug= build tag; otherwise the stream returns an
// unknown-service error which is surfaced inline.
export function CacheSeedTab({
  sx,
  rootResource,
}: CacheSeedTabProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const root = rootResource.value
  const [entries, setEntries] = useState<CacheSeedEntry[]>([])
  const [error, setError] = useState<string | null>(null)
  const abortRef = useRef<AbortController | null>(null)

  const start = useCallback(
    (signal: AbortSignal) => {
      if (!root) return
      const svc: CacheSeedInspector = new CacheSeedInspectorClient(root.client)
      void (async () => {
        try {
          await Promise.resolve()
          if (signal.aborted) return
          setEntries([])
          setError(null)
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
    start(ctrl.signal)
    return () => ctrl.abort()
  }, [start])

  if (error) {
    return (
      <div sx={[stylexStyles.rule1, sx]}>
        <div sx={stylexStyles.rule2}>
          <LuRadar sx={stylexStyles.rule3} />
          <span>Cache-seed inspector unavailable.</span>
        </div>
        <div sx={stylexStyles.rule4}>{error}</div>
        <div sx={stylexStyles.rule4}>
          Rebuild the provider with the <code>alphadebug</code> build tag.
        </div>
      </div>
    )
  }

  if (entries.length === 0) {
    return (
      <div sx={[stylexStyles.rule5, sx]}>Waiting for cache-seed entries…</div>
    )
  }

  return (
    <div sx={[stylexStyles.rule6, sx]}>
      {entries.map((entry) => (
        <div
          key={`${entry.timestampMs}-${entry.reason}-${entry.path}`}
          sx={stylexStyles.rule7}
        >
          <span suppressHydrationWarning sx={stylexStyles.rule8}>
            {formatCacheSeedTime(entry.timestampMs)}
          </span>
          <span sx={stylexStyles.rule9}>{entry.reason || '(untagged)'}</span>
          <span sx={stylexStyles.rule10}>{entry.path}</span>
        </div>
      ))}
    </div>
  )
}
