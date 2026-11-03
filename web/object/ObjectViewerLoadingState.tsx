import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import * as stylex from '@stylexjs/stylex'

import {
  colors as themeColors,
  layout as themeLayout,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    flex: 1,
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-primary'],
    display: 'flex',
    justifyContent: 'center',
    height: '100%',
    width: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: { maxWidth: 'var(--container-sm)', width: '100%' },
})

// ObjectViewerLoadingState renders the shared object-viewer loading surface.
export function ObjectViewerLoadingState({
  sx,
  title = 'Loading object',
  detail = 'Resolving object type and preparing the viewer.',
}: {
  title?: string
  detail?: string
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div sx={[stylexStyles2.rule1, sx]}>
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
