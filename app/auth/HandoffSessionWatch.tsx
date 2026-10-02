import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { LuCircleX, LuTimerOff } from 'react-icons/lu'

import { useParams } from '@s4wave/web/router/router.js'
import { useCloudProviderConfig } from '@s4wave/app/provider/spacewave/useSpacewaveAuth.js'
import {
  clearStoredHandoffPayload,
  clientTypeLabel,
  decodeHandoffRequest,
  getStoredHandoffRequest,
} from './handoff-state.js'

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
    <div className="bg-background-landing relative flex flex-1 flex-col items-center justify-center gap-6 p-6">
      <div className="relative z-10 flex max-w-sm flex-col items-center gap-4 text-center">
        <div className="bg-foreground/5 border-foreground/10 flex size-16 items-center justify-center rounded-full border">
          <Icon className="text-foreground-alt size-8" />
        </div>
        <h1 className="text-xl font-semibold tracking-wide">
          {end === 'expired' ? 'Sign-in link expired' : 'Sign-in canceled'}
        </h1>
        <p className="text-foreground-alt text-sm">
          {end === 'expired'
            ? `This link is no longer valid. Start the sign-in again from Spacewave ${label} to get a new one.`
            : `Spacewave ${label} stopped waiting for this sign-in. Start it again to get a new link.`}
        </p>
      </div>
    </div>
  )
}
