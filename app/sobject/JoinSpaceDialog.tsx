import { useCallback, useEffect, useId, useReducer, useRef } from 'react'
import {
  LuCheck,
  LuLogIn,
  LuShieldCheck,
  LuTriangleAlert,
} from 'react-icons/lu'

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
import { PENDING_BEARER_INVITE_PREFIX } from '@s4wave/app/routes/pendingJoin.js'

export interface JoinSpaceDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onAccepted: (sharedObjectId: string) => void
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
      return {
        ...state,
        code: action.code,
        phase: 'input',
        error: undefined,
        spaceId: undefined,
      }
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
  onAccepted,
  initialCode,
}: JoinSpaceDialogProps) {
  const inputId = useId()
  const submitGenerationRef = useRef(0)
  const session = useResourceValue(SessionContext.useContext())
  const { isCloud } = useSessionInfo(session)
  const [state, dispatch] = useReducer(reducer, {
    ...initialState,
    code: initialCode ?? '',
  })

  useEffect(() => {
    if (!open) {
      submitGenerationRef.current += 1
      dispatch({ type: 'reset' })
    }
    return () => {
      submitGenerationRef.current += 1
    }
  }, [open])

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) {
        submitGenerationRef.current += 1
        dispatch({ type: 'reset' })
      }
      onOpenChange(next)
    },
    [onOpenChange],
  )

  const handleSubmit = useCallback(async () => {
    if (!session || !state.code.trim()) return
    const input = state.code.trim()
    const generation = ++submitGenerationRef.current

    dispatch({ type: 'resolving' })
    try {
      const inviteMsg = await resolveInvite(session, input, isCloud)
      if (generation !== submitGenerationRef.current) return
      dispatch({ type: 'connecting' })
      const resp = await session.joinSpaceViaInvite(inviteMsg)
      if (generation !== submitGenerationRef.current) return
      switch (
        resp.result ??
        JoinSpaceViaInviteResult.JoinSpaceViaInviteResult_UNSPECIFIED
      ) {
        case JoinSpaceViaInviteResult.JoinSpaceViaInviteResult_ACCEPTED: {
          const sharedObjectId = resp.sharedObjectId?.trim()
          if (!sharedObjectId) {
            throw new Error('Accepted invite did not return a shared Space')
          }
          dispatch({ type: 'enrolled', spaceId: sharedObjectId })
          return
        }
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
      if (generation !== submitGenerationRef.current) return
      dispatch({
        type: 'error',
        message: err instanceof Error ? err.message : 'Failed to join space',
      })
    }
  }, [session, state.code, isCloud])

  const busy = state.phase === 'resolving' || state.phase === 'connecting'

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="border-foreground/10 bg-background-get-started max-w-md overflow-hidden border p-0">
        <DialogHeader className="border-foreground/8 border-b px-6 py-5 text-left">
          <div className="flex items-start gap-3">
            <div className="bg-brand/10 text-brand flex size-10 shrink-0 items-center justify-center rounded-lg">
              <LuLogIn className="size-4" />
            </div>
            <div className="min-w-0">
              <DialogTitle>Join Space</DialogTitle>
              <DialogDescription className="mt-1.5 leading-relaxed">
                {isCloud
                  ? 'Enter an invite code or paste an invite link.'
                  : 'Paste an invite link to continue.'}
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>

        <form
          className="space-y-4 px-6 py-5"
          onSubmit={(event) => {
            event.preventDefault()
            if (!busy) void handleSubmit()
          }}
        >
          <div>
            <label
              htmlFor={inputId}
              className="text-foreground mb-1.5 block text-xs font-medium"
            >
              {isCloud ? 'Invite code or link' : 'Invite link'}
            </label>
            <input
              id={inputId}
              value={state.code}
              onChange={(e) =>
                dispatch({ type: 'set_code', code: e.target.value })
              }
              placeholder={isCloud ? 'Invite code or link' : 'Invite link'}
              disabled={busy || state.phase === 'enrolled'}
              aria-invalid={state.phase === 'error'}
              aria-describedby={`${inputId}-guidance${state.error ? ` ${inputId}-error` : ''}`}
              className={cn(
                'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 font-mono text-sm transition-colors outline-none',
                'focus:border-brand/50 focus:ring-brand/20 focus:ring-2',
                'disabled:opacity-50',
              )}
            />
            <p
              id={`${inputId}-guidance`}
              className="text-foreground-alt/50 mt-2 flex items-start gap-1.5 text-xs leading-relaxed"
            >
              <LuShieldCheck className="mt-0.5 size-3.5 shrink-0" />
              The invite determines which Space you can access. It does not link
              devices or accounts.
            </p>
          </div>

          {state.phase === 'enrolled' ? (
            <div className="border-success/20 bg-success/5 rounded-lg border p-4 text-center">
              <LuCheck className="text-success mx-auto mb-2 size-5" />
              <p className="text-foreground text-sm font-medium">
                Joined successfully!
              </p>
              <p className="text-foreground-alt/60 mt-1 text-xs">
                Your shared Space is ready.
              </p>
              <button
                type="button"
                onClick={() => {
                  if (state.spaceId) onAccepted(state.spaceId)
                }}
                className={cn(
                  'mt-3 flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm transition-all',
                  'border-foreground/20 hover:border-foreground/40 hover:bg-foreground/5',
                )}
              >
                Open the shared Space
              </button>
            </div>
          ) : state.phase === 'pending' ? (
            <div className="text-center">
              <p className="text-foreground text-sm font-medium">
                Awaiting owner approval
              </p>
              <p className="text-foreground-alt/60 mt-1 text-xs">
                The owner must approve this invite before you can open the
                shared Space. Return here to retry after approval.
              </p>
              <button
                type="button"
                onClick={() => handleOpenChange(false)}
                className={cn(
                  'mt-3 flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm transition-all',
                  'border-foreground/20 hover:border-foreground/40 hover:bg-foreground/5',
                )}
              >
                Close
              </button>
            </div>
          ) : state.phase === 'owner_online_required' ? (
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
                type="button"
                onClick={() => handleOpenChange(false)}
                className={cn(
                  'mt-3 flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm transition-all',
                  'border-foreground/20 hover:border-foreground/40 hover:bg-foreground/5',
                )}
              >
                Close
              </button>
            </div>
          ) : state.phase === 'rejected' ? (
            <div className="text-center">
              <p className="text-foreground text-sm font-medium">
                Invite rejected
              </p>
              <p className="text-foreground-alt/60 mt-1 text-xs">
                This invite was denied or is no longer valid.
              </p>
              <button
                type="button"
                onClick={() => handleOpenChange(false)}
                className={cn(
                  'mt-3 flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm transition-all',
                  'border-foreground/20 hover:border-foreground/40 hover:bg-foreground/5',
                )}
              >
                Close
              </button>
            </div>
          ) : (
            <>
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
                  type="submit"
                  disabled={!state.code.trim() || !session}
                  className={cn(
                    'flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm transition-all',
                    'border-brand/30 bg-brand/10 text-foreground hover:border-brand/40 hover:bg-brand/15',
                    'focus-visible:ring-brand/40 focus-visible:ring-2 focus-visible:outline-none',
                    'disabled:cursor-not-allowed disabled:opacity-50',
                  )}
                >
                  {state.phase === 'error' ? 'Try again' : 'Join Space'}
                </button>
              )}
            </>
          )}

          {state.error && (
            <div
              id={`${inputId}-error`}
              role="alert"
              className="border-destructive/20 bg-destructive/5 text-destructive flex items-start gap-2 rounded-md border px-3 py-2.5 text-xs leading-relaxed"
            >
              <LuTriangleAlert className="mt-0.5 size-3.5 shrink-0" />
              <span>{state.error}</span>
            </div>
          )}
        </form>
      </DialogContent>
    </Dialog>
  )
}

// resolveInvite resolves the user's input to an SOInviteMessage.
// Full links and pending bearer handoffs decode locally; unmarked inputs are
// cloud short codes.
async function resolveInvite(
  session: Session,
  input: string,
  isCloud: boolean,
): Promise<SOInviteMessage> {
  let encoded: string | undefined
  if (input.startsWith('http')) {
    const url = new URL(input)
    const path = url.hash ? url.hash.slice(1) : url.pathname
    const segments = path.split('/')
    encoded = segments[segments.length - 1]
  } else if (input.startsWith(PENDING_BEARER_INVITE_PREFIX)) {
    encoded = input.slice(PENDING_BEARER_INVITE_PREFIX.length)
  }

  if (encoded !== undefined) {
    if (!encoded) throw new Error('Invalid invite link')
    const bytes = base58Decode(encoded)
    return SOInviteMessage.fromBinary(bytes)
  }

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
