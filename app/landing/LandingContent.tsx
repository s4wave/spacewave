import * as ownerStylex from '@stylexjs/stylex'

import type { IconType } from '@s4wave/web/images/icons.js'
import React, { useCallback, useState, useMemo } from 'react'
import { cn } from '@s4wave/web/style/utils.js'
import { useScrollReveal } from './useScrollReveal.js'
import {
  LuBookOpen,
  LuCheck,
  LuCloudOff,
  LuCircuitBoard,
  LuCode,
  LuCpu,
  LuCreditCard,
  LuDatabase,
  LuDownload,
  LuEyeOff,
  LuFolderSync,
  LuGift,
  LuGithub,
  LuGlobe,
  LuLaptop,
  LuLock,
  LuMessageSquare,
  LuMousePointer,
  LuPlus,
  LuRocket,
  LuServer,
  LuShield,
  LuSmartphone,
  LuTerminal,
  LuUsers,
  LuUserX,
  LuWifiOff,
  LuX,
  LuZap,
} from '@s4wave/web/images/icons.js'
import {
  PiRocketLaunchDuotone,
  PiAppStoreLogoBold,
} from '@s4wave/web/ui/phosphor-icons.js'
import { isDesktop } from '@aptre/bldr'
import { GITHUB_REPO_URL } from '@s4wave/app/github.js'
import { useNavLinks } from '@s4wave/app/nav-links.js'
import {
  useIsStaticMode,
  useStaticHref,
} from '@s4wave/app/prerender/StaticContext.js'
import { ExternalLink } from './ExternalLink.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

import { utilityState as themeUtilityState } from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

import {
  radii as themeRadii,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

import { groupMarker as groupMarker3 } from '../../web/style/relations.stylex.js'

const stylexStyles7 = ownerStylex.create({
  rule1: {
    position: 'absolute',
    insetInline: `calc(${themeLayout['--spacing']} * 4)`,
    top: 0,
    marginInline: 'auto',
    height: '1px',
    maxWidth: 'var(--container-5xl)',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 7%, transparent)`,
  },
  rule2: { marginInline: 'auto', maxWidth: 'var(--container-5xl)' },
  rule3: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: '.2em',
    color: themeColors['--color-foreground-alt'],
    textTransform: 'uppercase',
  },
  rule4: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
    fontSize: {
      default: 'var(--text-3xl)',
      '@container (min-width: 32rem)': 'var(--text-4xl)',
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-3xl--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-4xl--line-height))',
    },
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    textWrap: 'balance',
    color: themeColors['--color-foreground'],
  },
  rule5: {
    marginInline: 'auto',
    marginBottom: `calc(${themeLayout['--spacing']} * 12)`,
    maxWidth: 'var(--container-2xl)',
    textAlign: 'center',
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    lineHeight: {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule6: {
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-landing-control'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)), 0 1px 2px var(--tw-shadow-color, oklch(0% 0 0 / .3)), 0 10px 24px -14px var(--tw-shadow-color, oklch(0% 0 0 / .6))`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
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
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  rule7: {
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
    color: themeColors['--color-foreground'],
  },
  rule8: {
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
    color: themeColors['--color-foreground'],
  },
  rule9: {
    marginTop: `calc(${themeLayout['--spacing']} * 10)`,
    display: 'flex',
    flexWrap: 'wrap',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule10: {
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
    textAlign: 'center',
    fontSize: {
      default: 'var(--text-3xl)',
      '@container (min-width: 32rem)': 'var(--text-4xl)',
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-3xl--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-4xl--line-height))',
    },
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    textWrap: 'balance',
    color: themeColors['--color-foreground'],
  },
  rule11: { WebkitUserSelect: 'none', userSelect: 'none' },
  rule12: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    whiteSpace: 'nowrap',
    color: {
      default: themeColors['--color-foreground'],
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]:
          themeColors['--color-brand'],
      },
    },
    '--tw-drop-shadow-size':
      'drop-shadow(0 1px 2px var(--tw-drop-shadow-color, #0000001a)) drop-shadow(0 1px 1px var(--tw-drop-shadow-color, #0000000f))',
    '--tw-drop-shadow':
      'drop-shadow(0 1px 2px #0000001a) drop-shadow(0 1px 1px #0000000f)',
    filter:
      'var(--tw-blur, ) var(--tw-brightness, ) var(--tw-contrast, ) var(--tw-grayscale, ) var(--tw-hue-rotate, ) var(--tw-invert, ) var(--tw-saturate, ) var(--tw-sepia, ) var(--tw-drop-shadow, )',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule13: {
    display: 'flex',
    alignItems: 'flex-start',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule14: {
    fontFamily: themeFonts['--font-heading'],
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    lineHeight: {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
    '--tw-leading': 'var(--leading-relaxed)',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule15: { color: themeColors['--color-foreground'] },
  rule16: {
    color: {
      default: themeColors['--color-foreground'],
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]:
          themeColors['--color-brand'],
      },
    },
  },
  rule17: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule18: {
    rotate: '45deg',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 12%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule19: {
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]:
          `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
      },
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]:
          themeColors['--color-brand'],
      },
    },
  },
  rule20: {
    display: 'grid',
    transitionProperty: 'all',
    transitionTimingFunction: 'var(--ease-in-out)',
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    '--tw-ease': 'var(--ease-in-out)',
  },
  rule21: { gridTemplateRows: '1fr', opacity: 1 },
  rule22: { gridTemplateRows: '0fr', opacity: 0 },
  rule23: { overflow: 'hidden' },
  rule24: {
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule25: { marginInline: 'auto', maxWidth: 'var(--container-3xl)' },
  rule26: {
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
    display: 'block',
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: '.2em',
    color: themeColors['--color-foreground-alt'],
    textTransform: 'uppercase',
  },
  rule27: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule28: { textAlign: 'center' },
  rule29: { position: 'absolute', inset: 0, zIndex: -10, overflow: 'hidden' },
  rule30: {
    position: 'absolute',
    inset: 0,
    backgroundColor: themeColors['--color-background'],
  },
  rule31: {
    position: 'absolute',
    height: '100%',
    width: '100%',
    opacity: 0.03,
  },
  rule32: { color: themeColors['--color-background-dark'] },
  rule33: {
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
    display: {
      default: 'inline-block',
      '@media (pointer: coarse)': 'inline-flex',
    },
    cursor: 'pointer',
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand'],
    textDecorationLine: 'none',
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)), 0 1px 2px var(--tw-shadow-color, oklch(0% 0 0 / .3)), 0 10px 24px -14px var(--tw-shadow-color, oklch(0% 0 0 / .6))`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
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
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    alignItems: { default: null, '@media (pointer: coarse)': 'center' },
  },
  rule34: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
  },
  rule35: {
    marginInline: 'auto',
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
    display: 'flex',
    maxWidth: 'var(--container-xl)',
    flexDirection: 'column',
    alignItems: 'center',
  },
  rule36: {
    marginInline: 'auto',
    display: 'grid',
    maxWidth: 'var(--container-2xl)',
    gridTemplateColumns: {
      default: 'repeat(1, minmax(0, 1fr))',
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
    },
    columnGap: `calc(${themeLayout['--spacing']} * 8)`,
    rowGap: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: 'var(--text-base)',
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
  },
  rule37: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexWrap: 'wrap',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule38: {
    color: 'var(--landing-feature-color)',
    display: 'flex',
    height: '100%',
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
  },
  rule39: { height: '100%', width: '100%' },
  rule40: {
    display: 'flex',
    height: '100%',
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    color: themeColors['--color-foreground-alt'],
    opacity: 0.6,
  },
  rule41: {
    display: 'flex',
    height: '100%',
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    color: themeColors['--color-foreground-alt'],
    opacity: 0.4,
  },
  rule42: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 8)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule43: {
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)), 0 24px 48px -32px var(--tw-shadow-color, oklch(0% 0 0 / .7))`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule44: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: '.2em',
    color: themeColors['--color-foreground-alt'],
    textTransform: 'uppercase',
    opacity: 0.5,
  },
  rule45: { height: `calc(${themeLayout['--spacing']} * 64)` },
  rule46: {
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)), 0 24px 48px -32px var(--tw-shadow-color, oklch(0% 0 0 / .7))`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule47: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: '.2em',
    color: themeColors['--color-brand'],
    textTransform: 'uppercase',
  },
  rule48: {
    marginTop: `calc(${themeLayout['--spacing']} * 8)`,
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(3, minmax(0, 1fr))',
    },
  },
  rule49: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule50: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    textWrap: 'balance',
    color: themeColors['--color-foreground-alt'],
  },
  rule51: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule52: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]:
          `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    '--tw-inset-ring-shadow':
      'inset 0 0 0 1px var(--tw-inset-ring-color, currentcolor)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-inset-ring-color': `color-mix(in oklab, ${themeColors['--color-brand']} 14%, transparent)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule53: {
    fontSize: themeLayout['--text-metadata'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 70%, transparent)`,
    textTransform: 'uppercase',
  },
  rule54: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: 'var(--text-base)',
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule55: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    textWrap: 'balance',
    color: themeColors['--color-foreground-alt'],
  },
  rule56: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
      '@container (min-width: 42rem)': 'repeat(3, minmax(0, 1fr))',
    },
  },
  rule57: {
    marginInline: 'auto',
    maxWidth: 'var(--container-lg)',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule58: {
    transitionDelay: 'var(--landing-stack-delay)',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.5s',
    '--tw-duration': '.5s',
  },
  rule59: {
    '--tw-translate-x': '0px',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    opacity: 1,
  },
  rule60: {
    '--tw-translate-x': `calc(${themeLayout['--spacing']} * -4)`,
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    opacity: 0,
  },
  rule61: {
    position: 'relative',
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: themeColors['--color-background-card'],
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)), 0 24px 48px -32px var(--tw-shadow-color, oklch(0% 0 0 / .7))`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule62: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule63: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 9)`,
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
  },
  rule64: { minWidth: 0, flex: 1 },
  rule65: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule66: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule67: {
    transitionDelay: 'var(--landing-stack-delay)',
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '.55rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.5s',
    '--tw-duration': '.5s',
  },
  rule68: { opacity: 1 },
  rule69: { opacity: 0 },
  rule70: {
    marginLeft: 'var(--landing-stack-offset)',
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderTopLeftRadius: themeRadii['--radius-lg'],
    borderTopRightRadius: themeRadii['--radius-lg'],
    borderInlineStyle: themeUtilityState['--tw-border-style'],
    borderInlineWidth: '1px',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 80%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule71: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
  },
  rule72: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
  },
  rule73: {
    fontSize: '.6rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule74: {
    transitionDelay: 'var(--landing-cta-delay)',
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'flex-end',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.5s',
    '--tw-duration': '.5s',
  },
  rule75: {
    '--tw-translate-y': '0px',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    opacity: 1,
  },
  rule76: {
    '--tw-translate-y': `calc(${themeLayout['--spacing']} * 2)`,
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    opacity: 0,
  },
  rule77: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: themeLayout['--spacing'],
  },
  rule78: {
    fontSize: '.5rem',
    lineHeight: 1.5,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: themeColors['--color-brand'],
    textTransform: 'uppercase',
  },
  rule79: {
    marginInline: 'auto',
    maxWidth: 'var(--container-2xl)',
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)), 0 24px 48px -32px var(--tw-shadow-color, oklch(0% 0 0 / .7))`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule80: {
    marginInline: 'auto',
    marginTop: `calc(${themeLayout['--spacing']} * 10)`,
    maxWidth: 'var(--container-3xl)',
  },
  rule81: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(3, minmax(0, 1fr))',
    },
  },
  rule82: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-brand'],
  },
  rule83: {
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule84: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule85: {
    height: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-warning'],
  },
  rule86: {
    marginInline: 'auto',
    maxWidth: 'var(--container-4xl)',
    fontSize: {
      default: 'var(--text-base)',
      '@container (min-width: 28rem)': 'var(--text-lg)',
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-base--line-height))',
      '@container (min-width: 28rem)':
        'var(--tw-leading, var(--text-lg--line-height))',
    },
  },
  rule87: {
    overflow: 'hidden',
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: themeColors['--color-background-card-alt'],
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)), 0 24px 48px -32px var(--tw-shadow-color, oklch(0% 0 0 / .7))`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule88: {
    gridTemplateColumns:
      'minmax(0, 1fr) clamp(5.5rem, 24%, 12rem) clamp( 3.25rem, 16%, 12rem )',
    display: 'grid',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
  },
  rule89: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule90: {
    paddingInline: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 28rem)': 'var(--text-base)',
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
      '@container (min-width: 28rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-white'],
    padding: {
      default: null,
      '@container (min-width: 28rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
  },
  rule91: {
    paddingInline: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 28rem)': 'var(--text-base)',
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
      '@container (min-width: 28rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
    padding: {
      default: null,
      '@container (min-width: 28rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
  },
  rule92: {
    gridTemplateColumns:
      'minmax(0, 1fr) clamp(5.5rem, 24%, 12rem) clamp( 3.25rem, 16%, 12rem )',
    display: 'grid',
    alignItems: 'center',
  },
  rule93: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule94: { flexShrink: 0, color: themeColors['--color-foreground-alt'] },
  rule95: {
    display: 'flex',
    justifyContent: 'center',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    padding: {
      default: null,
      '@container (min-width: 28rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
  },
  rule96: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-partial'],
  },
  rule97: {
    height: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-warning'],
  },
  rule98: {
    marginTop: `calc(${themeLayout['--spacing']} * 6)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule99: {
    position: 'relative',
    width: '100%',
    backgroundColor: themeColors['--color-background-card'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule100: {
    width: '100%',
    maxWidth: {
      default: null,
      '@media (min-width: 40rem)': '40rem',
      '@media (min-width: 48rem)': '48rem',
      '@media (min-width: 64rem)': '64rem',
      '@media (min-width: 80rem)': '80rem',
      '@media (min-width: 96rem)': '96rem',
    },
    marginInline: 'auto',
    display: 'flex',
    flexDirection: {
      default: 'column',
      '@container (min-width: 32rem)': 'row',
    },
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 42rem)': `calc(${themeLayout['--spacing']} * 6)`,
    },
  },
  rule101: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule102: {
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    '--tw-inset-ring-shadow':
      'inset 0 0 0 1px var(--tw-inset-ring-color, currentcolor)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-inset-ring-color': `color-mix(in oklab, ${themeColors['--color-brand']} 14%, transparent)`,
  },
  rule103: {
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule104: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule105: {
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    borderRadius: themeRadii['--radius-landing-control'],
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
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    textDecorationLine: 'none',
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)), 0 1px 2px var(--tw-shadow-color, oklch(0% 0 0 / .3)), 0 10px 24px -14px var(--tw-shadow-color, oklch(0% 0 0 / .6))`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  rule106: {
    display: 'flex',
    width: '100%',
    flexShrink: 0,
    flexDirection: {
      default: 'column',
      '@container (min-width: 32rem)': 'row',
    },
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: themeColors['--color-background-landing'],
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 42rem)': `calc(${themeLayout['--spacing']} * 6)`,
    },
    paddingBlock: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule107: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule108: {
    display: 'flex',
    gap: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 6)`,
    },
    marginLeft: { default: null, '@container (min-width: 32rem)': 'auto' },
  },
  rule109: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    textUnderlineOffset: '4px',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
    display: { default: null, '@media (pointer: coarse)': 'flex' },
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    alignItems: { default: null, '@media (pointer: coarse)': 'center' },
    paddingInline: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 2)`,
    },
  },
})

const stylexStyles6 = ownerStylex.create({
  rule1: {
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand-highlight'],
      },
    },
    textDecorationLine: 'underline',
    textUnderlineOffset: '2px',
  },
  rule2: {
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-landing-control'],
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
    textDecorationLine: 'none',
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)), 0 1px 2px var(--tw-shadow-color, oklch(0% 0 0 / .3)), 0 10px 24px -14px var(--tw-shadow-color, oklch(0% 0 0 / .6))`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
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
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  rule3: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
    display: { default: null, '@media (pointer: coarse)': 'inline-flex' },
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    alignItems: { default: null, '@media (pointer: coarse)': 'center' },
  },
})

const stylexStyles5 = ownerStylex.create({
  rule1: {
    marginRight: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'inline-block',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    '--tw-translate-y': `calc(${themeLayout['--spacing']} * -.5)`,
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles4 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: 'var(--ease-in-out)',
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    '--tw-ease': 'var(--ease-in-out)',
    WebkitUserSelect: 'none',
    userSelect: 'none',
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.25)`,
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
  rule2: {
    cursor: 'pointer',
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    '--tw-shadow': {
      default: `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)), 0 24px 48px -32px var(--tw-shadow-color, oklch(0% 0 0 / .7))`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 7%, transparent)), 0 32px 56px -30px var(--tw-shadow-color, oklch(0% 0 0 / .85))`,
      },
    },
    boxShadow: {
      default:
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      '@media (hover: hover)': {
        default: null,
        ':hover':
          'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule3: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 60%, transparent)`,
  },
  rule4: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
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
  rule5: {
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    '--tw-shadow': {
      default: `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)), 0 24px 48px -32px var(--tw-shadow-color, oklch(0% 0 0 / .7))`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 7%, transparent)), 0 32px 56px -30px var(--tw-shadow-color, oklch(0% 0 0 / .85))`,
      },
    },
    boxShadow: {
      default:
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      '@media (hover: hover)': {
        default: null,
        ':hover':
          'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
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
  rule6: {
    transitionDelay: 'var(--feature-reveal-delay)',
    cursor: 'pointer',
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    textDecorationLine: 'none',
    '--tw-shadow': {
      default: `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)), 0 24px 48px -32px var(--tw-shadow-color, oklch(0% 0 0 / .7))`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 7%, transparent)), 0 32px 56px -30px var(--tw-shadow-color, oklch(0% 0 0 / .85))`,
      },
    },
    boxShadow: {
      default:
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      '@media (hover: hover)': {
        default: null,
        ':hover':
          'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.5s',
    '--tw-duration': '.5s',
  },
  rule7: {
    '--tw-translate-y': {
      default: '0px',
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -1)`,
      },
    },
    translate: {
      default: 'var(--tw-translate-x) var(--tw-translate-y)',
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
    opacity: 1,
  },
  rule8: {
    '--tw-translate-y': `calc(${themeLayout['--spacing']} * 8)`,
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    opacity: 0,
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
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-brand'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-brand'],
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-brand'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule3: {
    marginRight: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-brand'],
  },
  rule7: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    '--tw-font-weight': 'var(--font-weight-bold)',
    fontWeight: 'var(--font-weight-bold)',
    color: themeColors['--color-success'],
  },
  rule8: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-red-400'],
  },
  rule9: {
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    '--tw-font-weight': 'var(--font-weight-bold)',
    fontWeight: 'var(--font-weight-bold)',
    color: themeColors['--color-success'],
  },
  rule10: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-red-400'],
  },
  rule11: {
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    color: themeColors['--color-brand'],
  },
  rule12: {
    marginRight: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    '--tw-scale-x': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]: '110%',
      },
    },
    '--tw-scale-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]: '110%',
      },
    },
    '--tw-scale-z': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]: '110%',
      },
    },
    scale: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          'var(--tw-scale-x) var(--tw-scale-y)',
      },
    },
  },
})

// Section provides consistent section styling with optional scroll-reveal.
function Section({
  children,
  className,
  id,
  withTopSeparator = false,
  reveal = false,
}: {
  children: React.ReactNode
  className?: string
  id?: string
  withTopSeparator?: boolean
  reveal?: boolean
}) {
  const { ref, visible } = useScrollReveal(0.08)
  const isStatic = useIsStaticMode()
  const isVisible = visible || isStatic
  return (
    <section
      id={id}
      ref={reveal && !isStatic ? ref : undefined}
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'relative w-full px-4 py-28 @lg:px-8 @2xl:px-12',
          reveal &&
            cn(
              'transition duration-700',
              isVisible
                ? 'translate-y-0 opacity-100'
                : 'translate-y-8 opacity-0',
            ),
          className,
        )
      }
    >
      {withTopSeparator && <div sx={stylexStyles7.rule1} />}
      <div sx={stylexStyles7.rule2}>{children}</div>
    </section>
  )
}

// SectionLabel renders the small uppercase tracking label above section headings.
function SectionLabel({ children }: { children: React.ReactNode }) {
  return <div sx={stylexStyles7.rule3}>{children}</div>
}

// SectionTitle renders a section heading.
function SectionTitle({ children }: { children: React.ReactNode }) {
  return <h2 sx={stylexStyles7.rule4}>{children}</h2>
}

// SectionSubtitle renders subtitle text under a heading.
function SectionSubtitle({ children }: { children: React.ReactNode }) {
  return <p sx={stylexStyles7.rule5}>{children}</p>
}

// CtaButton renders a call-to-action button.
function CtaButton({
  icon: Icon,
  children,
  variant = 'default',
  onClick,
}: {
  icon: IconType
  children: React.ReactNode
  variant?: 'default' | 'primary'
  onClick?: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      sx={[
        stylexStyles7.rule6,
        variant === 'primary' ? stylexStyles7.rule7 : stylexStyles7.rule8,
      ]}
    >
      <Icon sx={stylexStyles2.rule1} />
      <span>{children}</span>
    </button>
  )
}

// CtaRow renders a centered row of call-to-action buttons.
function CtaRow({ children }: { children: React.ReactNode }) {
  return <div sx={stylexStyles7.rule9}>{children}</div>
}

interface SectionHeadingProps {
  children: React.ReactNode
}

const SectionHeading: React.FC<SectionHeadingProps> = ({ children }) => (
  <h2 sx={stylexStyles7.rule10}>{children}</h2>
)

// Hero Components
interface HeroTextProps {
  className?: string
  children: React.ReactNode
}

const HeroText: React.FC<HeroTextProps> = ({ className, children }) => (
  <p
    className={
      /* TODO(stylex): dynamic class or forwarded className */ cn(
        'text-foreground-alt mx-auto max-w-2xl text-sm leading-relaxed @lg:text-base @lg:leading-relaxed',
        className,
      )
    }
  >
    {children}
  </p>
)

interface HeroButtonProps {
  icon: React.ReactNode
  children: React.ReactNode
  onClick?: () => void
}

const HeroButton: React.FC<HeroButtonProps> = ({ icon, children, onClick }) => (
  <button
    type="button"
    className={
      /* TODO(stylex): global class adds declarations outside the utility compiler: hero-button */ 'hero-button'
    }
    onClick={onClick}
  >
    {icon}
    <span sx={stylexStyles7.rule11}>{children}</span>
  </button>
)

interface HeroFeatureProps {
  icon: React.ReactNode
  text: string
  color?: 'primary' | 'emerald' | 'pink' | 'purple' | 'amber'
}

const HeroFeature: React.FC<HeroFeatureProps> = ({
  icon,
  text,
  color = 'primary',
}) => {
  const colorClasses = {
    primary: 'text-brand',
    emerald: 'text-green-400',
    pink: 'text-rose-400',
    purple: 'text-blue-400',
    amber: 'text-yellow-400',
  } as const

  return (
    <li sx={[stylexStyles4.rule1, groupMarker2]}>
      <span
        className={
          /* TODO(stylex): dynamic class or forwarded className */ cn(
            colorClasses[color],
            'mr-3 transition-transform duration-300 group-hover:scale-110',
          )
        }
      >
        {icon}
      </span>
      <span sx={stylexStyles7.rule12}>{text}</span>
    </li>
  )
}

// FAQ Components
interface FaqItemProps {
  question: React.ReactNode
  answer: React.ReactNode
  isOpen: boolean
  onToggle: () => void
}

const FaqItem: React.FC<FaqItemProps> = ({
  question,
  answer,
  isOpen,
  onToggle,
}) => {
  const handleFaqKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== 'Enter' && e.key !== ' ') return
    e.preventDefault()
    onToggle()
  }

  return (
    <div
      role="button"
      tabIndex={0}
      sx={[
        [stylexStyles4.rule2, groupMarker2],
        isOpen ? stylexStyles4.rule3 : stylexStyles4.rule4,
      ]}
      onClick={onToggle}
      onKeyDown={handleFaqKeyDown}
    >
      <div sx={stylexStyles7.rule13}>
        <h3
          sx={[
            stylexStyles7.rule14,
            isOpen ? stylexStyles7.rule15 : stylexStyles7.rule16,
          ]}
        >
          {question}
        </h3>
        <div
          sx={[
            stylexStyles7.rule17,
            isOpen ? stylexStyles7.rule18 : stylexStyles7.rule19,
          ]}
        >
          <LuPlus sx={stylexStyles.rule1} />
        </div>
      </div>
      <div
        sx={[
          stylexStyles7.rule20,
          isOpen ? stylexStyles7.rule21 : stylexStyles7.rule22,
        ]}
      >
        <div sx={stylexStyles7.rule23}>
          <p sx={stylexStyles7.rule24}>{answer}</p>
        </div>
      </div>
    </div>
  )
}

const FaqAccordion: React.FC = () => {
  const [openIndex, setOpenIndex] = useState<number>(0)

  const faqItems = useMemo(
    () => [
      {
        question: 'How is Spacewave different from traditional cloud apps?',
        answer: (
          <>
            Spacewave fundamentally rethinks how web apps function. Instead of
            keeping your data locked on remote servers, Spacewave runs
            everything directly on your devices while giving you the freedom to
            store information wherever you want. It's like having all the power
            of the cloud with the independence of your personal devices.
          </>
        ),
      },
      {
        question: 'Is my data secure when using Spacewave?',
        answer: (
          <>
            Absolutely. Spacewave uses end-to-end encryption and optionally
            peer-to-peer (P2P) networking. Your data is encrypted on your device
            and stays private in transport and in storage. Protect your account
            with a password, a long PIN, or a passkey that can derive keys, and
            even we can't decrypt it. This is{' '}
            <ExternalLink
              href="https://www.cloudflare.com/learning/privacy/what-is-end-to-end-encryption/"
              sx={stylexStyles6.rule1}
            >
              a standard approach to data safety
            </ExternalLink>{' '}
            on the web.
          </>
        ),
      },
      {
        question: 'How does Spacewave handle data portability?',
        answer:
          "Spacewave prioritizes data portability, giving you full control over your information. With just a few clicks, you can move your data between different storage options including your own devices and the cloud. This flexibility ensures you're never tied to a single storage solution and can take your data with you wherever you go.",
      },
      {
        question: 'Why is Spacewave open source?',
        answer:
          'We believe in transparency, community collaboration, and user freedom. Being open source means anyone can inspect our code, contribute improvements, or run their own custom version. This creates trust, accelerates innovation, and ensures Spacewave remains focused on user needs. We invite you to make it your own!',
      },
    ],
    [],
  )

  const handleToggle = useCallback((index: number) => {
    setOpenIndex((prevIndex) => (prevIndex === index ? -1 : index))
  }, [])

  return (
    <div sx={stylexStyles7.rule25}>
      <span sx={stylexStyles7.rule26}>Frequently asked questions</span>
      <div sx={stylexStyles7.rule27}>
        {faqItems.map((item, index) => (
          <FaqItem
            key={item.question}
            question={item.question}
            answer={item.answer}
            isOpen={index === openIndex}
            onToggle={() => handleToggle(index)}
          />
        ))}
      </div>
    </div>
  )
}

// Section Components
const HeroSection: React.FC = () => {
  const nav = useNavLinks()
  const blogHref = useStaticHref('/blog/2026/04/launch')
  return (
    <Section withTopSeparator>
      <div sx={stylexStyles7.rule28}>
        <div sx={stylexStyles7.rule29}>
          <div sx={stylexStyles7.rule30}>
            <svg
              sx={stylexStyles7.rule31}
              xmlns="http://www.w3.org/2000/svg"
              width="100%"
              height="100%"
              viewBox="0 0 100 100"
            >
              <defs>
                <pattern
                  id="spacewave-pattern"
                  patternUnits="userSpaceOnUse"
                  width="50"
                  height="50"
                  patternTransform="rotate(11)"
                >
                  <circle
                    cx="25"
                    cy="25"
                    r="1"
                    fill="none"
                    stroke="currentColor"
                    sx={stylexStyles7.rule32}
                  />
                  <circle
                    cx="25"
                    cy="25"
                    r="6"
                    fill="none"
                    stroke="currentColor"
                    sx={stylexStyles7.rule32}
                    strokeWidth="0.2"
                  />
                  <circle
                    cx="25"
                    cy="25"
                    r="12"
                    fill="none"
                    stroke="currentColor"
                    sx={stylexStyles7.rule32}
                    strokeWidth="0.15"
                  />
                  <circle
                    cx="25"
                    cy="25"
                    r="18"
                    fill="none"
                    stroke="currentColor"
                    sx={stylexStyles7.rule32}
                    strokeWidth="0.1"
                  />
                </pattern>
              </defs>
              <rect
                x="0"
                y="0"
                width="100%"
                height="100%"
                fill="url(#spacewave-pattern)"
              />
            </svg>
          </div>
        </div>

        <a href={blogHref} sx={stylexStyles7.rule33}>
          <PiRocketLaunchDuotone sx={stylexStyles5.rule1} />
          Announcing open beta
        </a>

        <SectionHeading>The internet without the internet</SectionHeading>

        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-0 */ 'mb-8 space-y-0'
          }
        >
          <HeroText>
            The web was built for servers.{' '}
            <strong sx={stylexStyles7.rule34}>
              Spacewave was built for you
            </strong>
            .
          </HeroText>

          <HeroText>
            Your spaces work <strong sx={stylexStyles7.rule34}>offline</strong>,
            sync <strong sx={stylexStyles7.rule34}>instantly</strong>, and store
            data <strong sx={stylexStyles7.rule34}>anywhere</strong>.
          </HeroText>

          <HeroText>
            Devices talk <strong sx={stylexStyles7.rule34}>directly</strong>{' '}
            instead of through distant servers. That's{' '}
            <strong sx={stylexStyles7.rule34}>real freedom</strong>.
          </HeroText>
        </div>

        <div sx={stylexStyles7.rule35}>
          <ul sx={stylexStyles7.rule36}>
            <HeroFeature
              icon={<PiAppStoreLogoBold sx={stylexStyles5.rule2} />}
              text="Spaces for any purpose"
              color="emerald"
            />
            <HeroFeature
              icon={<LuMousePointer sx={stylexStyles.rule2} />}
              text="Instant live sync"
              color="pink"
            />
            <HeroFeature
              icon={<LuShield sx={stylexStyles.rule2} />}
              text="End-to-end encrypted"
              color="purple"
            />
            <HeroFeature
              icon={<LuGithub sx={stylexStyles.rule2} />}
              text="Open-source & extensible"
              color="amber"
            />
          </ul>
        </div>

        <div sx={stylexStyles7.rule37}>
          <HeroButton
            icon={<LuRocket sx={stylexStyles.rule3} />}
            onClick={nav.getStarted}
          >
            Get started (free)
          </HeroButton>
          {!isDesktop && (
            <HeroButton
              icon={<LuDownload sx={stylexStyles.rule3} />}
              onClick={nav.download}
            >
              Download app
            </HeroButton>
          )}
        </div>
      </div>
    </Section>
  )
}

// SVG Components

// AnimatedConnection renders an animated line between two nodes.
function AnimatedConnection({
  x1,
  y1,
  x2,
  y2,
  delay,
  color,
}: {
  x1: number
  y1: number
  x2: number
  y2: number
  delay: number
  color: string
}) {
  const length = Math.hypot(x2 - x1, y2 - y1)
  const r = (n: number) => Math.round(n * 1e4) / 1e4
  return (
    <line
      x1={x1}
      y1={y1}
      x2={x2}
      y2={y2}
      stroke={color}
      strokeWidth="1"
      strokeDasharray={`${r(length * 0.3)} ${r(length * 0.7)}`}
      opacity="0.5"
    >
      <animate
        attributeName="stroke-dashoffset"
        values={`${r(length)};0;${r(-length)}`}
        dur="4s"
        begin={`${delay}s`}
        repeatCount="indefinite"
      />
      <animate
        attributeName="opacity"
        values="0.2;0.6;0.2"
        dur="4s"
        begin={`${delay}s`}
        repeatCount="indefinite"
      />
    </line>
  )
}

// DataPacket renders an animated dot traveling along a path.
function DataPacket({
  x1,
  y1,
  x2,
  y2,
  delay,
  color,
}: {
  x1: number
  y1: number
  x2: number
  y2: number
  delay: number
  color: string
}) {
  return (
    <circle r="3" fill={color} opacity="0.8">
      <animate
        attributeName="cx"
        values={`${x1};${x2}`}
        dur="2s"
        begin={`${delay}s`}
        repeatCount="indefinite"
      />
      <animate
        attributeName="cy"
        values={`${y1};${y2}`}
        dur="2s"
        begin={`${delay}s`}
        repeatCount="indefinite"
      />
      <animate
        attributeName="opacity"
        values="0;0.9;0.9;0"
        dur="2s"
        begin={`${delay}s`}
        repeatCount="indefinite"
      />
    </circle>
  )
}

// NetworkNode renders an animated device node in the SVG topology.
function NetworkNode({
  x,
  y,
  label,
  icon: Icon,
  delay,
  color,
}: {
  x: number
  y: number
  label: string
  icon: IconType
  delay: number
  color: string
}) {
  return (
    <g>
      <circle
        cx={x}
        cy={y}
        r="28"
        fill="none"
        stroke={color}
        strokeWidth="1"
        opacity="0.3"
      >
        <animate
          attributeName="r"
          values="28;36;28"
          dur="3s"
          begin={`${delay}s`}
          repeatCount="indefinite"
        />
        <animate
          attributeName="opacity"
          values="0.3;0.1;0.3"
          dur="3s"
          begin={`${delay}s`}
          repeatCount="indefinite"
        />
      </circle>
      <circle
        cx={x}
        cy={y}
        r="24"
        fill="var(--color-background-card)"
        stroke={color}
        strokeWidth="1.5"
      />
      <text
        x={x}
        y={y + 42}
        textAnchor="middle"
        fill="var(--color-foreground-alt)"
        fontSize="10"
        fontFamily="inherit"
      >
        {label}
      </text>
      <foreignObject x={x - 10} y={y - 10} width="20" height="20">
        <div
          sx={stylexStyles7.rule38}
          style={{ '--landing-feature-color': color }}
        >
          <Icon sx={stylexStyles2.rule1} />
        </div>
      </foreignObject>
    </g>
  )
}

// TraditionalDiagram renders the server-centric architecture.
function TraditionalDiagram({ label }: { label: string }) {
  const gray = 'var(--color-foreground)'
  const server = { x: 240, y: 55 }
  const devices = [
    { x: 100, y: 175, label: 'Laptop', icon: LuLaptop },
    { x: 240, y: 175, label: 'Phone', icon: LuSmartphone },
    { x: 380, y: 175, label: 'Pi', icon: LuCpu },
  ]

  return (
    <svg
      viewBox="0 0 480 260"
      sx={stylexStyles7.rule39}
      role="img"
      aria-label="Traditional cloud architecture: all devices route through a central server"
    >
      {devices.map((d, i) => (
        <g key={`conn-${d.label}`}>
          <AnimatedConnection
            x1={server.x}
            y1={server.y + 24}
            x2={d.x}
            y2={d.y - 24}
            delay={i * 0.7}
            color={gray}
          />
          <DataPacket
            x1={d.x}
            y1={d.y - 24}
            x2={server.x}
            y2={server.y + 24}
            delay={i * 1.1}
            color={gray}
          />
          <DataPacket
            x1={server.x}
            y1={server.y + 24}
            x2={d.x}
            y2={d.y - 24}
            delay={i * 1.1 + 2.5}
            color={gray}
          />
        </g>
      ))}
      <circle
        cx={server.x}
        cy={server.y}
        r="28"
        fill="none"
        stroke={gray}
        strokeWidth="1"
        opacity="0.15"
      >
        <animate
          attributeName="r"
          values="28;34;28"
          dur="3s"
          repeatCount="indefinite"
        />
        <animate
          attributeName="opacity"
          values="0.15;0.06;0.15"
          dur="3s"
          repeatCount="indefinite"
        />
      </circle>
      <circle
        cx={server.x}
        cy={server.y}
        r="24"
        fill="var(--color-background-card)"
        stroke={gray}
        strokeWidth="1.5"
        opacity="0.4"
      />
      <foreignObject x={server.x - 10} y={server.y - 10} width="20" height="20">
        <div sx={stylexStyles7.rule40}>
          <LuServer sx={stylexStyles.rule4} />
        </div>
      </foreignObject>
      <text
        x={server.x}
        y={server.y - 32}
        textAnchor="middle"
        fill="var(--color-foreground-alt)"
        fontSize="10"
        opacity="0.5"
      >
        Cloud Server
      </text>
      {devices.map((d) => (
        <g key={`dev-${d.label}`}>
          <circle
            cx={d.x}
            cy={d.y}
            r="20"
            fill="var(--color-background-card)"
            stroke={gray}
            strokeWidth="1"
            opacity="0.3"
          />
          <foreignObject x={d.x - 8} y={d.y - 8} width="16" height="16">
            <div sx={stylexStyles7.rule41}>
              <d.icon sx={stylexStyles3.rule1} />
            </div>
          </foreignObject>
          <text
            x={d.x}
            y={d.y + 34}
            textAnchor="middle"
            fill="var(--color-foreground-alt)"
            fontSize="10"
            opacity="0.4"
          >
            {d.label}
          </text>
        </g>
      ))}
      <text
        x={240}
        y={248}
        textAnchor="middle"
        fill="var(--color-foreground-alt)"
        fontSize="11"
        fontWeight="600"
        opacity="0.5"
      >
        {label}
      </text>
    </svg>
  )
}

// SpacewaveDiagram renders the peer-to-peer mesh architecture.
function SpacewaveDiagram({ label }: { label: string }) {
  const brand = 'var(--color-brand)'
  const nodes = [
    { x: 120, y: 55, label: 'Laptop', icon: LuLaptop, delay: 0 },
    { x: 360, y: 55, label: 'Phone', icon: LuSmartphone, delay: 0.5 },
    { x: 240, y: 155, label: 'Pi', icon: LuCpu, delay: 1 },
  ]

  const connections: [number, number][] = [
    [0, 1],
    [1, 2],
    [0, 2],
  ]

  return (
    <svg
      viewBox="0 0 480 260"
      sx={stylexStyles7.rule39}
      role="img"
      aria-label="Spacewave mesh: devices connect directly to each other"
    >
      {connections.map(([a, b], i) => (
        <g key={`c-${nodes[a].label}-${nodes[b].label}`}>
          <AnimatedConnection
            x1={nodes[a].x}
            y1={nodes[a].y}
            x2={nodes[b].x}
            y2={nodes[b].y}
            delay={i * 0.6}
            color={brand}
          />
          <DataPacket
            x1={nodes[a].x}
            y1={nodes[a].y}
            x2={nodes[b].x}
            y2={nodes[b].y}
            delay={i * 0.8 + 1}
            color={brand}
          />
        </g>
      ))}
      {nodes.map((n) => (
        <NetworkNode
          key={n.label}
          x={n.x}
          y={n.y}
          label={n.label}
          icon={n.icon}
          delay={n.delay}
          color={brand}
        />
      ))}
      <text
        x={240}
        y={248}
        textAnchor="middle"
        fill="var(--color-brand)"
        fontSize="11"
        fontWeight="600"
      >
        {label}
      </text>
    </svg>
  )
}

// New Sections

const HOW_ICONS = [LuZap, LuLock, LuCloudOff]

// HowItWorksSection renders the architecture comparison and feature cards.
function HowItWorksSection() {
  const cards = [
    {
      title: 'Every device makes it stronger',
      body: 'Your laptop, phone, Raspberry Pi, Linux box... each one joins the swarm and adds power. The more you connect, the more you can do.',
    },
    {
      title: 'Encrypted. Direct. Private.',
      body: 'Devices find each other and connect peer-to-peer. Every connection is encrypted. No server ever sees your data.',
    },
    {
      title: 'Works without the internet',
      body: 'Every device carries a full workspace. Go offline whenever you want. Everything syncs the moment you reconnect.',
    },
  ]

  return (
    <Section id="how-it-works" withTopSeparator reveal>
      <SectionLabel>How it works</SectionLabel>
      <SectionTitle>All your devices. One system.</SectionTitle>
      <SectionSubtitle>
        Spacewave turns your devices into an encrypted network. Every device
        runs a full workspace with its apps, files and data, and syncs directly
        with the others.
      </SectionSubtitle>

      <div sx={stylexStyles7.rule42}>
        <div sx={stylexStyles7.rule43}>
          <div sx={stylexStyles7.rule44}>Traditional Cloud</div>
          <div sx={stylexStyles7.rule45}>
            <TraditionalDiagram label="Before: Everything goes through their servers" />
          </div>
        </div>

        <div sx={stylexStyles7.rule46}>
          <div sx={stylexStyles7.rule47}>Spacewave</div>
          <div sx={stylexStyles7.rule45}>
            <SpacewaveDiagram label="Spacewave: Your devices talk directly" />
          </div>
        </div>
      </div>

      <div sx={stylexStyles7.rule48}>
        {cards.map((card, i) => {
          const CardIcon = HOW_ICONS[i]
          return (
            <div key={card.title} sx={[stylexStyles4.rule5, groupMarker2]}>
              <CardIcon sx={stylexStyles2.rule2} />
              <h3 sx={stylexStyles7.rule49}>{card.title}</h3>
              <p sx={stylexStyles7.rule50}>{card.body}</p>
            </div>
          )
        })}
      </div>
    </Section>
  )
}

const CASE_ICONS = [
  LuFolderSync,
  LuTerminal,
  LuCode,
  LuBookOpen,
  LuMessageSquare,
  LuZap,
]
const CASE_HIGHLIGHTS = [
  'Files & data',
  'Devices & servers',
  'Apps & tools',
  'Notes & knowledge',
  'Social & messaging',
  'CLI & automation',
]

const USE_CASES = [
  {
    title: 'Own your files',
    description:
      'A private Drive in your browser that syncs to every device you link, with encrypted cloud backup only if you want it.',
    href: '/landing/drive',
  },
  {
    title: 'Reach your computers',
    description:
      'Link machines with the CLI or add SSH hosts, then open a terminal from one Computers dashboard.',
    href: '/landing/devices',
  },
  {
    title: 'Build anything',
    description:
      'Plugins in Go or TypeScript add new kinds of objects, like a SQL Database, and run on your own devices.',
    href: '/landing/plugins',
  },
  {
    title: 'Think clearly',
    description:
      'A Markdown notebook that lives on your devices, works offline, and syncs when they reconnect.',
    href: '/landing/notes',
  },
  {
    title: 'Talk privately',
    description:
      'Encrypted channels in a Space you own. The history stays on your devices and those of the people you invite.',
    href: '/landing/chat',
  },
  {
    title: 'Command your stack',
    description:
      'One spacewave binary for Spaces, files and devices. Script it from a shell or run it on a headless server.',
    href: '/landing/cli',
  },
]

// UseCaseCard renders a single use-case card with scroll-reveal animation.
function UseCaseCard({
  icon: Icon,
  highlight,
  title,
  description,
  href,
  index,
}: {
  icon: IconType
  highlight: string
  title: string
  description: string
  href: string
  index: number
}) {
  const { ref, visible } = useScrollReveal<HTMLAnchorElement>(0.1)
  const resolvedHref = useStaticHref(href)
  return (
    <a
      ref={ref}
      href={resolvedHref}
      sx={[
        [stylexStyles4.rule6, groupMarker2],
        visible ? stylexStyles4.rule7 : stylexStyles4.rule8,
      ]}
      style={{ '--feature-reveal-delay': `${index * 80}ms` }}
    >
      <div sx={stylexStyles7.rule51}>
        <div sx={stylexStyles7.rule52}>
          <Icon sx={stylexStyles2.rule3} />
        </div>
        <span sx={stylexStyles7.rule53}>{highlight}</span>
      </div>
      <h3 sx={stylexStyles7.rule54}>{title}</h3>
      <p sx={stylexStyles7.rule55}>{description}</p>
    </a>
  )
}

// UseCasesSection renders the use-case cards grid.
function UseCasesSection() {
  const nav = useNavLinks()
  return (
    <Section id="use-cases" reveal>
      <SectionLabel>What you can do</SectionLabel>
      <SectionTitle>Start with one thing. Then do everything.</SectionTitle>
      <SectionSubtitle>
        Pick what matters to you. Combine them however you want. This is your
        system.
      </SectionSubtitle>

      <div sx={stylexStyles7.rule56}>
        {USE_CASES.map((c, i) => (
          <UseCaseCard
            key={c.title}
            icon={CASE_ICONS[i]}
            highlight={CASE_HIGHLIGHTS[i]}
            title={c.title}
            description={c.description}
            href={c.href}
            index={i}
          />
        ))}
      </div>

      <CtaRow>
        <CtaButton icon={LuRocket} variant="primary" onClick={nav.getStarted}>
          Get Started
        </CtaButton>
      </CtaRow>
    </Section>
  )
}

const STACK_LAYERS = [
  { name: 'Bifrost', desc: 'Encrypted P2P networking', icon: LuGlobe },
  { name: 'Hydra', desc: 'Data storage and sync', icon: LuDatabase },
  { name: 'Bldr', desc: 'Cross-platform runtime', icon: LuCpu },
  { name: 'Spacewave SDK', desc: 'Full-stack developer API', icon: LuCode },
]

// ArchitectureStackDiagram renders the Spacewave stack as peeking tabs with the top card expanded.
function ArchitectureStackDiagram() {
  const { ref, visible } = useScrollReveal(0.1)

  return (
    <div ref={ref} sx={stylexStyles7.rule57}>
      <div
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: space-y-tight */ 'space-y-tight'
        }
      >
        {STACK_LAYERS.map((layer, i) => {
          const LayerIcon = layer.icon
          const isTop = i === STACK_LAYERS.length - 1
          return (
            <div
              key={layer.name}
              sx={[
                stylexStyles7.rule58,
                visible ? stylexStyles7.rule59 : stylexStyles7.rule60,
              ]}
              style={{
                '--landing-stack-delay': `${(STACK_LAYERS.length - 1 - i) * 100}ms`,
              }}
            >
              {isTop ? (
                <div
                  sx={[
                    stylexStyles7.rule61,
                    stackOrder.layer(STACK_LAYERS.length - i),
                  ]}
                >
                  <div sx={stylexStyles7.rule62}>
                    <div sx={stylexStyles7.rule63}>
                      <LayerIcon sx={stylexStyles2.rule4} />
                    </div>
                    <div sx={stylexStyles7.rule64}>
                      <div sx={stylexStyles7.rule65}>{layer.name}</div>
                      <div sx={stylexStyles7.rule66}>{layer.desc}</div>
                    </div>
                    <div
                      sx={[
                        stylexStyles7.rule67,
                        visible ? stylexStyles7.rule68 : stylexStyles7.rule69,
                      ]}
                      style={{
                        '--landing-stack-delay': `${STACK_LAYERS.length * 100 + 300}ms`,
                      }}
                    >
                      <LuPlus sx={stylexStyles.rule5} />
                      plugins
                    </div>
                  </div>
                </div>
              ) : (
                <div
                  sx={[
                    stylexStyles7.rule70,
                    stackOrder.layer(STACK_LAYERS.length - i),
                  ]}
                  style={{
                    '--landing-stack-offset': `${(STACK_LAYERS.length - 1 - i) * 6}px`,
                  }}
                >
                  <div sx={stylexStyles7.rule71}>
                    <LayerIcon sx={stylexStyles2.rule5} />
                  </div>
                  <span sx={stylexStyles7.rule72}>{layer.name}</span>
                  <span sx={stylexStyles7.rule73}>{layer.desc}</span>
                </div>
              )}
            </div>
          )
        })}
      </div>

      <div
        sx={[
          stylexStyles7.rule74,
          visible ? stylexStyles7.rule75 : stylexStyles7.rule76,
        ]}
        style={{ '--landing-cta-delay': '600ms' }}
      >
        <div sx={stylexStyles7.rule77}>
          <LuCircuitBoard sx={stylexStyles.rule6} />
          <span sx={stylexStyles7.rule78}>
            ControllerBus connects all layers
          </span>
        </div>
      </div>
    </div>
  )
}

const DEV_CARDS = [
  {
    title: 'Full-stack, one API',
    body: 'Databases, networking, files and UI share one SDK. Write in Go or TypeScript.',
  },
  {
    title: 'Deploy everywhere',
    body: 'Write it once. Run it in browsers, on desktops, on embedded devices. WebAssembly, native, and cross-compiled.',
  },
  {
    title: 'Open ecosystem',
    body: 'Publish your plugins or keep them private. Browse community contributions. Install with one click.',
  },
]

const DEV_CARD_ICONS = [LuCode, LuGlobe, LuUsers]

// ForDevelopersSection renders the developer-focused plugin ecosystem diagram and SDK info.
function ForDevelopersSection() {
  const nav = useNavLinks()
  return (
    <Section id="plugins" withTopSeparator reveal>
      <SectionLabel>For developers</SectionLabel>
      <SectionTitle>If you can imagine it, you can build it</SectionTitle>
      <SectionSubtitle>
        Spacewave was built with the same SDK you have access to. Every plugin,
        every feature, every interface was created with the open-source tools
        right in front of you.
      </SectionSubtitle>

      <div sx={stylexStyles7.rule79}>
        <ArchitectureStackDiagram />
      </div>

      <div sx={stylexStyles7.rule80}>
        <div sx={stylexStyles7.rule43}>
          <div sx={stylexStyles7.rule81}>
            {DEV_CARDS.map((card, i) => {
              const CardIcon = DEV_CARD_ICONS[i]
              return (
                <div key={card.title}>
                  <div sx={stylexStyles7.rule82}>
                    <CardIcon sx={stylexStyles2.rule1} />
                    {card.title}
                  </div>
                  <p sx={stylexStyles7.rule50}>{card.body}</p>
                </div>
              )
            })}
          </div>
        </div>
      </div>

      <CtaRow>
        <CtaButton icon={LuBookOpen} variant="primary" onClick={nav.docs}>
          Read the Docs
        </CtaButton>
        <ExternalLink href={GITHUB_REPO_URL} sx={stylexStyles6.rule2}>
          <LuGithub sx={stylexStyles.rule4} />
          <span>Browse Source</span>
        </ExternalLink>
      </CtaRow>
    </Section>
  )
}

interface FeatureItem {
  name: string
  spacewave: boolean | 'partial'
  traditional: boolean | 'partial'
  icon: React.ReactNode
}

const ComparisonChart: React.FC = () => {
  // Features organized by key benefits and capabilities
  const features: FeatureItem[] = [
    // Core features
    {
      name: 'Free and open-source',
      spacewave: true,
      traditional: false,
      icon: <LuGift sx={stylexStyles.rule4} />,
    },
    {
      name: 'No account required',
      spacewave: true,
      traditional: false,
      icon: <LuUserX sx={stylexStyles.rule4} />,
    },
    {
      name: 'Works offline without limitations',
      spacewave: true,
      traditional: 'partial',
      icon: <LuWifiOff sx={stylexStyles.rule4} />,
    },
    {
      name: 'Runs on your devices w/ p2p sync',
      spacewave: true,
      traditional: false,
      icon: <PiAppStoreLogoBold sx={stylexStyles5.rule3} />,
    },
    {
      name: 'End-to-end encryption by default',
      spacewave: true,
      traditional: 'partial',
      icon: <LuLock sx={stylexStyles.rule4} />,
    },
    {
      name: 'Low-cost cloud storage and APIs',
      spacewave: true,
      traditional: 'partial',
      icon: <LuCreditCard sx={stylexStyles.rule4} />,
    },
    {
      name: 'No telemetry or ad tracking',
      spacewave: true,
      traditional: false,
      icon: <LuEyeOff sx={stylexStyles.rule4} />,
    },
  ]

  // Legend for partial support
  const renderLegend = () => (
    <div sx={stylexStyles7.rule83}>
      <div sx={stylexStyles7.rule84}>
        <LuCheck sx={stylexStyles.rule7} />
        <span>Full support</span>
      </div>
      <div sx={stylexStyles7.rule84}>
        <div sx={stylexStyles7.rule85} />
        <span>Partial support</span>
      </div>
      <div sx={stylexStyles7.rule84}>
        <LuX sx={stylexStyles.rule8} />
        <span>Not supported</span>
      </div>
    </div>
  )
  const legend = renderLegend()

  return (
    <div sx={stylexStyles7.rule86}>
      <div sx={stylexStyles7.rule87}>
        {/* Header */}
        <div sx={stylexStyles7.rule88}>
          <div sx={stylexStyles7.rule89}>Feature</div>
          <div sx={stylexStyles7.rule90}>Spacewave</div>
          <div sx={stylexStyles7.rule91}>Cloud</div>
        </div>

        {/* Features */}
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: divide-y */ 'divide-foreground/6 divide-y'
          }
        >
          {features.map((feature) => (
            <div key={feature.name} sx={stylexStyles7.rule92}>
              <div sx={stylexStyles7.rule93}>
                <span sx={stylexStyles7.rule94}>{feature.icon}</span>
                <span>{feature.name}</span>
              </div>
              <div sx={stylexStyles7.rule95}>
                {feature.spacewave === true ? (
                  <LuCheck sx={stylexStyles.rule9} />
                ) : feature.spacewave === 'partial' ? (
                  <div sx={stylexStyles7.rule96} />
                ) : (
                  <LuX sx={stylexStyles.rule10} />
                )}
              </div>
              <div sx={stylexStyles7.rule95}>
                {feature.traditional === true ? (
                  <LuCheck sx={stylexStyles.rule9} />
                ) : feature.traditional === 'partial' ? (
                  <div sx={stylexStyles7.rule97} />
                ) : (
                  <LuX sx={stylexStyles.rule10} />
                )}
              </div>
            </div>
          ))}
        </div>
      </div>

      {legend}

      <p sx={stylexStyles7.rule98}>
        Spacewave is free software and can run directly on your devices without
        relying on the cloud.
      </p>
    </div>
  )
}

const OpenSourceSection: React.FC = () => {
  return (
    <Section
      id="open-source"
      {...ownerStylex.props(stylexStyles7.rule99)}
      withTopSeparator
    >
      <div sx={stylexStyles7.rule100}>
        <div sx={stylexStyles7.rule101}>
          <div sx={stylexStyles7.rule102}>
            <LuCode sx={stylexStyles.rule11} />
          </div>
          <div>
            <h2 sx={stylexStyles7.rule103}>Open Source Software</h2>
            <p sx={stylexStyles7.rule104}>Built in the open, for everyone</p>
          </div>
        </div>
        <ExternalLink
          href={GITHUB_REPO_URL}
          sx={[stylexStyles7.rule105, groupMarker3]}
        >
          <LuGithub sx={stylexStyles.rule12} />
          <span sx={stylexStyles7.rule11}>View on GitHub</span>
        </ExternalLink>
      </div>
    </Section>
  )
}

function Footer() {
  const links = [
    { label: 'DMCA', href: useStaticHref('/dmca') },
    { label: 'Terms of Service', href: useStaticHref('/tos') },
    { label: 'Privacy', href: useStaticHref('/privacy') },
  ]

  return (
    <footer sx={stylexStyles7.rule106}>
      <p sx={stylexStyles7.rule107}>
        &copy; 2018-2026{' '}
        <ExternalLink
          href="https://github.com/aperturerobotics"
          sx={stylexStyles6.rule3}
        >
          Aperture Robotics
        </ExternalLink>
        , LLC. and contributors
      </p>
      <nav sx={stylexStyles7.rule108}>
        {links.map((link) => (
          <a key={link.label} sx={stylexStyles7.rule109} href={link.href}>
            {link.label}
          </a>
        ))}
      </nav>
    </footer>
  )
}

// LandingContent renders the main Spacewave landing page.
export default function LandingContent(): React.ReactElement {
  return (
    <>
      <HeroSection />
      <HowItWorksSection />
      <UseCasesSection />
      <Section id="comparison" withTopSeparator>
        <ComparisonChart />
      </Section>
      <ForDevelopersSection />
      <Section id="faq" withTopSeparator>
        <FaqAccordion />
      </Section>
      <OpenSourceSection />
      <Footer />
    </>
  )
}

// stackOrder keeps dynamic layer order in the native stylesheet contract.
const stackOrder = ownerStylex.create({
  layer: (zIndex: number) => ({ zIndex }),
})
