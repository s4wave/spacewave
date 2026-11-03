import { useMemo } from 'react'

import type { ViewMode } from './route.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * .5)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule2: {
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule3: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-white']} 8%, transparent)`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule4: {
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
        ':hover': `color-mix(in oklab, ${themeColors['--color-white']} 3%, transparent)`,
      },
    },
  },
})

// ViewModeSelectorProps are props for the ViewModeSelector component.
export interface ViewModeSelectorProps {
  mode: ViewMode
  onModeChange: (mode: ViewMode) => void
  hasReadme: boolean
  availableModes?: ViewMode[]
}

// defaultTabs defines the label for each view mode.
const modeLabels: Record<ViewMode, string> = {
  files: 'Files',
  readme: 'README',
  log: 'Log',
  commit: 'Commit',
  workdir: 'Workdir',
  changes: 'Changes',
}

// ViewModeSelector renders a tab bar for switching content panels.
export function ViewModeSelector({
  mode,
  onModeChange,
  hasReadme,
  availableModes,
}: ViewModeSelectorProps) {
  const tabs = useMemo(() => {
    if (availableModes) {
      return availableModes.flatMap((m) =>
        m !== 'readme' || hasReadme ? [{ key: m, label: modeLabels[m] }] : [],
      )
    }
    const result: { key: ViewMode; label: string }[] = [
      { key: 'files', label: 'Files' },
    ]
    if (hasReadme) {
      result.push({ key: 'readme', label: 'README' })
    }
    result.push({ key: 'log', label: 'Log' })
    return result
  }, [availableModes, hasReadme])

  return (
    <div sx={stylexStyles.rule1}>
      {tabs.map((tab) => (
        <button
          type="button"
          key={tab.key}
          sx={[
            stylexStyles.rule2,
            mode === tab.key ? stylexStyles.rule3 : stylexStyles.rule4,
          ]}
          onClick={() => onModeChange(tab.key)}
        >
          {tab.label}
        </button>
      ))}
    </div>
  )
}
