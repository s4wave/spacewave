import * as stylex from '@stylexjs/stylex'
import { viewerBarMarker } from './viewer-bar.stylex.js'
import { commandDialogMarker } from './relations.stylex.js'
import { colors } from './tokens.stylex.js'

/** menuIconMarker gates the menu's default SVG dimensions to its DOM descendants. */
export const menuIconMarker = stylex.defineMarker()
/** menuMutedIconMarker gates the neutral item color without coloring checkbox indicators. */
export const menuMutedIconMarker = stylex.defineMarker()
/** commandIconMarker gates command item defaults to its rendered descendants. */
export const commandIconMarker = stylex.defineMarker()
/** buttonGroupIconMarker gates the button group label's SVG defaults. */
export const buttonGroupIconMarker = stylex.defineMarker()
/** traceIconMarker gates trace indicator dimensions. */
export const traceIconMarker = stylex.defineMarker()
/** codeTitleIconMarker gates the code title's compact SVG dimensions. */
export const codeTitleIconMarker = stylex.defineMarker()

/** bottomBarIconMarker scopes direct-child bar icon requirements. */
export const bottomBarIconMarker = stylex.defineMarker()

/** inspectorIconMarker scopes the inspector heading's direct glyph. */
export const inspectorIconMarker = stylex.defineMarker()
/** agentPanelIconMarker scopes glyphs in a connection action. */
export const agentPanelIconMarker = stylex.defineMarker()

/** iconDefaults composes DOM-scoped defaults before an icon's explicit caller styles. */
export const iconDefaults = stylex.create({
  menu: {
    height: {
      default: null,
      [stylex.when.ancestor(':is(*)', menuIconMarker)]: 16,
    },
    width: {
      default: null,
      [stylex.when.ancestor(':is(*)', menuIconMarker)]: 16,
    },
  },
  menuMuted: {
    color: {
      default: null,
      [stylex.when.ancestor(':is(*)', menuMutedIconMarker)]:
        colors['--color-muted-foreground'],
    },
  },
  command: {
    color: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandIconMarker)]:
        colors['--color-foreground-alt'],
    },
    height: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandIconMarker)]: 16,
    },
    width: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandIconMarker)]: 16,
    },
  },
  buttonGroup: {
    height: {
      default: null,
      [stylex.when.ancestor(':is(*)', buttonGroupIconMarker)]: 16,
    },
    width: {
      default: null,
      [stylex.when.ancestor(':is(*)', buttonGroupIconMarker)]: 16,
    },
  },
  trace: {
    height: {
      default: null,
      [stylex.when.ancestor(':is(*)', traceIconMarker)]: 16,
    },
    width: {
      default: null,
      [stylex.when.ancestor(':is(*)', traceIconMarker)]: 16,
    },
  },
  codeTitle: {
    height: {
      default: null,
      [stylex.when.ancestor(':is(*)', codeTitleIconMarker)]: 14,
    },
    width: {
      default: null,
      [stylex.when.ancestor(':is(*)', codeTitleIconMarker)]: 14,
    },
  },
})

/** iconRequired retains menu interaction rules independently of explicit caller defaults. */
export const iconRequired = stylex.create({
  inspector: {
    height: {
      default: null,
      [stylex.when.ancestor(':is(*) >', inspectorIconMarker)]: 16,
    },
    width: {
      default: null,
      [stylex.when.ancestor(':is(*) >', inspectorIconMarker)]: 16,
    },
  },
  agentPanel: {
    height: {
      default: null,
      [stylex.when.ancestor(':is(*)', agentPanelIconMarker)]: 14,
    },
    width: {
      default: null,
      [stylex.when.ancestor(':is(*)', agentPanelIconMarker)]: 14,
    },
  },
  bottomBar: {
    height: {
      default: null,
      [stylex.when.ancestor(':is(*) >', bottomBarIconMarker)]: '12px',
    },
    marginRight: {
      default: null,
      [stylex.when.ancestor(':is(*) >', bottomBarIconMarker)]: {
        default: null,
        ':not(:only-child)': '4px',
      },
    },
    width: {
      default: null,
      [stylex.when.ancestor(':is(*) >', bottomBarIconMarker)]: '12px',
    },
  },
  viewerBar: {
    height: {
      default: null,
      '@media (max-width: 640px) and (pointer: coarse), (max-height: 470px) and (hover: none) and (pointer: coarse)':
        {
          default: null,
          [stylex.when.ancestor(':is(*) [role="button"] >', viewerBarMarker)]:
            '16px',
        },
    },
    width: {
      default: null,
      '@media (max-width: 640px) and (pointer: coarse), (max-height: 470px) and (hover: none) and (pointer: coarse)':
        {
          default: null,
          [stylex.when.ancestor(':is(*) [role="button"] >', viewerBarMarker)]:
            '16px',
        },
    },
  },

  command: {
    flexShrink: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandIconMarker)]: 0,
    },
    pointerEvents: {
      default: null,
      [stylex.when.ancestor(':is(*)', commandIconMarker)]: 'none',
    },
  },
  commandDialog: {
    height: {
      default: null,
      [stylex.when.ancestor(
        ':is(*) :is([cmdk-item], [cmdk-input-wrapper])',
        commandDialogMarker,
      )]: '16px',
    },
    width: {
      default: null,
      [stylex.when.ancestor(
        ':is(*) :is([cmdk-item], [cmdk-input-wrapper])',
        commandDialogMarker,
      )]: '16px',
    },
  },
  buttonGroup: {
    pointerEvents: {
      default: null,
      [stylex.when.ancestor(':is(*)', buttonGroupIconMarker)]: 'none',
    },
  },
  codeTitle: {
    display: {
      default: null,
      [stylex.when.ancestor(':is(*)', codeTitleIconMarker)]: 'inline-block',
    },
    marginInlineEnd: {
      default: null,
      [stylex.when.ancestor(':is(*) >', codeTitleIconMarker)]: '6px',
    },
    pointerEvents: {
      default: null,
      [stylex.when.ancestor(':is(*)', codeTitleIconMarker)]: 'none',
    },
    verticalAlign: {
      default: null,
      [stylex.when.ancestor(':is(*)', codeTitleIconMarker)]: 'middle',
    },
  },
  trace: {
    pointerEvents: {
      default: null,
      [stylex.when.ancestor(':is(*)', traceIconMarker)]: 'none',
    },
  },
  menu: {
    flexShrink: {
      default: null,
      [stylex.when.ancestor(':is(*)', menuIconMarker)]: 0,
    },
    pointerEvents: {
      default: null,
      [stylex.when.ancestor(':is(*)', menuIconMarker)]: 'none',
    },
  },
  menuDestructive: {
    color: {
      default: null,
      [stylex.when.ancestor(
        ':is([data-variant="destructive"]) >',
        menuIconMarker,
      )]: `${colors['--color-destructive']} !important`,
    },
  },
})
