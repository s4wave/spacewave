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

const stylexStyles = stylex.create({
  rule1: {
    display: { default: 'none', '@media (min-width: 40rem)': 'grid' },
    minHeight: 0,
    placeItems: 'center',
    padding: `calc(${themeLayout['--spacing']} * 8)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule2: {
    display: 'flex',
    minHeight: 0,
    flexDirection: 'column',
    overflow: 'hidden',
  },
  rule3: {
    display: { default: 'none', '@media (min-width: 40rem)': 'flex' },
    minHeight: 0,
    flexDirection: 'column',
    overflow: 'hidden',
  },
  rule4: {
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 14)`,
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 5)`,
    },
  },
  rule5: {
    marginLeft: `calc(${themeLayout['--spacing']} * -2)`,
    display: { default: 'flex', '@media (min-width: 40rem)': 'none' },
    minHeight: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
  },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule7: { minWidth: 0 },
  rule8: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule9: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule10: {
    minHeight: 0,
    flex: 1,
    overflow: 'auto',
    padding: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@media (min-width: 40rem)': `calc(${themeLayout['--spacing']} * 5)`,
    },
    paddingBottom: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule11: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-warning'],
  },
})

export function KeybindingCommandDetails() {
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
      <section sx={stylexStyles.rule1}>
        Choose a command to see its shortcuts.
      </section>
    )
  }

  const conflicts = pendingConflict ? [pendingConflict] : commandConflicts

  return (
    <section sx={selectedCommandId ? stylexStyles.rule2 : stylexStyles.rule3}>
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
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-4 */ 'mx-auto max-w-2xl space-y-4'
          }
        >
          {selectedLayerStatus && (
            <div sx={stylexStyles.rule11}>{selectedLayerStatus}</div>
          )}
          <KeybindingBindingsSection />
          {conflicts.length > 0 && (
            <KeybindingConflictList conflicts={conflicts} />
          )}
        </div>
      </div>
    </section>
  )
}
