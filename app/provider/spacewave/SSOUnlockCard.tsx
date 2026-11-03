import { useCallback } from 'react'
import { LuArrowLeft, LuCheck, LuLock } from '@s4wave/web/images/icons.js'

import {
  AuthCard,
  AuthPrimaryActionButton,
  AuthSecondaryActionButton,
  authInputStyles,
  getProviderLabel,
  ProviderIcon,
} from './auth-flow-shared.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule4: { display: 'flex', flexDirection: 'column', minWidth: 0 },
  rule5: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule6: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule7: {
    overflow: 'hidden',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule8: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule9: {
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule10: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule11: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule12: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
    display: 'flex',
    justifyContent: 'center',
  },
  rule13: {
    alignItems: 'center',
    color: themeColors['--color-foreground-alt'],
    columnGap: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    flexWrap: 'wrap',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    rowGap: themeLayout['--spacing'],
  },
  rule14: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    display: 'flex',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    color: themeColors['--color-foreground'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

export interface SSOUnlockCardProps {
  provider: string
  email?: string
  username?: string
  pin: string
  pinError: string
  busy: boolean
  onPinChange: (value: string) => void
  onSubmit: () => void
  onCancel: () => void
  cancelLabel?: string
}

const UNLOCK_HIGHLIGHTS = ['End-to-end encrypted', 'Local-first', 'Open source']

// SSOUnlockCard renders the post-SSO PIN unlock card. Mirrors SSOConfirmPage's
// provider context header, AuthCard wrapper, and trust-signal row so the
// signed-in identity flow stays visually continuous with the signup flow.
export function SSOUnlockCard(props: SSOUnlockCardProps) {
  const {
    provider,
    email,
    username,
    pin,
    pinError,
    busy,
    onPinChange,
    onSubmit,
    onCancel,
    cancelLabel = 'Back to login',
  } = props
  const providerLabel = getProviderLabel(provider)
  const handlePinRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  return (
    <div sx={stylexStyles3.rule1}>
      <AuthCard>
        <div sx={stylexStyles3.rule2}>
          <div sx={stylexStyles3.rule3}>
            <ProviderIcon provider={provider} sx={stylexStyles2.rule1} />
          </div>
          <div sx={stylexStyles3.rule4}>
            <h2 sx={stylexStyles3.rule5}>Signing in with {providerLabel}</h2>
            {username && <p sx={stylexStyles3.rule6}>{username}</p>}
            {email && <p sx={stylexStyles3.rule7}>{email}</p>}
          </div>
        </div>

        <div sx={stylexStyles3.rule1}>
          <label sx={stylexStyles3.rule8}>
            <span sx={stylexStyles3.rule9}>
              <LuLock sx={stylexStyles.rule1} />
              PIN
            </span>
            <input
              ref={handlePinRef}
              id="sso-unlock-pin"
              type="password"
              value={pin}
              onChange={(e) => onPinChange(e.target.value)}
              placeholder="Enter your PIN"
              disabled={busy}
              sx={
                pinError ? authInputStyles.invalidSubtle : authInputStyles.base
              }
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !busy) {
                  e.preventDefault()
                  onSubmit()
                }
              }}
            />
            {pinError ? (
              <p sx={stylexStyles3.rule10}>{pinError}</p>
            ) : (
              <p sx={stylexStyles3.rule11}>
                The PIN you set when this account was created.
              </p>
            )}
          </label>
          <AuthPrimaryActionButton
            onClick={onSubmit}
            disabled={busy || !pin}
            icon={<LuLock sx={stylexStyles.rule2} />}
          >
            Unlock and continue
          </AuthPrimaryActionButton>
          <AuthSecondaryActionButton
            onClick={onCancel}
            sx={stylexStyles3.rule12}
          >
            <LuArrowLeft sx={stylexStyles.rule3} />
            {cancelLabel}
          </AuthSecondaryActionButton>
        </div>
      </AuthCard>

      <div sx={stylexStyles3.rule13}>
        {UNLOCK_HIGHLIGHTS.map((text) => (
          <span key={text} sx={stylexStyles3.rule14}>
            <LuCheck sx={stylexStyles.rule4} />
            {text}
          </span>
        ))}
      </div>
    </div>
  )
}
