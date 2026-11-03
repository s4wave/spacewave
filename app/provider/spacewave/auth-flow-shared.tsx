import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import type { MouseEventHandler, ReactNode } from 'react'
import { FcGoogle } from '@s4wave/web/images/icons.js'
import { LuGithub } from '@s4wave/web/images/icons.js'

import { SpacewaveProvider } from '@s4wave/sdk/provider/spacewave/spacewave.js'
import type { Root } from '@s4wave/sdk/root/root.js'

import { dnsLabelRegex } from './keypair-utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
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
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule2: {
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: themeColors['--color-destructive'],
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
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
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule3: {
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 50%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
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
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    backgroundColor: themeColors['--color-background-get-started'],
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule2: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
  },
  rule3: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule4: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule5: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule6: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

// authInputStyles preserves each complete input state, including its focus border.
export const authInputStyles = {
  base: [[stylexStyles2.rule1], false],
  invalid: stylexStyles2.rule2,
  invalidSubtle: stylexStyles2.rule3,
} as const

export function getProviderLabel(provider: string): string {
  if (provider === 'google') return 'Google'
  if (provider === 'github') return 'GitHub'
  return provider
}

export function ProviderIcon({
  provider,
  sx,
}: {
  provider: string
  sx?: import('@stylexjs/stylex').StyleXStyles
}) {
  if (provider === 'google') return <FcGoogle sx={sx} />
  if (provider === 'github') return <LuGithub sx={sx} />
  return null
}

export function normalizeUsernameInput(value: string): {
  username: string
  error: string
} {
  const username = value.toLowerCase()
  if (username && !dnsLabelRegex.test(username)) {
    return {
      username,
      error: 'Lowercase letters, numbers, and hyphens only',
    }
  }
  return { username, error: '' }
}

export function validateUsername(username: string): string | null {
  if (!username || !dnsLabelRegex.test(username)) {
    return 'Please enter a valid username'
  }
  return null
}

export function validateOptionalPin(
  pin: string,
  confirmPin: string,
): string | null {
  const wantsPin = pin.length > 0 || confirmPin.length > 0
  if (!wantsPin) return null
  if (!pin || !confirmPin) {
    return 'Enter and confirm a PIN, or leave both fields blank'
  }
  if (pin !== confirmPin) {
    return 'PINs do not match'
  }
  return null
}

export function isUsernameTakenError(err: unknown): boolean {
  const message = err instanceof Error ? err.message : String(err)
  return (
    message.includes('username_taken') ||
    message.includes('Username already exists')
  )
}

export function getErrorMessage(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback
}

export async function withSpacewaveProvider<T>(
  root: Root,
  fn: (spacewave: SpacewaveProvider) => Promise<T>,
  abortSignal?: AbortSignal,
): Promise<T> {
  using provider = await root.lookupProvider('spacewave', abortSignal)
  return await fn(new SpacewaveProvider(provider.resourceRef))
}

export async function loginWithEntityPem(
  root: Root,
  pemPrivateKey: Uint8Array,
): Promise<number> {
  return await withSpacewaveProvider(root, async (spacewave) => {
    const loginResp = await spacewave.loginWithEntityKey(pemPrivateKey)
    return loginResp.sessionListEntry?.sessionIndex ?? 0
  })
}

export function AuthCard({
  children,
  sx,
}: Omit<
  {
    children: ReactNode
    className?: string
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return <div sx={[stylexStyles.rule1, sx]}>{children}</div>
}

export function AuthStatusPanel({
  icon,
  message,
  children,
}: {
  icon: ReactNode
  message: ReactNode
  children?: ReactNode
}) {
  return (
    <AuthCard>
      <div sx={stylexStyles.rule2}>
        {icon}
        <p sx={stylexStyles.rule3}>{message}</p>
        {children}
      </div>
    </AuthCard>
  )
}

export function AuthPrimaryActionButton({
  onClick,
  icon,
  children,
  disabled,
  type = 'button',
}: {
  onClick?: MouseEventHandler<HTMLButtonElement>
  icon?: ReactNode
  children: ReactNode
  disabled?: boolean
  type?: 'button' | 'submit'
}) {
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      sx={[stylexStyles.rule4]}
    >
      {icon}
      <span sx={stylexStyles.rule5}>{children}</span>
    </button>
  )
}

export function AuthSecondaryActionButton({
  onClick,
  children,
  sx,
}: Omit<
  {
    onClick?: MouseEventHandler<HTMLButtonElement>
    children: ReactNode
    className?: string
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <button type="button" onClick={onClick} sx={[stylexStyles.rule6, sx]}>
      {children}
    </button>
  )
}
