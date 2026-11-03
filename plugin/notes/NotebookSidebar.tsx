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
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
  rule2: {
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-border'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
  },
  rule3: {
    display: 'flex',
    height: '100%',
    flexDirection: 'column',
    overflowY: 'auto',
  },
  rule4: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: themeColors['--color-border'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    color: themeColors['--color-foreground-alt'],
    textTransform: 'uppercase',
  },
  rule5: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule6: {
    display: 'flex',
    width: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
    height: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    padding: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': themeLayout['--spacing'],
      },
    },
  },
  rule7: { flex: 1, overflowY: 'auto' },
  rule8: {
    display: 'flex',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    backgroundColor: {
      default: themeColors['--color-list-active-selection-background'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-list-active-selection-foreground'],
    paddingBlock: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 1.5)`,
      },
    },
  },
  rule9: {
    display: 'flex',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    paddingBlock: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * 1.5)`,
      },
    },
  },
  rule10: {
    display: 'flex',
    minHeight: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 0,
      },
    },
    minWidth: 0,
    flex: 1,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    textAlign: 'left',
  },
  rule11: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
  },
  rule12: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule13: {
    display: 'flex',
    width: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
    height: {
      default: `calc(${themeLayout['--spacing']} * 11)`,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': 'auto',
      },
    },
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-list-hover-background'],
      },
    },
    padding: {
      default: null,
      '@media (min-width: 48rem)': {
        default: null,
        '@media (pointer: fine)': `calc(${themeLayout['--spacing']} * .5)`,
      },
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    flexShrink: 0,
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
        <button type="button" sx={stylexStyles2.rule2} onClick={onAddSource}>
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
            sx={stylexStyles2.rule6}
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
                  sx={stylexStyles2.rule10}
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
                    sx={stylexStyles2.rule13}
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
                    sx={stylexStyles2.rule13}
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
                    sx={stylexStyles2.rule13}
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
