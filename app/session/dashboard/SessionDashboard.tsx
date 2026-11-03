import type { IconType } from '@s4wave/web/images/icons.js'
import {
  useCallback,
  useId,
  useMemo,
  useReducer,
  useRef,
  useState,
} from 'react'
import {
  LuBuilding2,
  LuCheck,
  LuChevronDown,
  LuChevronRight,
  LuDownload,
  LuKeyRound,
  LuLayers,
  LuLink,
  LuLock,
  LuLockOpen,
  LuLogIn,
  LuShieldCheck,
} from '@s4wave/web/images/icons.js'
import { isDesktop } from '@aptre/bldr'
import {
  useResourceValue,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'

import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useNavLinks } from '@s4wave/app/nav-links.js'
import { usePageNavigate } from '@s4wave/app/routes/usePageNavigate.js'
import { QuickstartCommands } from '@s4wave/app/quickstart/QuickstartCommands.js'
import {
  getQuickstartPath,
  type QuickstartOption,
} from '@s4wave/app/quickstart/options.js'
import { useVisibleQuickstartOptions } from '@s4wave/app/quickstart/useQuickstartOptions.js'
import { NewSpaceStorageChoice } from '@s4wave/app/session/dashboard/NewSpaceStorageChoice.js'
import { useSessionOnboardingState } from '@s4wave/app/session/setup/LocalSessionOnboardingContext.js'
import { SessionLockMode } from '@s4wave/core/session/session.pb.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import { downloadPemFile } from '@s4wave/web/download.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useBottomBarSetOpenMenu } from '@s4wave/web/frame/bottom-bar-context.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import {
  getObjectTypeIconComponent,
  type ObjectTypeMetadataById,
} from '@s4wave/web/space/object-tree.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/persist.js'
import { cn } from '@s4wave/web/style/utils.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { ExperimentalBadge } from '@s4wave/web/ui/ExperimentalBadge.js'
import { CopyButton } from '@s4wave/web/ui/CopyButton.js'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@s4wave/web/ui/command.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'
import { groupMarker } from '../../../web/style/relations.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../../web/style/relations.stylex.js'

import { fonts as themeFonts } from '../../../web/style/tokens.stylex.js'

const stylexStyles6 = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-landing'],
  },
  rule2: {
    position: 'relative',
    zIndex: 10,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    pointerEvents: 'none',
    position: 'absolute',
    insetInline: 0,
    top: `calc(${themeLayout['--spacing']} * 14)`,
    zIndex: 20,
    display: 'flex',
    justifyContent: 'center',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: {
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-border']} 60%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 80%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: themeLayout['--spacing'],
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-backdrop-blur': 'blur(8px)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule5: {
    position: 'relative',
    zIndex: 10,
    display: 'flex',
    minHeight: 0,
    flex: 1,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: {
      default: 'safe center',
      '@media (max-height: 470px)': 'flex-start',
    },
    overflowY: 'auto',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 8)`,
      '@media (max-height: 470px)': `calc(${themeLayout['--spacing']} * 2)`,
    },
  },
  rule6: {
    width: '100%',
    maxWidth: {
      default: 'var(--container-md)',
      '@media (max-height: 470px)': {
        default: null,
        '@media (min-width: 40rem)': 'var(--container-2xl)',
      },
    },
    flexShrink: { default: null, '@media (max-height: 470px)': 0 },
  },
  rule7: {
    position: 'relative',
    zIndex: 10,
    paddingBottom: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'center',
  },
  rule8: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    display: { default: null, '@media (pointer: coarse)': 'inline-flex' },
    alignItems: { default: null, '@media (pointer: coarse)': 'center' },
  },
  rule9: {
    cursor: 'pointer',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: { default: null, '@media (pointer: coarse)': 'inline-flex' },
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    minWidth: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    alignItems: { default: null, '@media (pointer: coarse)': 'center' },
    justifyContent: { default: null, '@media (pointer: coarse)': 'center' },
    paddingInline: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 2)`,
    },
  },
  rule10: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: { default: null, '@media (pointer: coarse)': 'center' },
    columnGap: {
      default: null,
      '@media (pointer: coarse)': themeLayout['--spacing'],
    },
  },
  rule11: { display: 'flex', alignItems: 'center' },
  rule12: {
    paddingInline: themeLayout['--spacing'],
    fontSize: themeLayout['--text-metadata'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
    display: { default: null, '@media (pointer: coarse)': 'none' },
  },
  rule13: {
    backgroundColor: '#0000',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    whiteSpace: 'nowrap',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
      '@media (pointer: coarse)': `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    },
    textTransform: 'uppercase',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    minWidth: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  rule14: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    textAlign: 'center',
  },
  rule15: {
    fontSize: 'var(--text-2xl)',
    lineHeight: 'var(--tw-leading, var(--text-2xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    color: themeColors['--color-foreground'],
  },
  rule16: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule17: {
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
    width: '100%',
    maxWidth: 'var(--container-md)',
  },
  rule18: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-ui-outline']} 50%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule19: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule20: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule21: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule22: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule23: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule24: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule25: {
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule26: {
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
  rule27: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule28: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule29: {
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-ui-outline']} 30%, transparent)`,
  },
  rule30: {
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
  rule31: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule32: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule33: {
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule34: { display: 'flex', alignItems: 'center', justifyContent: 'center' },
  rule35: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 7)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    gap: themeLayout['--spacing'],
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 80%, transparent)`,
    fontSize: '10px',
    lineHeight: 1.5,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule36: {
    marginLeft: `calc(${themeLayout['--spacing']} * 1.5)`,
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule37: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule38: {
    cursor: 'pointer',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
  },
  rule39: {
    cursor: 'pointer',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-normal)',
    letterSpacing: 'var(--tracking-normal)',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 70%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
    textTransform: 'none',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule40: { minWidth: 0, flex: 1 },
  rule41: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule42: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule43: {
    flexShrink: 0,
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 1,
    '--tw-leading': '1',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule44: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule45: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule46: {
    maxWidth: `calc(${themeLayout['--spacing']} * 28)`,
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
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule47: {
    position: 'relative',
    marginInline: themeLayout['--spacing'],
    display: { default: 'none', ':has([cmdk-item])': 'block' },
  },
})

const stylexStyles5 = stylex.create({
  rule1: { marginInline: 0 },
  rule2: { marginInline: themeLayout['--spacing'] },
})

const stylexStyles4 = stylex.create({
  rule1: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
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
})

const stylexStyles3 = stylex.create({
  rule1: { color: themeColors['--color-brand'] },
  rule2: { color: themeColors['--color-foreground-alt'] },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const commandLayout = stylex.create({
  list: { overscrollBehaviorY: 'contain' },
  sizer: {
    display: {
      default: null,
      '@media (max-height: 470px) and (min-width: 40rem)': 'grid',
    },
    gridTemplateColumns: {
      default: null,
      '@media (max-height: 470px) and (min-width: 40rem)':
        'repeat(2, minmax(0, 1fr))',
    },
    alignItems: {
      default: null,
      '@media (max-height: 470px) and (min-width: 40rem)': 'flex-start',
    },
  },
  separator: {
    display: {
      default: null,
      '@media (max-height: 470px) and (min-width: 40rem)': 'none',
    },
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-brand'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: themeColors['--color-foreground'],
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
      [stylex.when.ancestor(':is([data-selected="true"])', groupMarker)]:
        themeColors['--color-foreground-alt'],
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    position: 'absolute',
    top: '50%',
    right: {
      default: themeLayout['--spacing'],
      '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 2)`,
    },
    width: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 7)`,
    },
    height: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 7)`,
    },
    '--tw-translate-y': 'calc(calc(1 / 2 * 100%) * -1)',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
  },
})

export interface DashboardSpace {
  id: string
  name: string
  orgId?: string
  source?: string
  objectType?: string
}

export interface DashboardOrg {
  id: string
  displayName: string
}

export interface SessionDashboardProps {
  spaces: DashboardSpace[] | undefined
  orgs?: DashboardOrg[]
  onSpaceClick?: (space: DashboardSpace) => void
  // onQuickstartClick starts a quickstart. storage is a storage backend id,
  // or 'account' for the account's own storage; undefined uses the default.
  onQuickstartClick?: (quickstartId: string, storage?: string) => void
  readOnly?: boolean
  isCloud?: boolean
  accountResource?: Resource<Account | null>
  session?: Session
  topStatus?: string
  objectTypeMetadataById?: ObjectTypeMetadataById
}

// SessionDashboard displays the main session dashboard page.
// When the session has no spaces (empty state), shows a welcome state with
// quickstart CTAs and a "Secure Your Account" section. Cloud subscribers
// additionally see a congrats heading and a create organization section.
// Local sessions show quickstart CTAs and secure account but no congrats
// or organization section.
export function SessionDashboard({
  spaces,
  orgs,
  onSpaceClick,
  onQuickstartClick,
  readOnly,
  isCloud,
  accountResource,
  session,
  topStatus,
  objectTypeMetadataById,
}: SessionDashboardProps) {
  // Resolve dashboard loading, available quickstarts and the selected storage.
  const navigate = usePageNavigate()
  const isLoading = spaces === undefined
  const isEmpty = spaces?.length === 0
  const quickstartOptions = useVisibleQuickstartOptions()
  const [storage, setStorage] = useState('')

  // Open the community page from the dashboard.
  const goToCommunity = useCallback(() => {
    navigate({ path: '/community' })
  }, [navigate])

  // Carry the chosen storage into quickstart creation.
  const handleQuickstartClick = useCallback(
    (quickstartId: string) => {
      onQuickstartClick?.(quickstartId, storage || undefined)
    },
    [onQuickstartClick, storage],
  )
  const handleQuickstartCommand = useCallback(
    (opt: QuickstartOption) => {
      handleQuickstartClick(opt.id)
    },
    [handleQuickstartClick],
  )

  return (
    <div sx={stylexStyles6.rule1}>
      <QuickstartCommands onQuickstart={handleQuickstartCommand} />
      <div sx={stylexStyles6.rule2}>
        <DashboardNav />
      </div>

      {topStatus && (
        <div sx={stylexStyles6.rule3}>
          <div sx={stylexStyles6.rule4}>
            <LoadingInline label={topStatus} tone="muted" size="sm" />
          </div>
        </div>
      )}

      <div sx={stylexStyles6.rule5}>
        {isEmpty && isCloud && !readOnly && <WelcomeHeading />}
        {!isEmpty && (
          <AnimatedLogo
            followMouse={true}
            containerClassName="mb-8 very-short:hidden"
          />
        )}

        <div sx={stylexStyles6.rule6}>
          <DashboardCommandPalette
            spaces={spaces}
            orgs={orgs}
            onSpaceClick={onSpaceClick}
            onQuickstartClick={handleQuickstartClick}
            quickstartOptions={quickstartOptions}
            objectTypeMetadataById={objectTypeMetadataById}
            isLoading={isLoading}
            isEmpty={isEmpty}
            canCreate={!readOnly}
          />
          {!readOnly && (
            <NewSpaceStorageChoice value={storage} onChange={setStorage} />
          )}
        </div>

        {isEmpty && !isLoading && !readOnly && (
          <InlineSecureAccountSection
            accountResource={accountResource}
            session={session}
          />
        )}

        {isEmpty && !isLoading && isCloud && !readOnly && <CreateOrgSection />}
      </div>

      <div sx={stylexStyles6.rule7}>
        <p sx={stylexStyles6.rule8}>
          local-first · encrypted ·{' '}
          <button
            type="button"
            onClick={goToCommunity}
            sx={stylexStyles6.rule9}
          >
            community
          </button>
        </p>
      </div>
    </div>
  )
}

function DashboardNav() {
  const nav = useNavLinks()
  const links = [
    ...(!isDesktop ? [{ text: 'Download', onClick: nav.download }] : []),
    { text: 'Docs', onClick: nav.docs },
    { text: 'Blog', onClick: nav.blog },
    { text: 'Release Notes', onClick: nav.changelog },
    { text: 'Support', onClick: nav.support },
    { text: 'Legal', onClick: nav.legal },
  ]

  return (
    <nav sx={stylexStyles6.rule10}>
      {links.map((link, i) => (
        <span key={link.text} sx={stylexStyles6.rule11}>
          {i > 0 && <span sx={stylexStyles6.rule12}>·</span>}
          <NavLink text={link.text} onClick={link.onClick} />
        </span>
      ))}
    </nav>
  )
}

function NavLink({ text, onClick }: { text: string; onClick?: () => void }) {
  const handleNavSelect = useCallback(() => onClick?.(), [onClick])

  return (
    <button type="button" onClick={handleNavSelect} sx={stylexStyles6.rule13}>
      {text}
    </button>
  )
}

// WelcomeHeading renders a congrats message for new cloud subscribers.
function WelcomeHeading() {
  return (
    <div sx={stylexStyles6.rule14}>
      <h1 sx={stylexStyles6.rule15}>Welcome to Spacewave!</h1>
      <p sx={stylexStyles6.rule16}>
        Your subscription is active. Create your first space to get started.
      </p>
    </div>
  )
}

// LockState bundles the coupled lock-mode UI inputs that transition together
// under each user action in the lock setup flow.
interface LockState {
  mode: 'auto' | 'pin'
  pin: string
  confirmPin: string
  error: string | null
}

type LockAction =
  | { type: 'set-mode'; mode: 'auto' | 'pin' }
  | { type: 'set-pin'; pin: string }
  | { type: 'set-confirm-pin'; confirmPin: string }
  | { type: 'set-error'; error: string | null }

const initialLockState: LockState = {
  mode: 'auto',
  pin: '',
  confirmPin: '',
  error: null,
}

function lockReducer(state: LockState, action: LockAction): LockState {
  switch (action.type) {
    case 'set-mode':
      return { ...state, mode: action.mode, error: null }
    case 'set-pin':
      return { ...state, pin: action.pin, error: null }
    case 'set-confirm-pin':
      return { ...state, confirmPin: action.confirmPin, error: null }
    case 'set-error':
      return { ...state, error: action.error }
  }
}

// InlineSecureAccountSection renders PEM download and PIN setup inline
// on the dashboard welcome state. It updates local session onboarding
// completion when actions are completed.
function InlineSecureAccountSection(props: {
  accountResource?: Resource<Account | null>
  session?: Session
}) {
  // Keep credential form identities and the current onboarding status together.
  const passwordInputId = useId()
  const pinInputId = useId()
  const confirmPinInputId = useId()
  const onboarding = useSessionOnboardingState()

  // Keep backup-key input and download feedback within this account panel.
  const [password, setPassword] = useState('')
  const [downloading, setDownloading] = useState(false)
  const [pemError, setPemError] = useState<string | null>(null)

  // Keep lock-mode editing and its save state together.
  const [lock, dispatchLock] = useReducer(lockReducer, initialLockState)
  const [savingLock, setSavingLock] = useState(false)

  // Use the account and session supplied to this dashboard panel.
  const account = props.accountResource?.value
  const session = props.session

  // Generate and download a password-protected backup key for the current account.
  const handleDownloadPem = useCallback(async () => {
    // Require both an account and password before generating its backup key.
    if (!account) return
    if (!password) {
      setPemError('Password is required to generate a backup key')
      return
    }
    setDownloading(true)
    setPemError(null)
    try {
      const resp = await account.generateBackupKey({
        credential: {
          credential: { case: 'password' as const, value: password },
        },
      })
      const pemData = resp.pemData
      if (!pemData || pemData.length === 0) {
        setPemError('No PEM data returned')
        return
      }

      downloadPemFile(pemData)

      onboarding.markBackupComplete()
      setPassword('')
      toast.success('Backup key downloaded')
    } catch (err) {
      const msg =
        err instanceof Error ? err.message : 'Failed to generate backup key'
      setPemError(msg)
    } finally {
      setDownloading(false)
    }
  }, [account, password, onboarding])

  // Validate and persist the selected account lock mode.
  const handleSetLockMode = useCallback(async () => {
    if (lock.mode === 'pin') {
      if (lock.pin.length < 4) {
        dispatchLock({
          type: 'set-error',
          error: 'PIN must be at least 4 digits',
        })
        return
      }
      if (lock.pin !== lock.confirmPin) {
        dispatchLock({ type: 'set-error', error: 'PINs do not match' })
        return
      }
    }
    dispatchLock({ type: 'set-error', error: null })
    setSavingLock(true)
    try {
      if (session) {
        const mode =
          lock.mode === 'pin'
            ? SessionLockMode.PIN_ENCRYPTED
            : SessionLockMode.AUTO_UNLOCK
        const pinBytes =
          lock.mode === 'pin' ? new TextEncoder().encode(lock.pin) : undefined
        await session.setLockMode(mode, pinBytes)
      }
      onboarding.markLockComplete()
      toast.success('Lock mode set')
    } catch (err) {
      dispatchLock({
        type: 'set-error',
        error: err instanceof Error ? err.message : 'Failed to set lock mode',
      })
    } finally {
      setSavingLock(false)
    }
  }, [session, lock.mode, lock.pin, lock.confirmPin, onboarding])

  return (
    <div sx={stylexStyles6.rule17}>
      <div sx={stylexStyles6.rule18}>
        <h2 sx={stylexStyles6.rule19}>
          <LuShieldCheck sx={stylexStyles2.rule1} />
          Secure Your Account
        </h2>
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-4 */ 'space-y-4'
          }
        >
          {onboarding.onboarding.backupComplete ? (
            <div sx={stylexStyles6.rule20}>
              <LuCheck sx={stylexStyles2.rule2} />
              <span>Backup key downloaded</span>
            </div>
          ) : (
            <div
              className={
                /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
              }
            >
              <div sx={stylexStyles6.rule21}>
                <div sx={stylexStyles6.rule22}>
                  <LuDownload sx={stylexStyles2.rule3} />
                </div>
                <div>
                  <p sx={stylexStyles6.rule23}>Download a backup key</p>
                  <p sx={stylexStyles6.rule24}>
                    A backup key gives you a second way to recover your account
                    if you lose your password.
                  </p>
                </div>
              </div>
              <div>
                <label htmlFor={passwordInputId} sx={stylexStyles6.rule25}>
                  Account password
                </label>
                <input
                  id={passwordInputId}
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="Enter your password"
                  sx={[stylexStyles6.rule26]}
                />
              </div>
              <button
                type="button"
                onClick={() => void handleDownloadPem()}
                disabled={downloading || !account || !password}
                sx={[[stylexStyles4.rule1, groupMarker2]]}
              >
                <LuDownload sx={stylexStyles2.rule4} />
                <span sx={stylexStyles6.rule27}>
                  {downloading ? 'Generating…' : 'Download backup .pem'}
                </span>
              </button>
              {pemError && <p sx={stylexStyles6.rule28}>{pemError}</p>}
            </div>
          )}

          <div sx={stylexStyles6.rule29} />

          {onboarding.onboarding.lockComplete ? (
            <div sx={stylexStyles6.rule20}>
              <LuCheck sx={stylexStyles2.rule2} />
              <span>Session lock configured</span>
            </div>
          ) : (
            <div
              className={
                /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
              }
            >
              <div sx={stylexStyles6.rule21}>
                <div sx={stylexStyles6.rule22}>
                  <LuKeyRound sx={stylexStyles2.rule3} />
                </div>
                <div>
                  <p sx={stylexStyles6.rule23}>Set up session lock</p>
                  <p sx={stylexStyles6.rule24}>
                    Controls how your session key is protected when the app is
                    closed.
                  </p>
                </div>
              </div>
              <div
                className={
                  /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
                }
              >
                <RadioOption
                  selected={lock.mode === 'auto'}
                  onSelect={() =>
                    dispatchLock({ type: 'set-mode', mode: 'auto' })
                  }
                  icon={<LuLockOpen sx={stylexStyles2.rule1} />}
                  label="Auto-unlock"
                  description="Key stored on disk. No PIN needed on launch."
                />
                <RadioOption
                  selected={lock.mode === 'pin'}
                  onSelect={() =>
                    dispatchLock({ type: 'set-mode', mode: 'pin' })
                  }
                  icon={<LuLock sx={stylexStyles2.rule1} />}
                  label="PIN lock"
                  description="Key encrypted with PIN. Enter PIN on each app launch."
                />
              </div>
              {lock.mode === 'pin' && (
                <div>
                  <div sx={nativeChildSpacing.s2}>
                    <label htmlFor={pinInputId} sx={stylexStyles6.rule25}>
                      PIN
                    </label>
                    <input
                      id={pinInputId}
                      type="password"
                      value={lock.pin}
                      onChange={(e) =>
                        dispatchLock({ type: 'set-pin', pin: e.target.value })
                      }
                      placeholder="Enter PIN"
                      sx={[stylexStyles6.rule26]}
                    />
                  </div>
                  <div sx={nativeChildSpacing.s2}>
                    <label
                      htmlFor={confirmPinInputId}
                      sx={stylexStyles6.rule25}
                    >
                      Confirm PIN
                    </label>
                    <input
                      id={confirmPinInputId}
                      type="password"
                      value={lock.confirmPin}
                      onChange={(e) =>
                        dispatchLock({
                          type: 'set-confirm-pin',
                          confirmPin: e.target.value,
                        })
                      }
                      placeholder="Confirm PIN"
                      sx={
                        lock.confirmPin.length > 0 &&
                        lock.pin !== lock.confirmPin
                          ? stylexStyles6.rule30
                          : stylexStyles6.rule26
                      }
                    />
                  </div>
                </div>
              )}
              <button
                type="button"
                onClick={() => void handleSetLockMode()}
                disabled={savingLock}
                sx={[[stylexStyles4.rule1, groupMarker2]]}
              >
                <span sx={stylexStyles6.rule27}>
                  {savingLock ? 'Saving…' : 'Set lock mode'}
                </span>
              </button>
              {lock.error && <p sx={stylexStyles6.rule28}>{lock.error}</p>}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

// CreateOrgSection renders a prompt to create an organization.
// Only shown for cloud subscribers in the welcome state.
function CreateOrgSection() {
  // Keep organization creation and its feedback tied to the current session.
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const [orgName, setOrgName] = useState('')
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [created, setCreated] = useState(false)

  // Create the named organization and retain its completed state.
  const handleCreate = useCallback(async () => {
    if (!session || !orgName.trim()) return
    setCreating(true)
    setError(null)
    try {
      await session.spacewave.createOrganization(orgName.trim())
      setCreated(true)
      toast.success('Organization created')
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to create organization',
      )
    } finally {
      setCreating(false)
    }
  }, [session, orgName])

  // Replace the creation form with confirmation after the organization is created.
  if (created) {
    return (
      <div sx={stylexStyles6.rule17}>
        <div sx={stylexStyles6.rule18}>
          <div sx={stylexStyles6.rule31}>
            <LuCheck sx={stylexStyles2.rule2} />
            <span>Organization created</span>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div sx={stylexStyles6.rule17}>
      <div sx={stylexStyles6.rule18}>
        <h2 sx={stylexStyles6.rule19}>
          <LuBuilding2 sx={stylexStyles2.rule1} />
          Create an Organization
        </h2>
        <p sx={stylexStyles6.rule32}>
          Organizations let you collaborate with others and manage shared
          resources.
        </p>
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
          }
        >
          <input
            type="text"
            value={orgName}
            onChange={(e) => setOrgName(e.target.value)}
            placeholder="Organization name"
            aria-label="Organization name"
            sx={[stylexStyles6.rule26]}
          />
          <button
            type="button"
            onClick={() => void handleCreate()}
            disabled={creating || !orgName.trim()}
            sx={[[stylexStyles4.rule1, groupMarker2]]}
          >
            <LuBuilding2 sx={stylexStyles2.rule4} />
            <span sx={stylexStyles6.rule27}>
              {creating ? 'Creating…' : 'Create organization'}
            </span>
          </button>
          {error && <p sx={stylexStyles6.rule28}>{error}</p>}
        </div>
      </div>
    </div>
  )
}

interface DashboardCommandPaletteProps {
  spaces: DashboardSpace[] | undefined
  orgs?: DashboardOrg[]
  objectTypeMetadataById?: ObjectTypeMetadataById
  onSpaceClick?: (space: DashboardSpace) => void
  onQuickstartClick?: (quickstartId: string) => void
  quickstartOptions: QuickstartOption[]
  isLoading: boolean
  isEmpty: boolean
  canCreate: boolean
}

// DashboardJoinGroup owns navigation to the two non-creation entry points.
function DashboardJoinGroup() {
  // Resolve navigation and the account menu's persisted detail path.
  const navigate = useNavigate()
  const setOpenMenu = useBottomBarSetOpenMenu()
  const namespace = useStateNamespace(['session-settings'])
  const [, setDetailsPath] = useStateAtom<string>(
    namespace,
    'details-path',
    '/',
  )

  // Open device linking inside the account menu.
  const handleLinkDevice = useCallback(() => {
    setDetailsPath('/link-device')
    setOpenMenu?.('account')
  }, [setDetailsPath, setOpenMenu])

  return (
    <>
      <CommandGroup heading="Join" variant="compact">
        <DashboardItem
          value="join-space"
          icon={LuLogIn}
          iconTone="muted"
          label="Join Space"
          sublabel="Join a shared space via invite code or link"
          onSelect={() => navigate({ path: './join' })}
        />
        <DashboardItem
          value="join-link-device"
          icon={LuLink}
          iconTone="muted"
          label="Link a device"
          sublabel="Link to an existing device via pairing code"
          onSelect={handleLinkDevice}
        />
      </CommandGroup>
      <CommandSeparator variant="subtle" sx={commandLayout.separator} />
    </>
  )
}

interface DashboardQuickstartGroupsProps {
  primaryQuickstart: QuickstartOption | undefined
  blankSpaceQuickstart: QuickstartOption | undefined
  browseQuickstartOptions: QuickstartOption[]
  onSelect: (option: QuickstartOption) => void
}

// DashboardQuickstartGroups renders the guided starts shown before any Space
// exists in the session.
function DashboardQuickstartGroups({
  primaryQuickstart,
  blankSpaceQuickstart,
  browseQuickstartOptions,
  onSelect,
}: DashboardQuickstartGroupsProps) {
  return (
    <>
      {primaryQuickstart && (
        <CommandGroup heading="Continue" variant="compact">
          <DashboardItem
            value={`continue-${primaryQuickstart.id}`}
            icon={primaryQuickstart.icon}
            iconTone="brand"
            label={primaryQuickstart.name}
            sublabel={primaryQuickstart.description}
            experimental={
              'experimental' in primaryQuickstart &&
              !!primaryQuickstart.experimental
            }
            onSelect={() => onSelect(primaryQuickstart)}
          />
        </CommandGroup>
      )}

      {blankSpaceQuickstart &&
        blankSpaceQuickstart.id !== primaryQuickstart?.id && (
          <CommandGroup heading="Other starts" variant="compact">
            <DashboardItem
              value={`create-${blankSpaceQuickstart.id}`}
              icon={blankSpaceQuickstart.icon}
              iconTone="muted"
              label={blankSpaceQuickstart.name}
              sublabel={blankSpaceQuickstart.description}
              experimental={
                'experimental' in blankSpaceQuickstart &&
                !!blankSpaceQuickstart.experimental
              }
              onSelect={() => onSelect(blankSpaceQuickstart)}
            />
          </CommandGroup>
        )}

      {browseQuickstartOptions.length > 0 && (
        <CommandGroup heading="Browse templates" variant="compact">
          {browseQuickstartOptions.map((option) => (
            <DashboardItem
              key={option.id}
              value={`create-${option.id}`}
              icon={option.icon}
              iconTone="muted"
              label={option.name}
              sublabel={option.description}
              experimental={'experimental' in option && !!option.experimental}
              onSelect={() => onSelect(option)}
            />
          ))}
        </CommandGroup>
      )}
    </>
  )
}

interface DashboardSpaceGroupsProps {
  hasSpaces: boolean
  personalSpaces: DashboardSpace[]
  orgSections: Array<{ org: DashboardOrg; spaces: DashboardSpace[] }>
  createQuickstartOptions: QuickstartOption[]
  objectTypeMetadataById: ObjectTypeMetadataById | undefined
  onSpaceSelect: (space: DashboardSpace) => void
  onQuickstartSelect: (option: QuickstartOption) => void
}

// DashboardSpaceGroups renders personal, organization, and creation sections
// after the session contains at least one Space.
function DashboardSpaceGroups({
  hasSpaces,
  personalSpaces,
  orgSections,
  createQuickstartOptions,
  objectTypeMetadataById,
  onSpaceSelect,
  onQuickstartSelect,
}: DashboardSpaceGroupsProps) {
  const navigate = useNavigate()
  const hasOrgs = orgSections.length > 0

  return (
    <>
      {hasSpaces && personalSpaces.length > 0 && (
        <CommandGroup
          heading={
            <SectionHeading
              label={hasOrgs ? 'Personal' : 'Spaces'}
              count={personalSpaces.length}
            />
          }
          variant="compact"
        >
          {personalSpaces.map((space) => (
            <DashboardSpaceItem
              key={space.id}
              space={space}
              objectTypeMetadataById={objectTypeMetadataById}
              onSelect={onSpaceSelect}
            />
          ))}
        </CommandGroup>
      )}

      {orgSections.map(({ org, spaces }) => (
        <CommandGroup
          key={org.id}
          heading={
            <SectionHeading
              label={org.displayName}
              count={spaces.length}
              onLabelClick={() => navigate({ path: `./org/${org.id}` })}
            />
          }
          variant="compact"
        >
          {spaces.length === 0 ? (
            <div sx={stylexStyles6.rule33}>No spaces yet</div>
          ) : (
            spaces.map((space) => (
              <DashboardSpaceItem
                key={space.id}
                space={space}
                objectTypeMetadataById={objectTypeMetadataById}
                orgName={org.displayName}
                onSelect={onSpaceSelect}
              />
            ))
          )}
        </CommandGroup>
      ))}

      {createQuickstartOptions.length > 0 && (
        <CommandGroup heading="Create" variant="compact">
          {createQuickstartOptions.map((option) => (
            <DashboardItem
              key={option.id}
              value={`create-${option.id}`}
              icon={option.icon}
              iconTone="muted"
              label={option.name}
              sublabel={option.description}
              experimental={'experimental' in option && !!option.experimental}
              onSelect={() => onQuickstartSelect(option)}
            />
          ))}
        </CommandGroup>
      )}
    </>
  )
}

interface DashboardSpaceItemProps {
  space: DashboardSpace
  objectTypeMetadataById: ObjectTypeMetadataById | undefined
  orgName?: string
  onSelect: (space: DashboardSpace) => void
}

// DashboardSpaceItem applies object-type presentation to one selectable Space.
function DashboardSpaceItem({
  space,
  objectTypeMetadataById,
  orgName,
  onSelect,
}: DashboardSpaceItemProps) {
  return (
    <DashboardItem
      value={`space-${space.name}-${space.id}`}
      icon={
        space.objectType
          ? getObjectTypeIconComponent(
              space.objectType,
              objectTypeMetadataById,
              LuLayers,
            )
          : LuLayers
      }
      iconTone="brand"
      label={space.name}
      sublabel={getSpaceSourceLabel(space.source)}
      identifier={space.id}
      orgName={orgName}
      keywords={[
        space.id,
        space.source ?? '',
        space.objectType ?? '',
        orgName ?? '',
      ]}
      onSelect={() => onSelect(space)}
    />
  )
}

function DashboardCommandPalette({
  spaces,
  orgs,
  objectTypeMetadataById,
  onSpaceClick,
  onQuickstartClick,
  quickstartOptions,
  isLoading,
  isEmpty,
  canCreate,
}: DashboardCommandPaletteProps) {
  // Resolve the command input and navigation for this dashboard.
  const inputRef = useRef<HTMLInputElement>(null)
  const navigate = useNavigate()

  // Offer creation quickstarts only when this account may create spaces.
  const createQuickstartOptions = useMemo(
    () =>
      canCreate
        ? quickstartOptions.filter(
            (opt) => opt.id !== 'account' && opt.id !== 'pair',
          )
        : [],
    [canCreate, quickstartOptions],
  )

  // Separate primary, blank-space and browsing quickstarts for the dashboard commands.
  const primaryQuickstart =
    createQuickstartOptions.find((opt) => opt.id === 'drive') ??
    createQuickstartOptions[0]
  const blankSpaceQuickstart = createQuickstartOptions.find(
    (opt) => opt.id === 'space',
  )
  const browseQuickstartOptions = createQuickstartOptions.filter(
    (opt) =>
      opt.id !== primaryQuickstart?.id && opt.id !== blankSpaceQuickstart?.id,
  )

  // Render every space so search can match all of them; the CommandList
  // scrolls past the fixed palette height instead of truncating the list.
  const recentSpaces = useMemo(() => spaces ?? [], [spaces])

  // Keep command filtering and the recent-spaces scroll cue together.
  const hasSpaces = recentSpaces.length > 0
  const [query, setQuery] = useState('')
  const showScrollCue = recentSpaces.length > 2 && query.length === 0

  // Open the selected existing space through the dashboard's callback.
  const handleSpaceSelect = useCallback(
    (space: DashboardSpace) => {
      onSpaceClick?.(space)
    },
    [onSpaceClick],
  )

  // Use the caller's quickstart action or the normal quickstart route.
  const handleQuickstartSelect = useCallback(
    (opt: QuickstartOption) => {
      if (onQuickstartClick) {
        onQuickstartClick(opt.id)
        return
      }
      navigate({ path: getQuickstartPath(opt) })
    },
    [navigate, onQuickstartClick],
  )

  // Group recent spaces into personal and organization command sections.
  const { personalSpaces, orgSections } = useMemo(() => {
    // Preserve recent-space order while grouping each organization's spaces.
    const personal: DashboardSpace[] = []
    const orgMap = new Map<string, DashboardSpace[]>()
    for (const space of recentSpaces) {
      if (space.orgId) {
        const list = orgMap.get(space.orgId)
        if (list) {
          list.push(space)
        } else {
          orgMap.set(space.orgId, [space])
        }
      } else {
        personal.push(space)
      }
    }
    const sections: Array<{ org: DashboardOrg; spaces: DashboardSpace[] }> = []
    if (orgs) {
      for (const org of orgs) {
        sections.push({ org, spaces: orgMap.get(org.id) ?? [] })
      }
    }
    return { personalSpaces: personal, orgSections: sections }
  }, [recentSpaces, orgs])

  return (
    <Command variant="dashboard">
      <CommandInput
        ref={inputRef}
        variant="dashboard"
        placeholder={hasSpaces ? 'Search spaces...' : 'Get started...'}
        value={query}
        onValueChange={setQuery}
      />
      <CommandList
        variant="transparent"
        sx={commandLayout.list}
        sizerSx={commandLayout.sizer}
      >
        <CommandEmpty variant="dashboard">
          {isLoading ? (
            <div sx={stylexStyles6.rule34}>
              <LoadingInline label="Loading spaces" tone="muted" size="sm" />
            </div>
          ) : (
            'No results'
          )}
        </CommandEmpty>

        <DashboardJoinGroup />

        {isEmpty ? (
          <DashboardQuickstartGroups
            primaryQuickstart={primaryQuickstart}
            blankSpaceQuickstart={blankSpaceQuickstart}
            browseQuickstartOptions={browseQuickstartOptions}
            onSelect={handleQuickstartSelect}
          />
        ) : (
          <DashboardSpaceGroups
            hasSpaces={hasSpaces}
            personalSpaces={personalSpaces}
            orgSections={orgSections}
            createQuickstartOptions={createQuickstartOptions}
            objectTypeMetadataById={objectTypeMetadataById}
            onSpaceSelect={handleSpaceSelect}
            onQuickstartSelect={handleQuickstartSelect}
          />
        )}
      </CommandList>
      {showScrollCue && (
        <div role="note" sx={stylexStyles6.rule35}>
          <LuChevronDown sx={stylexStyles2.rule5} />
          Scroll to see all {recentSpaces.length} spaces
        </div>
      )}
    </Command>
  )
}

function getSpaceSourceLabel(source: string | undefined): string {
  switch (source) {
    case 'created':
      return 'Owned Space'
    case 'shared':
      return 'Shared Space'
    default:
      return 'Space'
  }
}

function SectionHeading(props: {
  label: string
  count?: number
  onLabelClick?: () => void
  actionLabel?: string
  onAction?: () => void
}) {
  const labelNode = (
    <>
      {props.label}
      {props.count !== undefined && props.count > 0 && (
        <span sx={stylexStyles6.rule36}>{props.count}</span>
      )}
    </>
  )
  return (
    <span sx={stylexStyles6.rule37}>
      {props.onLabelClick ? (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation()
            props.onLabelClick?.()
          }}
          sx={stylexStyles6.rule38}
        >
          {labelNode}
        </button>
      ) : (
        <span>{labelNode}</span>
      )}
      {props.actionLabel && props.onAction && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation()
            props.onAction?.()
          }}
          sx={stylexStyles6.rule39}
        >
          + {props.actionLabel}
        </button>
      )}
    </span>
  )
}

type IconTone = 'brand' | 'muted'

const iconToneClasses: Record<
  IconTone,
  { icon: import('@stylexjs/stylex').StyleXStyles; bg: string }
> = {
  brand: {
    icon: stylexStyles3.rule1,
    bg: 'bg-brand/10 group-data-[selected=true]:bg-brand/20',
  },
  muted: {
    icon: stylexStyles3.rule2,
    bg: 'bg-foreground/5 group-data-[selected=true]:bg-foreground/10',
  },
}

// IconButton renders the lozenge-shaped icon container shared by every
// DashboardItem row.
function IconButton({ icon: Icon, tone }: { icon: IconType; tone: IconTone }) {
  const tones = iconToneClasses[tone]
  return (
    <div
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'flex size-7 shrink-0 items-center justify-center rounded-md transition-colors',
          tones.bg,
        )
      }
    >
      <Icon sx={[stylexStyles3.rule3, tones.icon]} />
    </div>
  )
}

interface DashboardItemProps {
  value: string
  icon: IconType
  iconTone: IconTone
  label: string
  sublabel?: string
  experimental?: boolean
  identifier?: string
  orgName?: string
  onSelect: () => void
  keywords?: string[]
}

function DashboardItem({
  value,
  icon,
  iconTone,
  label,
  sublabel,
  identifier,
  experimental,
  orgName,
  onSelect,
  keywords,
}: DashboardItemProps) {
  const item = (
    <CommandItem
      value={value}
      keywords={keywords}
      variant={identifier ? 'dashboardIdentifier' : 'dashboard'}
      sx={identifier ? stylexStyles5.rule1 : stylexStyles5.rule2}
      onSelect={onSelect}
    >
      <IconButton icon={icon} tone={iconTone} />
      <div sx={stylexStyles6.rule40}>
        <div sx={stylexStyles6.rule41}>
          <div sx={stylexStyles6.rule42}>{label}</div>
          {orgName && <span sx={stylexStyles6.rule43}>{orgName}</span>}
          {experimental && <ExperimentalBadge />}
        </div>
        {(sublabel || identifier) && (
          <div sx={stylexStyles6.rule44}>
            {sublabel && <div sx={stylexStyles6.rule45}>{sublabel}</div>}
            {identifier && (
              <span sx={stylexStyles6.rule46} title={identifier}>
                {identifier}
              </span>
            )}
          </div>
        )}
      </div>
      <LuChevronRight sx={stylexStyles2.rule6} />
    </CommandItem>
  )

  if (!identifier) return item

  return (
    <div sx={stylexStyles6.rule47}>
      {item}
      <CopyButton
        text={identifier}
        label={`Copy ${label} ID`}
        variant="toolbar"
        sx={stylexStyles.rule1}
      />
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
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
