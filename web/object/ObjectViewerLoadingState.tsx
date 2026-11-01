import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import * as stylex from '@stylexjs/stylex'

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
    <div className="bg-background-primary flex h-full w-full flex-1 items-center justify-center p-4">
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
