import { LuArrowLeft } from '@s4wave/web/images/icons.js'

import { KeybindingBindingsSection } from './KeybindingBindingsSection.js'
import { KeybindingConflictList } from './KeybindingConflictList.js'
import { useKeybindingEditorContext } from './KeybindingEditorContext.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
  utilityState as themeUtilityState,
  radii as themeRadii,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { marginInline: 'auto', maxWidth: 'var(--container-2xl)' },
})

const childSpacing = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 8)`,
    placeItems: 'center',
    color: themeColors['--color-foreground-alt'],
    display: { default: 'none', '@media (min-width: 40rem)': 'grid' },
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textAlign: 'center',
    minHeight: 0,
  },
  rule2: {
    overflow: 'hidden',
    display: 'flex',
    flexDirection: 'column',
    minHeight: 0,
  },
  rule3: {
    overflow: 'hidden',
    display: { default: 'none', '@media (min-width: 40rem)': 'flex' },
    flexDirection: 'column',
    minHeight: 0,
  },
  rule4: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 5)`,
    },
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    minHeight: `calc(${themeLayout['--spacing']} * 14)`,
  },
  rule5: {
    borderRadius: '.25rem',
    gap: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: { default: 'flex', '@media (min-width: 40rem)': 'none' },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginLeft: `calc(${themeLayout['--spacing']} * -2)`,
    minHeight: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule6: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule7: { minWidth: 0 },
  rule8: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule9: {
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule10: {
    padding: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 5)`,
    },
    flex: 1,
    overflow: 'auto',
    minHeight: 0,
    paddingBottom: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule11: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
    color: themeColors['--color-warning'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

export function KeybindingCommandDetails({
  sx,
}: { sx?: stylex.StyleXStyles; className?: never } = {}) {
  const {
    commandConflicts,
    pendingConflict,
    selectedCommandId,
    selectedLayerStatus,
    selectedRow,
    setSelectedCommandId,
  } = useKeybindingEditorContext()

  if (!selectedRow) {
    return (
      <section sx={[stylexStyles.rule1, sx]}>
        Choose a command to see its shortcuts.
      </section>
    )
  }

  const conflicts = pendingConflict ? [pendingConflict] : commandConflicts

  return (
    <section
      sx={[selectedCommandId ? stylexStyles.rule2 : stylexStyles.rule3, sx]}
    >
      <div sx={stylexStyles.rule4}>
        <button
          type="button"
          sx={stylexStyles.rule5}
          onClick={() => setSelectedCommandId(null)}
        >
          <LuArrowLeft sx={stylexStyles.rule6} />
          Back
        </button>
        <div sx={stylexStyles.rule7}>
          <h2 sx={stylexStyles.rule8}>{selectedRow.label}</h2>
          {selectedRow.menuPath && (
            <div sx={stylexStyles.rule9}>
              {selectedRow.menuPath.split('/').join(' › ')}
            </div>
          )}
        </div>
      </div>
      <div sx={stylexStyles.rule10}>
        <div sx={stylexStyles2.rule1}>
          {selectedLayerStatus && (
            <div sx={[stylexStyles.rule11, childSpacing.s4]}>
              {selectedLayerStatus}
            </div>
          )}
          <KeybindingBindingsSection sx={childSpacing.s4} />
          {conflicts.length > 0 && (
            <KeybindingConflictList
              sx={childSpacing.s4}
              conflicts={conflicts}
            />
          )}
        </div>
      </div>
    </section>
  )
}
