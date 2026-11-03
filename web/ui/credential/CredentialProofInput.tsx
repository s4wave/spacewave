import { useCallback, useId } from 'react'
import { LuUpload } from '@s4wave/web/images/icons.js'

import type { StyleXArray, StyleXStyles } from '@stylexjs/stylex'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule3: { opacity: 0.5 },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: {
    flex: 1,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    height: '1px',
  },
  rule6: {
    color: themeColors['--color-foreground-alt'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule7: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },

  rule9: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

/** credentialInputStyles shares the credential field's size and focus treatment. */
export const credentialInputStyles = stylex.create({
  input: {
    '--tw-outline-style': 'none',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    outlineStyle: 'none',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  invalid: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 50%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
  },
  code: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    fontFamily: themeFonts['--font-mono'],
    fontSize: 'var(--text-lg)',
    letterSpacing: '0.3em',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    textAlign: 'center',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  compactCode: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    fontFamily: themeFonts['--font-mono'],
    fontSize: 'var(--text-base)',
    letterSpacing: '0.3em',
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
    textAlign: 'center',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  pem: {
    '--tw-outline-style': 'none',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 30%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    outlineStyle: 'none',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
})

const spacingStyles = stylex.create({
  child: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--credential-proof-space, calc(${themeLayout['--spacing']} * 3)) * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--credential-proof-space, calc(${themeLayout['--spacing']} * 3)) * var(--tw-space-y-reverse))`,
    },
  },
})

/** CredentialProofInputProps delegates proof state and file selection to its caller. */
export interface CredentialProofInputProps {
  password: string
  onPasswordChange: (v: string) => void
  pemFileName?: string | null
  onFileChange?: (e: React.ChangeEvent<HTMLInputElement>) => void
  fileInputRef?: React.RefObject<HTMLInputElement | null>
  showPassword?: boolean
  showPem?: boolean
  passwordLabel?: string
  passwordPlaceholder?: string
  pemLabel?: string
  error?: string | null
  disabled?: boolean
  focusOnMount?: boolean
  onPasswordKeyDown?: (e: React.KeyboardEvent) => void
  sx?: StyleXArray<
    StyleXStyles | StyleXStyles<{ '--credential-proof-space': string }>
  >
  className?: never
}

// CredentialProofInput renders password + optional PEM file upload for
// EntityCredential proof. Presentational only, all state managed by parent
// or useCredentialProof hook.
export function CredentialProofInput({
  password,
  onPasswordChange,
  pemFileName,
  onFileChange,
  fileInputRef,
  showPassword = true,
  showPem = true,
  passwordLabel = 'Account password',
  passwordPlaceholder = 'Enter your password',
  pemLabel = 'Backup key (.pem)',
  error,
  disabled,
  focusOnMount,
  onPasswordKeyDown,
  sx,
}: CredentialProofInputProps) {
  const passwordId = useId()
  const pemId = useId()
  const handlePasswordRef = useCallback(
    (node: HTMLInputElement | null) => {
      if (focusOnMount) node?.focus()
    },
    [focusOnMount],
  )

  return (
    <div {...stylex.props(sx)}>
      {showPassword && (
        <div sx={spacingStyles.child}>
          <label htmlFor={passwordId} sx={stylexStyles.rule1}>
            {passwordLabel}
          </label>
          <input
            id={passwordId}
            ref={handlePasswordRef}
            type="password"
            value={password}
            onChange={(e) => onPasswordChange(e.target.value)}
            placeholder={passwordPlaceholder}
            disabled={disabled}
            readOnly={disabled}
            onKeyDown={onPasswordKeyDown}
            sx={[
              [credentialInputStyles.input],
              !!disabled && stylexStyles.rule3,
            ]}
          />
        </div>
      )}
      {showPem && (
        <>
          {showPassword && (
            <div sx={[spacingStyles.child, stylexStyles.rule4]}>
              <div sx={stylexStyles.rule5} />
              <span>or</span>
              <div sx={stylexStyles.rule5} />
            </div>
          )}
          <div sx={spacingStyles.child}>
            <label htmlFor={pemId} sx={stylexStyles.rule1}>
              {pemLabel}
            </label>
            <button
              id={pemId}
              type="button"
              onClick={() => fileInputRef?.current?.click()}
              disabled={disabled}
              sx={[credentialInputStyles.pem, disabled && stylexStyles.rule3]}
            >
              <LuUpload sx={stylexStyles.rule6} />
              <span sx={[!pemFileName && stylexStyles.rule7]}>
                {pemFileName ?? 'Choose .pem file'}
              </span>
            </button>
            <input
              ref={fileInputRef}
              type="file"
              accept=".pem"
              onChange={onFileChange}
              disabled={disabled}
              hidden
            />
          </div>
        </>
      )}
      {error && <p sx={[spacingStyles.child, stylexStyles.rule9]}>{error}</p>}
    </div>
  )
}
