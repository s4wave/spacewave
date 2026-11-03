import { useCallback, useId, useState } from 'react'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

import type { Account } from '@s4wave/sdk/account/account.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

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
  rule3: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule4: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule5: {
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
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
  rule6: {
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
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
})

export interface ChangePasswordDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  account: Resource<Account>
}

// ChangePasswordDialog prompts for current and new password, then calls ChangePassword RPC.
export function ChangePasswordDialog({
  open,
  onOpenChange,
  account,
}: ChangePasswordDialogProps) {
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const currentPasswordId = useId()
  const newPasswordId = useId()
  const confirmPasswordId = useId()

  const passwordsMatch = newPassword === confirmPassword
  const canSubmit =
    !!currentPassword &&
    !!newPassword &&
    !!confirmPassword &&
    passwordsMatch &&
    !submitting

  const handleSubmit = useCallback(async () => {
    if (!canSubmit || !account.value) return
    setSubmitting(true)
    setError(null)
    try {
      await account.value.changePassword({
        oldPassword: currentPassword,
        newPassword,
      })
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
      onOpenChange(false)
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Operation failed'
      if (msg.includes('unknown_keypair')) {
        setError('Incorrect current password. Please try again.')
      } else {
        setError(msg)
      }
    } finally {
      setSubmitting(false)
    }
  }, [canSubmit, account.value, currentPassword, newPassword, onOpenChange])

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) {
        setCurrentPassword('')
        setNewPassword('')
        setConfirmPassword('')
        setError(null)
      }
      onOpenChange(next)
    },
    [onOpenChange],
  )

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Change password</DialogTitle>
          <DialogDescription>
            Enter your current password and choose a new one.
          </DialogDescription>
        </DialogHeader>

        <div>
          <div sx={nativeChildSpacing.s3}>
            <label sx={stylexStyles.rule1} htmlFor={currentPasswordId}>
              Current password
            </label>
            <input
              id={currentPasswordId}
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              placeholder="Enter current password"
              sx={[stylexStyles.rule2]}
            />
          </div>
          <div sx={nativeChildSpacing.s3}>
            <label sx={stylexStyles.rule1} htmlFor={newPasswordId}>
              New password
            </label>
            <input
              id={newPasswordId}
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              placeholder="Enter new password"
              sx={[stylexStyles.rule2]}
            />
          </div>
          <div sx={nativeChildSpacing.s3}>
            <label sx={stylexStyles.rule1} htmlFor={confirmPasswordId}>
              Confirm new password
            </label>
            <input
              id={confirmPasswordId}
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              placeholder="Confirm new password"
              onKeyDown={(e) => {
                if (e.key === 'Enter') void handleSubmit()
              }}
              sx={[stylexStyles.rule2]}
            />
            {confirmPassword && !passwordsMatch && (
              <p sx={stylexStyles.rule3}>Passwords do not match.</p>
            )}
          </div>
        </div>

        {error && <p sx={stylexStyles.rule4}>{error}</p>}

        <DialogFooter>
          <button
            type="button"
            onClick={() => handleOpenChange(false)}
            disabled={submitting}
            sx={stylexStyles.rule5}
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void handleSubmit()}
            disabled={!canSubmit}
            sx={[stylexStyles.rule6]}
          >
            {submitting ? 'Changing…' : 'Change password'}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
