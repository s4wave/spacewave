import type { ComponentProps, ReactNode, Ref } from 'react'

import { GitToolbar } from './layout/GitToolbar.js'
import { ViewModeSelector } from './layout/ViewModeSelector.js'
import type { ViewMode } from './layout/route.js'
import { SelectedRefBar } from './refs/SelectedRefBar.js'

import * as stylex from '@stylexjs/stylex'
import {
  fonts as themeFonts,
  colors as themeColors,
  layout as themeLayout,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    overflow: 'hidden',
  },
  rule2: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule3: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
  },
  rule4: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule5: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    overflow: 'hidden',
  },
})

export function GitViewerCenteredState({
  containerRef,
  title,
  subtitle,
  detail,
  action,
}: {
  containerRef?: Ref<HTMLDivElement>
  title: ReactNode
  subtitle?: ReactNode
  detail?: ReactNode
  action?: ReactNode
}) {
  return (
    <div ref={containerRef} sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>{title}</div>
      {subtitle && <div sx={stylexStyles.rule3}>{subtitle}</div>}
      {detail && <div sx={stylexStyles.rule4}>{detail}</div>}
      {action}
    </div>
  )
}

export function GitViewerFrame({
  containerRef,
  toolbarProps,
  refBarProps,
  mode,
  onModeChange,
  hasReadme,
  availableModes,
  children,
}: {
  containerRef?: Ref<HTMLDivElement>
  toolbarProps: ComponentProps<typeof GitToolbar>
  refBarProps?: ComponentProps<typeof SelectedRefBar>
  mode?: ViewMode
  onModeChange?: (mode: ViewMode) => void
  hasReadme?: boolean
  availableModes?: ViewMode[]
  children: ReactNode
}) {
  return (
    <div ref={containerRef} sx={stylexStyles.rule5}>
      <GitToolbar {...toolbarProps} />
      {refBarProps && <SelectedRefBar {...refBarProps} />}
      {mode && onModeChange && (
        <ViewModeSelector
          mode={mode}
          onModeChange={onModeChange}
          hasReadme={!!hasReadme}
          availableModes={availableModes}
        />
      )}
      {children}
    </div>
  )
}
