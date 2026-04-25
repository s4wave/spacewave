import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

// ObjectViewerLoadingState renders the shared object-viewer loading surface.
export function ObjectViewerLoadingState() {
  return (
    <div className="bg-background-primary flex h-full w-full flex-1 items-center justify-center p-4">
      <LoadingCard
        view={{
          state: 'loading',
          title: 'Loading object',
          detail: 'Resolving object type and preparing the viewer.',
        }}
        className="w-full max-w-sm"
      />
    </div>
  )
}
