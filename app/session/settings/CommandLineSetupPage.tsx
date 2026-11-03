import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { isDesktop } from '@aptre/bldr'
import { useBldrContext } from '@aptre/bldr-react'
import {
  LuArrowLeft,
  LuArrowUpRight,
  LuDownload,
  LuCircle,
  LuRefreshCw,
  LuSettings,
  LuTerminal,
  LuUsers,
} from '@s4wave/web/images/icons.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { Client as ResourceClient } from '@aptre/bldr-sdk/resource/index.js'
import { ResourceServiceClient } from '@aptre/bldr-sdk/resource/resource_srpc.pb.js'
import { Client as SRPCClient } from 'starpc'
import type { Root } from '@s4wave/sdk/root'
import {
  DesktopCLIInstallActionKind,
  DesktopCLIInstallStatus,
  type DesktopCLIInstallActionItem,
  type DesktopCLIInstallTarget,
  type DesktopCLIEntrypointIdentity,
  type DesktopCLIInstallState,
  type WatchCLIInstallStateResponse,
} from '@go/github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime/desktop-runtime.pb.js'
import { DesktopCLIInstallResourceServiceClient } from '@go/github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime/desktop-runtime_srpc.pb.js'

import { useRuntimeHandoff } from '@s4wave/app/listener/RuntimeHandoffContext.js'
import {
  useListenerStatus,
  type ListenerStatus,
} from '@s4wave/app/hooks/useListenerStatus.js'
import { useShellTabs } from '@s4wave/app/ShellTabContext.js'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { useRootResourceWithClient } from '@s4wave/web/hooks/useRootResource.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { Button } from '@s4wave/web/ui/button.js'
import { CopyButton } from '@s4wave/web/ui/CopyButton.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/persist.js'

import {
  buildSpacewaveCommand,
  type CommandOptions,
} from './command-line-commands.js'

import * as stylex from '@stylexjs/stylex'

import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  motion as themeMotion,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    containerType: 'inline-size',
    display: 'flex',
    minWidth: 0,
    flex: 1,
    flexDirection: 'column',
    overflowY: 'auto',
    backgroundColor: themeColors['--color-background-landing'],
  },
  rule2: {
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-3xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 5)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
  },
  rule3: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'inline-flex',
    minHeight: `calc(${themeLayout['--spacing']} * 9)`,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '.25rem',
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
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-brand'],
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
  rule4: { marginBottom: `calc(${themeLayout['--spacing']} * 7)` },
  rule5: {
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule6: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule7: {
    display: 'flex',
    flexDirection: {
      default: 'column',
      '@container (min-width: 32rem)': 'row',
    },
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    justifyContent: {
      default: null,
      '@container (min-width: 32rem)': 'space-between',
    },
  },
  rule8: { minWidth: 0 },
  rule9: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule10: {
    marginTop: themeLayout['--spacing'],
    maxWidth: 'var(--container-md)',
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule11: {
    marginTop: `calc(${themeLayout['--spacing']} * 5)`,
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
  },
  rule12: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 3%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule13: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: themeColors['--color-foreground'],
  },
  rule14: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    columnGap: `calc(${themeLayout['--spacing']} * 4)`,
    rowGap: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule15: {
    width: `calc(${themeLayout['--spacing']} * 24)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule16: {
    flex: 1,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule17: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule18: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule19: { minWidth: 0, flex: 1 },
  rule20: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule21: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule22: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule23: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 48rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule24: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-amber-500']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-amber-500']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule25: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule26: {
    marginTop: themeLayout['--spacing'],
    display: 'block',
    maxWidth: '100%',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: '.7rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 90%, transparent)`,
  },
  rule27: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-danger'],
  },
  rule28: {
    marginBottom: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
    textTransform: 'uppercase',
  },
  rule29: {
    display: 'block',
    maxWidth: '100%',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: themeLayout['--spacing'],
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: '.7rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 90%, transparent)`,
  },
  rule30: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule31: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule32: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule33: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
    textTransform: 'uppercase',
  },
  rule34: { display: 'grid', gap: `calc(${themeLayout['--spacing']} * 2)` },
  rule35: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.7 },
  },
  rule36: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 2%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.7 },
  },
  rule37: {
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule38: {
    marginTop: themeLayout['--spacing'],
    display: 'block',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: '.68rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 90%, transparent)`,
  },
  rule39: {
    marginTop: themeLayout['--spacing'],
    display: 'block',
    fontSize: '.68rem',
    lineHeight: 1.5,
    color: themeColors['--color-foreground-alt'],
  },
  rule40: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
  rule41: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 7)`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
      ':disabled': `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    cursor: { default: null, ':disabled': 'not-allowed' },
  },
  rule42: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule43: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule44: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule45: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule46: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexWrap: 'wrap',
    columnGap: `calc(${themeLayout['--spacing']} * 5)`,
    rowGap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule47: {
    display: 'inline-flex',
    minHeight: `calc(${themeLayout['--spacing']} * 9)`,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '.25rem',
    color: {
      default: themeColors['--color-foreground'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-brand'],
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
  rule48: {
    display: 'inline-flex',
    minHeight: `calc(${themeLayout['--spacing']} * 9)`,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: '.25rem',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-brand'],
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
  },
  rule49: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule50: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule51: {
    width: 'fit-content',
    maxWidth: '100%',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    fontVariantLigatures: 'none',
  },
  rule52: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule53: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule54: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-brand'],
  },
  rule55: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule56: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule57: {
    minWidth: 0,
    flex: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    fontVariantLigatures: 'none',
  },
  rule58: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule59: { display: 'flex', minWidth: 0, flex: 1, flexDirection: 'column' },
  rule60: {
    width: 'fit-content',
    maxWidth: '100%',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: '.65rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 80%, transparent)`,
  },
  rule61: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule62: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule63: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule64: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule65: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    fontSize: '.65rem',
    lineHeight: 1.5,
    color: themeColors['--color-foreground-alt'],
  },
  rule66: {
    fontSize: '.7rem',
    lineHeight: 1.5,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: {
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
    flexShrink: 0,
    fill: 'currentColor',
  },
  rule5: { color: themeColors['--color-emerald-500'] },
  rule6: { color: themeColors['--color-brand'] },
  rule7: { color: themeColors['--color-danger'] },
  rule8: { color: themeColors['--color-amber-500'] },
  rule9: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
})

const stylexStyles = stylex.create({ rule1: { flexShrink: 0 } })

// CommandLineSetupPage launches the session CLI and offers desktop installation.
export function CommandLineSetupPage() {
  const navigate = useNavigate()
  const sessionIdx = useSessionIndex()
  const status = useListenerStatus()
  const desktopRootResource = useDesktopRuntimeRootResource()
  const { activeTabId, openPathInActiveTabset } = useShellTabs()
  const cliInstall = useDesktopCLIInstallState(desktopRootResource)

  const handleBack = useCallback(() => {
    navigate({ path: '../../' })
  }, [navigate])

  const handleOpenTerminal = useCallback(() => {
    const terminalPath =
      sessionIdx != null
        ? `/u/${sessionIdx}/settings/cli/terminal`
        : '/settings/cli/terminal'
    openPathInActiveTabset(terminalPath, {
      afterTabId: activeTabId,
      focusExisting: false,
      select: true,
    })
  }, [activeTabId, openPathInActiveTabset, sessionIdx])
  const handleCLIInstallAction = useCallback(
    async (action: DesktopCLIInstallActionItem) => {
      const root = desktopRootResource.value
      if (!root) return
      const service = new DesktopCLIInstallResourceServiceClient(root.client)
      await service.InvokeCLIInstallAction({
        actionId: action.id,
        generation: action.generation,
      })
    },
    [desktopRootResource.value],
  )

  const opts: CommandOptions = useMemo(
    () => ({
      sessionIndex: sessionIdx,
      socketPath: status?.socketPath || '',
    }),
    [sessionIdx, status?.socketPath],
  )

  return (
    <div sx={stylexStyles4.rule1}>
      <div sx={stylexStyles4.rule2}>
        <button type="button" onClick={handleBack} sx={stylexStyles4.rule3}>
          <LuArrowLeft aria-hidden="true" sx={stylexStyles2.rule1} />
          Back to dashboard
        </button>

        <header sx={stylexStyles4.rule4}>
          <h1 sx={stylexStyles4.rule5}>Command Line</h1>
          <p sx={stylexStyles4.rule6}>
            Work with your spaces from the Spacewave CLI.
          </p>
        </header>

        <InAppTerminalLauncher onOpen={handleOpenTerminal} />

        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-4 */ 'border-foreground/10 mt-8 space-y-4 border-t pt-6'
          }
        >
          <InstallGuidanceSection />
          {isDesktop && (
            <DesktopCLIInstallCard
              state={cliInstall.value?.state}
              loading={cliInstall.loading}
              error={cliInstall.error}
              onInvokeAction={handleCLIInstallAction}
            />
          )}
          {isDesktop && <ListenerStatusChip />}
          {isDesktop && <WalkthroughSection opts={opts} />}
          {isDesktop && <MoreCommandsSection opts={opts} />}
        </div>
      </div>
    </div>
  )
}

// InAppTerminalLauncher introduces the built-in CLI and its first commands.
function InAppTerminalLauncher({ onOpen }: { onOpen: () => void }) {
  return (
    <section aria-labelledby="built-in-cli-title">
      <div sx={stylexStyles4.rule7}>
        <div sx={stylexStyles4.rule8}>
          <h2 id="built-in-cli-title" sx={stylexStyles4.rule9}>
            Built-in terminal
          </h2>
          <p sx={stylexStyles4.rule10}>
            Open a new shell tab with the Spacewave CLI, connected to this
            session. No installation needed.
          </p>
        </div>
        <Button
          type="button"
          variant="brandOutline"
          onClick={onOpen}
          sx={stylexStyles.rule1}
        >
          <LuTerminal aria-hidden="true" />
          Open terminal
        </Button>
      </div>

      <div sx={stylexStyles4.rule11}>
        <div sx={stylexStyles4.rule12}>
          Try these at the <code sx={stylexStyles4.rule13}>spacewave&gt;</code>{' '}
          prompt
        </div>
        <dl
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: divide-y */ 'divide-foreground/8 divide-y'
          }
        >
          {[
            ['help', 'Explore available commands'],
            ['status', 'Check the current session'],
            ['space list', 'List your spaces'],
          ].map(([command, description]) => (
            <div key={command} sx={stylexStyles4.rule14}>
              <dt sx={stylexStyles4.rule15}>{command}</dt>
              <dd sx={stylexStyles4.rule16}>{description}</dd>
              <CopyButton text={command} label={`Copy ${command}`} />
            </div>
          ))}
        </dl>
      </div>
    </section>
  )
}

function useDesktopRuntimeRootResource(): Resource<Root> {
  const bldrContext = useBldrContext()
  const webDocument = bldrContext?.webDocument
  const [resourceClient, setResourceClient] = useState<ResourceClient | null>(
    null,
  )

  useEffect(() => {
    if (!isDesktop || !webDocument) return

    const abortController = new AbortController()
    const rpcClient = new SRPCClient(
      webDocument.openWebDocumentHostStream.bind(webDocument),
    )
    const service = new ResourceServiceClient(rpcClient)
    const client = new ResourceClient(service, abortController.signal)
    setResourceClient(client)

    return () => {
      client.dispose()
      setResourceClient(null)
      abortController.abort()
    }
  }, [webDocument])

  return useRootResourceWithClient(resourceClient)
}

function useDesktopCLIInstallState(rootResource: Resource<Root>) {
  return useStreamingResource(
    rootResource,
    (root: Root, signal: AbortSignal) =>
      watchDesktopCLIInstallState(root, signal),
    [],
  )
}

async function* watchDesktopCLIInstallState(
  root: Root,
  signal: AbortSignal,
): AsyncIterable<WatchCLIInstallStateResponse> {
  if (!isDesktop) return
  const service = new DesktopCLIInstallResourceServiceClient(root.client)
  for await (const item of service.WatchCLIInstallState({}, signal)) {
    yield item
  }
}

// DESKTOP_CLI_CHECK_TIMEOUT_MS bounds the "Checking command line tool" state:
// past this the card reports the check as failed instead of pending forever
// when the desktop runtime never yields an install state.
const DESKTOP_CLI_CHECK_TIMEOUT_MS = 15000

// useCheckTimedOut reports whether the install state check has stayed pending
// for DESKTOP_CLI_CHECK_TIMEOUT_MS. The deadline restarts with each check.
function useCheckTimedOut(checking: boolean): boolean {
  const [checkTimedOut, setCheckTimedOut] = useState(false)
  useEffect(() => {
    if (!checking) {
      setCheckTimedOut(false)
      return
    }
    const timer = setTimeout(
      () => setCheckTimedOut(true),
      DESKTOP_CLI_CHECK_TIMEOUT_MS,
    )
    return () => clearTimeout(timer)
  }, [checking])
  return checkTimedOut
}

// describeTarget returns the detail line for an install target.
function describeTarget(target: DesktopCLIInstallTarget): string {
  if (target.detail) return target.detail
  return target.writable ? 'Writable user target' : 'Manual target review'
}

// visibleInstallError returns the install state's error message, hiding the
// unimplemented error that the presentation reports instead.
function visibleInstallError(state?: DesktopCLIInstallState): string {
  const message = state?.errorMessage ?? ''
  return message.toLowerCase().includes('unimplemented') ? '' : message
}

export function DesktopCLIInstallCard({
  sx,
  state,
  loading,
  error,
  onInvokeAction,
}: {
  state?: DesktopCLIInstallState
  loading?: boolean
  error?: Error | null
  onInvokeAction?: (action: DesktopCLIInstallActionItem) => void | Promise<void>
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const checkTimedOut = useCheckTimedOut(!error && (loading || !state))
  const presentation = desktopCLIInstallPresentation(
    state,
    loading,
    error,
    checkTimedOut,
  )
  const actions = state?.actions ?? []
  const errorMessage = visibleInstallError(state)
  return (
    <section sx={[stylexStyles4.rule17, sx]}>
      <div sx={stylexStyles4.rule18}>
        <StatusDot tone={presentation.tone} />
        <div sx={stylexStyles4.rule19}>
          <h2 sx={stylexStyles4.rule20}>Desktop CLI install</h2>
          <p sx={stylexStyles4.rule21}>{presentation.label}</p>
        </div>
        <ActionButtons actions={actions} onInvokeAction={onInvokeAction} />
      </div>

      {presentation.detail && (
        <p sx={stylexStyles4.rule22}>{presentation.detail}</p>
      )}

      <div sx={stylexStyles4.rule23}>
        <SelectedTarget state={state} />
        <ReleaseIdentity state={state} />
      </div>

      <TargetOptions
        state={state}
        actions={actions}
        onInvokeAction={onInvokeAction}
      />

      {state?.conflictPath && (
        <div sx={stylexStyles4.rule24}>
          <p sx={stylexStyles4.rule25}>PATH conflict</p>
          <code sx={stylexStyles4.rule26}>{state.conflictPath}</code>
        </div>
      )}

      {errorMessage && <p sx={stylexStyles4.rule27}>{errorMessage}</p>}
    </section>
  )
}

// SelectedTarget shows the install target the CLI will be installed to.
function SelectedTarget({ state }: { state?: DesktopCLIInstallState }) {
  const selectedTarget = state?.targets?.find((target) => target.selected)

  return (
    <div sx={stylexStyles4.rule8}>
      <p sx={stylexStyles4.rule28}>Selected target</p>
      <code sx={stylexStyles4.rule29}>
        {selectedTarget?.path || 'Not selected'}
      </code>
      {selectedTarget && (
        <p sx={stylexStyles4.rule30}>{describeTarget(selectedTarget)}</p>
      )}
    </div>
  )
}

// ReleaseIdentity shows the available and installed CLI releases.
function ReleaseIdentity({ state }: { state?: DesktopCLIInstallState }) {
  return (
    <div sx={stylexStyles4.rule8}>
      <p sx={stylexStyles4.rule28}>Release identity</p>
      <p sx={stylexStyles4.rule31}>
        {formatCLIIdentity(state?.available) || 'Waiting for release metadata'}
      </p>
      {state?.installed?.manifestId && (
        <p sx={stylexStyles4.rule30}>
          Installed {formatCLIIdentity(state.installed)}
        </p>
      )}
    </div>
  )
}

function TargetOptions({
  state,
  actions,
  onInvokeAction,
}: {
  state?: DesktopCLIInstallState
  actions: DesktopCLIInstallActionItem[]
  onInvokeAction?: (action: DesktopCLIInstallActionItem) => void | Promise<void>
}) {
  const targets = state?.targets ?? []
  if (targets.length <= 1) return null
  return (
    <div sx={stylexStyles4.rule32}>
      <p sx={stylexStyles4.rule33}>Install target</p>
      <div sx={stylexStyles4.rule34}>
        {targets.map((target) => {
          const action = actions.find(
            (item) =>
              item.kind ===
                DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_SELECT_TARGET &&
              item.targetId === target.id,
          )
          const selected = target.selected ?? false
          return (
            <button
              key={target.id || target.path}
              type="button"
              disabled={selected || !(action?.enabled ?? false)}
              aria-pressed={selected}
              aria-label={`Use ${target.label || target.path || target.id}`}
              onClick={() => action && void onInvokeAction?.(action)}
              sx={selected ? stylexStyles4.rule35 : stylexStyles4.rule36}
            >
              <span sx={stylexStyles4.rule37}>
                {target.label || target.id || 'Target'}
              </span>
              <code sx={stylexStyles4.rule38}>{target.path}</code>
              <span sx={stylexStyles4.rule39}>{describeTarget(target)}</span>
            </button>
          )
        })}
      </div>
    </div>
  )
}

function ActionButtons({
  actions,
  onInvokeAction,
}: {
  actions: DesktopCLIInstallActionItem[]
  onInvokeAction?: (action: DesktopCLIInstallActionItem) => void | Promise<void>
}) {
  const visibleActions = actions.filter((action) => {
    switch (action.kind) {
      case DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_RECHECK:
      case DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_OPEN_SETTINGS:
      case DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_INSTALL:
      case DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_UPDATE:
        return true
      default:
        return false
    }
  })
  if (visibleActions.length === 0) return null

  return (
    <div sx={stylexStyles4.rule40}>
      {visibleActions.map((action) => {
        const Icon =
          action.kind ===
          DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_RECHECK
            ? LuRefreshCw
            : action.kind ===
                  DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_INSTALL ||
                action.kind ===
                  DesktopCLIInstallActionKind.DESKTOP_CLI_INSTALL_ACTION_KIND_UPDATE
              ? LuDownload
              : LuSettings
        return (
          <button
            key={action.id}
            type="button"
            disabled={!(action.enabled ?? false)}
            title={action.label || action.id}
            aria-label={action.label || action.id}
            onClick={() => void onInvokeAction?.(action)}
            sx={stylexStyles4.rule41}
          >
            <Icon sx={stylexStyles3.rule1} />
          </button>
        )
      })}
    </div>
  )
}

function desktopCLIInstallPresentation(
  state: DesktopCLIInstallState | undefined,
  loading: boolean | undefined,
  error: Error | null | undefined,
  checkTimedOut?: boolean,
): {
  tone: 'ready' | 'warning' | 'muted' | 'active' | 'error'
  label: string
  detail: string
} {
  if (error) {
    if (error.message.toLowerCase().includes('unimplemented')) {
      return {
        tone: 'muted',
        label: 'Desktop CLI install unavailable',
        detail:
          'This desktop build has not exposed managed CLI install yet. You can still use the built-in terminal.',
      }
    }
    return {
      tone: 'error',
      label: 'Install state unavailable',
      detail:
        'The desktop runtime could not load managed CLI install state. Use the built-in terminal, then check desktop logs if this persists.',
    }
  }
  if (loading || !state) {
    if (checkTimedOut) {
      return {
        tone: 'error',
        label: 'Install state check timed out',
        detail:
          'The desktop runtime did not report CLI install state. Use the built-in terminal, or restart the desktop app and reopen this page.',
      }
    }
    return {
      tone: 'muted',
      label: 'Checking command line tool',
      detail: '',
    }
  }
  switch (state.status) {
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_INSTALLED:
      return {
        tone: 'ready',
        label: state.label || 'Command line tool installed',
        detail: state.detail ?? '',
      }
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_UPDATE_AVAILABLE:
      return {
        tone: 'warning',
        label: state.label || 'Command line tool update available',
        detail: state.detail ?? '',
      }
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_CONFLICT:
      return {
        tone: 'warning',
        label: state.label || 'Command line tool conflict',
        detail: state.detail ?? '',
      }
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_ERROR:
      return {
        tone: 'error',
        label: state.label || 'Command line tool check failed',
        detail: state.detail ?? '',
      }
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_INSTALLING:
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_UPDATING:
      return {
        tone: 'active',
        label: state.label || 'Command line tool update running',
        detail: state.detail ?? '',
      }
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_MISSING:
      return {
        tone: 'muted',
        label: state.label || 'Command line tool not installed',
        detail: state.detail ?? '',
      }
    default:
      return {
        tone: 'muted',
        label: state.label || 'Checking command line tool',
        detail: state.detail ?? '',
      }
  }
}

function formatCLIIdentity(
  identity: DesktopCLIEntrypointIdentity | undefined,
): string {
  if (!identity?.manifestId) return ''
  const rev = identity.manifestRev ? ` rev ${identity.manifestRev}` : ''
  const platform = identity.platformId ? ` (${identity.platformId})` : ''
  return `${identity.manifestId}${rev}${platform}`
}

// WalkthroughSection renders the three-step CLI walkthrough bound to
// the active session (status, whoami, space list).
export function WalkthroughSection({
  sx,
  opts,
}: { opts: CommandOptions } & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <section sx={[stylexStyles4.rule17, sx]}>
      <h2 sx={stylexStyles4.rule42}>Try it out</h2>
      <p sx={stylexStyles4.rule43}>
        Run these three commands in a terminal to confirm the CLI is talking to
        this session.
      </p>
      <ol>
        <CommandStep
          sx={nativeChildSpacing.s3}
          index={1}
          command={buildSpacewaveCommand('status', opts)}
          explanation="Proves the CLI reached the desktop app session and reports socket, lock, and space count."
        />
        <CommandStep
          sx={nativeChildSpacing.s3}
          index={2}
          command={buildSpacewaveCommand('whoami', opts)}
          explanation="Confirms the session identity the CLI is acting as."
        />
        <CommandStep
          sx={nativeChildSpacing.s3}
          index={3}
          command={buildSpacewaveCommand('space list', opts)}
          explanation="Lists the spaces visible to this session."
        />
      </ol>
    </section>
  )
}

// InstallGuidanceSection links to the desktop CLI for an external shell.
function InstallGuidanceSection({
  sx,
}: { sx?: stylex.StyleXStyles; className?: never } = {}) {
  const cliDownloadHref = useStaticHref('/download/cli')
  const cliInstallHref = useStaticHref('/docs/users/cli/install')

  return (
    <section sx={sx} aria-labelledby="desktop-cli-title">
      <h2 id="desktop-cli-title" sx={stylexStyles4.rule9}>
        Use your own terminal
      </h2>
      <p sx={stylexStyles4.rule44}>
        Install the desktop CLI to run{' '}
        <code sx={stylexStyles4.rule45}>spacewave</code> commands from your
        computer’s shell. It connects to the Spacewave desktop app.
      </p>
      <div sx={stylexStyles4.rule46}>
        <a href={cliDownloadHref} sx={stylexStyles4.rule47}>
          <LuDownload aria-hidden="true" sx={stylexStyles2.rule1} />
          Download the spacewave CLI
        </a>
        <a href={cliInstallHref} sx={stylexStyles4.rule48}>
          Install and quickstart guide
          <LuArrowUpRight aria-hidden="true" sx={stylexStyles2.rule2} />
        </a>
      </div>
    </section>
  )
}

// MoreCommandsSection renders a collapsed panel that links to the
// next set of useful CLI commands without pulling them into the
// walkthrough. Covers the web listener, file listing, git helpers, and
// the developer docs index.
function MoreCommandsSection({ opts }: { opts: CommandOptions }) {
  const ns = useStateNamespace(['cli-setup'])
  const [open, setOpen] = useStateAtom<boolean>(ns, 'more-open', false)
  const developerCliHref = useStaticHref(
    '/docs/developers/cli/installation-and-commands',
  )

  return (
    <CollapsibleSection
      title="More commands"
      open={open}
      onOpenChange={setOpen}
    >
      <div>
        <ul sx={nativeChildSpacingNext.s3}>
          <MoreCommandRow
            sx={nativeChildSpacing.s2}
            command={buildSpacewaveCommand('web --bg', opts)}
            explanation="Open a local web listener that stays running in the background."
          />
          <MoreCommandRow
            sx={nativeChildSpacing.s2}
            command={buildSpacewaveCommand('fs ls', opts)}
            explanation="List files in the current space."
          />
          <MoreCommandRow
            sx={nativeChildSpacing.s2}
            command={buildSpacewaveCommand('git', opts)}
            explanation="Drive git helpers against a selected space."
          />
        </ul>
        <a
          href={developerCliHref}
          sx={[nativeChildSpacingNext.s3, stylexStyles4.rule49]}
        >
          Developer CLI reference
        </a>
      </div>
    </CollapsibleSection>
  )
}

// MoreCommandRow is a compact command listing row without a copy
// button. The walkthrough step handles copy; this section is for
// orientation only.
function MoreCommandRow({
  sx,
  command,
  explanation,
}: {
  command: string
  explanation: string
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <li sx={[stylexStyles4.rule50, sx]}>
      <code sx={stylexStyles4.rule51}>{command}</code>
      <p sx={stylexStyles4.rule52}>{explanation}</p>
    </li>
  )
}

// CommandStep renders a single walkthrough row with a copy button.
function CommandStep({
  sx,
  index,
  command,
  explanation,
}: {
  index: number
  command: string
  explanation: string
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <li sx={[stylexStyles4.rule53, sx]}>
      <div sx={stylexStyles4.rule54}>{index}</div>
      <div sx={stylexStyles4.rule55}>
        <div sx={stylexStyles4.rule56}>
          <code sx={stylexStyles4.rule57}>{command}</code>
          <CopyButton text={command} label="Copy command" />
        </div>
        <p sx={stylexStyles4.rule52}>{explanation}</p>
      </div>
    </li>
  )
}

// ListenerStatusChip renders a compact row showing the desktop
// resource listener's live status: socket path, Ready / Not listening
// state, and connected-client count. On non-desktop builds (WASM in the
// browser, where the listener is a no-op), renders a note that the CLI
// runs against the desktop app instead of a misleading "Not listening"
// state. While a remote runtime has taken the socket, shows "Not
// listening" with a note pointing at the reclaim affordance.
function ListenerStatusChip({
  sx,
}: { sx?: stylex.StyleXStyles; className?: never } = {}) {
  const status = useListenerStatus()
  const handoff = useRuntimeHandoff()

  if (!isDesktop) {
    return (
      <div sx={[stylexStyles4.rule58, sx]}>
        <StatusDot tone="muted" />
        <div sx={stylexStyles4.rule59}>
          <span sx={stylexStyles4.rule25}>
            CLI runs against the desktop app
          </span>
          <span sx={stylexStyles4.rule52}>
            Install the Spacewave desktop app to expose a local CLI socket.
          </span>
        </div>
      </div>
    )
  }

  const { tone, label } = listenerToneLabel(status, handoff.active)
  const listening = !!status?.listening && !handoff.active

  let socketRow: ReactNode
  if (status?.socketPath) {
    socketRow = <code sx={stylexStyles4.rule60}>{status.socketPath}</code>
  } else {
    socketRow = (
      <span sx={stylexStyles4.rule52}>Socket path not yet resolved.</span>
    )
  }

  return (
    <div sx={[[stylexStyles4.rule61], sx]}>
      <StatusDot tone={tone} />
      <div sx={stylexStyles4.rule62}>
        <div sx={stylexStyles4.rule63}>
          <span sx={stylexStyles4.rule64}>{label}</span>
          {listening && (
            <span sx={stylexStyles4.rule65}>
              <LuUsers sx={stylexStyles2.rule3} />
              {status?.connectedClients ?? 0} connected
            </span>
          )}
        </div>
        {socketRow}
        {handoff.active && (
          <span sx={stylexStyles4.rule66}>
            Runtime is handed off to{' '}
            {handoff.requesterName || 'spacewave serve'}. Reclaim it from the
            banner above to resume listening.
          </span>
        )}
      </div>
    </div>
  )
}

// listenerToneLabel returns the desktop listener chip tone and label
// for a given status/handoff pair. Caller must have already gated on
// isDesktop; this helper is only valid for the desktop path.
function listenerToneLabel(
  status: ListenerStatus | null,
  handoffActive: boolean,
): { tone: 'ready' | 'warning' | 'muted'; label: string } {
  if (handoffActive) return { tone: 'warning', label: 'Not listening' }
  if (status == null) return { tone: 'muted', label: 'Checking...' }
  if (status.listening) return { tone: 'ready', label: 'Ready' }
  return { tone: 'muted', label: 'Not listening' }
}

// StatusDot renders a small filled circle whose color matches the
// listener state tone.
function StatusDot({
  tone,
}: {
  tone: 'ready' | 'warning' | 'muted' | 'active' | 'error'
}) {
  return (
    <LuCircle
      sx={[
        stylexStyles2.rule4,
        tone === 'ready'
          ? stylexStyles2.rule5
          : tone === 'active'
            ? stylexStyles2.rule6
            : tone === 'error'
              ? stylexStyles2.rule7
              : tone === 'warning'
                ? stylexStyles2.rule8
                : stylexStyles2.rule9,
      ]}
      aria-hidden="true"
    />
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
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
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
