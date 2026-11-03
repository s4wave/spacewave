import * as ownerStylex from '@stylexjs/stylex'

/* eslint-disable react-doctor/no-giant-component */
import { useCallback, useMemo, useState } from 'react'
import {
  LuArrowLeft,
  LuGithub,
  LuHeart,
  LuShield,
  LuWifi,
  LuHardDrive,
  LuPuzzle,
  LuGitFork,
  LuMessageSquare,
  LuBookOpen,
  LuArrowRight,
  LuSearch,
} from '@s4wave/web/images/icons.js'
import {
  DISCORD_INVITE_URL,
  GITHUB_ISSUES_URL,
  GITHUB_REPO_URL,
} from '@s4wave/app/github.js'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { useInvokeCommand } from '@s4wave/web/command/index.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { ExternalLink } from './ExternalLink.js'
import {
  licenseEntries,
  groupByCategory,
  type AnnotatedLicenseEntry,
} from '@s4wave/app/licenses/data.js'
import { useLandingBackNavigation } from './useLandingBackNavigation.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  motion as themeMotion,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

import { groupMarker as groupMarker3 } from '../../web/style/relations.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

import { groupMarker as groupMarker4 } from '../../web/style/relations.stylex.js'

const stylexStyles5 = ownerStylex.create({
  rule1: {
    containerType: 'inline-size',
    display: 'flex',
    width: '100%',
    flex: 1,
    flexDirection: 'column',
    overflowY: 'auto',
    backgroundColor: themeColors['--color-background-landing'],
  },
  rule2: {
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingTop: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule3: {
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
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
  rule4: {
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-3xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingTop: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 20)`,
    },
    paddingBottom: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 16)`,
    },
    textAlign: 'center',
  },
  rule5: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    color: themeColors['--color-brand'],
  },
  rule6: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    fontSize: {
      default: 'var(--text-4xl)',
      '@container (min-width: 32rem)': 'var(--text-5xl)',
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-4xl--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-5xl--line-height))',
    },
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule7: { color: themeColors['--color-brand'] },
  rule8: {
    marginInline: 'auto',
    maxWidth: 'var(--container-xl)',
    fontSize: {
      default: 'var(--text-base)',
      '@container (min-width: 32rem)': 'var(--text-lg)',
    },
    lineHeight: {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-lg--line-height))',
    },
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule9: {
    marginTop: `calc(${themeLayout['--spacing']} * 8)`,
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule10: {
    display: 'inline-flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2.5)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      },
    },
  },
  rule11: {
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-4xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingBottom: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 16)`,
    },
  },
  rule12: {
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
    display: 'block',
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: '.2em',
    color: themeColors['--color-foreground-alt'],
    textTransform: 'uppercase',
  },
  rule13: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(3, minmax(0, 1fr))',
    },
  },
  rule14: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
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
  rule15: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule16: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule17: {
    marginTop: 'auto',
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand'],
  },
  rule18: {
    marginInline: 'auto',
    height: '1px',
    width: '100%',
    maxWidth: 'var(--container-4xl)',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 7%, transparent)`,
  },
  rule19: {
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-4xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 16)`,
    },
  },
  rule20: {
    display: 'grid',
    columnGap: `calc(${themeLayout['--spacing']} * 8)`,
    rowGap: `calc(${themeLayout['--spacing']} * 10)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule21: {
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
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker4)]:
          `color-mix(in oklab, ${themeColors['--color-brand']} 12%, transparent)`,
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule22: {
    marginBottom: themeLayout['--spacing'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule23: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'block',
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: '.2em',
    color: themeColors['--color-foreground-alt'],
    textTransform: 'uppercase',
  },
  rule24: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule25: {
    marginBottom: `calc(${themeLayout['--spacing']} * 10)`,
    display: 'flex',
    justifyContent: 'center',
  },
  rule26: {
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
  rule27: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule28: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule29: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: 'var(--radius-sm)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule30: {
    borderColor: themeColors['--color-brand'],
    backgroundColor: themeColors['--color-brand'],
  },
  rule31: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
  },
  rule32: { marginBottom: `calc(${themeLayout['--spacing']} * 6)` },
  rule33: {
    position: 'relative',
    marginInline: 'auto',
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
    maxWidth: 'var(--container-md)',
  },
  rule34: {
    width: '100%',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 3)`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 9)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
    },
  },
  rule35: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 8)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule36: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule37: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule38: { color: themeColors['--color-foreground'] },
  rule39: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule40: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 36rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule41: {
    display: 'flex',
    cursor: 'pointer',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
  rule42: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
  },
  rule43: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
  },
  rule44: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule45: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule46: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule47: {
    maxWidth: `calc(${themeLayout['--spacing']} * 28)`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: '.55rem',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule48: {
    marginLeft: 'auto',
    display: 'flex',
    flexShrink: 0,
    gap: themeLayout['--spacing'],
  },
  rule49: {
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '.55rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule50: {
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-cyan-500']} 15%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '.55rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-cyan-400']} 60%, transparent)`,
  },
  rule51: {
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-yellow-500']} 15%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: '.55rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-yellow-400']} 60%, transparent)`,
  },
  rule52: {
    fontSize: '.6rem',
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule53: {
    marginTop: `calc(${themeLayout['--spacing']} * 8)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule54: {
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
    textDecorationLine: 'underline',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule55: {
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-3xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 10)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 12)`,
    },
    textAlign: 'center',
  },
  rule56: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule57: {
    paddingBottom: `calc(${themeLayout['--spacing']} * 8)`,
    textAlign: 'center',
  },
  rule58: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
})

const stylexStyles4 = ownerStylex.create({
  rule1: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2.5)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      },
    },
  },
  rule2: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
  },
  rule3: { color: themeColors['--color-brand'] },
  rule4: { color: themeColors['--color-foreground'] },
  rule5: {
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'inline-flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand-highlight'],
      },
    },
    textDecorationLine: 'underline',
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]:
          themeColors['--color-brand'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-brand'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-translate-x': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeLayout['--spacing'],
      },
    },
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
  },
  rule4: {
    pointerEvents: 'none',
    position: 'absolute',
    top: '50%',
    left: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    '--tw-translate-y': 'calc(calc(1 / 2 * 100%) * -1)',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
  },
})

export const metadata = {
  title: 'Community - Spacewave',
  description:
    'Join the Spacewave open-source community. Built with Go, TypeScript, React, and WebAssembly. Contribute code, docs, ideas.',
  canonicalPath: '/community',
}

const PRINCIPLES = [
  {
    icon: LuShield,
    title: 'End-to-end encrypted',
    body: 'All data is end-to-end encrypted by default. Protect your account with a password, a long PIN, or a passkey that can derive keys, and only your devices can unlock it.',
  },
  {
    icon: LuWifi,
    title: 'Local-first, always',
    body: 'Your devices talk directly. Cloud relay and backup are optional and always encrypted.',
  },
  {
    icon: LuHardDrive,
    title: 'Your data, wherever you want it',
    body: "Most apps store your data in someone else's database. Spacewave runs on your device and stores data wherever you want.",
  },
  {
    icon: LuPuzzle,
    title: 'Fully extensible',
    body: 'Add plugins, modify the source, or build something entirely new. Share your creations with others through Spacewave itself.',
  },
]

const CONTRIBUTE_PATHS = [
  {
    icon: LuGitFork,
    title: 'Code',
    description: 'Fix a bug, build a feature, improve the architecture.',
    href: GITHUB_ISSUES_URL,
    linkText: 'Browse open issues',
  },
  {
    icon: LuBookOpen,
    title: 'Documentation',
    description: 'Write guides, improve docs, translate for new audiences.',
    href: `${GITHUB_REPO_URL}/blob/master/CONTRIBUTING.md`,
    linkText: 'Help with docs',
  },
  {
    icon: LuMessageSquare,
    title: 'Community',
    description: 'Connect with other Spacewave users and contributors.',
    href: DISCORD_INVITE_URL,
    linkText: 'Join our Discord',
  },
]

const devCount = licenseEntries.filter((e) => e.isDev).length

function describeSource(entry: AnnotatedLicenseEntry): string {
  if (entry.source === 'both') return 'Go + JS'
  if (entry.source === 'go') return 'Go'
  return 'JS'
}

function formatDependencyDetails(entry: AnnotatedLicenseEntry): string {
  const lines = [
    `Name: ${entry.name}`,
    `Version: ${entry.version}`,
    `License: ${entry.spdx}`,
    `Source: ${describeSource(entry)}`,
    `Category: ${entry.category}`,
  ]
  if (entry.purpose) lines.push(`Purpose: ${entry.purpose}`)
  if (entry.repo) lines.push(`URL: ${entry.repo}`)
  return lines.join('\n')
}

export function Community() {
  const goBack = useLandingBackNavigation()
  const invokeCommand = useInvokeCommand()
  const licensesHref = useStaticHref('/licenses')
  const [showDev, setShowDev] = useState(false)
  const [search, setSearch] = useState('')
  const filtered = useMemo(() => {
    let entries = showDev
      ? licenseEntries
      : licenseEntries.filter((e) => !e.isDev)
    if (search) {
      const q = search.toLowerCase()
      entries = entries.filter(
        (e) =>
          e.name.toLowerCase().includes(q) ||
          e.purpose?.toLowerCase().includes(q),
      )
    }
    return entries
  }, [showDev, search])
  const ossGroups = useMemo(() => groupByCategory(filtered), [filtered])
  const ossStats = useMemo(() => {
    const goCount = filtered.filter(
      (e) => e.source === 'go' || e.source === 'both',
    ).length
    const jsCount = filtered.filter(
      (e) => e.source === 'js' || e.source === 'both',
    ).length
    const spdxSet = new Set(filtered.map((e) => e.spdx))
    return {
      total: filtered.length,
      goCount,
      jsCount,
      uniqueLicenses: spdxSet.size,
    }
  }, [filtered])
  const handleCopyDependency = useCallback((entry: AnnotatedLicenseEntry) => {
    navigator.clipboard.writeText(formatDependencyDetails(entry)).then(
      () => {
        toast.success('Copied dependency details', {
          description: entry.name,
          duration: 2000,
        })
      },
      () => {
        toast.error('Copy failed', {
          description: `Could not copy ${entry.name}.`,
        })
      },
    )
  }, [])
  const handleEmailSupport = useCallback(() => {
    invokeCommand('spacewave.help.email-support')
  }, [invokeCommand])

  return (
    <div sx={stylexStyles5.rule1}>
      {/* Back button */}
      <div sx={stylexStyles5.rule2}>
        <button type="button" onClick={goBack} sx={stylexStyles5.rule3}>
          <LuArrowLeft sx={stylexStyles.rule1} />
          Back
        </button>
      </div>

      {/* Hero */}
      <header sx={stylexStyles5.rule4}>
        <div sx={stylexStyles5.rule5}>
          <LuHeart sx={stylexStyles.rule2} />
        </div>
        <h1 sx={stylexStyles5.rule6}>
          Built in the open,
          <br />
          <span sx={stylexStyles5.rule7}>for&nbsp;everyone</span>
        </h1>
        <p sx={stylexStyles5.rule8}>
          Spacewave is free and open-source software built by the community.
        </p>
        <div sx={stylexStyles5.rule9}>
          <ExternalLink href={GITHUB_REPO_URL} sx={stylexStyles4.rule1}>
            <LuGithub sx={stylexStyles.rule2} />
            View on GitHub
          </ExternalLink>
          <button
            type="button"
            onClick={handleEmailSupport}
            sx={stylexStyles5.rule10}
          >
            <LuMessageSquare sx={stylexStyles.rule2} />
            Email Support
          </button>
        </div>
      </header>

      {/* Contribute */}
      <section sx={stylexStyles5.rule11}>
        <span sx={stylexStyles5.rule12}>Get involved</span>
        <div sx={stylexStyles5.rule13}>
          {CONTRIBUTE_PATHS.map((c) => (
            <ExternalLink
              key={c.title}
              href={c.href}
              sx={[stylexStyles5.rule14, groupMarker4]}
            >
              <c.icon sx={stylexStyles2.rule1} />
              <h3 sx={stylexStyles5.rule15}>{c.title}</h3>
              <p sx={stylexStyles5.rule16}>{c.description}</p>
              <span sx={stylexStyles5.rule17}>
                {c.linkText}
                <LuArrowRight sx={stylexStyles.rule3} />
              </span>
            </ExternalLink>
          ))}
        </div>
      </section>

      {/* Divider */}
      <div sx={stylexStyles5.rule18} />

      {/* Principles */}
      <section sx={stylexStyles5.rule19}>
        <span sx={stylexStyles5.rule12}>What we believe</span>
        <div sx={stylexStyles5.rule20}>
          {PRINCIPLES.map((p) => (
            <div key={p.title} sx={[stylexStyles3.rule1, groupMarker3]}>
              <div sx={stylexStyles5.rule21}>
                <p.icon sx={stylexStyles2.rule2} />
              </div>
              <div>
                <h3 sx={stylexStyles5.rule22}>{p.title}</h3>
                <p sx={stylexStyles5.rule16}>{p.body}</p>
              </div>
            </div>
          ))}
        </div>
      </section>

      {/* Divider */}
      <div sx={stylexStyles5.rule18} />

      {/* Built With Open Source */}
      <section sx={stylexStyles5.rule19}>
        <span sx={stylexStyles5.rule23}>Built with open source</span>
        <p sx={stylexStyles5.rule24}>
          {ossStats.total} packages &middot; {ossStats.goCount} Go &middot;{' '}
          {ossStats.jsCount} JS &middot; {ossStats.uniqueLicenses} license types
        </p>
        {devCount > 0 && (
          <div sx={stylexStyles5.rule25}>
            <button
              type="button"
              onClick={() => setShowDev((v) => !v)}
              sx={[
                stylexStyles5.rule26,
                showDev ? stylexStyles5.rule27 : stylexStyles5.rule28,
              ]}
            >
              <span
                sx={[
                  stylexStyles5.rule29,
                  showDev ? stylexStyles5.rule30 : stylexStyles5.rule31,
                ]}
              />
              Show {devCount} dev{' '}
              {devCount === 1 ? 'dependency' : 'dependencies'}
            </button>
          </div>
        )}
        {!devCount && <div sx={stylexStyles5.rule32} />}
        <div sx={stylexStyles5.rule33}>
          <LuSearch sx={stylexStyles.rule4} />
          <input
            aria-label="Filter packages"
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Filter packages…"
            sx={stylexStyles5.rule34}
          />
        </div>
        {ossGroups.length === 0 ? (
          <div sx={stylexStyles5.rule35}>
            No packages match &ldquo;{search}&rdquo;
          </div>
        ) : (
          <div
            className={
              /* TODO(stylex): a descendant, group, peer or compound selector: space-y-8 */ 'space-y-8'
            }
          >
            {ossGroups.map(({ category, entries }) => (
              <div key={category.id}>
                <div sx={stylexStyles5.rule36}>
                  <h3
                    sx={[
                      stylexStyles5.rule37,
                      category.id === 'internal'
                        ? stylexStyles5.rule7
                        : stylexStyles5.rule38,
                    ]}
                  >
                    {category.id === 'internal' ? 'Our Stack' : category.label}
                  </h3>
                  <span sx={stylexStyles5.rule39}>{entries.length}</span>
                </div>
                <div sx={stylexStyles5.rule40}>
                  {entries.map((entry) => (
                    <div
                      key={`${entry.name}:${entry.version}:${entry.spdx}:${entry.source}:${entry.repo ?? 'local'}`}
                      onClick={() => handleCopyDependency(entry)}
                      onKeyDown={(e) => {
                        if (e.key !== 'Enter' && e.key !== ' ') return
                        e.preventDefault()
                        handleCopyDependency(entry)
                      }}
                      role="button"
                      tabIndex={0}
                      aria-label={`Copy dependency details for ${entry.name}`}
                      sx={[
                        stylexStyles5.rule41,
                        entry.internal
                          ? stylexStyles5.rule42
                          : stylexStyles5.rule43,
                      ]}
                    >
                      <div sx={stylexStyles5.rule44}>
                        <div sx={stylexStyles5.rule45}>
                          {entry.repo ? (
                            <ExternalLink
                              href={entry.repo}
                              onClick={(e) => e.stopPropagation()}
                              sx={[
                                stylexStyles4.rule2,
                                entry.internal
                                  ? stylexStyles4.rule3
                                  : stylexStyles4.rule4,
                              ]}
                            >
                              {entry.name}
                            </ExternalLink>
                          ) : (
                            <span
                              sx={[
                                stylexStyles5.rule46,
                                entry.internal
                                  ? stylexStyles5.rule7
                                  : stylexStyles5.rule38,
                              ]}
                            >
                              {entry.name}
                            </span>
                          )}
                          <span title={entry.version} sx={stylexStyles5.rule47}>
                            {entry.version}
                          </span>
                        </div>
                        <div sx={stylexStyles5.rule48}>
                          <span sx={stylexStyles5.rule49}>{entry.spdx}</span>
                          {(entry.source === 'go' ||
                            entry.source === 'both') && (
                            <span sx={stylexStyles5.rule50}>Go</span>
                          )}
                          {(entry.source === 'js' ||
                            entry.source === 'both') && (
                            <span sx={stylexStyles5.rule51}>JS</span>
                          )}
                        </div>
                      </div>
                      {entry.purpose && (
                        <p sx={stylexStyles5.rule52}>{entry.purpose}</p>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        )}
        <p sx={stylexStyles5.rule53}>
          Full license texts available at{' '}
          <a href={licensesHref} sx={stylexStyles5.rule54}>
            /licenses
          </a>
        </p>
      </section>

      {/* Divider */}
      <div sx={stylexStyles5.rule18} />

      {/* Open source acknowledgment */}
      <section sx={stylexStyles5.rule55}>
        <p sx={stylexStyles5.rule56}>
          Spacewave is built with Go, TypeScript, React, WebAssembly, and
          WebRTC, running in web browsers, Linux, and other operating systems.
          We are grateful to everyone behind these projects.
        </p>
        <ExternalLink href={GITHUB_REPO_URL} sx={stylexStyles4.rule5}>
          <LuGithub sx={stylexStyles.rule1} />
          Contribute to Spacewave
        </ExternalLink>
      </section>

      {/* Footer */}
      <footer sx={stylexStyles5.rule57}>
        <p sx={stylexStyles5.rule58}>
          © 2018–2026 Aperture Robotics, LLC. and contributors
        </p>
      </footer>
    </div>
  )
}
