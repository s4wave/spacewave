import { useCallback, useState } from 'react'
import {
  LuArrowLeft,
  LuArrowRight,
  LuCheck,
  LuDownload,
  LuKey,
  LuLock,
  LuLockOpen,
  LuShieldCheck,
  LuTrash2,
  LuUpload,
} from '@s4wave/web/images/icons.js'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { downloadPemFile } from '@s4wave/web/download.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { CredentialProofInput } from '@s4wave/web/ui/credential/CredentialProofInput.js'
import { useCredentialProof } from '@s4wave/web/ui/credential/useCredentialProof.js'
import {
  mapAuthError,
  methodLabel,
  truncatePeerId,
} from '@s4wave/web/ui/credential/auth-utils.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type {
  EntityKeypairState,
  WatchAccountInfoResponse,
} from '@s4wave/sdk/account/account.pb.js'

import { useAccountDashboardState } from './AccountDashboardStateContext.js'
import { useEntityKeypairs } from './useEntityKeypairs.js'

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: { minHeight: `calc(${themeLayout['--spacing']} * 30)` },
  rule2: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule4: {
    borderRadius: themeRadii['--radius-md'],
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule5: { flex: 1 },
  rule6: {
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
  rule7: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    cursor: { default: null, ':disabled': 'not-allowed' },
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule8: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule9: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
      },
    },
    color: themeColors['--color-destructive'],
  },
  rule10: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
  },
  rule11: {
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule12: {
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
  },
  rule13: {
    flex: 1,
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
  },
  rule14: {
    borderRadius: '3.40282e38px',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: themeLayout['--spacing'],
    width: '100%',
  },
  rule15: { backgroundColor: themeColors['--color-brand'] },
  rule16: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
  },
  rule17: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
  },
  rule18: {
    '--tw-leading': 'var(--leading-tight)',
    fontSize: '10px',
    lineHeight: 'var(--leading-tight)',
  },
  rule19: { color: themeColors['--color-brand'] },
  rule20: { color: themeColors['--color-foreground'] },
  rule21: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule22: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
  },
  rule23: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule24: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: themeColors['--color-foreground-alt'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    marginTop: themeLayout['--spacing'],
  },
  rule25: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule26: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'flex-start',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    display: 'flex',
  },
  rule27: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule28: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule29: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: '3.40282e38px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule30: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule31: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule32: {
    borderRadius: '3.40282e38px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: '100%',
  },
  rule33: {
    transition: 'width 300ms',
    backgroundColor: themeColors['--color-brand'],
    height: '100%',
    width: 'var(--progress-width)',
  },
  rule34: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule35: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    minWidth: 0,
  },
  rule36: { minWidth: 0 },
  rule37: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    overflow: 'hidden',
    color: themeColors['--color-foreground-alt'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
  },
  rule38: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule39: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule40: { flex: 1, minWidth: 0 },
  rule41: {
    '--tw-outline-style': 'none',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
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
  rule42: {
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
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    outlineStyle: 'none',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule43: { display: 'none' },
  rule44: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    cursor: { default: null, ':disabled': 'not-allowed' },
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule45: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
  },
  rule46: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
  },
  rule47: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-yellow-500']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-yellow-500']} 5%, transparent)`,
  },
  rule48: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: themeLayout['--spacing'],
  },
  rule49: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule50: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule51: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule52: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: themeColors['--color-foreground-alt'],
    fontFamily: themeFonts['--font-mono'],
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule53: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
  },
  rule54: {
    borderRadius: '3.40282e38px',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule55: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    color: themeColors['--color-brand'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    color: themeColors['--color-brand'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule4: {
    color: themeColors['--color-foreground-alt'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule5: {
    color: themeColors['--color-foreground-alt'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule6: {
    color: themeColors['--color-brand'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule7: {
    color: themeColors['--color-yellow-500'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule8: {
    color: themeColors['--color-brand'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule9: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
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

// WizardMode determines the operation type.
export type WizardMode = 'add' | 'remove'

// AddMethodType identifies the method type to add.
export type AddMethodType = 'pem' | 'passkey'

export interface AuthMutationWizardProps {
  open: boolean
  onClose: () => void
  mode: WizardMode
  account: Resource<Account>
  retainAfterClose?: boolean
  // For remove: the peer ID of the method being removed.
  removePeerId?: string
  removeMethodLabel?: string
  // For add: the method type being added.
  addMethodType?: AddMethodType
}

// stepLabels returns the step labels for the given mode.
function stepLabels(mode: WizardMode): string[] {
  if (mode === 'add') {
    return ['Provide credential', 'Unlock keypairs', 'Confirm']
  }
  return ['Confirm removal', 'Unlock keypairs', 'Execute']
}

// AuthMutationWizard provides a sequential step-through wizard for add/remove
// auth method operations. Each step must be completed before advancing. When
// auth_threshold > 0, the unlock step requires enough keypairs to be unlocked.
// For threshold=0 (single-sig), the unlock step is condensed.
export function AuthMutationWizard({
  account,
  ...props
}: AuthMutationWizardProps) {
  const state = useAccountDashboardState(account)
  if (state) {
    return (
      <AuthMutationWizardContent
        {...props}
        account={account}
        accountInfoResource={state.accountInfo}
      />
    )
  }

  return <AuthMutationWizardWithWatch {...props} account={account} />
}

function AuthMutationWizardWithWatch(props: AuthMutationWizardProps) {
  const accountInfoResource = useStreamingResource(
    props.account,
    (acc, signal) => acc.watchAccountInfo({}, signal),
    [],
  )

  return (
    <AuthMutationWizardContent
      {...props}
      accountInfoResource={accountInfoResource}
    />
  )
}

interface AuthMutationWizardContentProps extends AuthMutationWizardProps {
  accountInfoResource: Resource<WatchAccountInfoResponse>
}

// describeMutation returns the dialog title and description for the wizard
// operation.
function describeMutation(
  mode: WizardMode,
  addMethodType?: AddMethodType,
  removeMethodLabel?: string,
): { title: string; description: string } {
  if (mode === 'remove') {
    return {
      title: 'Remove auth method',
      description: `Remove "${removeMethodLabel ?? 'auth method'}" from your account.`,
    }
  }
  return {
    title: addMethodType === 'pem' ? 'Add backup key' : 'Add auth method',
    description: 'Step through the wizard to securely add a new auth method.',
  }
}

interface MutationExecutionOptions {
  account: Resource<Account>
  mode: WizardMode
  addMethodType?: AddMethodType
  removePeerId?: string
  cred: ReturnType<typeof useCredentialProof>
  canExecute: boolean
}

// useMutationExecution runs the add or remove operation once enough keypairs
// are unlocked, and tracks its progress and failure.
function useMutationExecution({
  account,
  mode,
  addMethodType,
  removePeerId,
  cred,
  canExecute,
}: MutationExecutionOptions) {
  const [error, setError] = useState<string | null>(null)
  const [executing, setExecuting] = useState(false)
  const [complete, setComplete] = useState(false)

  const execute = async () => {
    const mounted = account.value
    if (!mounted || !canExecute) return
    setExecuting(true)
    setError(null)
    try {
      if (mode === 'add' && addMethodType === 'pem') {
        const credential = cred.credential
        if (!credential) return
        const resp = await mounted.generateBackupKey({ credential })
        const data = resp.pemData
        if (data && data.length > 0) {
          downloadPemFile(data)
        }
      } else if (mode === 'remove' && removePeerId) {
        // In multi-sig mode the server uses unlocked keypairs to sign
        // internally, so pass an empty credential.
        await mounted.removeAuthMethod({
          peerId: removePeerId,
          credential: cred.credential ?? {
            credential: { case: 'password' as const, value: '' },
          },
        })
      }
      setComplete(true)
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Operation failed'
      setError(mapAuthError(msg))
    } finally {
      setExecuting(false)
    }
  }

  const reset = () => {
    setError(null)
    setExecuting(false)
    setComplete(false)
  }

  return { error, setError, executing, complete, execute, reset }
}

function AuthMutationWizardContent({
  open,
  onClose,
  mode,
  account,
  retainAfterClose = false,
  removePeerId,
  removeMethodLabel,
  addMethodType,
  accountInfoResource,
}: AuthMutationWizardContentProps) {
  // Derive the current step and required signatures from the mounted account.
  const [step, setStep] = useState(0)
  const cred = useCredentialProof()
  const {
    keypairs,
    unlockedCount,
    loading: keypairsLoading,
  } = useEntityKeypairs(account)
  const required = (accountInfoResource.value?.authThreshold ?? 0) + 1
  const canExecute = unlockedCount >= required
  const execution = useMutationExecution({
    account,
    mode,
    addMethodType,
    removePeerId,
    cred,
    canExecute,
  })
  const { title, description } = describeMutation(
    mode,
    addMethodType,
    removeMethodLabel,
  )

  // Reset credentials and operation progress when the wizard closes.
  const close = () => {
    // Lock keypairs unless the caller retains them across the close.
    if (!retainAfterClose) {
      void account.value?.lockAllEntityKeypairs().catch(() => {
        // best-effort cleanup
      })
    }
    setStep(0)
    execution.reset()
    cred.reset()
    onClose()
  }

  // Step 0 readiness depends on mode.
  const step0Ready = mode === 'add' ? cred.hasCredential : !!removePeerId

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) close()
      }}
    >
      <DialogContent sx={dialogStyles.content}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>

        <StepIndicator
          labels={stepLabels(mode)}
          current={step}
          complete={execution.complete}
        />

        <div sx={stylexStyles2.rule1}>
          {execution.complete ? (
            <CompleteView mode={mode} addMethodType={addMethodType} />
          ) : (
            <MutationStep
              step={step}
              mode={mode}
              removePeerId={removePeerId}
              removeMethodLabel={removeMethodLabel}
              addMethodType={addMethodType}
              cred={cred}
              keypairs={keypairs}
              unlockedCount={unlockedCount}
              required={required}
              keypairsLoading={keypairsLoading}
              account={account}
              onError={execution.setError}
            />
          )}

          {execution.error && <p sx={stylexStyles2.rule2}>{execution.error}</p>}
        </div>

        <DialogFooter>
          {execution.complete ? (
            <button type="button" onClick={close} sx={[stylexStyles2.rule3]}>
              Close
            </button>
          ) : (
            <MutationNav
              step={step}
              mode={mode}
              executing={execution.executing}
              canExecute={canExecute}
              step0Ready={step0Ready}
              onBack={() => {
                setStep((current) => current - 1)
                execution.setError(null)
              }}
              onNext={() => {
                execution.setError(null)
                setStep((current) => current + 1)
              }}
              onCancel={close}
              onExecute={() => void execution.execute()}
            />
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

interface MutationStepProps {
  step: number
  mode: WizardMode
  removePeerId?: string
  removeMethodLabel?: string
  addMethodType?: AddMethodType
  cred: ReturnType<typeof useCredentialProof>
  keypairs: EntityKeypairState[]
  unlockedCount: number
  required: number
  keypairsLoading: boolean
  account: Resource<Account>
  onError: (msg: string | null) => void
}

// MutationStep renders the content of the current wizard step.
function MutationStep({
  step,
  mode,
  removePeerId,
  removeMethodLabel,
  addMethodType,
  cred,
  keypairs,
  unlockedCount,
  required,
  keypairsLoading,
  account,
  onError,
}: MutationStepProps) {
  if (step === 0) {
    return (
      <Step0Content
        mode={mode}
        removePeerId={removePeerId}
        removeMethodLabel={removeMethodLabel}
        addMethodType={addMethodType}
        cred={cred}
        keypairs={keypairs}
      />
    )
  }
  if (step === 1) {
    return (
      <Step1Unlock
        keypairs={keypairs}
        unlockedCount={unlockedCount}
        required={required}
        loading={keypairsLoading}
        account={account}
        onError={onError}
      />
    )
  }
  return (
    <Step2Confirm
      mode={mode}
      addMethodType={addMethodType}
      removeMethodLabel={removeMethodLabel}
      keypairs={keypairs}
      unlockedCount={unlockedCount}
      required={required}
    />
  )
}

interface MutationNavProps {
  step: number
  mode: WizardMode
  executing: boolean
  canExecute: boolean
  step0Ready: boolean
  onBack: () => void
  onNext: () => void
  onCancel: () => void
  onExecute: () => void
}

// MutationNav renders the back, cancel, and next or execute buttons.
function MutationNav({
  step,
  mode,
  executing,
  canExecute,
  step0Ready,
  onBack,
  onNext,
  onCancel,
  onExecute,
}: MutationNavProps) {
  return (
    <>
      {step > 0 && (
        <button
          type="button"
          onClick={onBack}
          disabled={executing}
          sx={stylexStyles2.rule4}
        >
          <LuArrowLeft sx={stylexStyles.rule1} />
          Back
        </button>
      )}
      <div sx={stylexStyles2.rule5} />
      <button
        type="button"
        onClick={onCancel}
        disabled={executing}
        sx={stylexStyles2.rule6}
      >
        Cancel
      </button>
      {step < 2 ? (
        <button
          type="button"
          onClick={onNext}
          disabled={step === 0 && !step0Ready}
          sx={[stylexStyles2.rule7]}
        >
          Next
          <LuArrowRight sx={stylexStyles.rule1} />
        </button>
      ) : (
        <button
          type="button"
          onClick={onExecute}
          disabled={executing || !canExecute}
          sx={[
            stylexStyles2.rule8,
            mode === 'remove' ? stylexStyles2.rule9 : stylexStyles2.rule10,
            stylexStyles2.rule11,
          ]}
        >
          <ExecuteLabel mode={mode} executing={executing} />
        </button>
      )}
    </>
  )
}

// ExecuteLabel renders the execute button content for its state.
function ExecuteLabel({
  mode,
  executing,
}: {
  mode: WizardMode
  executing: boolean
}) {
  if (executing) {
    return (
      <>
        <Spinner size="sm" />
        Executing…
      </>
    )
  }
  if (mode === 'remove') {
    return (
      <>
        <LuTrash2 sx={stylexStyles.rule1} />
        Remove
      </>
    )
  }
  return (
    <>
      <LuDownload sx={stylexStyles.rule1} />
      Generate and download
    </>
  )
}

// StepIndicator renders the horizontal step bar.
function StepIndicator({
  labels,
  current,
  complete,
}: {
  labels: string[]
  current: number
  complete: boolean
}) {
  return (
    <div sx={stylexStyles2.rule12}>
      {labels.map((label, i) => {
        const done = complete || i < current
        const active = !complete && i === current
        return (
          <div key={label} sx={stylexStyles2.rule13}>
            <div
              sx={[
                stylexStyles2.rule14,
                done
                  ? stylexStyles2.rule15
                  : active
                    ? stylexStyles2.rule16
                    : stylexStyles2.rule17,
              ]}
            />
            <span
              sx={[
                stylexStyles2.rule18,
                done
                  ? stylexStyles2.rule19
                  : active
                    ? stylexStyles2.rule20
                    : stylexStyles2.rule21,
              ]}
            >
              {label}
            </span>
          </div>
        )
      })}
    </div>
  )
}

// Step0Content renders the first step content (credential for add, confirmation for remove).
function Step0Content({
  mode,
  removePeerId,
  removeMethodLabel,
  addMethodType,
  cred,
  keypairs,
}: {
  mode: WizardMode
  removePeerId?: string
  removeMethodLabel?: string
  addMethodType?: AddMethodType
  cred: ReturnType<typeof useCredentialProof>
  keypairs: EntityKeypairState[]
}) {
  if (mode === 'remove') {
    const truncated = truncatePeerId(removePeerId ?? '')
    return (
      <div>
        <div sx={[nativeChildSpacing.s3, stylexStyles2.rule22]}>
          <p sx={stylexStyles2.rule23}>
            Remove {removeMethodLabel ?? 'auth method'}
          </p>
          <p sx={stylexStyles2.rule24}>{truncated}</p>
        </div>
        <p sx={[nativeChildSpacing.s3, stylexStyles2.rule25]}>
          This action cannot be undone. You will need to unlock enough keypairs
          in the next step to authorize this removal.
        </p>
      </div>
    )
  }

  // Add mode: collect credential to prove identity.
  const hasPassword = keypairs.some(
    (kp) => kp.keypair?.authMethod === 'password',
  )

  return (
    <div>
      {addMethodType === 'pem' && (
        <div sx={[nativeChildSpacing.s3, stylexStyles2.rule26]}>
          <LuKey sx={stylexStyles.rule2} />
          <div>
            <p sx={stylexStyles2.rule23}>Backup key (.pem)</p>
            <p sx={stylexStyles2.rule27}>
              A new Ed25519 keypair will be generated and the private key
              downloaded as a .pem file for offline recovery.
            </p>
          </div>
        </div>
      )}

      <p sx={[nativeChildSpacing.s3, stylexStyles2.rule25]}>
        Confirm your identity to proceed. Enter your password or upload a backup
        key file.
      </p>

      <CredentialProofInput
        sx={nativeChildSpacing.s3}
        password={cred.password}
        onPasswordChange={cred.setPassword}
        pemFileName={cred.pemFileName}
        onFileChange={cred.handleFileChange}
        fileInputRef={cred.fileInputRef}
        showPassword={hasPassword}
        showPem={!hasPassword}
        pemLabel="Select your backup key file"
      />
    </div>
  )
}

// Step1Unlock renders the keypair unlock step.
function Step1Unlock({
  keypairs,
  unlockedCount,
  required,
  loading,
  account,
  onError,
}: {
  keypairs: EntityKeypairState[]
  unlockedCount: number
  required: number
  loading: boolean
  account: Resource<Account>
  onError: (msg: string | null) => void
}) {
  const ready = unlockedCount >= required

  return (
    <div>
      <div sx={[nativeChildSpacingNextNext.s3, stylexStyles2.rule28]}>
        <span sx={stylexStyles2.rule25}>
          {unlockedCount} of {required} unlocked
        </span>
        <div
          sx={[
            stylexStyles2.rule29,
            ready ? stylexStyles2.rule30 : stylexStyles2.rule31,
          ]}
        >
          {ready ? 'Ready' : 'Unlock more'}
        </div>
      </div>

      <div sx={[nativeChildSpacingNextNext.s3, stylexStyles2.rule32]}>
        <div
          sx={stylexStyles2.rule33}
          style={{
            '--progress-width': `${Math.min(100, (unlockedCount / required) * 100)}%`,
          }}
        />
      </div>

      {loading && (
        <p sx={[nativeChildSpacingNextNext.s3, stylexStyles2.rule25]}>
          Loading keypairs…
        </p>
      )}

      {!loading && keypairs.length > 0 && (
        <div sx={nativeChildSpacingNextNext.s3}>
          {keypairs.map((kp) => (
            <UnlockRow
              sx={nativeChildSpacingNext.s1_5}
              key={kp.keypair?.peerId ?? 'unknown'}
              keypairState={kp}
              account={account}
              onError={onError}
            />
          ))}
        </div>
      )}
    </div>
  )
}

// UnlockRow renders a single entity keypair with unlock controls.
function UnlockRow({
  sx,
  keypairState,
  account,
  onError,
}: {
  keypairState: EntityKeypairState
  account: Resource<Account>
  onError: (msg: string | null) => void
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const peerId = keypairState.keypair?.peerId ?? ''
  const method = keypairState.keypair?.authMethod ?? 'unknown'
  const unlocked = keypairState.unlocked ?? false
  const truncated = truncatePeerId(peerId)

  const cred = useCredentialProof()
  const { fileInputRef, handleFileChange } = cred
  const [unlocking, setUnlocking] = useState(false)

  const needsPassword = method === 'password'

  const handleUnlock = useCallback(async () => {
    if (!account.value || !peerId || !cred.credential) return

    setUnlocking(true)
    onError(null)
    try {
      await account.value.unlockEntityKeypair(peerId, cred.credential)
      cred.reset()
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Unlock failed'
      onError(mapAuthError(msg))
    } finally {
      setUnlocking(false)
    }
  }, [account.value, peerId, cred, onError])

  const handleLock = useCallback(async () => {
    if (!account.value || !peerId) return
    try {
      await account.value.lockEntityKeypair(peerId)
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Lock failed'
      onError(msg)
    }
  }, [account.value, peerId, onError])

  const canUnlock = needsPassword ? !!cred.password : !!cred.pemData

  if (unlocked) {
    return (
      <div sx={[stylexStyles2.rule34, sx]}>
        <div sx={stylexStyles2.rule35}>
          <LuLockOpen sx={stylexStyles.rule3} />
          <div sx={stylexStyles2.rule36}>
            <p sx={stylexStyles2.rule23}>{methodLabel(method)}</p>
            <p sx={stylexStyles2.rule37}>{truncated}</p>
          </div>
        </div>
        <button
          type="button"
          onClick={() => void handleLock()}
          sx={stylexStyles2.rule38}
        >
          Lock
        </button>
      </div>
    )
  }

  return (
    <div sx={[stylexStyles3.rule1, sx]}>
      <div sx={[nativeChildSpacing.s2, stylexStyles2.rule39]}>
        <LuLock sx={stylexStyles.rule4} />
        <div sx={stylexStyles2.rule40}>
          <p sx={stylexStyles2.rule23}>{methodLabel(method)}</p>
          <p sx={stylexStyles2.rule37}>{truncated}</p>
        </div>
      </div>

      <div sx={[nativeChildSpacing.s2, stylexStyles2.rule39]}>
        {needsPassword ? (
          <input
            type="password"
            value={cred.password}
            onChange={(e) => cred.setPassword(e.target.value)}
            aria-label="Password"
            placeholder="Enter password"
            onKeyDown={(e) => {
              if (e.key === 'Enter' && canUnlock) void handleUnlock()
            }}
            sx={[stylexStyles2.rule41]}
          />
        ) : (
          <>
            <button
              type="button"
              onClick={() => fileInputRef.current?.click()}
              sx={[stylexStyles2.rule42]}
            >
              <LuUpload sx={stylexStyles.rule5} />
              <span sx={[!cred.pemFileName && stylexStyles2.rule21]}>
                {cred.pemFileName ?? 'Choose .pem file'}
              </span>
            </button>
            <input
              ref={fileInputRef}
              type="file"
              aria-label="Backup key file"
              accept=".pem"
              onChange={handleFileChange}
              sx={stylexStyles2.rule43}
            />
          </>
        )}
        <button
          type="button"
          onClick={() => void handleUnlock()}
          disabled={unlocking || !canUnlock}
          sx={[stylexStyles2.rule44]}
        >
          {unlocking ? '...' : 'Unlock'}
        </button>
      </div>
    </div>
  )
}

// Step2Confirm renders the confirmation summary before execution.
function Step2Confirm({
  mode,
  addMethodType,
  removeMethodLabel,
  keypairs,
  unlockedCount,
  required,
}: {
  mode: WizardMode
  addMethodType?: AddMethodType
  removeMethodLabel?: string
  keypairs: EntityKeypairState[]
  unlockedCount: number
  required: number
}) {
  const ready = unlockedCount >= required
  const signers = keypairs.filter((kp) => kp.unlocked)

  return (
    <div>
      <div
        sx={[
          nativeChildSpacing.s3,
          [
            stylexStyles2.rule45,
            ready ? stylexStyles2.rule46 : stylexStyles2.rule47,
          ],
        ]}
      >
        <div sx={stylexStyles2.rule39}>
          {ready ? (
            <LuShieldCheck sx={stylexStyles.rule6} />
          ) : (
            <LuLock sx={stylexStyles.rule7} />
          )}
          <p sx={stylexStyles2.rule23}>
            {ready
              ? 'Ready to execute'
              : `Need ${required - unlockedCount} more unlock${required - unlockedCount === 1 ? '' : 's'}`}
          </p>
        </div>

        {!ready && (
          <p sx={stylexStyles2.rule48}>
            Go back and unlock more keypairs to meet the threshold.
          </p>
        )}
      </div>

      <div sx={nativeChildSpacing.s3}>
        <p sx={stylexStyles2.rule49}>Operation</p>
        <p sx={stylexStyles2.rule50}>
          {mode === 'add'
            ? addMethodType === 'pem'
              ? 'Generate and download a new backup key (.pem)'
              : 'Add a new auth method'
            : `Remove "${removeMethodLabel ?? 'auth method'}"`}
        </p>
      </div>

      {signers.length > 0 && (
        <div sx={nativeChildSpacing.s3}>
          <p sx={stylexStyles2.rule49}>Signing keypairs ({signers.length})</p>
          <div>
            {signers.map((kp) => {
              const pid = kp.keypair?.peerId ?? ''
              const truncated = truncatePeerId(pid)
              return (
                <div
                  key={pid}
                  sx={[nativeChildSpacingNextNext.s1, stylexStyles2.rule51]}
                >
                  <LuCheck sx={stylexStyles.rule8} />
                  <span sx={stylexStyles2.rule20}>
                    {methodLabel(kp.keypair?.authMethod ?? '')}
                  </span>
                  <span sx={stylexStyles2.rule52}>{truncated}</span>
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}

// CompleteView renders the success state.
function CompleteView({
  mode,
  addMethodType,
}: {
  mode: WizardMode
  addMethodType?: AddMethodType
}) {
  return (
    <div sx={stylexStyles2.rule53}>
      <div sx={stylexStyles2.rule54}>
        <LuCheck sx={stylexStyles.rule9} />
      </div>
      <p sx={stylexStyles2.rule23}>
        {mode === 'add'
          ? addMethodType === 'pem'
            ? 'Backup key generated and downloaded'
            : 'Auth method added'
          : 'Auth method removed'}
      </p>
      <p sx={stylexStyles2.rule55}>
        {mode === 'add'
          ? addMethodType === 'pem'
            ? 'Store the .pem file in a safe location. You will need it to recover your account.'
            : 'The new auth method is now active on your account.'
          : 'The auth method has been permanently removed from your account.'}
      </p>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNextNext = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
  },
})
