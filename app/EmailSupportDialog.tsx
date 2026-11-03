import { useCallback } from 'react'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

import * as stylex from '@stylexjs/stylex'

import {
  colors as themeColors,
  radii as themeRadii,
  layout as themeLayout,
  fonts as themeFonts,
  motion as themeMotion,
  utilityState as themeUtilityState,
} from '../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand-highlight'],
      },
    },
    textDecorationLine: 'underline',
    textUnderlineOffset: '4px',
  },
  rule2: {
    borderRadius: themeRadii['--radius-md'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule3: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    color: themeColors['--color-brand'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const dialogStyles = stylex.create({
  content: {
    maxWidth: {
      default: 'var(--container-md)',
      '@media (min-width: 40rem)': 'var(--container-lg)',
    },
  },
})

export const SUPPORT_EMAIL = 'support@aperture.us'

export interface EmailSupportDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

// EmailSupportDialog explains how to contact support and offers a mailto action.
export function EmailSupportDialog({
  open,
  onOpenChange,
}: EmailSupportDialogProps) {
  const handleOpenEmail = useCallback(() => {
    window.open(`mailto:${SUPPORT_EMAIL}`)
    onOpenChange(false)
  }, [onOpenChange])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent sx={dialogStyles.content}>
        <DialogHeader>
          <DialogTitle>Email Support</DialogTitle>
          <DialogDescription>
            Need a hand? Reach out at{' '}
            <a href={`mailto:${SUPPORT_EMAIL}`} sx={stylexStyles.rule1}>
              {SUPPORT_EMAIL}
            </a>{' '}
            and we'll get back to you.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <button
            type="button"
            onClick={() => onOpenChange(false)}
            sx={stylexStyles.rule2}
          >
            Close
          </button>
          <button
            type="button"
            onClick={handleOpenEmail}
            sx={[stylexStyles.rule3]}
          >
            Open Email
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
