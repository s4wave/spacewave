import type { ReactNode } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    alignItems: 'center',
    color: themeColors['--color-muted-foreground'],
    display: 'flex',
    justifyContent: 'center',
    height: '100%',
  },
  rule2: {
    alignItems: 'center',
    color: themeColors['--color-destructive'],
    display: 'flex',
    justifyContent: 'center',
    height: '100%',
  },
  rule3: {
    alignItems: 'center',
    color: themeColors['--color-muted-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: '100%',
  },
})

// ViewerStatusShellProps configures the loading, error, and empty state messages.
interface ViewerStatusShellProps {
  resource: Resource<unknown>
  state: Resource<unknown>
  loadingText: string
  emptyText?: string
  sources?: unknown[]
  children: ReactNode
}

// ViewerStatusShell renders loading, error, and optionally empty states
// with consistent styling. Renders children when the resource is ready.
export function ViewerStatusShell({
  sx,
  resource,
  state,
  loadingText,
  emptyText,
  sources,
  children,
}: ViewerStatusShellProps & { sx?: stylex.StyleXStyles; className?: never }) {
  if (resource.loading || state.loading) {
    return <div sx={[stylexStyles.rule1, sx]}>{loadingText}</div>
  }

  if (resource.error) {
    return <div sx={[stylexStyles.rule2, sx]}>{resource.error.message}</div>
  }

  if (emptyText && sources !== undefined && sources.length === 0) {
    return <div sx={[stylexStyles.rule3, sx]}>{emptyText}</div>
  }

  return <>{children}</>
}
