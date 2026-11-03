import type { DragEvent, MouseEvent, ReactNode } from 'react'
import { LuUpload } from '@s4wave/web/images/icons.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    flex: 1,
    overflow: 'hidden',
    backgroundColor: themeColors['--color-file-back'],
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    minHeight: 0,
  },
  rule2: {
    '--tw-backdrop-blur': 'blur(8px)',
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    '--tw-translate-x': 'calc(calc(1 / 2 * 100%) * -1)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 85%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    pointerEvents: 'none',
    position: 'absolute',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    zIndex: 10,
    bottom: `calc(${themeLayout['--spacing']} * 3)`,
    left: '50%',
  },
  rule3: {
    '--tw-border-style': 'dashed',
    inset: 0,
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: 'dashed',
    borderWidth: '2px',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    pointerEvents: 'none',
    position: 'absolute',
    zIndex: 10,
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
  },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
})

interface UnixFSBrowserDropSurfaceProps {
  children: ReactNode
  isDragging: boolean
  floatingDiagnostics?: ReactNode
  onContextMenu: (event: MouseEvent<HTMLDivElement>) => void
  onDragOver: (event: DragEvent<HTMLDivElement>) => void
  onDragLeave: (event: DragEvent<HTMLDivElement>) => void
  onDrop: (event: DragEvent<HTMLDivElement>) => void
}

export function UnixFSBrowserDropSurface({
  children,
  isDragging,
  floatingDiagnostics,
  onContextMenu,
  onDragOver,
  onDragLeave,
  onDrop,
}: UnixFSBrowserDropSurfaceProps) {
  return (
    <div
      data-testid="unixfs-upload-drop-target"
      sx={stylexStyles2.rule1}
      onContextMenu={onContextMenu}
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
      onDrop={onDrop}
    >
      {children}
      {floatingDiagnostics && (
        <div sx={stylexStyles2.rule2}>{floatingDiagnostics}</div>
      )}
      {isDragging && (
        <div sx={stylexStyles2.rule3}>
          <div sx={stylexStyles2.rule4}>
            <LuUpload sx={stylexStyles.rule1} />
            <span sx={stylexStyles2.rule5}>Drop files to upload</span>
          </div>
        </div>
      )}
    </div>
  )
}
