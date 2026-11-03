import { useCallback, useState } from 'react'
import { LuCheck, LuCrown, LuUsers } from '@s4wave/web/images/icons.js'

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

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  motion as themeMotion,
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
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule4: {
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
  rule5: {
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
  rule6: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule7: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule8: { minWidth: 0, flex: 1 },
  rule9: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule10: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule11: {
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
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

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
    <div sx={stylexStyles2.rule1}>
      <h3 sx={stylexStyles2.rule2}>Who controls the Space</h3>
      <InfoCard>
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-2 */ 'space-y-2'
          }
        >
          <div role="radiogroup" aria-label="Who controls the Space">
            <RadioOption
              sx={nativeChildSpacingNextNext.s1_5}
              selected={!group}
              onSelect={() => void choose(SpaceControl.SpaceControl_OWNER)}
              disabled={!canSet || !group}
              icon={<LuCrown sx={stylexStyles.rule1} />}
              label="The owner"
              description="The owner changes members and settings."
            />
            <RadioOption
              sx={nativeChildSpacingNextNext.s1_5}
              selected={group}
              onSelect={() => setConfirmGroup(true)}
              disabled={!canSet || group}
              icon={<LuUsers sx={stylexStyles.rule1} />}
              label="The group"
              description="No one changes members or settings alone, including the owner."
            />
          </div>
          <p sx={stylexStyles2.rule3}>{controlStatus(spaceSharingState)}</p>
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
              sx={stylexStyles2.rule4}
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => void choose(SpaceControl.SpaceControl_GROUP)}
              disabled={pending}
              sx={[stylexStyles2.rule5]}
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
  sx,
  state,
  changes,
  canAgree,
  onAgree,
}: {
  state: SpaceSharingState
  changes: SpaceGroupChange[]
  canAgree: boolean
  onAgree: (hash: Uint8Array) => void
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const quorum = state.quorumWeight ?? 0n
  return (
    <div sx={sx}>
      <h4 sx={[nativeChildSpacingNext.s1_5, stylexStyles2.rule6]}>
        Waiting for the group
      </h4>
      <ul sx={nativeChildSpacingNext.s1_5}>
        {changes.map((change) => (
          <li
            key={hashKey(change.hash)}
            sx={[nativeChildSpacing.s1_5, stylexStyles2.rule7]}
          >
            <div sx={stylexStyles2.rule8}>
              <p sx={stylexStyles2.rule9}>{describeChange(state, change)}</p>
              <p sx={stylexStyles2.rule3}>
                {`${change.weight ?? 0n} of ${quorum} votes`}
              </p>
            </div>
            {change.viewerAgreed ? (
              <span sx={stylexStyles2.rule10}>
                <LuCheck sx={stylexStyles.rule2} />
                You agreed
              </span>
            ) : (
              canAgree &&
              change.hash && (
                <DashboardButton
                  icon={<LuCheck sx={stylexStyles.rule2} />}
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
        sx={stylexStyles2.rule11}
        onClick={() => setShow(!show)}
      >
        {show ? 'Hide technical details' : 'Show technical details'}
      </button>
      {show && (
        <p sx={stylexStyles2.rule3}>
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNextNext = stylex.create({
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
