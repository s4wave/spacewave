import { controlTargetStyles } from '@s4wave/web/style/ControlStylesProvider.js'
import { useCallback } from 'react'

import type { NotebookSource } from './proto/notebook.pb.js'

import { useStateAtom, type StateNamespace } from '@s4wave/web/state/index.js'
import {
  LuChevronRight,
  LuChevronDown,
  LuFolder,
  LuPlus,
  LuTrash2,
  LuArrowUp,
  LuArrowDown,
} from '@s4wave/web/images/icons.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '@s4wave/web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '@s4wave/web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    color: themeColors['--color-muted-foreground'],
    display: 'flex',
    flexDirection: 'column',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    height: '100%',
  },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: themeColors['--color-border'],
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
  },
  rule3: {
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    overflowY: 'auto',
  },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    borderColor: themeColors['--color-border'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textTransform: 'uppercase',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule6: {
    padding: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': themeLayout['--spacing'],
      },
    },
    borderRadius: '.25rem',
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: 'flex',
    justifyContent: 'center',
    height: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
    width: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
  },
  rule7: { flex: 1, overflowY: 'auto' },
  rule8: {
    gap: themeLayout['--spacing'],
    paddingBlock: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 1.5)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: themeColors['--color-list-active-selection-background'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    color: themeColors['--color-list-active-selection-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
  },
  rule9: {
    gap: themeLayout['--spacing'],
    paddingBlock: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 1.5)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
  },
  rule10: {
    flex: 1,
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
    textAlign: 'left',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    minWidth: 0,
  },
  rule11: {
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule12: {
    gap: `calc(${themeLayout['--spacing']} * .5)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
  },
  rule13: {
    padding: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * .5)`,
      },
    },
    borderRadius: '.25rem',
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    display: 'flex',
    justifyContent: 'center',
    height: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
    width: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

interface NotebookSidebarProps {
  sources: NotebookSource[]
  selectedSource: number
  onSelectSource: (index: number) => void
  onAddSource?: () => void
  onRemoveSource?: (index: number) => void
  onMoveSource?: (index: number, delta: -1 | 1) => void
  namespace: StateNamespace
}

/** NotebookSidebar shows the source list from the Notebook. */
function NotebookSidebar({
  sources,
  selectedSource,
  onSelectSource,
  onAddSource,
  onRemoveSource,
  onMoveSource,
  namespace,
}: NotebookSidebarProps) {
  const [expandedSources, setExpandedSources] = useStateAtom<
    Record<number, boolean>
  >(namespace, 'expandedSources', {})

  const toggleExpanded = useCallback(
    (index: number) => {
      setExpandedSources((prev) => ({
        ...prev,
        [index]: !prev[index],
      }))
    },
    [setExpandedSources],
  )

  if (sources.length === 0) {
    return (
      <div sx={stylexStyles2.rule1}>
        <span>No sources configured</span>
        <button
          type="button"
          sx={[stylexStyles2.rule2, controlTargetStyles.minimum]}
          onClick={onAddSource}
        >
          Add source
        </button>
      </div>
    )
  }

  return (
    <div sx={stylexStyles2.rule3}>
      <div sx={stylexStyles2.rule4}>
        <div sx={stylexStyles2.rule5}>
          <span>Sources</span>
          <button
            type="button"
            sx={[stylexStyles2.rule6, controlTargetStyles.minimum]}
            title="Add source"
            onClick={onAddSource}
          >
            <LuPlus sx={stylexStyles.rule1} />
          </button>
        </div>
      </div>
      <div sx={stylexStyles2.rule7}>
        {sources.map((source, index) => {
          const expanded = expandedSources[index] ?? true
          const selected = selectedSource === index
          return (
            <div key={source.ref || source.name || `source-${selectedSource}`}>
              <div sx={selected ? stylexStyles2.rule8 : stylexStyles2.rule9}>
                <button
                  type="button"
                  sx={[stylexStyles2.rule10, controlTargetStyles.minimum]}
                  aria-current={selected ? 'true' : undefined}
                  onClick={() => {
                    onSelectSource(index)
                    toggleExpanded(index)
                  }}
                >
                  {expanded ? (
                    <LuChevronDown sx={stylexStyles.rule2} />
                  ) : (
                    <LuChevronRight sx={stylexStyles.rule2} />
                  )}
                  <LuFolder sx={stylexStyles.rule2} />
                  <span sx={stylexStyles2.rule11}>
                    {source.name || `Source ${index + 1}`}
                  </span>
                </button>
                <span sx={stylexStyles2.rule12}>
                  <button
                    type="button"
                    sx={[stylexStyles2.rule13, controlTargetStyles.minimum]}
                    title="Move source up"
                    disabled={index === 0}
                    onClick={(e) => {
                      e.stopPropagation()
                      onMoveSource?.(index, -1)
                    }}
                  >
                    <LuArrowUp sx={stylexStyles.rule1} />
                  </button>
                  <button
                    type="button"
                    sx={[stylexStyles2.rule13, controlTargetStyles.minimum]}
                    title="Move source down"
                    disabled={index === sources.length - 1}
                    onClick={(e) => {
                      e.stopPropagation()
                      onMoveSource?.(index, 1)
                    }}
                  >
                    <LuArrowDown sx={stylexStyles.rule1} />
                  </button>
                  <button
                    type="button"
                    sx={[stylexStyles2.rule13, controlTargetStyles.minimum]}
                    title="Remove source"
                    onClick={(e) => {
                      e.stopPropagation()
                      onRemoveSource?.(index)
                    }}
                  >
                    <LuTrash2 sx={stylexStyles.rule1} />
                  </button>
                </span>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

export default NotebookSidebar
