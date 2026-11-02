import { useCallback, useState } from 'react'
import {
  LuCircleAlert,
  LuGitMerge,
  LuListOrdered,
  LuMonitor,
} from '@s4wave/web/images/icons.js'

import { useWatchStateRpc } from '@aptre/bldr-react'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { SharedObjectHealth } from '@s4wave/core/sobject/sobject.pb.js'
import type { Space } from '@s4wave/sdk/space/space.js'
import {
  SpaceControl,
  SpaceSequencer,
  type SpaceSharingState,
} from '@s4wave/sdk/space/space.pb.js'
import {
  WatchSharedObjectHealthRequest,
  WatchSharedObjectHealthResponse,
} from '@s4wave/sdk/session/session.pb.js'
import { SessionContext, SpaceContext } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useStateAtom } from '@s4wave/web/state/index.js'
import { cn } from '@s4wave/web/style/utils.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { deviceName } from './device-name.js'
import { SyncPeerList } from './SyncPeerList.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// TakeOver is the wording of one way to make this device the main device.
interface TakeOver {
  title: string
  description: string
  confirm: string
}

// MAKE_MAIN asks to move the order to this device while the main device works.
const MAKE_MAIN: TakeOver = {
  title: 'Make this the main device?',
  description:
    'This device will put edits in order from now on. Keep it online so edits become final.',
  confirm: 'Make main device',
}

// MAIN_LOST replaces a main device that is gone for good.
const MAIN_LOST: TakeOver = {
  title: 'Replace the lost main device?',
  description:
    'This device continues from the order it already has. Edits the lost device ordered after that are ordered again here. If the lost device comes back, its late ordering is ignored.',
  confirm: 'Replace main device',
}

// PendingSequencer is a sequencer change the watched sharing state does not
// show yet.
interface PendingSequencer {
  from: SpaceSequencer
  to: SpaceSequencer
}

// SpaceSyncSection shows how the Space's edits sync, and lets owners, or the
// group's voters under group control, choose: Merge, where every device keeps
// working and edits combine, or One order, where Spacewave Cloud or a main
// device puts every edit in order. It lists the other devices this device
// syncs the Space with.
export function SpaceSyncSection() {
  const { spaceId, spaceSharingState } = SpaceContainerContext.useContext()
  const space = useResourceValue(SpaceContext.useContext())
  const health = useSharedObjectHealth(spaceId)
  const [takeOver, setTakeOver] = useState<TakeOver | null>(null)
  const [showDetails, setShowDetails] = useStateAtom<boolean>(
    null,
    'showTechnicalDetails',
    false,
  )

  const current =
    spaceSharingState?.sequencer ?? SpaceSequencer.SpaceSequencer_UNKNOWN
  const closeTakeOver = useCallback(() => setTakeOver(null), [])
  const { sequencer, switching, choose } = useSequencerChoice(
    space,
    current,
    closeTakeOver,
  )

  if (!spaceSharingState || current === SpaceSequencer.SpaceSequencer_UNKNOWN)
    return null

  const choices = spaceSharingState.sequencerChoices ?? []
  // Under group control a voter asks the group to change how edits sync.
  const canManage =
    (spaceSharingState.control === SpaceControl.SpaceControl_GROUP
      ? !!spaceSharingState.canVote
      : !!spaceSharingState.canManage) && !switching
  const merged = sequencer === SpaceSequencer.SpaceSequencer_MERGE
  const canTakeOver =
    canManage &&
    sequencer === SpaceSequencer.SpaceSequencer_OTHER_DEVICE &&
    choices.includes(SpaceSequencer.SpaceSequencer_THIS_DEVICE)

  return (
    <div className="mt-2">
      <h3 className="text-foreground-alt mb-1 text-xs select-none">
        How edits sync
      </h3>
      <InfoCard>
        <div className="space-y-2">
          <SyncChoice
            sequencer={sequencer}
            choices={choices}
            disabled={!canManage}
            onChoose={(next) => void choose(next)}
          />
          <SyncStatus
            state={spaceSharingState}
            sequencer={sequencer}
            switching={switching}
            unordered={health?.unorderedCount ?? 0}
          />
          {canTakeOver && (
            <div className="flex flex-wrap gap-2">
              <DashboardButton
                icon={<LuMonitor sx={stylexStyles.rule1} />}
                onClick={() => setTakeOver(MAKE_MAIN)}
              >
                Make this the main device
              </DashboardButton>
              <DashboardButton
                icon={<LuCircleAlert sx={stylexStyles.rule1} />}
                onClick={() => setTakeOver(MAIN_LOST)}
              >
                The main device is lost
              </DashboardButton>
            </div>
          )}
          {health && (
            <SyncPeerList
              state={spaceSharingState}
              peers={health.syncPeers ?? []}
              showDetails={showDetails}
            />
          )}
          <TechnicalDetails
            merged={merged}
            peerId={spaceSharingState.sequencerPeerId}
            show={showDetails}
            onToggle={() => setShowDetails(!showDetails)}
          />
        </div>
      </InfoCard>
      <TakeOverDialog
        takeOver={takeOver}
        pending={switching}
        onCancel={() => setTakeOver(null)}
        onConfirm={() => void choose(SpaceSequencer.SpaceSequencer_THIS_DEVICE)}
      />
    </div>
  )
}

// useSequencerChoice sends a sequencer change and returns the sequencer to
// show. A choice shows as selected from the moment it is made, and stays
// switching until the watched sharing state moves off the sequencer it
// replaced. A failed change, no change, or one the group must approve first
// ends it early.
function useSequencerChoice(
  space: Space | undefined | null,
  current: SpaceSequencer,
  onSent: () => void,
) {
  const [pending, setPending] = useState<PendingSequencer | null>(null)
  const settled = !pending || pending.from !== current
  if (pending && settled) setPending(null)

  const choose = useCallback(
    async (next: SpaceSequencer) => {
      if (!space) return
      const choice = { from: current, to: next }
      const end = () => setPending((cur) => (cur === choice ? null : cur))
      setPending(choice)
      try {
        const resp = await space.setSpaceSequencer(next)
        onSent()
        if (!resp.changed || resp.awaitingGroup) end()
        if (resp.awaitingGroup) {
          toast.info('Asked the group', {
            description: 'Edits sync the new way once enough members agree.',
          })
        }
      } catch (err) {
        end()
        toast.error('Could not change how edits sync', {
          description: err instanceof Error ? err.message : String(err),
        })
      }
    },
    [space, current, onSent],
  )

  return {
    sequencer: settled ? current : pending.to,
    switching: !settled,
    choose,
  }
}

// useSharedObjectHealth watches the shared object's health, or returns
// undefined until the first value arrives.
function useSharedObjectHealth(
  sharedObjectId: string,
): SharedObjectHealth | undefined {
  const session = useResourceValue(SessionContext.useContext())
  const resp = useWatchStateRpc(
    useCallback(
      (req: WatchSharedObjectHealthRequest, signal: AbortSignal) =>
        session?.watchSharedObjectHealth(req, signal) ?? null,
      [session],
    ),
    { sharedObjectId },
    WatchSharedObjectHealthRequest.equals,
    WatchSharedObjectHealthResponse.equals,
  )
  return resp?.health
}

// SyncChoice is the Merge or One order radio pair. One order appoints the
// provider or this device, whichever the choices offer.
function SyncChoice({
  sequencer,
  choices,
  disabled,
  onChoose,
}: {
  sequencer: SpaceSequencer
  choices: SpaceSequencer[]
  disabled: boolean
  onChoose: (sequencer: SpaceSequencer) => void
}) {
  const merged = sequencer === SpaceSequencer.SpaceSequencer_MERGE
  const oneOrder = choices.find(
    (c) =>
      c === SpaceSequencer.SpaceSequencer_PROVIDER ||
      c === SpaceSequencer.SpaceSequencer_THIS_DEVICE,
  )
  return (
    <div className="space-y-1.5" role="radiogroup" aria-label="How edits sync">
      <RadioOption
        selected={merged}
        onSelect={() => onChoose(SpaceSequencer.SpaceSequencer_MERGE)}
        disabled={disabled || merged}
        icon={<LuGitMerge sx={stylexStyles.rule2} />}
        label="Merge"
        description="Everyone keeps working and edits combine."
      />
      <RadioOption
        selected={!merged}
        onSelect={() => oneOrder && onChoose(oneOrder)}
        disabled={disabled || !merged || !oneOrder}
        icon={<LuListOrdered sx={stylexStyles.rule2} />}
        label="One order"
        description={oneOrderDescription(merged ? oneOrder : sequencer)}
      />
    </div>
  )
}

// SyncStatus says whose order the Space follows and how many edits wait for
// it, or which way the Space is switching to while a change is pending.
function SyncStatus({
  state,
  sequencer,
  switching,
  unordered,
}: {
  state: SpaceSharingState
  sequencer: SpaceSequencer
  switching: boolean
  unordered: number
}) {
  const merged = sequencer === SpaceSequencer.SpaceSequencer_MERGE
  if (switching) {
    return (
      <p className="text-foreground-alt text-xs" role="status">
        {switchingStatus(sequencer)}
      </p>
    )
  }
  return (
    <>
      <p className="text-foreground-alt text-xs">
        {sequencerStatus(state, sequencer)}
      </p>
      {!merged && unordered > 0 && (
        <p className="text-warning text-xs">
          {unordered === 1
            ? '1 edit is waiting to be put in order.'
            : `${unordered} edits are waiting to be put in order.`}
        </p>
      )}
    </>
  )
}

// TechnicalDetails shows the sequencer terms behind the plain wording, behind
// the global technical details toggle.
function TechnicalDetails({
  merged,
  peerId,
  show,
  onToggle,
}: {
  merged: boolean
  peerId?: string
  show: boolean
  onToggle: () => void
}) {
  return (
    <>
      <button
        type="button"
        className="text-foreground-alt hover:text-foreground text-xs transition-colors"
        onClick={onToggle}
      >
        {show ? 'Hide technical details' : 'Show technical details'}
      </button>
      {show && (
        <div className="space-y-1.5">
          <p className="text-foreground-alt text-xs">
            {merged
              ? 'No sequencer is appointed. Each operation becomes stable once every device has acknowledged it.'
              : 'The sequencer signs a position for each operation. Operations with a position are stable.'}
          </p>
          {peerId && <CopyableField label="Sequencer peer ID" value={peerId} />}
        </div>
      )}
    </>
  )
}

// TakeOverDialog confirms making this device the main device.
function TakeOverDialog({
  takeOver,
  pending,
  onCancel,
  onConfirm,
}: {
  takeOver: TakeOver | null
  pending: boolean
  onCancel: () => void
  onConfirm: () => void
}) {
  return (
    <Dialog open={!!takeOver} onOpenChange={(open) => !open && onCancel()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{takeOver?.title}</DialogTitle>
          <DialogDescription>{takeOver?.description}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <button
            type="button"
            onClick={onCancel}
            disabled={pending}
            className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={onConfirm}
            disabled={pending}
            className={cn(
              'rounded-md border px-4 py-2 text-sm transition-all',
              'border-brand/30 bg-brand/10 hover:bg-brand/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            {takeOver?.confirm}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// oneOrderDescription says who puts the edits in order under One order.
function oneOrderDescription(who: SpaceSequencer | undefined): string {
  switch (who) {
    case SpaceSequencer.SpaceSequencer_PROVIDER:
      return 'Edits take turns through Spacewave Cloud.'
    case SpaceSequencer.SpaceSequencer_THIS_DEVICE:
      return 'Edits take turns through this device.'
    case SpaceSequencer.SpaceSequencer_OTHER_DEVICE:
      return 'Edits take turns through the main device.'
    default:
      return 'Edits take turns through one place.'
  }
}

// sequencerStatus says whose order the Space follows now.
function sequencerStatus(
  state: SpaceSharingState,
  sequencer: SpaceSequencer,
): string {
  switch (sequencer) {
    case SpaceSequencer.SpaceSequencer_PROVIDER:
      return 'Spacewave Cloud puts every edit in order.'
    case SpaceSequencer.SpaceSequencer_THIS_DEVICE:
      return 'This device puts every edit in order. Keep it online so edits become final.'
    case SpaceSequencer.SpaceSequencer_OTHER_DEVICE:
      return `${deviceName(state, state.sequencerPeerId ?? '', 'The main device')} puts every edit in order.`
    default:
      return 'Edits become final once every device has them.'
  }
}

// switchingStatus says which way the Space is switching to.
function switchingStatus(to: SpaceSequencer): string {
  switch (to) {
    case SpaceSequencer.SpaceSequencer_MERGE:
      return 'Switching to Merge…'
    case SpaceSequencer.SpaceSequencer_THIS_DEVICE:
      return 'Making this the main device…'
    default:
      return 'Switching to One order…'
  }
}
