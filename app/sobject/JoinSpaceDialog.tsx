import { useCallback, useReducer } from 'react'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import { SOInviteMessage } from '@s4wave/core/sobject/sobject.pb.js'
import { JoinSpaceViaInviteResult } from '@s4wave/sdk/session/session.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

import { base58Decode } from '@s4wave/app/provider/spacewave/keypair-utils.js'

export interface JoinSpaceDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialCode?: string
}

type JoinPhase =
  | 'input'
  | 'resolving'
  | 'connecting'
  | 'pending'
  | 'owner_online_required'
  | 'rejected'
  | 'enrolled'
  | 'error'

interface JoinState {
  code: string
  phase: JoinPhase
  error: string | undefined
  spaceId: string | undefined
}

type JoinAction =
  | { type: 'reset' }
  | { type: 'set_code'; code: string }
  | { type: 'resolving' }
  | { type: 'connecting' }
  | { type: 'pending' }
  | { type: 'owner_online_required' }
  | { type: 'rejected' }
  | { type: 'enrolled'; spaceId: string }
  | { type: 'error'; message: string }

const initialState: JoinState = {
  code: '',
  phase: 'input',
  error: undefined,
  spaceId: undefined,
}

function reducer(state: JoinState, action: JoinAction): JoinState {
  switch (action.type) {
    case 'reset':
      return initialState
    case 'set_code':
      return { ...state, code: action.code, error: undefined }
    case 'resolving':
      return { ...state, phase: 'resolving', error: undefined }
    case 'connecting':
      return { ...state, phase: 'connecting' }
    case 'pending':
      return { ...state, phase: 'pending' }
    case 'owner_online_required':
      return { ...state, phase: 'owner_online_required' }
    case 'rejected':
      return { ...state, phase: 'rejected' }
    case 'enrolled':
      return { ...state, phase: 'enrolled', spaceId: action.spaceId }
    case 'error':
      return { ...state, phase: 'error', error: action.message }
  }
}

const phaseLabels: Record<JoinPhase, string> = {
  input: '',
  resolving: 'Looking up invite...',
  connecting: 'Submitting invite...',
  pending: '',
  owner_online_required: '',
  rejected: '',
  enrolled: 'Joined successfully!',
  error: '',
}

// JoinSpaceDialog allows a user to join a shared space via invite code or link.
export function JoinSpaceDialog({
  open,
  onOpenChange,
  initialCode,
}: JoinSpaceDialogProps) {
  const session = useResourceValue(SessionContext.useContext())
  const { isCloud } = useSessionInfo(session)
  const [state, dispatch] = useReducer(reducer, {
    ...initialState,
    code: initialCode ?? '',
  })

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) dispatch({ type: 'reset' })
      onOpenChange(next)
    },
    [onOpenChange],
  )

  const handleSubmit = useCallback(async () => {
    if (!session || !state.code.trim()) return
    const input = state.code.trim()

    dispatch({ type: 'resolving' })
    try {
      const inviteMsg = await resolveInvite(session, input, isCloud)
      dispatch({ type: 'connecting' })
      const resp = await session.joinSpaceViaInvite(inviteMsg)
      switch (
        resp.result ??
        JoinSpaceViaInviteResult.JoinSpaceViaInviteResult_UNSPECIFIED
      ) {
        case JoinSpaceViaInviteResult.JoinSpaceViaInviteResult_ACCEPTED:
          dispatch({ type: 'enrolled', spaceId: resp.sharedObjectId ?? '' })
          return
        case JoinSpaceViaInviteResult.JoinSpaceViaInviteResult_PENDING_OWNER_APPROVAL:
          dispatch({ type: 'pending' })
          return
        case JoinSpaceViaInviteResult.JoinSpaceViaInviteResult_OWNER_MUST_BE_ONLINE:
          dispatch({ type: 'owner_online_required' })
          return
        case JoinSpaceViaInviteResult.JoinSpaceViaInviteResult_REJECTED:
          dispatch({ type: 'rejected' })
          return
        default:
          throw new Error('Invite join returned an unknown result')
      }
    } catch (err) {
      dispatch({
        type: 'error',
        message: err instanceof Error ? err.message : 'Failed to join space',
      })
    }
  }, [session, state.code, isCloud])

  const busy = state.phase === 'resolving' || state.phase === 'connecting'

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Join Space</DialogTitle>
          <DialogDescription>
            {isCloud ?
              'Enter an invite code or paste an invite link.'
            : 'Paste an invite link to join a space.'}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3 pt-2">
          <input
            value={state.code}
            onChange={(e) =>
              dispatch({ type: 'set_code', code: e.target.value })
            }
            placeholder={isCloud ? 'Invite code or link' : 'Invite link'}
            disabled={busy || state.phase === 'enrolled'}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !busy) void handleSubmit()
            }}
            className={cn(
              'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 font-mono text-sm transition-colors outline-none',
              'focus:border-foreground/40',
              'disabled:opacity-50',
            )}
          />

          {state.phase === 'enrolled' ?
            <div className="text-center">
              <p className="text-foreground text-sm font-medium">
                Joined successfully!
              </p>
              <p className="text-foreground-alt/60 mt-1 text-xs">
                The space will appear in your sidebar.
              </p>
              <button
                onClick={() => handleOpenChange(false)}
                className={cn(
                  'mt-3 flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm transition-all',
                  'border-foreground/20 hover:border-foreground/40 hover:bg-foreground/5',
                )}
              >
                Close
              </button>
            </div>
          : state.phase === 'pending' ?
            <div className="text-center">
              <p className="text-foreground text-sm font-medium">
                Awaiting owner approval
              </p>
              <p className="text-foreground-alt/60 mt-1 text-xs">
                The owner still needs to process this invite before the space
                appears in your sidebar.
              </p>
              <button
                onClick={() => handleOpenChange(false)}
                className={cn(
                  'mt-3 flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm transition-all',
                  'border-foreground/20 hover:border-foreground/40 hover:bg-foreground/5',
                )}
              >
                Close
              </button>
            </div>
          : state.phase === 'owner_online_required' ?
            <div className="text-center">
              <p className="text-foreground text-sm font-medium">
                Owner must be online
              </p>
              <p className="text-foreground-alt/60 mt-1 text-xs">
                This local-first join path completes directly through the space
                owner. Ask the owner to open the space, then try this invite
                link again.
              </p>
              <button
                onClick={() => handleOpenChange(false)}
                className={cn(
                  'mt-3 flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm transition-all',
                  'border-foreground/20 hover:border-foreground/40 hover:bg-foreground/5',
                )}
              >
                Close
              </button>
            </div>
          : state.phase === 'rejected' ?
            <div className="text-center">
              <p className="text-foreground text-sm font-medium">
                Invite rejected
              </p>
              <p className="text-foreground-alt/60 mt-1 text-xs">
                This invite was denied or is no longer valid.
              </p>
              <button
                onClick={() => handleOpenChange(false)}
                className={cn(
                  'mt-3 flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm transition-all',
                  'border-foreground/20 hover:border-foreground/40 hover:bg-foreground/5',
                )}
              >
                Close
              </button>
            </div>
          : <>
              {busy && (
                <div className="flex items-center justify-center gap-2 py-2">
                  <Spinner className="text-foreground-alt" />
                  <span className="text-foreground-alt text-xs">
                    {phaseLabels[state.phase]}
                  </span>
                </div>
              )}
              {!busy && (
                <button
                  onClick={() => void handleSubmit()}
                  disabled={!state.code.trim() || !session}
                  className={cn(
                    'flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm transition-all',
                    'border-foreground/20 hover:border-foreground/40 hover:bg-foreground/5',
                    'disabled:cursor-not-allowed disabled:opacity-50',
                  )}
                >
                  Join Space
                </button>
              )}
            </>
          }

          {state.error && (
            <p className="text-destructive text-xs">{state.error}</p>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}

// resolveInvite resolves the user's input to an SOInviteMessage.
// Accepts full invite links (with b58-encoded message) and, for cloud sessions,
// short alphanumeric invite codes resolved via the spacewave lookup RPC.
async function resolveInvite(
  session: Session,
  input: string,
  isCloud: boolean,
): Promise<SOInviteMessage> {
  // Check if input is a URL containing a b58-encoded invite.
  if (input.startsWith('http')) {
    const url = new URL(input)
    // Hash-router links have the path in the fragment (e.g. /#/join/{encoded}).
    const path = url.hash ? url.hash.slice(1) : url.pathname
    const segments = path.split('/')
    const encoded = segments[segments.length - 1]
    if (!encoded) throw new Error('Invalid invite link')
    const bytes = base58Decode(encoded)
    return SOInviteMessage.fromBinary(bytes)
  }

  // Short codes require the cloud provider.
  if (!isCloud) {
    throw new Error(
      'Paste an invite link (short codes require a cloud account)',
    )
  }
  const resp = await session.spacewave.lookupInviteCode(input)
  if (!resp.inviteMessage) {
    throw new Error('Invite code not found')
  }
  return resp.inviteMessage
}
