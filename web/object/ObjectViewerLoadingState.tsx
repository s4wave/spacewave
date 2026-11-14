import { ViewerStatus } from './ViewerStatus.js'

// ObjectViewerLoadingState fills a viewer's place while the object, its type
// or the plugin that views it loads. what names what is read, such as "the
// object". It brings its own pane container, since it may stand before any
// viewer pane exists.
export function ObjectViewerLoadingState({
  what = 'the object',
}: {
  what?: string
}) {
  return (
    <div className="bg-background-primary viewer-pane flex h-full w-full flex-1 flex-col">
      <ViewerStatus state="waiting" what={what} />
    </div>
  )
}
