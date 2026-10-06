import {
  useCallback,
  useEffect,
  useId,
  useReducer,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import {
  LuCheck,
  LuLogIn,
  LuShieldCheck,
  LuTriangleAlert,
} from 'react-icons/lu'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { SOInviteMessage } from '@s4wave/core/sobject/sobject.pb.js'
import {
  JoinSpaceViaInviteResult,
  type JoinSpaceViaInviteResponse,
} from '@s4wave/sdk/session/session.pb.js'
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
import { mountSpace } from '@s4wave/app/space/space.js'

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

const panelButtonClass = cn(
  'mt-3 flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm transition-all',
  'border-foreground/20 hover:border-foreground/40 hover:bg-foreground/5',
)

/**
 * joinResultAction maps the join response to the state transition for it. An
 * accepted join also starts the optional whole-Space download.
 */
function joinResultAction(
  session: Session,
  resp: JoinSpaceViaInviteResponse,
  backfill: boolean,
): JoinAction {
  switch (
    resp.result ??
    JoinSpaceViaInviteResult.JoinSpaceViaInviteResult_UNSPECIFIED
  ) {
    case JoinSpaceViaInviteResult.JoinSpaceViaInviteResult_ACCEPTED: {
      const sharedObjectId = resp.sharedObjectId?.trim()
      if (!sharedObjectId) {
        throw new Error('Accepted invite did not return a shared Space')
      }
      if (backfill) void backfillJoinedSpace(session, sharedObjectId)
      return { type: 'enrolled', spaceId: sharedObjectId }
    }
    case JoinSpaceViaInviteResult.JoinSpaceViaInviteResult_PENDING_OWNER_APPROVAL:
      return { type: 'pending' }
    case JoinSpaceViaInviteResult.JoinSpaceViaInviteResult_OWNER_MUST_BE_ONLINE:
      return { type: 'owner_online_required' }
    case JoinSpaceViaInviteResult.JoinSpaceViaInviteResult_REJECTED:
      return { type: 'rejected' }
    default:
      throw new Error('Invite join returned an unknown result')
  }
}

/** JoinNoticePanel renders a terminal join outcome with a close action. */
function JoinNoticePanel({
  title,
  onClose,
  children,
}: {
  title: string
  onClose: () => void
  children: ReactNode
}) {
  return (
    <div className="text-center">
      <p className="text-foreground text-sm font-medium">{title}</p>
      <p className="text-foreground-alt/60 mt-1 text-xs">{children}</p>
      <button type="button" onClick={onClose} className={panelButtonClass}>
        Close
      </button>
    </div>
  )
}

/** JoinEnrolledPanel renders the success state with the open action. */
function JoinEnrolledPanel({ onOpen }: { onOpen: () => void }) {
  return (
    <div className="border-success/20 bg-success/5 rounded-lg border p-4 text-center">
      <LuCheck className="text-success mx-auto mb-2 size-5" />
      <p className="text-foreground text-sm font-medium">
        Joined successfully!
      </p>
      <p className="text-foreground-alt/60 mt-1 text-xs">
        Your shared Space is ready.
      </p>
      <button type="button" onClick={onOpen} className={panelButtonClass}>
        Open the shared Space
      </button>
    </div>
  )
}

/** JoinSubmitControls renders the busy status or the submit button. */
function JoinSubmitControls({
  state,
  busy,
  canSubmit,
}: {
  state: JoinState
  busy: boolean
  canSubmit: boolean
}) {
  if (busy) {
    return (
      <div className="flex items-center justify-center gap-2 py-2">
        <Spinner variant="muted" />
        <span className="text-foreground-alt text-xs">
          {phaseLabels[state.phase]}
        </span>
      </div>
    )
  }
  return (
    <button
      type="submit"
      disabled={!canSubmit}
      className={cn(
        'flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm transition-all',
        'border-brand/30 bg-brand/10 text-foreground hover:border-brand/40 hover:bg-brand/15',
        'focus-visible:ring-brand/40 focus-visible:ring-2 focus-visible:outline-none',
        'disabled:cursor-not-allowed disabled:opacity-50',
      )}
    >
      {state.phase === 'error' ? 'Try again' : 'Join Space'}
    </button>
  )
}

/** JoinPhasePanel renders the part of the form that depends on the join phase. */
function JoinPhasePanel({
  state,
  backfill,
  busy,
  canSubmit,
  onAccepted,
  onClose,
}: {
  state: JoinState
  backfill: boolean
  busy: boolean
  canSubmit: boolean
  onAccepted: (sharedObjectId: string) => void
  onClose: () => void
}) {
  switch (state.phase) {
    case 'enrolled':
      return (
        <JoinEnrolledPanel
          onOpen={() => {
            if (state.spaceId) onAccepted(state.spaceId)
          }}
        />
      )
    case 'pending':
      return (
        <JoinNoticePanel title="Awaiting owner approval" onClose={onClose}>
          The owner must approve this invite before you can open the shared
          Space. Return here to retry after approval.
          {backfill &&
            ' To download the whole Space, choose Whole Space in its settings after approval.'}
        </JoinNoticePanel>
      )
    case 'owner_online_required':
      return (
        <JoinNoticePanel title="Owner must be online" onClose={onClose}>
          This local-first join path completes directly through the space owner.
          Ask the owner to open the space, then try this invite link again.
        </JoinNoticePanel>
      )
    case 'rejected':
      return (
        <JoinNoticePanel title="Invite rejected" onClose={onClose}>
          This invite was denied or is no longer valid.
        </JoinNoticePanel>
      )
    default:
      return (
        <JoinSubmitControls state={state} busy={busy} canSubmit={canSubmit} />
      )
  }
}

/** JoinInviteField renders the invite input and its guidance. */
function JoinInviteField({
  inputId,
  state,
  isCloud,
  busy,
  onChange,
}: {
  inputId: string
  state: JoinState
  isCloud: boolean
  busy: boolean
  onChange: (code: string) => void
}) {
  const label = isCloud ? 'Invite code or link' : 'Invite link'

  return (
    <div>
      <label
        htmlFor={inputId}
        className="text-foreground mb-1.5 block text-xs font-medium"
      >
        {label}
      </label>
      <input
        id={inputId}
        value={state.code}
        onChange={(e) => onChange(e.target.value)}
        placeholder={label}
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
  )
}

/** JoinError renders the join failure announced by the invite input. */
function JoinError({ id, message }: { id: string; message: string }) {
  return (
    <div
      id={id}
      role="alert"
      className="border-destructive/20 bg-destructive/5 text-destructive flex items-start gap-2 rounded-md border px-3 py-2.5 text-xs leading-relaxed"
    >
      <LuTriangleAlert className="mt-0.5 size-3.5 shrink-0" />
      <span>{message}</span>
    </div>
  )
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
  const [backfill, setBackfill] = useState(false)

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
      dispatch(joinResultAction(session, resp, backfill))
    } catch (err) {
      if (generation !== submitGenerationRef.current) return
      dispatch({
        type: 'error',
        message: err instanceof Error ? err.message : 'Failed to join space',
      })
    }
  }, [session, state.code, isCloud, backfill])

  const busy = state.phase === 'resolving' || state.phase === 'connecting'
  const showBackfill = state.phase !== 'enrolled' && state.phase !== 'pending'

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent variant="panel">
        <DialogHeader variant="panel">
          <div className="flex items-start gap-3">
            <div className="bg-brand/10 text-brand flex size-10 shrink-0 items-center justify-center rounded-lg">
              <LuLogIn className="size-4" />
            </div>
            <div className="min-w-0">
              <DialogTitle>Join Space</DialogTitle>
              <DialogDescription variant="relaxed" className="mt-1.5">
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
          <JoinInviteField
            inputId={inputId}
            state={state}
            isCloud={isCloud}
            busy={busy}
            onChange={(code) => dispatch({ type: 'set_code', code })}
          />

          {showBackfill && (
            <label className="text-foreground-alt flex items-start gap-2 text-xs select-none">
              <input
                type="checkbox"
                className="mt-0.5"
                checked={backfill}
                disabled={busy}
                onChange={(e) => setBackfill(e.target.checked)}
              />
              <span>
                Download the whole Space to this device. Otherwise items
                download when you open them.
              </span>
            </label>
          )}

          <JoinPhasePanel
            state={state}
            backfill={backfill}
            busy={busy}
            canSubmit={!!state.code.trim() && !!session}
            onAccepted={onAccepted}
            onClose={() => handleOpenChange(false)}
          />

          {state.error && (
            <JoinError id={`${inputId}-error`} message={state.error} />
          )}
        </form>
      </DialogContent>
    </Dialog>
  )
}

// backfillJoinedSpace makes this device download the whole joined Space. The
// join already succeeded, so a failure only reports where to choose again.
async function backfillJoinedSpace(
  session: Session,
  sharedObjectId: string,
): Promise<void> {
  const resources: Array<{ [Symbol.dispose](): void }> = []
  const cleanup = <T extends { [Symbol.dispose](): void } | null | undefined>(
    resource: T,
  ): T => {
    if (resource) resources.push(resource)
    return resource
  }
  try {
    const space = await mountSpace({
      session,
      spaceResp: {
        sharedObjectRef: { providerResourceRef: { id: sharedObjectId } },
      },
      abortSignal: new AbortController().signal,
      cleanup,
    })
    await space.setSpaceBackfill(true)
  } catch (err) {
    toast.error('Could not download the whole Space', {
      description: `${err instanceof Error ? err.message : String(err)}. Choose Whole Space in the Space settings to try again.`,
    })
  } finally {
    for (const resource of resources.reverse()) resource[Symbol.dispose]()
  }
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
