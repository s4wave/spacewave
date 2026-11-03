import { useState, useCallback, useRef, type FormEvent } from 'react'
import { isDesktop } from '@aptre/bldr'
import { useNavigate } from '@s4wave/web/router/router.js'
import { LuCheck, LuArrowRight, LuGithub } from '@s4wave/web/images/icons.js'

import { EmailCaptureRequest } from '@s4wave/core/provider/spacewave/api/api.pb.js'
import {
  Turnstile,
  TURNSTILE_PROD_SITE_KEY,
  type TurnstileInstance,
} from '@s4wave/web/ui/turnstile.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
  radii as themeRadii,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    marginTop: `calc(${themeLayout['--spacing']} * 16)`,
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    borderRadius: 'var(--radius-2xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
  },
  rule3: {
    display: 'flex',
    flexDirection: {
      default: 'column',
      '@container (min-width: 32rem)': 'row',
    },
    gap: {
      default: `calc(${themeLayout['--spacing']} * 8)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    alignItems: { default: null, '@container (min-width: 32rem)': 'center' },
  },
  rule4: {
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    fontSize: 'var(--text-2xl)',
    lineHeight: 'var(--tw-leading, var(--text-2xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule6: {
    maxWidth: 'var(--container-md)',
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule7: {
    marginTop: themeLayout['--spacing'],
    display: 'flex',
    flexWrap: 'wrap',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule8: {
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
  },
  rule9: {
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
  },
  rule10: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    width: {
      default: null,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 72)`,
    },
    flexShrink: { default: null, '@container (min-width: 32rem)': 0 },
  },
  rule11: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule12: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule13: { display: 'flex', gap: `calc(${themeLayout['--spacing']} * 2)` },
  rule14: {
    minWidth: 0,
    flex: 1,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-dark']} 60%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule15: {
    flexShrink: 0,
    cursor: 'pointer',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      },
    },
    pointerEvents: { default: null, ':disabled': 'none' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule16: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-error'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
  },
})

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
        // Pass Turnstile, then store the address.
        const turnstile = await waitForTurnstile()
        const turnstileToken = await turnstile.takeToken()
        if (!turnstileToken) throw new Error('Turnstile verification failed')
        const response = await fetch('/api/email/capture', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/octet-stream',
            'X-Turnstile-Token': turnstileToken,
          },
          body: EmailCaptureRequest.toBinary({ email, source: 'blog' }).slice(),
        })
        if (!response.ok) {
          const data = (await response.json().catch(() => ({}))) as {
            code?: string
          }
          setErrorMessage(parseErrorMessage(response.status, data.code))
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
    <section sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <div sx={stylexStyles2.rule3}>
          <div sx={stylexStyles2.rule4}>
            <h2 sx={stylexStyles2.rule5}>Join the community</h2>
            <p sx={stylexStyles2.rule6}>
              Get development updates and release announcements.
            </p>
            <div sx={stylexStyles2.rule7}>
              <button
                type="button"
                onClick={goToQuickstart}
                sx={stylexStyles2.rule8}
              >
                Get started
                <LuArrowRight sx={stylexStyles.rule1} />
              </button>
              <button
                type="button"
                onClick={goToCommunity}
                sx={stylexStyles2.rule9}
              >
                <LuGithub sx={stylexStyles.rule1} />
                Join community
              </button>
            </div>
          </div>

          {!isDesktop && (
            <div sx={stylexStyles2.rule10}>
              {formState === 'success' ? (
                <div sx={stylexStyles2.rule11}>
                  <LuCheck sx={stylexStyles.rule2} />
                  Subscribed. Thanks for joining.
                </div>
              ) : (
                <form
                  onSubmit={(e) => {
                    void handleSubmit(e)
                  }}
                  sx={stylexStyles2.rule12}
                >
                  <div sx={stylexStyles2.rule13}>
                    <input
                      aria-label="Email address"
                      type="email"
                      value={email}
                      onChange={(e) => setEmail(e.target.value)}
                      placeholder="your@email.com"
                      required
                      disabled={formState === 'submitting'}
                      sx={stylexStyles2.rule14}
                    />
                    <button
                      type="submit"
                      disabled={formState === 'submitting'}
                      sx={stylexStyles2.rule15}
                    >
                      {formState === 'submitting' ? 'Sending…' : 'Subscribe'}
                    </button>
                  </div>
                  {formState === 'error' && errorMessage && (
                    <p sx={stylexStyles2.rule16}>{errorMessage}</p>
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
      </div>
    </section>
  )
}
