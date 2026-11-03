import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as ownerStylex from '@stylexjs/stylex'

import type { IconType } from '@s4wave/web/images/icons.js'
import React, { useCallback, useState, useMemo } from 'react'

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

import { groupMarker as groupMarker4 } from '../../web/style/relations.stylex.js'

const stylexStyles11 = ownerStylex.create({
  rule1: {
    '--tw-duration': '.3s',
    '--tw-scale-x': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker4)]: '110%',
      },
    },
    '--tw-scale-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker4)]: '110%',
      },
    },
    '--tw-scale-z': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker4)]: '110%',
      },
    },
    scale: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker4)]:
          'var(--tw-scale-x) var(--tw-scale-y)',
      },
    },
    transitionDuration: '.3s',
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    marginRight: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const stylexStyles10 = ownerStylex.create({
  rule1: { color: themeColors['--color-brand'] },
  rule2: { color: themeColors['--color-green-400'] },
  rule3: { color: 'var(--color-rose-400)' },
  rule4: { color: themeColors['--color-blue-400'] },
  rule5: { color: themeColors['--color-yellow-400'] },
})

const stylexStyles9 = ownerStylex.create({
  rule1: { marginBottom: `calc(${themeLayout['--spacing']} * 8)` },
})

const stylexStyles8 = ownerStylex.create({
  rule1: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 28)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
      '@container (min-width: 42rem)': `calc(${themeLayout['--spacing']} * 12)`,
    },
    position: 'relative',
    width: '100%',
  },
  rule2: {
    '--tw-duration': '.7s',
    transitionDuration: '.7s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule3: {
    '--tw-translate-y': '0px',
    opacity: 1,
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
  },
  rule4: {
    '--tw-translate-y': `calc(${themeLayout['--spacing']} * 8)`,
    opacity: 0,
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
  },
  rule5: {
    '--tw-leading': {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)': 'var(--leading-relaxed)',
    },
    marginInline: 'auto',
    color: themeColors['--color-foreground-alt'],
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    lineHeight: {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)': 'var(--leading-relaxed)',
    },
    maxWidth: 'var(--container-2xl)',
  },
  rule6: {
    '--tw-duration': '.3s',
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)), 0 1px 2px var(--tw-shadow-color, oklch(0% 0 0 / .3)), 0 10px 24px -14px var(--tw-shadow-color, oklch(0% 0 0 / .6))`,
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': '-1px' },
    },
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-landing-control'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      },
    },
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    cursor: 'pointer',
    display: 'flex',
    textDecorationLine: 'none',
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
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
})

const stylexStyles7 = ownerStylex.create({
  rule1: {
    insetInline: `calc(${themeLayout['--spacing']} * 4)`,
    marginInline: 'auto',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 7%, transparent)`,
    position: 'absolute',
    height: '1px',
    maxWidth: 'var(--container-5xl)',
    top: 0,
  },
  rule2: { marginInline: 'auto', maxWidth: 'var(--container-5xl)' },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: '.2em',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    textTransform: 'uppercase',
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    fontSize: {
      default: 'var(--text-3xl)',
      '@container (min-width: 32rem)': 'var(--text-4xl)',
    },
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: {
      default: 'var(--tw-leading, var(--text-3xl--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-4xl--line-height))',
    },
    textAlign: 'center',
    textWrap: 'balance',
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    '--tw-leading': 'var(--leading-relaxed)',
    marginInline: 'auto',
    color: themeColors['--color-foreground-alt'],
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    lineHeight: {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
    textAlign: 'center',
    marginBottom: `calc(${themeLayout['--spacing']} * 12)`,
    maxWidth: 'var(--container-2xl)',
  },
  rule6: {
    '--tw-duration': '.3s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)), 0 1px 2px var(--tw-shadow-color, oklch(0% 0 0 / .3)), 0 10px 24px -14px var(--tw-shadow-color, oklch(0% 0 0 / .6))`,
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    WebkitUserSelect: 'none',
    borderRadius: themeRadii['--radius-landing-control'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: 'center',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    cursor: 'pointer',
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration: '.3s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
    userSelect: 'none',
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
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexWrap: 'wrap',
    justifyContent: 'center',
    marginTop: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule10: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    fontSize: {
      default: 'var(--text-3xl)',
      '@container (min-width: 32rem)': 'var(--text-4xl)',
    },
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: {
      default: 'var(--tw-leading, var(--text-3xl--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-4xl--line-height))',
    },
    textAlign: 'center',
    textWrap: 'balance',
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule11: { WebkitUserSelect: 'none', userSelect: 'none' },
  rule12: {
    '--tw-drop-shadow':
      'drop-shadow(0 1px 2px #0000001a) drop-shadow(0 1px 1px #0000000f)',
    '--tw-drop-shadow-size':
      'drop-shadow(0 1px 2px var(--tw-drop-shadow-color, #0000001a)) drop-shadow(0 1px 1px var(--tw-drop-shadow-color, #0000000f))',
    '--tw-duration': '.3s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    color: {
      default: themeColors['--color-foreground'],
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]:
          themeColors['--color-brand'],
      },
    },
    filter:
      'var(--tw-blur, ) var(--tw-brightness, ) var(--tw-contrast, ) var(--tw-grayscale, ) var(--tw-hue-rotate, ) var(--tw-invert, ) var(--tw-saturate, ) var(--tw-sepia, ) var(--tw-drop-shadow, )',
    fontWeight: themeFonts['--font-weight-medium'],
    transitionDuration: '.3s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    userSelect: 'none',
    whiteSpace: 'nowrap',
  },
  rule13: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'flex-start',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule14: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-leading': 'var(--leading-relaxed)',
    fontFamily: themeFonts['--font-heading'],
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
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
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule18: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 12%, transparent)`,
    color: themeColors['--color-brand'],
    rotate: '45deg',
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
    '--tw-duration': '.3s',
    '--tw-ease': 'var(--ease-in-out)',
    display: 'grid',
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: 'var(--ease-in-out)',
  },
  rule21: { gridTemplateRows: '1fr', opacity: 1 },
  rule22: { gridTemplateRows: '0fr', opacity: 0 },
  rule23: { overflow: 'hidden' },
  rule24: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule25: { marginInline: 'auto', maxWidth: 'var(--container-3xl)' },
  rule26: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: '.2em',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    textTransform: 'uppercase',
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule27: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule28: { textAlign: 'center' },
  rule29: { inset: 0, overflow: 'hidden', position: 'absolute', zIndex: -10 },
  rule30: {
    inset: 0,
    backgroundColor: themeColors['--color-background'],
    position: 'absolute',
  },
  rule31: {
    opacity: 0.03,
    position: 'absolute',
    height: '100%',
    width: '100%',
  },
  rule32: { color: themeColors['--color-background-dark'] },
  rule33: {
    '--tw-duration': '.3s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)), 0 1px 2px var(--tw-shadow-color, oklch(0% 0 0 / .3)), 0 10px 24px -14px var(--tw-shadow-color, oklch(0% 0 0 / .6))`,
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      },
    },
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: { default: null, '@media (pointer: coarse)': 'center' },
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    color: themeColors['--color-brand'],
    cursor: 'pointer',
    display: {
      default: 'inline-block',
      '@media (pointer: coarse)': 'inline-flex',
    },
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textDecorationLine: 'none',
    transitionDuration: '.3s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  rule34: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
  },
  rule35: {
    marginInline: 'auto',
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
    maxWidth: 'var(--container-xl)',
  },
  rule36: {
    marginInline: 'auto',
    columnGap: `calc(${themeLayout['--spacing']} * 8)`,
    display: 'grid',
    fontSize: 'var(--text-base)',
    gridTemplateColumns: {
      default: 'repeat(1, minmax(0, 1fr))',
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
    },
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
    rowGap: `calc(${themeLayout['--spacing']} * 3)`,
    maxWidth: 'var(--container-2xl)',
  },
  rule37: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    flexWrap: 'wrap',
    justifyContent: 'center',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule38: {
    alignItems: 'center',
    color: 'var(--landing-feature-color)',
    display: 'flex',
    justifyContent: 'center',
    height: '100%',
    width: '100%',
  },
  rule39: { height: '100%', width: '100%' },
  rule40: {
    alignItems: 'center',
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    justifyContent: 'center',
    opacity: 0.6,
    height: '100%',
    width: '100%',
  },
  rule41: {
    alignItems: 'center',
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    justifyContent: 'center',
    opacity: 0.4,
    height: '100%',
    width: '100%',
  },
  rule42: {
    gap: `calc(${themeLayout['--spacing']} * 8)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule43: {
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)), 0 24px 48px -32px var(--tw-shadow-color, oklch(0% 0 0 / .7))`,
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule44: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: '.2em',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: 0.5,
    textAlign: 'center',
    textTransform: 'uppercase',
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule45: { height: `calc(${themeLayout['--spacing']} * 64)` },
  rule46: {
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)), 0 24px 48px -32px var(--tw-shadow-color, oklch(0% 0 0 / .7))`,
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule47: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: '.2em',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    textTransform: 'uppercase',
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule48: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(3, minmax(0, 1fr))',
    },
    marginTop: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule49: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule50: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    textWrap: 'balance',
  },
  rule51: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule52: {
    '--tw-inset-ring-color': `color-mix(in oklab, ${themeColors['--color-brand']} 14%, transparent)`,
    '--tw-inset-ring-shadow':
      'inset 0 0 0 1px var(--tw-inset-ring-color, currentcolor)',
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]:
          `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule53: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 70%, transparent)`,
    fontSize: themeLayout['--text-metadata'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    textTransform: 'uppercase',
  },
  rule54: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-base)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule55: {
    '--tw-font-weight': 'var(--font-weight-normal)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-normal)',
    lineHeight: 'var(--leading-relaxed)',
    textWrap: 'balance',
  },
  rule56: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
      '@container (min-width: 42rem)': 'repeat(3, minmax(0, 1fr))',
    },
  },
  rule57: {
    marginInline: 'auto',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    maxWidth: 'var(--container-lg)',
  },
  rule58: {
    '--tw-duration': '.5s',
    transitionDelay: 'var(--landing-stack-delay)',
    transitionDuration: '.5s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule59: {
    '--tw-translate-x': '0px',
    opacity: 1,
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
  },
  rule60: {
    '--tw-translate-x': `calc(${themeLayout['--spacing']} * -4)`,
    opacity: 0,
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
  },
  rule61: {
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)), 0 24px 48px -32px var(--tw-shadow-color, oklch(0% 0 0 / .7))`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    backgroundColor: themeColors['--color-background-card'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    position: 'relative',
  },
  rule62: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule63: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    width: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule64: { flex: 1, minWidth: 0 },
  rule65: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule66: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule67: {
    '--tw-duration': '.5s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: 1.5,
    transitionDelay: 'var(--landing-stack-delay)',
    transitionDuration: '.5s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule68: { opacity: 1 },
  rule69: { opacity: 0 },
  rule70: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderInlineStyle: themeUtilityState['--tw-border-style'],
    borderInlineWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 80%, transparent)`,
    display: 'flex',
    borderTopLeftRadius: themeRadii['--radius-lg'],
    borderTopRightRadius: themeRadii['--radius-lg'],
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    marginLeft: 'var(--landing-stack-offset)',
  },
  rule71: {
    borderRadius: '.25rem',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule72: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule73: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 1.5,
  },
  rule74: {
    '--tw-duration': '.5s',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'flex-end',
    transitionDelay: 'var(--landing-cta-delay)',
    transitionDuration: '.5s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule75: {
    '--tw-translate-y': '0px',
    opacity: 1,
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
  },
  rule76: {
    '--tw-translate-y': `calc(${themeLayout['--spacing']} * 2)`,
    opacity: 0,
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
  },
  rule77: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    display: 'flex',
  },
  rule78: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-widest)',
    color: themeColors['--color-brand'],
    fontSize: '10px',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
  },
  rule79: {
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)), 0 24px 48px -32px var(--tw-shadow-color, oklch(0% 0 0 / .7))`,
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    marginInline: 'auto',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    maxWidth: 'var(--container-2xl)',
  },
  rule80: {
    marginInline: 'auto',
    marginTop: `calc(${themeLayout['--spacing']} * 10)`,
    maxWidth: 'var(--container-3xl)',
  },
  rule81: {
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(3, minmax(0, 1fr))',
    },
  },
  rule82: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-brand'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule83: {
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule84: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule85: {
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-warning'],
    height: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule86: {
    marginInline: 'auto',
    fontSize: {
      default: 'var(--text-base)',
      '@container (min-width: 28rem)': 'var(--text-lg)',
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-base--line-height))',
      '@container (min-width: 28rem)':
        'var(--tw-leading, var(--text-lg--line-height))',
    },
    maxWidth: 'var(--container-4xl)',
  },
  rule87: {
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)), 0 24px 48px -32px var(--tw-shadow-color, oklch(0% 0 0 / .7))`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-card-alt'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule88: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    display: 'grid',
    gridTemplateColumns:
      'minmax(0, 1fr) clamp(5.5rem, 24%, 12rem) clamp( 3.25rem, 16%, 12rem )',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule89: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule90: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    paddingInline: {
      default: themeLayout['--spacing'],
      '@container (min-width: 28rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
    color: themeColors['--color-white'],
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 28rem)': 'var(--text-base)',
    },
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
      '@container (min-width: 28rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
    textAlign: 'center',
  },
  rule91: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    paddingInline: {
      default: themeLayout['--spacing'],
      '@container (min-width: 28rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
    color: themeColors['--color-foreground-alt'],
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 28rem)': 'var(--text-base)',
    },
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
      '@container (min-width: 28rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
    textAlign: 'center',
  },
  rule92: {
    alignItems: 'center',
    display: 'grid',
    gridTemplateColumns:
      'minmax(0, 1fr) clamp(5.5rem, 24%, 12rem) clamp( 3.25rem, 16%, 12rem )',
  },
  rule93: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule94: { color: themeColors['--color-foreground-alt'], flexShrink: 0 },
  rule95: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      '@container (min-width: 28rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
    display: 'flex',
    justifyContent: 'center',
  },
  rule96: {
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-partial'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule97: {
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-warning'],
    height: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule98: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textAlign: 'center',
    marginTop: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule99: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 12)`,
    backgroundColor: themeColors['--color-background-card'],
    position: 'relative',
    width: '100%',
  },
  rule100: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    marginInline: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 42rem)': `calc(${themeLayout['--spacing']} * 6)`,
    },
    alignItems: 'center',
    display: 'flex',
    flexDirection: {
      default: 'column',
      '@container (min-width: 32rem)': 'row',
    },
    justifyContent: 'space-between',
    maxWidth: {
      default: null,
      '@media (min-width: 40rem)': '40rem',
      '@media (min-width: 48rem)': '48rem',
      '@media (min-width: 64rem)': '64rem',
      '@media (min-width: 80rem)': '80rem',
      '@media (min-width: 96rem)': '96rem',
    },
    width: '100%',
  },
  rule101: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule102: {
    '--tw-inset-ring-color': `color-mix(in oklab, ${themeColors['--color-brand']} 14%, transparent)`,
    '--tw-inset-ring-shadow':
      'inset 0 0 0 1px var(--tw-inset-ring-color, currentcolor)',
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule103: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-xl)',
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
  },
  rule104: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule105: {
    '--tw-duration': '.3s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)), 0 1px 2px var(--tw-shadow-color, oklch(0% 0 0 / .3)), 0 10px 24px -14px var(--tw-shadow-color, oklch(0% 0 0 / .6))`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-landing-control'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
      },
    },
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    color: themeColors['--color-foreground'],
    cursor: 'pointer',
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textDecorationLine: 'none',
    transitionDuration: '.3s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  rule106: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 6)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 42rem)': `calc(${themeLayout['--spacing']} * 6)`,
    },
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-landing'],
    display: 'flex',
    flexDirection: {
      default: 'column',
      '@container (min-width: 32rem)': 'row',
    },
    flexShrink: 0,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    width: '100%',
  },
  rule107: {
    WebkitUserSelect: 'none',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule108: {
    gap: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 6)`,
    },
    display: 'flex',
    marginLeft: { default: null, '@container (min-width: 32rem)': 'auto' },
  },
  rule109: {
    WebkitUserSelect: 'none',
    paddingInline: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 2)`,
    },
    alignItems: 'center',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
    textUnderlineOffset: '4px',
    userSelect: 'none',
    minHeight: {
      default: 24,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
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
    '--tw-duration': '.3s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-shadow': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)), 0 1px 2px var(--tw-shadow-color, oklch(0% 0 0 / .3)), 0 10px 24px -14px var(--tw-shadow-color, oklch(0% 0 0 / .6))`,
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    WebkitUserSelect: 'none',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-landing-control'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
      },
    },
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    color: themeColors['--color-foreground'],
    cursor: 'pointer',
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textDecorationLine: 'none',
    transitionDuration: '.3s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
    userSelect: 'none',
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  rule3: {
    alignItems: { default: null, '@media (pointer: coarse)': 'center' },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: { default: null, '@media (pointer: coarse)': 'inline-flex' },
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
})

const stylexStyles5 = ownerStylex.create({
  rule1: {
    '--tw-translate-y': `calc(${themeLayout['--spacing']} * -.5)`,
    display: 'inline-block',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    height: `calc(${themeLayout['--spacing']} * 4)`,
    marginRight: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles4 = stylex.create({
  rule1: {
    '--tw-duration': '.3s',
    '--tw-ease': 'var(--ease-in-out)',
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.25)`,
      },
    },
    WebkitUserSelect: 'none',
    alignItems: 'center',
    display: 'flex',
    transitionDuration: '.3s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: 'var(--ease-in-out)',
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
    userSelect: 'none',
  },
  rule2: {
    '--tw-shadow': {
      default: `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)), 0 24px 48px -32px var(--tw-shadow-color, oklch(0% 0 0 / .7))`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 7%, transparent)), 0 32px 56px -30px var(--tw-shadow-color, oklch(0% 0 0 / .85))`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    boxShadow: {
      default:
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      '@media (hover: hover)': {
        default: null,
        ':hover':
          'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      },
    },
    cursor: 'pointer',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule3: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 60%, transparent)`,
  },
  rule4: {
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
  },
  rule5: {
    '--tw-duration': '.3s',
    '--tw-shadow': {
      default: `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)), 0 24px 48px -32px var(--tw-shadow-color, oklch(0% 0 0 / .7))`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 7%, transparent)), 0 32px 56px -30px var(--tw-shadow-color, oklch(0% 0 0 / .85))`,
      },
    },
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    boxShadow: {
      default:
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      '@media (hover: hover)': {
        default: null,
        ':hover':
          'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      },
    },
    transitionDuration: '.3s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
  },
  rule6: {
    '--tw-duration': '.5s',
    '--tw-shadow': {
      default: `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)), 0 24px 48px -32px var(--tw-shadow-color, oklch(0% 0 0 / .7))`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `inset 0 1px 0 var(--tw-shadow-color, color-mix(in oklab, ${themeColors['--color-foreground']} 7%, transparent)), 0 32px 56px -30px var(--tw-shadow-color, oklch(0% 0 0 / .85))`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    boxShadow: {
      default:
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      '@media (hover: hover)': {
        default: null,
        ':hover':
          'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      },
    },
    cursor: 'pointer',
    textDecorationLine: 'none',
    transitionDelay: 'var(--feature-reveal-delay)',
    transitionDuration: '.5s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule7: {
    '--tw-translate-y': {
      default: '0px',
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -1)`,
      },
    },
    opacity: 1,
    translate: {
      default: 'var(--tw-translate-x) var(--tw-translate-y)',
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
  },
  rule8: {
    '--tw-translate-y': `calc(${themeLayout['--spacing']} * 8)`,
    opacity: 0,
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule3: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule4: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    marginRight: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule6: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule7: {
    '--tw-font-weight': 'var(--font-weight-bold)',
    color: themeColors['--color-success'],
    fontWeight: 'var(--font-weight-bold)',
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule8: {
    color: themeColors['--color-red-400'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule9: {
    '--tw-font-weight': 'var(--font-weight-bold)',
    color: themeColors['--color-success'],
    fontWeight: 'var(--font-weight-bold)',
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule10: {
    color: themeColors['--color-red-400'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule11: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule12: {
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
    transitionDuration: '.3s',
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    marginRight: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

// Section provides consistent section styling with optional scroll-reveal.
function Section({
  children,
  sx,
  id,
  withTopSeparator = false,
  reveal = false,
}: Omit<
  {
    children: React.ReactNode
    className?: string
    id?: string
    withTopSeparator?: boolean
    reveal?: boolean
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  const { ref, visible } = useScrollReveal(0.08)
  const isStatic = useIsStaticMode()
  const isVisible = visible || isStatic
  return (
    <section
      id={id}
      ref={reveal && !isStatic ? ref : undefined}
      sx={[
        stylexStyles8.rule1,
        !!reveal && [
          stylexStyles8.rule2,
          isVisible ? stylexStyles8.rule3 : stylexStyles8.rule4,
        ],
        sx,
      ]}
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
  return <h2 sx={[stylexStyles7.rule4, publicText.balanced]}>{children}</h2>
}

// SectionSubtitle renders subtitle text under a heading.
function SectionSubtitle({ children }: { children: React.ReactNode }) {
  return <p sx={[stylexStyles7.rule5, publicText.balanced]}>{children}</p>
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
  <h2 sx={[stylexStyles7.rule10, publicText.balanced]}>{children}</h2>
)

// Hero Components
interface HeroTextProps {
  className?: never
  sx?: ComponentStyles
  children: React.ReactNode
}

function HeroText({
  sx,
  children,
}: Omit<HeroTextProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return <p sx={[[stylexStyles8.rule5, sx], publicText.balanced]}>{children}</p>
}

interface HeroButtonProps {
  icon: React.ReactNode
  children: React.ReactNode
  onClick?: () => void
}

const HeroButton: React.FC<HeroButtonProps> = ({ icon, children, onClick }) => (
  <button type="button" sx={stylexStyles8.rule6} onClick={onClick}>
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
    primary: stylexStyles10.rule1,
    emerald: stylexStyles10.rule2,
    pink: stylexStyles10.rule3,
    purple: stylexStyles10.rule4,
    amber: stylexStyles10.rule5,
  } as const

  return (
    <li sx={[stylexStyles4.rule1, groupMarker2]}>
      <span sx={[[stylexStyles11.rule1], [colorClasses[color]]]}>{icon}</span>
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
            [
              stylexStyles7.rule14,
              isOpen ? stylexStyles7.rule15 : stylexStyles7.rule16,
            ],
            publicText.balanced,
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
          <p sx={[stylexStyles7.rule24, publicText.balanced]}>{answer}</p>
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

        <div sx={stylexStyles9.rule1}>
          <HeroText sx={nativeChildSpacing.s0}>
            The web was built for servers.{' '}
            <strong sx={stylexStyles7.rule34}>
              Spacewave was built for you
            </strong>
            .
          </HeroText>

          <HeroText sx={nativeChildSpacing.s0}>
            Your spaces work <strong sx={stylexStyles7.rule34}>offline</strong>,
            sync <strong sx={stylexStyles7.rule34}>instantly</strong>, and store
            data <strong sx={stylexStyles7.rule34}>anywhere</strong>.
          </HeroText>

          <HeroText sx={nativeChildSpacing.s0}>
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
              <h3 sx={[stylexStyles7.rule49, publicText.balanced]}>
                {card.title}
              </h3>
              <p sx={[stylexStyles7.rule50, publicText.balanced]}>
                {card.body}
              </p>
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
      <h3 sx={[stylexStyles7.rule54, publicText.balanced]}>{title}</h3>
      <p sx={[stylexStyles7.rule55, publicText.balanced]}>{description}</p>
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
      <div sx={false}>
        {STACK_LAYERS.map((layer, i) => {
          const LayerIcon = layer.icon
          const isTop = i === STACK_LAYERS.length - 1
          return (
            <div
              key={layer.name}
              sx={[
                stylexStyles7.rule58,
                stackLayerStyles.spacing,
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
                  <p sx={[stylexStyles7.rule50, publicText.balanced]}>
                    {card.body}
                  </p>
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
        <div>
          {features.map((feature) => (
            <div
              key={feature.name}
              sx={[stylexStyles7.rule92, nativeDividers.t6]}
            >
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

      <p sx={[stylexStyles7.rule98, publicText.balanced]}>
        Spacewave is free software and can run directly on your devices without
        relying on the cloud.
      </p>
    </div>
  )
}

const OpenSourceSection: React.FC = () => {
  return (
    <Section id="open-source" sx={stylexStyles7.rule99} withTopSeparator>
      <div sx={stylexStyles7.rule100}>
        <div sx={stylexStyles7.rule101}>
          <div sx={stylexStyles7.rule102}>
            <LuCode sx={stylexStyles.rule11} />
          </div>
          <div>
            <h2 sx={[stylexStyles7.rule103, publicText.balanced]}>
              Open Source Software
            </h2>
            <p sx={[stylexStyles7.rule104, publicText.balanced]}>
              Built in the open, for everyone
            </p>
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
      <p sx={[stylexStyles7.rule107, publicText.balanced]}>
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

// nativeDividers applies the list border only to its nonfinal physical rows.
const nativeDividers = stylex.create({
  t6: {
    '--tw-divide-y-reverse': { default: null, ':not(:last-child)': 0 },
    borderColor: {
      default: null,
      ':not(:last-child)':
        'color-mix(in oklab, var(--color-foreground) 6%, transparent)',
    },
    borderBottomStyle: {
      default: null,
      ':not(:last-child)': themeUtilityState['--tw-border-style'],
    },
    borderBottomWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * calc(1 - var(--tw-divide-y-reverse)))',
    },
    borderTopStyle: {
      default: null,
      ':not(:last-child)': themeUtilityState['--tw-border-style'],
    },
    borderTopWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * var(--tw-divide-y-reverse))',
    },
  },
})

// stackLayerStyles keeps the original one-pixel overlap at the physical cards.
const stackLayerStyles = stylex.create({
  spacing: { marginBlockEnd: { default: null, ':not(:last-child)': '-1px' } },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s0: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 0 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 0 * var(--tw-space-y-reverse))`,
    },
  },
})

// publicText balances short page copy within its actual text boxes.
const publicText = stylex.create({ balanced: { textWrap: 'balance' } })
