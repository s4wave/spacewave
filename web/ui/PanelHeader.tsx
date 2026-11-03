import { useState, type ComponentPropsWithRef, type ReactNode } from 'react'
import * as stylex from '@stylexjs/stylex'
import { useObjectViewer } from '@s4wave/web/object/ObjectViewerContext.js'
import { ComponentSelector } from '@s4wave/web/object/ComponentSelector.js'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  colors as themeColors,
  motion as themeMotion,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'
const styles = stylex.create({
  header: {
    borderColor: themeColors['--color-ui-outline'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    fontSize: themeLayout['--text-ui'],
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: 'var(--panel-header-height)',
  },
  touchSelector: {
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
  defaultHeader: { backgroundColor: themeColors['--color-panel-header'] },
  secondaryHeader: { backgroundColor: themeColors['--color-outliner-header'] },
  transparentHeader: { backgroundColor: '#0000' },
  compactHeader: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: themeColors['--color-panel-header'],
  },
  button: {
    '--focus-outline-color': themeColors['--color-brand'],
    '--focus-outline-width': '2px',
    '--tw-outline-style': 'none',
    padding: `calc(${themeLayout['--spacing']} * .5)`,
    borderRadius: '.25rem',
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-pulldown-hover'],
      },
    },
    display: 'flex',
    justifyContent: 'center',
    outlineColor: {
      default: null,
      '@media (forced-colors: active)': 'transparent',
    },
    outlineOffset: {
      default: null,
      ':focus-visible': '2px',
      '@media (forced-colors: active)': '2px',
    },
    outlineStyle: {
      default: 'none',
      ':focus-visible': 'solid',
      '@media (forced-colors: active)': 'solid',
    },
    outlineWidth: {
      default: null,
      '@media (forced-colors: active)': 'var(--focus-outline-width)',
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  viewerSelector: {
    overflow: 'hidden',
    color: themeColors['--color-muted-foreground'],
    cursor: 'pointer',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  viewerName: {
    overflow: 'hidden',
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
})

/** PanelHeaderVariant selects the header's background and density. */
export type PanelHeaderVariant =
  | 'default'
  | 'secondary'
  | 'transparent'
  | 'compact'

interface PanelHeaderProps extends Omit<
  ComponentPropsWithRef<'div'>,
  'className'
> {
  sx?: stylex.StyleXStyles
  className?: never
  children?: ReactNode
  height?: number
  variant?: PanelHeaderVariant
}

const variantStyles: Record<PanelHeaderVariant, stylex.StyleXStyles> = {
  default: styles.defaultHeader,
  secondary: styles.secondaryHeader,
  transparent: styles.transparentHeader,
  compact: styles.compactHeader,
}

/** PanelHeader renders a panel header with an optional object viewer selector. */
export function PanelHeader({
  children,
  sx,
  height = 25,
  style: _style,
  variant = 'default',
  ...props
}: PanelHeaderProps) {
  // Choose the component selector or static viewer name from the current viewer.
  const [selectorOpen, setSelectorOpen] = useState(false)
  const viewer = useObjectViewer()
  const showSelector = viewer && viewer.visibleComponents.length > 1
  const showStaticName = viewer && viewer.visibleComponents.length === 1

  return (
    <div
      {...props}
      data-drag-handle=""
      {...stylex.props(
        styles.header,
        showSelector && styles.touchSelector,
        variantStyles[variant],
        sx,
      )}
      style={{ '--panel-header-height': `${height}px` }}
    >
      {showSelector && viewer.selectedComponent && (
        <ComponentSelector
          open={selectorOpen}
          placement="below"
          onOpenChange={setSelectorOpen}
          components={viewer.visibleComponents}
          selectedComponent={viewer.selectedComponent}
          onSelectComponent={viewer.onSelectComponent}
        >
          <div sx={styles.viewerSelector}>{viewer.selectedComponent.name}</div>
        </ComponentSelector>
      )}
      {showStaticName && viewer.visibleComponents[0] && (
        <div sx={styles.viewerName}>{viewer.visibleComponents[0].name}</div>
      )}
      {children}
    </div>
  )
}

interface PanelHeaderButtonProps {
  children: ReactNode
  onClick?: () => void
  sx?: stylex.StyleXStyles
  className?: never
  title?: string
}

// PanelHeaderButton renders a consistent icon button for panel headers.
export function PanelHeaderButton({
  children,
  onClick,
  sx,
  title,
}: PanelHeaderButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={title}
      sx={[styles.button, sx]}
    >
      {children}
    </button>
  )
}
