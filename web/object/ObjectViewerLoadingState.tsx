import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import * as stylex from '@stylexjs/stylex'

import {
  colors as themeColors,
  layout as themeLayout,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: themeColors['--color-background-primary'],
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles = stylex.create({
  rule1: { width: '100%', maxWidth: 'var(--container-sm)' },
})

// ObjectViewerLoadingState renders the shared object-viewer loading surface.
export function ObjectViewerLoadingState({
  title = 'Loading object',
  detail = 'Resolving object type and preparing the viewer.',
}: {
  title?: string
  detail?: string
}) {
  return (
    <div sx={stylexStyles2.rule1}>
      <LoadingCard
        view={{
          state: 'loading',
          title,
          detail,
        }}
        sx={stylexStyles.rule1}
      />
    </div>
  )
}
