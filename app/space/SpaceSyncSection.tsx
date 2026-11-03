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

import {
  fonts as themeFonts,
  colors as themeColors,
  motion as themeMotion,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { marginTop: `calc(${themeLayout['--spacing']} * 2)` },
  rule2: {
    marginBottom: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule3: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-warning'],
  },
  rule6: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule7: {
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule8: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
})

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
    <div sx={stylexStyles2.rule1}>
      <h3 sx={stylexStyles2.rule2}>How edits sync</h3>
      <InfoCard>
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
          }
        >
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
            <div sx={stylexStyles2.rule3}>
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
    <div role="radiogroup" aria-label="How edits sync">
      <RadioOption
        sx={nativeChildSpacingNext.s1_5}
        selected={merged}
        onSelect={() => onChoose(SpaceSequencer.SpaceSequencer_MERGE)}
        disabled={disabled || merged}
        icon={<LuGitMerge sx={stylexStyles.rule2} />}
        label="Merge"
        description="Everyone keeps working and edits combine."
      />
      <RadioOption
        sx={nativeChildSpacingNext.s1_5}
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
      <p sx={stylexStyles2.rule4} role="status">
        {switchingStatus(sequencer)}
      </p>
    )
  }
  return (
    <>
      <p sx={stylexStyles2.rule4}>{sequencerStatus(state, sequencer)}</p>
      {!merged && unordered > 0 && (
        <p sx={stylexStyles2.rule5}>
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
      <button type="button" sx={stylexStyles2.rule6} onClick={onToggle}>
        {show ? 'Hide technical details' : 'Show technical details'}
      </button>
      {show && (
        <div>
          <p sx={[nativeChildSpacing.s1_5, stylexStyles2.rule4]}>
            {merged
              ? 'No sequencer is appointed. Each operation becomes stable once every device has acknowledged it.'
              : 'The sequencer signs a position for each operation. Operations with a position are stable.'}
          </p>
          {peerId && (
            <CopyableField
              sx={nativeChildSpacing.s1_5}
              label="Sequencer peer ID"
              value={peerId}
            />
          )}
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
            sx={stylexStyles2.rule7}
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={onConfirm}
            disabled={pending}
            sx={[stylexStyles2.rule8]}
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
