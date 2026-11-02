import { useCallback, useId } from 'react'
import { LuUpload } from '@s4wave/web/images/icons.js'

import { cn } from '@s4wave/web/style/utils.js'

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
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule2: {
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
  rule3: { opacity: 0.5 },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule5: {
    height: '1px',
    flex: 1,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
  },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
  },
  rule7: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },

  rule9: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
})

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
  className?: string
}

export const inputClass = cn(
  'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none',
  'focus:border-brand/50',
)

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
  className,
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
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ cn(
          'space-y-3',
          className,
        )
      }
    >
      {showPassword && (
        <div>
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
            sx={[[stylexStyles.rule2], !!disabled && stylexStyles.rule3]}
          />
        </div>
      )}
      {showPem && (
        <>
          {showPassword && (
            <div sx={stylexStyles.rule4}>
              <div sx={stylexStyles.rule5} />
              <span>or</span>
              <div sx={stylexStyles.rule5} />
            </div>
          )}
          <div>
            <label htmlFor={pemId} sx={stylexStyles.rule1}>
              {pemLabel}
            </label>
            <button
              id={pemId}
              type="button"
              onClick={() => fileInputRef?.current?.click()}
              disabled={disabled}
              className={
                /* TODO(stylex): conditional property composition: borderColor */ cn(
                  inputClass,
                  'flex items-center gap-2 text-left',
                  'hover:border-foreground/30',
                  disabled && 'opacity-50',
                )
              }
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
      {error && <p sx={stylexStyles.rule9}>{error}</p>}
    </div>
  )
}
