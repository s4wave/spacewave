import { ViewerStatus } from './ViewerStatus.js'

export interface ObjectViewerNotFoundStateProps {
  objectKey?: string
}

// ObjectViewerNotFoundState fills a viewer's place when its object is gone.
// Reading again cannot bring it back, so it offers no Retry.
export function ObjectViewerNotFoundState({
  objectKey,
}: ObjectViewerNotFoundStateProps) {
  return (
    <div className="bg-background-primary viewer-pane flex h-full w-full flex-1 flex-col">
      <ViewerStatus
        state="failed"
        message={`Object not found: ${objectKey || 'the selected object'} is no longer available.`}
      />
    </div>
  )
}
