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
} from '@s4wave/web/images/icons.js'

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

import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

import { base58Decode } from '@s4wave/app/provider/spacewave/keypair-utils.js'
import { BEARER_INVITE_PREFIX } from '@s4wave/app/routes/pendingRoute.js'
import { mountSpace } from '@s4wave/app/space/space.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import { colors as themeColors } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: { textAlign: 'center' },
  rule2: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule3: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule4: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 40%, transparent)`,
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
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule5: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-success']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-success']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
  },
  rule6: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule7: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule8: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule9: {
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule10: {
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus':
        'var(--tw-ring-inset, ) 0 0 0 calc(2px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    boxShadow: {
      default: null,
      ':focus':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule11: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule12: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-destructive'],
  },
  rule13: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule14: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule15: { minWidth: 0 },
  rule16: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule17: { marginTop: `calc(${themeLayout['--spacing']} * .5)` },
})

const stylexStyles = stylex.create({
  rule1: {
    marginInline: 'auto',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-success'],
  },
  rule2: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const dialogStyles = stylex.create({
  description: { marginTop: `calc(${themeLayout['--spacing']} * 1.5)` },
})

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
  // serverAddr is the owner's UDP host:port a dev build may join through.
  serverAddr: string
  phase: JoinPhase
  error: string | undefined
  spaceId: string | undefined
}

type JoinAction =
  | { type: 'reset' }
  | { type: 'set_code'; code: string }
  | { type: 'set_server_addr'; serverAddr: string }
  | { type: 'resolving' }
  | { type: 'connecting' }
  | { type: 'pending' }
  | { type: 'owner_online_required' }
  | { type: 'rejected' }
  | { type: 'enrolled'; spaceId: string }
  | { type: 'error'; message: string }

const initialState: JoinState = {
  code: '',
  serverAddr: '',
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
    case 'set_server_addr':
      return {
        ...state,
        serverAddr: action.serverAddr,
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
  sx,
  title,
  onClose,
  children,
}: {
  title: string
  onClose: () => void
  children: ReactNode
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div sx={[stylexStyles2.rule1, sx]}>
      <p sx={stylexStyles2.rule2}>{title}</p>
      <p sx={stylexStyles2.rule3}>{children}</p>
      <button type="button" onClick={onClose} sx={[stylexStyles2.rule4]}>
        Close
      </button>
    </div>
  )
}

/** JoinEnrolledPanel renders the success state with the open action. */
function JoinEnrolledPanel({
  sx,
  onOpen,
}: { onOpen: () => void } & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div sx={[stylexStyles2.rule5, sx]}>
      <LuCheck sx={stylexStyles.rule1} />
      <p sx={stylexStyles2.rule2}>Joined successfully!</p>
      <p sx={stylexStyles2.rule3}>Your shared Space is ready.</p>
      <button type="button" onClick={onOpen} sx={[stylexStyles2.rule4]}>
        Open the shared Space
      </button>
    </div>
  )
}

/** JoinSubmitControls renders the busy status or the submit button. */
function JoinSubmitControls({
  sx,
  state,
  busy,
  canSubmit,
}: {
  state: JoinState
  busy: boolean
  canSubmit: boolean
} & { sx?: stylex.StyleXStyles; className?: never }) {
  if (busy) {
    return (
      <div sx={[stylexStyles2.rule6, sx]}>
        <Spinner variant="muted" />
        <span sx={stylexStyles2.rule7}>{phaseLabels[state.phase]}</span>
      </div>
    )
  }
  return (
    <button
      type="submit"
      disabled={!canSubmit}
      sx={[[stylexStyles2.rule8], sx]}
    >
      {state.phase === 'error' ? 'Try again' : 'Join Space'}
    </button>
  )
}

/** JoinPhasePanel renders the part of the form that depends on the join phase. */
function JoinPhasePanel({
  sx,
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
} & { sx?: stylex.StyleXStyles; className?: never }) {
  switch (state.phase) {
    case 'enrolled':
      return (
        <JoinEnrolledPanel
          sx={sx}
          onOpen={() => {
            if (state.spaceId) onAccepted(state.spaceId)
          }}
        />
      )
    case 'pending':
      return (
        <JoinNoticePanel
          sx={sx}
          title="Awaiting owner approval"
          onClose={onClose}
        >
          The owner must approve this invite before you can open the shared
          Space. Return here to retry after approval.
          {backfill &&
            ' To download the whole Space, choose Whole Space in its settings after approval.'}
        </JoinNoticePanel>
      )
    case 'owner_online_required':
      if (state.serverAddr.trim()) {
        return (
          <JoinNoticePanel
            sx={sx}
            title="Server did not answer"
            onClose={onClose}
          >
            No reply from {state.serverAddr.trim()}. Check the address and that
            the server lists this Session.
          </JoinNoticePanel>
        )
      }
      return (
        <JoinNoticePanel sx={sx} title="Owner must be online" onClose={onClose}>
          This local-first join path completes directly through the space owner.
          Ask the owner to open the space, then try this invite link again.
        </JoinNoticePanel>
      )
    case 'rejected':
      return (
        <JoinNoticePanel sx={sx} title="Invite rejected" onClose={onClose}>
          This invite was denied or is no longer valid.
        </JoinNoticePanel>
      )
    default:
      return (
        <JoinSubmitControls
          sx={sx}
          state={state}
          busy={busy}
          canSubmit={canSubmit}
        />
      )
  }
}

/** JoinInviteField renders the invite input and its guidance. */
function JoinInviteField({
  sx,
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
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const label = isCloud ? 'Invite code or link' : 'Invite link'

  return (
    <div sx={sx}>
      <label htmlFor={inputId} sx={stylexStyles2.rule9}>
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
        sx={[stylexStyles2.rule10]}
      />
      <p id={`${inputId}-guidance`} sx={stylexStyles2.rule11}>
        <LuShieldCheck sx={stylexStyles.rule2} />
        The invite determines which Space you can access. It does not link
        devices or accounts.
      </p>
    </div>
  )
}

/**
 * JoinServerField renders the dev build's owner address input and the Session
 * peer a server must list before it answers.
 */
function JoinServerField({
  sx,
  inputId,
  state,
  peerId,
  busy,
  onChange,
}: {
  inputId: string
  state: JoinState
  peerId: string
  busy: boolean
  onChange: (serverAddr: string) => void
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div sx={sx}>
      <div sx={nativeChildSpacing.s3}>
        <label htmlFor={inputId} sx={stylexStyles2.rule9}>
          Server address
        </label>
        <input
          id={inputId}
          value={state.serverAddr}
          onChange={(e) => onChange(e.target.value)}
          placeholder="100.64.0.1:4101"
          disabled={busy || state.phase === 'enrolled'}
          sx={[stylexStyles2.rule10]}
        />
      </div>
      {peerId && (
        <CopyableField
          sx={nativeChildSpacing.s3}
          label="This Session"
          value={peerId}
        />
      )}
    </div>
  )
}

/** JoinError renders the join failure announced by the invite input. */
function JoinError({
  sx,
  id,
  message,
}: { id: string; message: string } & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  return (
    <div id={id} role="alert" sx={[stylexStyles2.rule12, sx]}>
      <LuTriangleAlert sx={stylexStyles.rule2} />
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
  // Keep form input and submission generation tied to this mounted dialog.
  const inputId = useId()
  const serverInputId = useId()
  const submitGenerationRef = useRef(0)
  const session = useResourceValue(SessionContext.useContext())
  const { isCloud, peerId } = useSessionInfo(session)
  const showServer = !!import.meta.env?.DEV && !isCloud
  const [state, dispatch] = useReducer(reducer, {
    ...initialState,
    code: initialCode ?? '',
  })
  const [backfill, setBackfill] = useState(false)

  // Invalidate pending results when the dialog closes or unmounts.
  useEffect(() => {
    if (!open) {
      submitGenerationRef.current += 1
      dispatch({ type: 'reset' })
    }
    return () => {
      submitGenerationRef.current += 1
    }
  }, [open])

  // Reset dismissed input before notifying the caller.
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

  // Resolve and join the submitted invite within one generation.
  const handleSubmit = useCallback(async () => {
    // Require a session and nonempty invite before starting.
    if (!session || !state.code.trim()) return
    const input = state.code.trim()
    const generation = ++submitGenerationRef.current

    // Ignore results from a dismissed or superseded submission.
    dispatch({ type: 'resolving' })
    try {
      const inviteMsg = await resolveInvite(session, input, isCloud)
      if (generation !== submitGenerationRef.current) return
      dispatch({ type: 'connecting' })
      const resp = await session.joinSpaceViaInvite(
        inviteMsg,
        undefined,
        showServer ? state.serverAddr.trim() : '',
      )
      if (generation !== submitGenerationRef.current) return
      dispatch(joinResultAction(session, resp, backfill))
    } catch (err) {
      if (generation !== submitGenerationRef.current) return
      dispatch({
        type: 'error',
        message: err instanceof Error ? err.message : 'Failed to join space',
      })
    }
  }, [session, state.code, state.serverAddr, showServer, isCloud, backfill])

  // Derive available controls from the current join phase.
  const busy = state.phase === 'resolving' || state.phase === 'connecting'
  const showBackfill = state.phase !== 'enrolled' && state.phase !== 'pending'

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent variant="panel">
        <DialogHeader variant="panel">
          <div sx={stylexStyles2.rule13}>
            <div sx={stylexStyles2.rule14}>
              <LuLogIn sx={stylexStyles.rule3} />
            </div>
            <div sx={stylexStyles2.rule15}>
              <DialogTitle>Join Space</DialogTitle>
              <DialogDescription
                variant="relaxed"
                sx={dialogStyles.description}
              >
                {isCloud
                  ? 'Enter an invite code or paste an invite link.'
                  : 'Paste an invite link to continue.'}
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>

        <form
          sx={stylexStyles3.rule1}
          onSubmit={(event) => {
            event.preventDefault()
            if (!busy) void handleSubmit()
          }}
        >
          <JoinInviteField
            sx={nativeChildSpacingNext.s4}
            inputId={inputId}
            state={state}
            isCloud={isCloud}
            busy={busy}
            onChange={(code) => dispatch({ type: 'set_code', code })}
          />

          {showServer && (
            <JoinServerField
              sx={nativeChildSpacingNext.s4}
              inputId={serverInputId}
              state={state}
              peerId={peerId}
              busy={busy}
              onChange={(serverAddr) =>
                dispatch({ type: 'set_server_addr', serverAddr })
              }
            />
          )}

          {showBackfill && (
            <label sx={[nativeChildSpacingNext.s4, stylexStyles2.rule16]}>
              <input
                type="checkbox"
                sx={stylexStyles2.rule17}
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
            sx={nativeChildSpacingNext.s4}
            state={state}
            backfill={backfill}
            busy={busy}
            canSubmit={!!state.code.trim() && !!session}
            onAccepted={onAccepted}
            onClose={() => handleOpenChange(false)}
          />

          {state.error && (
            <JoinError
              sx={nativeChildSpacingNext.s4}
              id={`${inputId}-error`}
              message={state.error}
            />
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
  } else if (input.startsWith(BEARER_INVITE_PREFIX)) {
    encoded = input.slice(BEARER_INVITE_PREFIX.length)
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
