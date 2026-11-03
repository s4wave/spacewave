import * as stylex from '@stylexjs/stylex'
import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useReducer,
  useRef,
  useState,
} from 'react'
import {
  LuBuilding2,
  LuCheck,
  LuCopy,
  LuLink,
  LuQrCode,
  LuSearch,
  LuUserPlus,
} from '@s4wave/web/images/icons.js'

import {
  SOInviteMessage,
  SOParticipantRole,
} from '@s4wave/core/sobject/sobject.pb.js'
import type { CreateSpaceInviteResponse } from '@s4wave/sdk/session/session.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import type { OrgMemberInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import {
  Tabs,
  TabsContent,
  tabsContentItemStyles,
  TabsList,
  TabsTrigger,
} from '@s4wave/web/ui/tabs.js'

import { base58Encode } from '@s4wave/app/provider/spacewave/keypair-utils.js'
import { buildInviteLink } from '@s4wave/app/urls.js'
import { useCloudProviderConfig } from '@s4wave/app/provider/spacewave/useSpacewaveAuth.js'

import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    maxHeight: `calc(${themeLayout['--spacing']} * 48)`,
    overflowY: 'auto',
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
  },
  rule2: {
    '--tw-duration': '.15s',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    cursor: { default: null, ':disabled': 'not-allowed' },
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration: '.15s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule3: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule4: { position: 'relative' },
})

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-duration': '.15s',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-success']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-success']} 40%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-success']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-success']} 10%, transparent)`,
      },
    },
    color: themeColors['--color-success'],
    cursor: { default: null, ':disabled': 'not-allowed' },
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration: '.15s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule2: {
    '--tw-duration': '.15s',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    cursor: { default: null, ':disabled': 'not-allowed' },
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    transitionDuration: '.15s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule3: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule4: {
    '--tw-duration': '.15s',
    '--tw-outline-style': 'none',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-foreground']} 30%, transparent)`,
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    outlineStyle: 'none',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    transitionDuration: '.15s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule5: {
    '--tw-tracking': 'var(--tracking-widest)',
    fontSize: 'var(--text-lg)',
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    textAlign: 'center',
  },
  rule6: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule7: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule8: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: themeLayout['--spacing'],
  },
  rule9: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule10: {
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule11: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: '3.40282e38px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-success']} 10%, transparent)`,
    color: themeColors['--color-success'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule12: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule13: {
    '--tw-outline-style': 'none',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-foreground']} 40%, transparent)`,
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    outlineStyle: 'none',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    paddingLeft: `calc(${themeLayout['--spacing']} * 8)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 3)`,
    width: '100%',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule14: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
  },
  rule15: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
  },
  rule16: {
    borderRadius: themeRadii['--radius-md'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-success']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    cursor: { default: null, ':disabled': 'default' },
    display: 'flex',
    justifyContent: 'space-between',
    opacity: { default: null, ':disabled': 0.6 },
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule17: {
    borderRadius: themeRadii['--radius-md'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    cursor: { default: null, ':disabled': 'default' },
    display: 'flex',
    justifyContent: 'space-between',
    opacity: { default: null, ':disabled': 0.6 },
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule18: { flex: 1, minWidth: 0 },
  rule19: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule20: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: '10px',
    lineHeight: 1.5,
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
  },
  rule21: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: '10px',
    lineHeight: 1.5,
  },
  rule22: { flexShrink: 0 },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    position: 'absolute',
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    left: `calc(${themeLayout['--spacing']} * 2.5)`,
    top: `calc(${themeLayout['--spacing']} * 2.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    color: themeColors['--color-success'],
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule4: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

export // Keep the native item class beside these callers until their utility styles convert.

interface AddUserDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  spaceName: string
  spaceId: string
  isCloudProvider?: boolean
  orgId?: string
  orgMembers?: OrgMemberInfo[]
  orgMembersLoading?: boolean
}

interface InviteState {
  inviteResp: CreateSpaceInviteResponse | null
  creating: boolean
  error: string | undefined
  copied: boolean
  username: string
  usernameSent: boolean
}

type InviteAction =
  | { type: 'reset' }
  | { type: 'creating' }
  | { type: 'created'; resp: CreateSpaceInviteResponse }
  | { type: 'error'; message: string }
  | { type: 'copied' }
  | { type: 'uncopied' }
  | { type: 'copy_error' }
  | { type: 'username'; value: string }
  | { type: 'usernameSent' }

const initialState: InviteState = {
  inviteResp: null,
  creating: false,
  error: undefined,
  copied: false,
  username: '',
  usernameSent: false,
}

function reducer(state: InviteState, action: InviteAction): InviteState {
  switch (action.type) {
    case 'reset':
      return initialState
    case 'creating':
      return { ...state, creating: true, error: undefined }
    case 'created':
      return { ...state, creating: false, inviteResp: action.resp }
    case 'error':
      return { ...state, creating: false, error: action.message }
    case 'copied':
      return { ...state, copied: true, error: undefined }
    case 'uncopied':
      return { ...state, copied: false }
    case 'copy_error':
      return {
        ...state,
        copied: false,
        error: 'Could not copy to the clipboard',
      }
    case 'username':
      return {
        ...state,
        username: action.value,
        usernameSent: false,
        error: undefined,
      }
    case 'usernameSent':
      return { ...state, creating: false, usernameSent: true }
  }
}

function getOrgMemberPrimaryLabel(member: OrgMemberInfo): string {
  return member.entityId || member.subjectId || 'Unknown member'
}

function getOrgMemberSecondaryLabel(member: OrgMemberInfo): string {
  if (member.entityId && member.subjectId) return member.subjectId
  return ''
}

/** inputClass styles the read-only and text inputs of the invite tabs. */

/**
 * useInviteFlow owns the invite state of the dialog: creating a code or link,
 * sending a username invite, and copying the result to the clipboard.
 */
function useInviteFlow(
  session: Session | null | undefined,
  spaceId: string,
  isCloudProvider: boolean,
) {
  const cloudProviderConfig = useCloudProviderConfig()
  const [state, dispatch] = useReducer(reducer, initialState)
  const copyGenerationRef = useRef(0)
  const copyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      copyGenerationRef.current += 1
      if (copyTimerRef.current != null) clearTimeout(copyTimerRef.current)
    }
  }, [])

  const reset = useCallback(() => dispatch({ type: 'reset' }), [])

  const setUsername = useCallback(
    (value: string) => dispatch({ type: 'username', value }),
    [],
  )

  const createInvite = useCallback(async () => {
    if (!session) return
    dispatch({ type: 'creating' })
    try {
      const resp = await session.createSpaceInvite({
        spaceId,
        role: SOParticipantRole.SOParticipantRole_WRITER,
      })
      dispatch({ type: 'created', resp })
    } catch (err) {
      dispatch({
        type: 'error',
        message: err instanceof Error ? err.message : 'Failed to create invite',
      })
    }
  }, [session, spaceId])

  const sendUsernameInvite = useCallback(async () => {
    const username = state.username.trim()
    if (!session || !username || !isCloudProvider) return
    dispatch({ type: 'creating' })
    try {
      await session.spacewave.createSpaceTargetedInvitationByUsername(
        username,
        spaceId,
        'writer',
      )
      dispatch({ type: 'usernameSent' })
    } catch (err) {
      dispatch({
        type: 'error',
        message:
          err instanceof Error ? err.message : 'Failed to send username invite',
      })
    }
  }, [isCloudProvider, session, spaceId, state.username])

  const inviteLink = useMemo(() => {
    if (!state.inviteResp?.inviteMessage) return ''
    const encoded = base58Encode(
      SOInviteMessage.toBinary(state.inviteResp.inviteMessage),
    )
    return buildInviteLink(cloudProviderConfig?.publicBaseUrl, encoded)
  }, [state.inviteResp, cloudProviderConfig?.publicBaseUrl])

  const copy = useCallback(async (text: string) => {
    const generation = ++copyGenerationRef.current
    if (copyTimerRef.current != null) clearTimeout(copyTimerRef.current)
    try {
      await navigator.clipboard.writeText(text)
      if (!mountedRef.current || generation !== copyGenerationRef.current)
        return
      dispatch({ type: 'copied' })
      copyTimerRef.current = setTimeout(() => {
        if (!mountedRef.current || generation !== copyGenerationRef.current)
          return
        dispatch({ type: 'uncopied' })
        copyTimerRef.current = null
      }, 2000)
    } catch {
      if (!mountedRef.current || generation !== copyGenerationRef.current)
        return
      dispatch({ type: 'copy_error' })
    }
  }, [])

  return {
    ...state,
    hasSession: !!session,
    shortCode: state.inviteResp?.shortCode ?? '',
    inviteLink,
    reset,
    setUsername,
    createInvite,
    sendUsernameInvite,
    copy,
  }
}

type InviteFlow = ReturnType<typeof useInviteFlow>

/** useOrgMemberSource resolves the org members shown in the Org Members tab. */
function useOrgMemberSource(
  orgId: string | undefined,
  orgMembers: OrgMemberInfo[] | undefined,
  orgMembersLoading: boolean | undefined,
) {
  const spaceContainer = SpaceContainerContext.useContextSafe()
  const effectiveOrgId = orgId ?? ''

  return {
    hasOrgTab: effectiveOrgId.length > 0,
    members: orgMembers ?? spaceContainer?.orgState?.members ?? [],
    loading:
      orgMembersLoading ?? (!!effectiveOrgId && !spaceContainer?.orgState),
  }
}

/** defaultTabFor picks the invite tab shown when the dialog opens. */
function defaultTabFor(hasOrgTab: boolean, isCloudProvider: boolean): string {
  if (hasOrgTab) return 'members'
  return isCloudProvider ? 'username' : 'code'
}

/** InviteCopyButton copies the invite text and flashes a copied state. */
function InviteCopyButton({
  sx,
  copied,
  label,
  onClick,
}: {
  copied: boolean
  label: string
  onClick: () => void
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <button
      type="button"
      onClick={onClick}
      sx={[copied ? stylexStyles2.rule1 : stylexStyles2.rule2, sx]}
    >
      {copied ? (
        <>
          <LuCheck sx={stylexStyles.rule1} />
          Copied
        </>
      ) : (
        <>
          <LuCopy sx={stylexStyles.rule1} />
          {label}
        </>
      )}
    </button>
  )
}

/** InviteCodeTab renders the short invite code and its copy action. */
function InviteCodeTab({ flow }: { flow: InviteFlow }) {
  const inviteCodeId = useId()

  if (flow.shortCode) {
    return (
      <div>
        <label
          htmlFor={inviteCodeId}
          sx={[nativeChildSpacing.s2, stylexStyles2.rule3]}
        >
          Invite code
        </label>
        <input
          id={inviteCodeId}
          value={flow.shortCode}
          readOnly
          sx={[
            nativeChildSpacing.s2,
            [[stylexStyles2.rule4], stylexStyles2.rule5],
          ]}
          onClick={(e) => (e.target as HTMLInputElement).select()}
        />
        <InviteCopyButton
          sx={nativeChildSpacing.s2}
          copied={flow.copied}
          label="Copy Code"
          onClick={() => void flow.copy(flow.shortCode)}
        />
      </div>
    )
  }

  if (flow.inviteResp) {
    return (
      <p sx={stylexStyles2.rule6}>
        Short codes are only available for cloud sessions. Use the Link tab
        instead.
      </p>
    )
  }

  return (
    <button
      type="button"
      onClick={() => void flow.createInvite()}
      disabled={flow.creating || !flow.hasSession}
      sx={[stylexStyles2.rule2]}
    >
      {flow.creating ? 'Creating…' : 'Create Invite Code'}
    </button>
  )
}

/** InviteUsernameTab renders the username field and its send action. */
function InviteUsernameTab({ flow }: { flow: InviteFlow }) {
  const usernameInputId = useId()

  return (
    <div>
      <label
        htmlFor={usernameInputId}
        sx={[nativeChildSpacing.s2, stylexStyles2.rule3]}
      >
        Spacewave username
      </label>
      <input
        id={usernameInputId}
        value={flow.username}
        onChange={(e) => flow.setUsername(e.target.value)}
        placeholder="alice"
        sx={[nativeChildSpacing.s2, [stylexStyles2.rule4]]}
      />
      <button
        type="button"
        onClick={() => void flow.sendUsernameInvite()}
        disabled={flow.creating || !flow.hasSession || !flow.username.trim()}
        sx={[
          nativeChildSpacing.s2,
          flow.usernameSent ? stylexStyles2.rule1 : stylexStyles2.rule2,
        ]}
      >
        {flow.usernameSent ? (
          <>
            <LuCheck sx={stylexStyles.rule1} />
            Sent
          </>
        ) : (
          <>
            <LuUserPlus sx={stylexStyles.rule1} />
            {flow.creating ? 'Sending…' : 'Send Invite'}
          </>
        )}
      </button>
    </div>
  )
}

/** InviteLinkTab renders the shareable invite link and its copy action. */
function InviteLinkTab({ flow }: { flow: InviteFlow }) {
  const inviteLinkId = useId()

  return (
    <>
      <div sx={[[stylexStyles3.rule1], tabsContentItemStyles.item]}>
        <div sx={stylexStyles2.rule7}>Share a Space invite</div>
        <p sx={stylexStyles2.rule8}>
          Create one link for this Space, then send it through a trusted
          channel. The recipient uses Join Space to accept it.
        </p>
      </div>
      {flow.inviteResp?.inviteMessage ? (
        <div sx={tabsContentItemStyles.item}>
          <div sx={[nativeChildSpacingNext.s2, stylexStyles2.rule9]}>
            <label htmlFor={inviteLinkId} sx={stylexStyles2.rule10}>
              Invite link
            </label>
            <span sx={stylexStyles2.rule11}>Ready to share</span>
          </div>
          <input
            id={inviteLinkId}
            value={flow.inviteLink}
            readOnly
            sx={[nativeChildSpacingNext.s2, [stylexStyles2.rule4]]}
            onClick={(e) => (e.target as HTMLInputElement).select()}
          />
          <InviteCopyButton
            sx={nativeChildSpacingNext.s2}
            copied={flow.copied}
            label="Copy Link"
            onClick={() => void flow.copy(flow.inviteLink)}
          />
        </div>
      ) : (
        <button
          type="button"
          onClick={() => void flow.createInvite()}
          disabled={flow.creating || !flow.hasSession}

          sx={[[[stylexStyles3.rule2]], tabsContentItemStyles.item]}
        >
          {flow.creating ? 'Creating secure link…' : 'Create Invite Link'}
        </button>
      )}
    </>
  )
}

/** AddUserTabList renders the tab triggers for the available invite methods. */
function AddUserTabList({
  hasOrgTab,
  isCloudProvider,
}: {
  hasOrgTab: boolean
  isCloudProvider: boolean
}) {
  return (
    <TabsList>
      {hasOrgTab && (
        <TabsTrigger value="members">
          <LuBuilding2 sx={stylexStyles.rule1} />
          Org Members
        </TabsTrigger>
      )}
      {isCloudProvider && (
        <TabsTrigger value="username">
          <LuUserPlus sx={stylexStyles.rule1} />
          Username
        </TabsTrigger>
      )}
      <TabsTrigger value="code">
        <LuQrCode sx={stylexStyles.rule1} />
        Code
      </TabsTrigger>
      <TabsTrigger value="link">
        <LuLink sx={stylexStyles.rule1} />
        Link
      </TabsTrigger>
    </TabsList>
  )
}

// AddUserDialog allows the space owner to share a space with other users.
export function AddUserDialog({
  open,
  onOpenChange,
  spaceId,
  isCloudProvider = false,
  orgId,
  orgMembers,
  orgMembersLoading,
}: AddUserDialogProps) {
  const session = useResourceValue(SessionContext.useContext())
  const flow = useInviteFlow(session, spaceId, isCloudProvider)
  const orgSource = useOrgMemberSource(orgId, orgMembers, orgMembersLoading)
  const { reset } = flow
  const defaultTab = defaultTabFor(orgSource.hasOrgTab, isCloudProvider)

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) reset()
      onOpenChange(next)
    },
    [onOpenChange, reset],
  )

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add User</DialogTitle>
          <DialogDescription>
            {isCloudProvider
              ? 'Add an existing org member, send a username invite, or create a shareable code or link.'
              : 'Add an existing org member, or create a shareable code or link.'}
          </DialogDescription>
        </DialogHeader>

        <Tabs defaultValue={defaultTab} key={defaultTab}>
          <AddUserTabList
            hasOrgTab={orgSource.hasOrgTab}
            isCloudProvider={isCloudProvider}
          />

          {orgSource.hasOrgTab && (
            <TabsContent value="members" variant="formTight">
              <OrgMembersTab
                session={session ?? null}
                spaceId={spaceId}
                members={orgSource.members}
                loading={orgSource.loading}
                isCloudProvider={isCloudProvider}
              />
            </TabsContent>
          )}

          <TabsContent value="code" variant="formTight">
            <InviteCodeTab flow={flow} />
          </TabsContent>

          {isCloudProvider && (
            <TabsContent value="username" variant="formTight">
              <InviteUsernameTab flow={flow} />
            </TabsContent>
          )}

          <TabsContent value="link" variant="form">
            <InviteLinkTab flow={flow} />
          </TabsContent>
        </Tabs>

        {flow.error && (
          <p role="alert" sx={stylexStyles2.rule12}>
            {flow.error}
          </p>
        )}
      </DialogContent>
    </Dialog>
  )
}

interface OrgMembersTabProps {
  session: Session | null
  spaceId: string
  members: OrgMemberInfo[]
  loading?: boolean
  isCloudProvider: boolean
}

// OrgMembersTab renders the org member picker with search and enrollment.
function OrgMembersTab({
  session,
  spaceId,
  members,
  loading,
  isCloudProvider,
}: OrgMembersTabProps) {
  // Keep search and enrollment feedback within the current member picker.
  const [search, setSearch] = useState('')
  const [enrolling, setEnrolling] = useState<string | null>(null)
  const [enrolled, setEnrolled] = useState<Set<string>>(new Set())
  const [enrollError, setEnrollError] = useState<string | null>(null)

  // Filter the supplied member snapshot by its visible identifiers.
  const filtered = useMemo(() => {
    if (!search) return members
    const q = search.toLowerCase()
    return members.filter(
      (m) =>
        m.entityId?.toLowerCase().includes(q) ||
        m.subjectId?.toLowerCase().includes(q) ||
        m.roleId?.toLowerCase().includes(q),
    )
  }, [members, search])

  // Enroll the selected account and retain each result in the member picker.
  const handleEnroll = useCallback(
    async (accountId: string) => {
      if (!session || enrolling) return
      setEnrolling(accountId)
      setEnrollError(null)
      try {
        const resp = await session.spacewave.enrollSpaceMember(
          spaceId,
          accountId,
          SOParticipantRole.SOParticipantRole_WRITER,
        )
        const results = resp.results ?? []
        if (results.length === 0) {
          throw new Error(
            `${accountId} has no active sessions. Use the Code or Link tab to invite them instead.`,
          )
        }
        const failed = results.find((r) => r.error)
        if (failed?.error) {
          throw new Error(failed.error)
        }
        const anyApplied = results.some(
          (r) => (r.enrolled ?? false) || (r.alreadyParticipant ?? false),
        )
        if (!anyApplied) {
          throw new Error('Failed to enroll member')
        }
        setEnrolled((prev) => new Set(prev).add(accountId))
      } catch (err) {
        const msg =
          err instanceof Error ? err.message : 'Failed to enroll member'
        if (msg.includes('no participant peers') || msg.includes('no active')) {
          setEnrollError(
            `${accountId} has no active sessions. Use the Code or Link tab to invite them instead.`,
          )
        } else {
          setEnrollError(msg)
        }
      } finally {
        setEnrolling(null)
      }
    },
    [session, spaceId, enrolling],
  )

  return (
    <>
      <p sx={[[stylexStyles3.rule3], tabsContentItemStyles.item]}>
        {isCloudProvider
          ? 'Choose someone already in this organization. Use Username, Code, or Link to share with anyone else.'
          : 'Choose someone already in this organization. Use Code or Link to share with anyone else.'}
      </p>
      <div sx={[[stylexStyles3.rule4], tabsContentItemStyles.item]}>
        <LuSearch sx={stylexStyles.rule2} />
        <input
          aria-label="Search org members"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search org members…"
          sx={[stylexStyles2.rule13]}
        />
      </div>
      <div sx={[stylexStyles4.rule1, tabsContentItemStyles.item]}>
        {loading && filtered.length === 0 && (
          <p sx={[nativeChildSpacingNext.s0_5, stylexStyles2.rule14]}>
            Loading org members…
          </p>
        )}
        {!loading && filtered.length === 0 && (
          <p sx={[nativeChildSpacingNext.s0_5, stylexStyles2.rule15]}>
            {search ? 'No matching members' : 'No org members'}
          </p>
        )}
        {filtered.map((member) => {
          // Derive the row label and action from this member's enrollment state.
          const accountId = member.subjectId ?? ''
          const primaryLabel = getOrgMemberPrimaryLabel(member)
          const secondaryLabel = getOrgMemberSecondaryLabel(member)
          const isEnrolled = enrolled.has(accountId)
          const isEnrolling = enrolling === accountId
          return (
            <button
              type="button"
              key={member.id}
              disabled={isEnrolled || isEnrolling || !session}
              onClick={() => void handleEnroll(accountId)}
              sx={[
                nativeChildSpacingNext.s0_5,
                isEnrolled ? stylexStyles2.rule16 : stylexStyles2.rule17,
              ]}
            >
              <div sx={stylexStyles2.rule18}>
                <div sx={stylexStyles2.rule19}>{primaryLabel}</div>
                {secondaryLabel && (
                  <div sx={stylexStyles2.rule20}>{secondaryLabel}</div>
                )}
                <div sx={stylexStyles2.rule21}>
                  {member.roleId === 'org:owner' ? 'Owner' : 'Member'}
                </div>
              </div>
              <div sx={stylexStyles2.rule22}>
                {isEnrolled ? (
                  <LuCheck sx={stylexStyles.rule3} />
                ) : isEnrolling ? (
                  <span sx={stylexStyles2.rule6}>Adding…</span>
                ) : (
                  <LuUserPlus sx={stylexStyles.rule4} />
                )}
              </div>
            </button>
          )
        })}
      </div>
      {enrollError && <p sx={stylexStyles2.rule12}>{enrollError}</p>}
    </>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
  s0_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 0.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 0.5 * var(--tw-space-y-reverse))`,
    },
  },
})
