import { useCallback, useState } from 'react'
import { LuCheck, LuCrown, LuUsers } from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SOConfigChangeType } from '@s4wave/core/sobject/sobject.pb.js'
import {
  SpaceControl,
  SpaceSequencer,
  type SpaceGroupChange,
  type SpaceSharingState,
} from '@s4wave/sdk/space/space.pb.js'
import { SpaceContext } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useStateAtom } from '@s4wave/web/state/index.js'
import { cn } from '@s4wave/web/style/utils.js'
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

// SpaceControlSection shows who controls the Space, the owner or the group,
// and the changes waiting for the group to agree. An owner hands control to
// the group; under group control each voter asks for changes and agrees to
// the changes others asked for.
export function SpaceControlSection() {
  const { spaceSharingState } = SpaceContainerContext.useContext()
  const space = useResourceValue(SpaceContext.useContext())
  const [pending, setPending] = useState(false)
  const [confirmGroup, setConfirmGroup] = useState(false)

  const run = useCallback(
    async (failure: string, action: () => Promise<boolean | void>) => {
      setPending(true)
      try {
        if (await action()) {
          toast.info('Asked the group', {
            description: 'The change happens once enough members agree.',
          })
        }
      } catch (err) {
        toast.error(failure, {
          description: err instanceof Error ? err.message : String(err),
        })
      } finally {
        setPending(false)
      }
    },
    [],
  )

  const choose = useCallback(
    (control: SpaceControl) =>
      run('Could not change who controls the Space', async () => {
        if (!space) return
        const resp = await space.setSpaceControl(control)
        setConfirmGroup(false)
        return !!resp.awaitingGroup
      }),
    [run, space],
  )

  const agree = useCallback(
    (hash: Uint8Array) =>
      run('Could not agree to the change', async () => {
        await space?.approveSpaceChange(hash)
      }),
    [run, space],
  )

  const control =
    spaceSharingState?.control ?? SpaceControl.SpaceControl_UNKNOWN
  if (!spaceSharingState || control === SpaceControl.SpaceControl_UNKNOWN)
    return null

  const group = control === SpaceControl.SpaceControl_GROUP
  const canSet = !!spaceSharingState.canSetControl && !pending
  const changes = spaceSharingState.groupChanges ?? []

  return (
    <div className="mt-2">
      <h3 className="text-foreground-alt mb-1 text-xs select-none">
        Who controls the Space
      </h3>
      <InfoCard>
        <div className="space-y-2">
          <div
            className="space-y-1.5"
            role="radiogroup"
            aria-label="Who controls the Space"
          >
            <RadioOption
              selected={!group}
              onSelect={() => void choose(SpaceControl.SpaceControl_OWNER)}
              disabled={!canSet || !group}
              icon={<LuCrown className="size-3.5" />}
              label="The owner"
              description="The owner changes members and settings."
            />
            <RadioOption
              selected={group}
              onSelect={() => setConfirmGroup(true)}
              disabled={!canSet || group}
              icon={<LuUsers className="size-3.5" />}
              label="The group"
              description="No one changes members or settings alone, including the owner."
            />
          </div>
          <p className="text-foreground-alt text-xs">
            {controlStatus(spaceSharingState)}
          </p>
          {group && changes.length !== 0 && (
            <GroupChanges
              state={spaceSharingState}
              changes={changes}
              canAgree={!!spaceSharingState.canVote && !pending}
              onAgree={(hash) => void agree(hash)}
            />
          )}
          <TechnicalDetails state={spaceSharingState} />
        </div>
      </InfoCard>
      <Dialog
        open={confirmGroup}
        onOpenChange={(open) => !open && setConfirmGroup(false)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Hand control to the group?</DialogTitle>
            <DialogDescription>
              From now on no one, including you, changes members or settings
              alone. Each change needs more than two thirds of the members to
              agree, and so does giving control back to the owner.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <button
              type="button"
              onClick={() => setConfirmGroup(false)}
              disabled={pending}
              className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => void choose(SpaceControl.SpaceControl_GROUP)}
              disabled={pending}
              className={cn(
                'rounded-md border px-4 py-2 text-sm transition-all',
                'border-brand/30 bg-brand/10 hover:bg-brand/20',
                'disabled:cursor-not-allowed disabled:opacity-50',
              )}
            >
              Hand control to the group
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

// GroupChanges lists the changes waiting for the group, with how many votes
// each has and a way for the viewer to agree.
function GroupChanges({
  state,
  changes,
  canAgree,
  onAgree,
}: {
  state: SpaceSharingState
  changes: SpaceGroupChange[]
  canAgree: boolean
  onAgree: (hash: Uint8Array) => void
}) {
  const quorum = state.quorumWeight ?? 0n
  return (
    <div className="space-y-1.5">
      <h4 className="text-foreground text-xs font-medium select-none">
        Waiting for the group
      </h4>
      <ul className="space-y-1.5">
        {changes.map((change) => (
          <li
            key={hashKey(change.hash)}
            className="border-foreground/10 flex items-center gap-2 rounded-md border p-2"
          >
            <div className="min-w-0 flex-1">
              <p className="text-foreground truncate text-xs">
                {describeChange(state, change)}
              </p>
              <p className="text-foreground-alt text-xs">
                {`${change.weight ?? 0n} of ${quorum} votes`}
              </p>
            </div>
            {change.viewerAgreed ? (
              <span className="text-foreground-alt flex items-center gap-1 text-xs">
                <LuCheck className="size-3" />
                You agreed
              </span>
            ) : (
              canAgree &&
              change.hash && (
                <DashboardButton
                  icon={<LuCheck className="size-3" />}
                  onClick={() => change.hash && onAgree(change.hash)}
                >
                  Agree
                </DashboardButton>
              )
            )}
          </li>
        ))}
      </ul>
    </div>
  )
}

// TechnicalDetails shows the voting terms behind the plain wording, behind the
// global technical details toggle.
function TechnicalDetails({ state }: { state: SpaceSharingState }) {
  const [show, setShow] = useStateAtom<boolean>(
    null,
    'showTechnicalDetails',
    false,
  )
  const group = state.control === SpaceControl.SpaceControl_GROUP
  return (
    <>
      <button
        type="button"
        className="text-foreground-alt hover:text-foreground text-xs transition-colors"
        onClick={() => setShow(!show)}
      >
        {show ? 'Hide technical details' : 'Show technical details'}
      </button>
      {show && (
        <p className="text-foreground-alt text-xs">
          {group
            ? `Group control: voters decide each control record and checkpoint in rounds. A decision needs precommits from more than two thirds of the voting weight, ${state.quorumWeight ?? 0n} of ${state.totalWeight ?? 0n}. Each member's first writing device votes.`
            : 'Owner control: any owner device signs control records and checkpoints alone.'}
        </p>
      )}
    </>
  )
}

// controlStatus says what a change needs now.
function controlStatus(state: SpaceSharingState): string {
  if (state.control !== SpaceControl.SpaceControl_GROUP)
    return 'An owner can change members and settings at any time.'
  const quorum = state.quorumWeight ?? 0n
  const total = state.totalWeight ?? 0n
  const needs =
    quorum === total
      ? 'Every change needs every member to agree.'
      : `Every change needs ${quorum} of ${total} votes.`
  return state.canVote ? `${needs} You have a vote.` : needs
}

// describeChange says in everyday words what a waiting change does.
function describeChange(
  state: SpaceSharingState,
  change: SpaceGroupChange,
): string {
  switch (change.changeType) {
    case SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT:
      return `Add ${names(state, change.addedPeerIds, 'a new member')}`
    case SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT:
      return `Remove ${names(state, change.removedPeerIds, 'a member')}`
    case SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_SET_CONTROL:
      return change.control === SpaceControl.SpaceControl_OWNER
        ? 'Give control back to the owner'
        : 'Change who votes'
    case SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_SET_SEQUENCER:
      return sequencerChange(change.sequencer)
    case SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_TRANSFER_OWNERSHIP:
      return 'Change the owner'
    case SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_ADD_INVITE:
      return 'Create an invite'
    case SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_REVOKE_INVITE:
      return 'Cancel an invite'
    case SOConfigChangeType.SO_CONFIG_CHANGE_TYPE_SET_ROSTER:
      return 'Change which devices the Space waits for'
    default:
      return 'Change the Space settings'
  }
}

// sequencerChange says how edits sync after a waiting change.
function sequencerChange(sequencer: SpaceSequencer | undefined): string {
  switch (sequencer) {
    case SpaceSequencer.SpaceSequencer_MERGE:
      return 'Merge edits'
    case SpaceSequencer.SpaceSequencer_PROVIDER:
      return 'Put edits in order through Spacewave Cloud'
    case SpaceSequencer.SpaceSequencer_THIS_DEVICE:
      return 'Make this device the main device'
    default:
      return 'Change the main device'
  }
}

// names lists the members whose devices are peerIds, each once, or fallback
// when none is a member.
function names(
  state: SpaceSharingState,
  peerIds: string[] | undefined,
  fallback: string,
): string {
  const found = new Set<string>()
  for (const peerId of peerIds ?? []) {
    const info = state.participantInfo?.find((p) => p.peerIds?.includes(peerId))
    if (!info) continue
    found.add(
      info.isSelf ? 'you' : info.entityId || info.accountId || 'a member',
    )
  }
  if (found.size === 0) return fallback
  return [...found].join(', ')
}

// hashKey renders a change hash as a stable React key.
function hashKey(hash: Uint8Array | undefined): string {
  return Array.from(hash ?? [], (b) => b.toString(16).padStart(2, '0')).join('')
}
