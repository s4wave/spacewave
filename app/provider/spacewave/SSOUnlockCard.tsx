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
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule4: { display: 'flex', minWidth: 0, flexDirection: 'column' },
  rule5: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule6: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule7: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule8: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule9: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule10: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule11: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule12: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
  },
  rule13: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'center',
    columnGap: `calc(${themeLayout['--spacing']} * 6)`,
    rowGap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule14: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
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
    color: themeColors['--color-foreground'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-brand'],
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
