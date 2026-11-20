import { ViewerStatus } from '@s4wave/web/object/ViewerStatus.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { FrameView } from './FrameView.js'
import type { FrameObjectState } from './frame-object-state.js'
import type { FrameSource } from './play-frames.js'

/** FrameObjectBody shows the frame track or the state that keeps it from opening. */
export function FrameObjectBody({
  view,
  source,
  label,
}: {
  view: FrameObjectState
  source?: FrameSource
  label: string
}) {
  switch (view.body) {
    case 'status':
      return <ViewerStatus {...view.status} />
    case 'offline':
      return (
        <div className="relative flex min-h-0 flex-1 items-center justify-center bg-black">
          <LoadingCard
            className="w-full max-w-sm"
            view={{
              state: 'active',
              title: 'Device offline',
              detail: 'The picture reconnects when the Device returns.',
            }}
          />
        </div>
      )
    case 'picture':
      return (
        <FrameView
          key={view.device?.peerId}
          source={source}
          label={`${view.name || label} picture`}
        />
      )
  }
}
