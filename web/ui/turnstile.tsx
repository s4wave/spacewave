// Loads the CF script on-demand when the first <Turnstile> mounts.

import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react'

// TURNSTILE_SITE_KEY is the Cloudflare Turnstile public site key.
export const TURNSTILE_PROD_SITE_KEY = '0x4AAAAAACr84TeZC0nMMY9O'
export const TURNSTILE_TEST_SITE_KEY = '1x00000000000000000000AA'
export const TURNSTILE_TEST_TOKEN = 'XXXX.DUMMY.TOKEN.XXXX'

export function isTurnstileBypassed(siteKey: string): boolean {
  return siteKey === TURNSTILE_TEST_SITE_KEY
}

// TurnstileInstance is the ref handle exposed by the Turnstile component.
export interface TurnstileInstance {
  getResponse(): string | undefined
  getResponsePromise(timeout?: number): Promise<string>
  reset(): void
}

// Minimal window.turnstile type surface.
interface TurnstileAPI {
  render(
    container: HTMLElement,
    params: {
      sitekey: string
      size?: string
      callback?: (token: string) => void
    },
  ): string | null | undefined
  getResponse(widgetId: string): string | undefined
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

function ensureScript() {
  if (scriptState !== 'idle') return scriptReady
  scriptState = 'loading'
  const turnstileWindow = window as TurnstileWindow
  turnstileWindow[CALLBACK_NAME] = () => {
    scriptState = 'ready'
    scriptResolve?.()
    delete turnstileWindow[CALLBACK_NAME]
  }
  const script = document.createElement('script')
  script.src = `${SCRIPT_URL}?onload=${CALLBACK_NAME}&render=explicit`
  script.async = true
  script.defer = true
  document.head.appendChild(script)
  return scriptReady
}

interface TurnstileProps {
  siteKey: string
}

interface PendingTurnstileRequest {
  reject(error: Error): void
  resolve(token: string): void
  timer: ReturnType<typeof setTimeout>
}

// Turnstile renders a hidden Cloudflare Turnstile widget.
export const Turnstile = forwardRef<TurnstileInstance, TurnstileProps>(
  ({ siteKey }, ref) => {
    const bypass = isTurnstileBypassed(siteKey)
    const containerRef = useRef<HTMLDivElement>(null)
    const widgetIdRef = useRef<string | null>(null)
    const solvedRef = useRef(false)
    const responseRef = useRef<string | undefined>(undefined)
    const pendingRef = useRef<PendingTurnstileRequest[]>([])

    useEffect(() => {
      solvedRef.current = bypass
      responseRef.current = bypass ? TURNSTILE_TEST_TOKEN : undefined
      if (bypass) return

      let cancelled = false
      void ensureScript()
        .then(() => {
          if (cancelled || !containerRef.current || !window.turnstile) return
          const id = window.turnstile.render(containerRef.current, {
            sitekey: siteKey,
            size: 'flexible',
            callback: (token) => {
              solvedRef.current = true
              responseRef.current = token
              const pending = pendingRef.current
              pendingRef.current = []
              pending.forEach(({ resolve, timer }) => {
                clearTimeout(timer)
                resolve(token)
              })
            },
          })
          widgetIdRef.current = id ?? null
        })
        .catch(() => {
          widgetIdRef.current = null
        })
      return () => {
        cancelled = true
        if (widgetIdRef.current && window.turnstile) {
          window.turnstile.remove(widgetIdRef.current)
          widgetIdRef.current = null
        }
        solvedRef.current = false
        responseRef.current = undefined
        const pending = pendingRef.current
        pendingRef.current = []
        pending.forEach(({ reject, timer }) => {
          clearTimeout(timer)
          reject(new Error('Turnstile unmounted'))
        })
      }
    }, [bypass, siteKey])

    useImperativeHandle(ref, () => ({
      getResponse() {
        if (bypass) return TURNSTILE_TEST_TOKEN
        if (responseRef.current) return responseRef.current
        if (!widgetIdRef.current || !window.turnstile) return undefined
        return window.turnstile.getResponse(widgetIdRef.current)
      },

      async getResponsePromise(timeout = 30000) {
        if (bypass) return TURNSTILE_TEST_TOKEN
        if (responseRef.current) return responseRef.current
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

      reset() {
        if (bypass) {
          solvedRef.current = true
          responseRef.current = TURNSTILE_TEST_TOKEN
          return
        }
        if (!widgetIdRef.current || !window.turnstile) return
        solvedRef.current = false
        responseRef.current = undefined
        window.turnstile.reset(widgetIdRef.current)
      },
    }))

    if (bypass) return null
    return <div ref={containerRef} />
  },
)

Turnstile.displayName = 'Turnstile'
