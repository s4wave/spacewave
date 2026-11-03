// Loads the CF script on-demand when the first <Turnstile> mounts.

import { useEffect, useImperativeHandle, useRef, type Ref } from 'react'
import * as stylex from '@stylexjs/stylex'

// TURNSTILE_SITE_KEY is the Cloudflare Turnstile public site key.
export const TURNSTILE_PROD_SITE_KEY = '0x4AAAAAACr84TeZC0nMMY9O'
export const TURNSTILE_TEST_SITE_KEY = '1x00000000000000000000AA'
export const TURNSTILE_TEST_TOKEN = 'XXXX.DUMMY.TOKEN.XXXX'

export function isTurnstileBypassed(siteKey: string): boolean {
  return siteKey === TURNSTILE_TEST_SITE_KEY
}

// TurnstileInstance is the ref handle exposed by the Turnstile component.
export interface TurnstileInstance {
  // takeToken returns an unspent token, waiting up to timeout milliseconds for
  // the widget to issue one. Cloudflare verifies a token once, so each token
  // goes to one caller and the widget then solves again for the next request.
  takeToken(timeout?: number): Promise<string>
}

// Minimal window.turnstile type surface.
interface TurnstileAPI {
  render(
    container: HTMLElement,
    params: {
      sitekey: string
      size?: 'flexible' | 'invisible'
      callback?: (token: string) => void
    },
  ): string | null | undefined
  remove(widgetId: string): void
  reset(widgetId: string): void
}

declare global {
  interface Window {
    turnstile?: TurnstileAPI
  }
}

const SCRIPT_URL = 'https://challenges.cloudflare.com/turnstile/v0/api.js'
const CALLBACK_NAME = 'onloadTurnstileCallback'
type TurnstileWindow = Window & {
  [CALLBACK_NAME]?: () => void
}

// Module-level singleton: one script load shared across all instances.
let scriptState: 'idle' | 'loading' | 'ready' = 'idle'
let scriptResolve: (() => void) | null = null
const scriptReady = new Promise<void>((resolve) => {
  scriptResolve = resolve
})

// ensureScript shares one script load and resolves after the vendor callback.
function ensureScript() {
  // Install one readiness callback before any widget requests the vendor script.
  if (scriptState !== 'idle') return scriptReady
  scriptState = 'loading'
  const turnstileWindow = window as TurnstileWindow
  turnstileWindow[CALLBACK_NAME] = () => {
    scriptState = 'ready'
    scriptResolve?.()
    delete turnstileWindow[CALLBACK_NAME]
  }

  // Load the script only after its readiness callback can receive completion.
  const script = document.createElement('script')
  script.src = `${SCRIPT_URL}?onload=${CALLBACK_NAME}&render=explicit`
  script.async = true
  script.defer = true
  document.head.appendChild(script)
  return scriptReady
}

interface TurnstileProps {
  sx?: stylex.StyleXStyles
  ref?: Ref<TurnstileInstance>
  siteKey: string
  size?: 'flexible' | 'invisible'
}

// PendingTurnstileRequest is a takeToken call waiting for the widget to issue
// a token.
interface PendingTurnstileRequest {
  reject(error: Error): void
  resolve(token: string): void
  timer: ReturnType<typeof setTimeout>
}

// Turnstile renders a Cloudflare Turnstile widget.
export function Turnstile({
  ref,
  siteKey,
  sx,
  size = 'flexible',
}: TurnstileProps) {
  // Retain the widget lifetime, issued token and pending token consumers.
  const bypass = isTurnstileBypassed(siteKey)
  const containerRef = useRef<HTMLDivElement>(null)
  const widgetIdRef = useRef<string | null>(null)
  const tokenRef = useRef<string | undefined>(undefined)
  const pendingRef = useRef<PendingTurnstileRequest[]>([])

  // Render the widget once the script loads, handing each token it issues to
  // the oldest waiting caller, or keeping it for the next one.
  useEffect(() => {
    if (bypass) return

    let cancelled = false
    void ensureScript()
      .then(() => {
        if (cancelled || !containerRef.current || !window.turnstile) return
        const id = window.turnstile.render(containerRef.current, {
          sitekey: siteKey,
          size,
          callback: (token) => {
            // Deliver each issued token to one waiting consumer, or retain it for the next.
            const next = pendingRef.current.shift()
            if (!next) {
              tokenRef.current = token
              return
            }
            clearTimeout(next.timer)
            next.resolve(token)
            spendToken()
          },
        })
        widgetIdRef.current = id ?? null
      })
      .catch(() => {
        widgetIdRef.current = null
      })
    return () => {
      // Remove the widget and fail the callers still waiting on it.
      cancelled = true
      if (widgetIdRef.current && window.turnstile) {
        window.turnstile.remove(widgetIdRef.current)
        widgetIdRef.current = null
      }
      tokenRef.current = undefined
      const pending = pendingRef.current
      pendingRef.current = []
      pending.forEach(({ reject, timer }) => {
        clearTimeout(timer)
        reject(new Error('Turnstile unmounted'))
      })
    }
  }, [bypass, siteKey, size])

  // spendToken drops the token just handed out and has the widget solve again.
  function spendToken() {
    tokenRef.current = undefined
    if (widgetIdRef.current && window.turnstile) {
      window.turnstile.reset(widgetIdRef.current)
    }
  }

  // Expose single-use tokens through the owner’s imperative request interface.
  useImperativeHandle(ref, () => ({
    async takeToken(timeout = 30000) {
      // Hand out the token the widget already issued.
      if (bypass) return TURNSTILE_TEST_TOKEN
      const token = tokenRef.current
      if (token) {
        spendToken()
        return token
      }

      // Wait for the widget to issue the next token.
      return await new Promise<string>((resolve, reject) => {
        const req: PendingTurnstileRequest = {
          resolve,
          reject,
          timer: setTimeout(() => {
            pendingRef.current = pendingRef.current.filter((v) => v !== req)
            reject(new Error('Turnstile timeout'))
          }, timeout),
        }
        pendingRef.current.push(req)
      })
    },
  }))

  // Test keys bypass the vendor DOM; real widgets retain native caller styles.
  if (bypass) return null
  return <div ref={containerRef} {...stylex.props(sx)} />
}

Turnstile.displayName = 'Turnstile'
