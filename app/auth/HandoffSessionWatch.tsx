import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { LuCircleX, LuTimerOff } from '@s4wave/web/images/icons.js'

import { useParams } from '@s4wave/web/router/router.js'
import { useCloudProviderConfig } from '@s4wave/app/provider/spacewave/useSpacewaveAuth.js'
import {
  clearStoredHandoffPayload,
  clientTypeLabel,
  decodeHandoffRequest,
  getStoredHandoffRequest,
} from './handoff-state.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    backgroundColor: themeColors['--color-background-landing'],
    padding: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule2: {
    position: 'relative',
    zIndex: 10,
    display: 'flex',
    maxWidth: 'var(--container-sm)',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
  },
  rule3: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 16)`,
    height: `calc(${themeLayout['--spacing']} * 16)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule4: {
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
  },
  rule5: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    color: themeColors['--color-foreground-alt'],
  },
})

// HandoffSessionEnd is how a handoff's auth session ended before completion.
type HandoffSessionEnd = 'expired' | 'canceled'

// maxWatchRetryMs caps the delay before reconnecting a dropped watch.
const maxWatchRetryMs = 30_000

// buildHandoffWatchURL returns the WebSocket URL that watches the auth session
// with the given nonce. apiUrl is any cloud API URL; only its origin is used.
function buildHandoffWatchURL(apiUrl: string, nonce: string): string {
  const base = new URL(apiUrl, window.location.href)
  const url = new URL(
    `/api/auth/session/${encodeURIComponent(nonce)}/watch`,
    base,
  )
  url.protocol = url.protocol === 'http:' ? 'ws:' : 'wss:'
  return url.toString()
}

// watchHandoffSession watches the auth session behind a handoff and calls
// onEnd once if it ends before completion. The server keeps a watched session
// alive while its device waits, closes the watch with 4008 when the session
// expires and 4000 when it is canceled, and drops it without a close frame
// when it restarts, which reconnects with backoff. Returns a stop function.
function watchHandoffSession(
  url: string,
  onEnd: (end: HandoffSessionEnd) => void,
): () => void {
  let ws: WebSocket | null = null
  let retry: ReturnType<typeof setTimeout> | undefined
  let retryMs = 1000
  let stopped = false

  const connect = () => {
    ws = new WebSocket(url)
    ws.onopen = () => {
      retryMs = 1000
    }
    ws.onclose = (ev) => {
      if (stopped || ev.code === 1000) return
      if (ev.code === 4008 || ev.code === 4000) {
        onEnd(ev.code === 4008 ? 'expired' : 'canceled')
        return
      }
      retry = setTimeout(connect, retryMs)
      retryMs = Math.min(retryMs * 2, maxWatchRetryMs)
    }
  }
  connect()

  return () => {
    stopped = true
    clearTimeout(retry)
    ws?.close()
  }
}

// useHandoffSessionEnd returns how the auth session with the given nonce
// ended, or null while it is live or once it completed. An ended handoff is
// forgotten so later sign-ins in this tab do not try to complete it.
function useHandoffSessionEnd(
  nonce: string,
  apiUrl: string,
): HandoffSessionEnd | null {
  const [end, setEnd] = useState<HandoffSessionEnd | null>(null)
  useEffect(() => {
    if (!nonce || !apiUrl) return
    return watchHandoffSession(buildHandoffWatchURL(apiUrl, nonce), (e) => {
      clearStoredHandoffPayload()
      setEnd(e)
    })
  }, [nonce, apiUrl])
  return end
}

// HandoffSessionWatch renders its children while the handoff behind the page
// is live and replaces them with a notice once its auth session ends early.
// The handoff comes from the route payload or, on the passkey and SSO pages,
// from the payload stored when the person left the handoff page.
export function HandoffSessionWatch({ children }: { children: ReactNode }) {
  const params = useParams()
  const request = useMemo(
    () => decodeHandoffRequest(params.payload) ?? getStoredHandoffRequest(),
    [params.payload],
  )
  const apiUrl = useCloudProviderConfig()?.exchangeUrl ?? ''
  const end = useHandoffSessionEnd(request?.sessionNonce ?? '', apiUrl)

  if (!request || !end) return children
  return (
    <HandoffEnded end={end} label={clientTypeLabel(request.clientType ?? '')} />
  )
}

// HandoffEnded tells the person that the sign-in they started has ended and
// how to start a new one.
function HandoffEnded({
  end,
  label,
}: {
  end: HandoffSessionEnd
  label: string
}) {
  const Icon = end === 'expired' ? LuTimerOff : LuCircleX
  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <div sx={stylexStyles2.rule3}>
          <Icon sx={stylexStyles.rule1} />
        </div>
        <h1 sx={stylexStyles2.rule4}>
          {end === 'expired' ? 'Sign-in link expired' : 'Sign-in canceled'}
        </h1>
        <p sx={stylexStyles2.rule5}>
          {end === 'expired'
            ? `This link is no longer valid. Start the sign-in again from Spacewave ${label} to get a new one.`
            : `Spacewave ${label} stopped waiting for this sign-in. Start it again to get a new link.`}
        </p>
      </div>
    </div>
  )
}
