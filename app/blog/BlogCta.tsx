import { useState, useCallback, useRef, type FormEvent } from 'react'
import { isDesktop } from '@aptre/bldr'
import { useNavigate } from '@s4wave/web/router/router.js'
import { LuCheck, LuArrowRight, LuGithub } from 'react-icons/lu'

import {
  Turnstile,
  TURNSTILE_PROD_SITE_KEY,
  type TurnstileInstance,
} from '@s4wave/web/ui/turnstile.js'

type FormState = 'idle' | 'submitting' | 'success' | 'error'

// parseErrorMessage maps API error responses to user-facing messages.
function parseErrorMessage(status: number, code?: string): string {
  if (status === 403 && code === 'turnstile_failed') {
    return 'Verification failed. Please try again.'
  }
  if (status === 429) {
    return 'Too many requests. Please try again later.'
  }
  if (status === 400 && (code === 'invalid_email' || code === 'invalid_body')) {
    return 'Please enter a valid email address.'
  }
  return 'Something went wrong. Please try again.'
}

// BlogCta renders the bottom-of-post call-to-action section.
export function BlogCta() {
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [formState, setFormState] = useState<FormState>('idle')
  const [errorMessage, setErrorMessage] = useState('')
  const turnstileRef = useRef<TurnstileInstance | null>(null)
  const [turnstileActive, setTurnstileActive] = useState(false)
  const turnstileWaitersRef = useRef<
    Array<(instance: TurnstileInstance) => void>
  >([])

  const goToQuickstart = useCallback(() => {
    navigate({ path: '/quickstart/local' })
  }, [navigate])

  const goToCommunity = useCallback(() => {
    navigate({ path: '/community' })
  }, [navigate])

  const setTurnstileInstance = useCallback(
    (instance: TurnstileInstance | null) => {
      turnstileRef.current = instance
      if (!instance) return
      const waiters = turnstileWaitersRef.current
      turnstileWaitersRef.current = []
      waiters.forEach((resolve) => resolve(instance))
    },
    [],
  )

  const waitForTurnstile = useCallback(async () => {
    if (turnstileRef.current) return turnstileRef.current
    return await new Promise<TurnstileInstance>((resolve) => {
      turnstileWaitersRef.current.push(resolve)
    })
  }, [])

  const handleSubmit = useCallback(
    async (e: FormEvent) => {
      e.preventDefault()
      if (!email || formState === 'submitting') return

      setFormState('submitting')
      setErrorMessage('')
      setTurnstileActive(true)

      try {
        const captureResponse = await fetch('/api/email/capture', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            email,
            source: 'blog',
          }),
        })

        if (!captureResponse.ok) {
          const data = (await captureResponse.json().catch(() => ({}))) as {
            code?: string
            error?: string
          }
          setErrorMessage(
            parseErrorMessage(captureResponse.status, data.error ?? data.code),
          )
          setFormState('error')
          return
        }

        const capture = (await captureResponse.json().catch(() => ({}))) as {
          capture_id?: string
        }
        if (!capture.capture_id) throw new Error('Missing capture id')

        const turnstile = await waitForTurnstile()
        const turnstileToken = await turnstile.getResponsePromise()
        if (!turnstileToken) throw new Error('Turnstile verification failed')

        const upgradeResponse = await fetch(
          `/api/email/capture/${encodeURIComponent(capture.capture_id)}/upgrade`,
          {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ turnstile_token: turnstileToken }),
          },
        )

        if (!upgradeResponse.ok) {
          const data = (await upgradeResponse.json().catch(() => ({}))) as {
            code?: string
            error?: string
          }
          setErrorMessage(
            parseErrorMessage(upgradeResponse.status, data.error ?? data.code),
          )
          setFormState('error')
          return
        }

        setFormState('success')
      } catch {
        setErrorMessage('Something went wrong. Please try again.')
        setFormState('error')
      }
    },
    [email, formState, waitForTurnstile],
  )

  return (
    <section className="mt-16 mb-4">
      <div className="bg-brand/20 mb-10 h-px w-full" />

      <div className="flex flex-col gap-8 @lg:flex-row @lg:gap-12">
        <div className="flex flex-1 flex-col gap-4">
          <h2 className="text-foreground text-xl font-semibold tracking-tight">
            Join the community
          </h2>
          <p className="text-foreground-alt/70 text-sm leading-relaxed">
            Get development updates and release announcements.
          </p>
          <div className="flex flex-wrap gap-3">
            <button
              onClick={goToQuickstart}
              className="border-brand/40 bg-brand/10 text-foreground hover:border-brand/60 hover:bg-brand/15 flex cursor-pointer items-center gap-2 rounded-md px-5 py-2.5 text-sm font-medium transition-all duration-300 select-none hover:-translate-y-0.5"
            >
              Get started
              <LuArrowRight className="size-3.5" />
            </button>
            <button
              onClick={goToCommunity}
              className="border-foreground/15 bg-background/50 text-foreground hover:border-brand/40 hover:bg-brand/8 flex cursor-pointer items-center gap-2 rounded-md px-5 py-2.5 text-sm font-medium transition-all duration-300 select-none hover:-translate-y-0.5"
            >
              <LuGithub className="size-3.5" />
              Join community
            </button>
          </div>
        </div>

        {!isDesktop && (
          <div className="flex flex-1 flex-col justify-center">
            {formState === 'success' ? (
              <div className="text-brand flex items-center gap-2 text-sm font-medium">
                <LuCheck className="size-4" />
                Subscribed.
              </div>
            ) : (
              <form
                onSubmit={(e) => {
                  void handleSubmit(e)
                }}
                className="flex flex-col gap-3"
              >
                <div className="flex gap-2">
                  <input
                    type="email"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="your@email.com"
                    required
                    disabled={formState === 'submitting'}
                    className="border-foreground/10 bg-background-dark/60 text-foreground placeholder:text-foreground-alt/30 focus:border-brand/40 focus:ring-brand/20 flex-1 rounded-md border px-4 py-2.5 text-sm transition-colors outline-none focus:ring-1 disabled:opacity-50"
                  />
                  <button
                    type="submit"
                    disabled={formState === 'submitting'}
                    className="border-brand/40 text-brand hover:bg-brand/10 hover:border-brand/60 cursor-pointer rounded-md border px-5 py-2.5 text-sm font-medium transition-all duration-300 select-none hover:-translate-y-0.5 disabled:pointer-events-none disabled:opacity-50"
                  >
                    {formState === 'submitting' ? 'Sending...' : 'Subscribe'}
                  </button>
                </div>
                {formState === 'error' && errorMessage && (
                  <p className="text-error text-xs">{errorMessage}</p>
                )}
                {turnstileActive && (
                  <Turnstile
                    ref={setTurnstileInstance}
                    siteKey={TURNSTILE_PROD_SITE_KEY}
                    size="invisible"
                  />
                )}
              </form>
            )}
          </div>
        )}
      </div>
    </section>
  )
}
