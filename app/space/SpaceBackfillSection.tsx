import { useCallback, useState } from 'react'
import { LuHardDriveDownload, LuMousePointerClick } from 'react-icons/lu'

import { useWatchStateRpc } from '@aptre/bldr-react'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  SpaceBackfillState,
  WatchSpaceBackfillRequest,
} from '@s4wave/sdk/space/space.pb.js'
import { SpaceContext } from '@s4wave/web/contexts/contexts.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'
import { toast } from '@s4wave/web/ui/toaster.js'

// SpaceBackfillSection lets this device choose between downloading the Space
// on demand and keeping a complete copy. The choice applies to this device
// only.
export function SpaceBackfillSection() {
  const space = useResourceValue(SpaceContext.useContext())
  const [pending, setPending] = useState(false)
  const state = useWatchStateRpc(
    useCallback(
      (req: WatchSpaceBackfillRequest, signal: AbortSignal) =>
        space?.watchSpaceBackfill(req, signal) ?? null,
      [space],
    ),
    {},
    WatchSpaceBackfillRequest.equals,
    SpaceBackfillState.equals,
  )

  const choose = useCallback(
    async (backfill: boolean) => {
      if (!space) return
      setPending(true)
      try {
        await space.setSpaceBackfill(backfill)
      } catch (err) {
        toast.error('Could not change storage on this device', {
          description: err instanceof Error ? err.message : String(err),
        })
      } finally {
        setPending(false)
      }
    },
    [space],
  )

  if (!state) return null
  const backfill = !!state.backfill

  return (
    <div className="mt-2">
      <h3 className="text-foreground-alt mb-1 text-xs select-none">
        Storage on this device
      </h3>
      <InfoCard>
        <div
          className="space-y-1.5"
          role="radiogroup"
          aria-label="Storage on this device"
        >
          <RadioOption
            selected={!backfill}
            onSelect={() => backfill && void choose(false)}
            disabled={pending}
            icon={<LuMousePointerClick className="size-3.5" />}
            label="On demand"
            description="Download items when you open them, and keep what you open or edit."
          />
          <RadioOption
            selected={backfill}
            onSelect={() => !backfill && void choose(true)}
            disabled={pending}
            icon={<LuHardDriveDownload className="size-3.5" />}
            label="Whole Space"
            description="Download everything in the background, so the whole Space is available offline."
          />
        </div>
      </InfoCard>
    </div>
  )
}
