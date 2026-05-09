import { useCallback, useId, useMemo, useReducer, useState } from 'react'
import {
  LuBuilding2,
  LuCheck,
  LuCopy,
  LuLink,
  LuQrCode,
  LuSearch,
  LuUserPlus,
} from 'react-icons/lu'

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
import { cn } from '@s4wave/web/style/utils.js'
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
  TabsList,
  TabsTrigger,
} from '@s4wave/web/ui/tabs.js'

import { base58Encode } from '@s4wave/app/provider/spacewave/keypair-utils.js'
import { buildInviteLink } from '@s4wave/app/urls.js'
import { useCloudProviderConfig } from '@s4wave/app/provider/spacewave/useSpacewaveAuth.js'

export interface AddUserDialogProps {
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
      return { ...state, copied: true }
    case 'uncopied':
      return { ...state, copied: false }
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
  const spaceContainer = SpaceContainerContext.useContextSafe()
  const cloudProviderConfig = useCloudProviderConfig()
  const [state, dispatch] = useReducer(reducer, initialState)
  const inviteCodeId = useId()
  const usernameInputId = useId()
  const inviteLinkId = useId()

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) dispatch({ type: 'reset' })
      onOpenChange(next)
    },
    [onOpenChange],
  )

  const handleCreateInvite = useCallback(async () => {
    if (!session) return
    dispatch({ type: 'creating' })
    try {
      const resp = await session.createSpaceInvite(
        spaceId,
        SOParticipantRole.SOParticipantRole_WRITER,
      )
      dispatch({ type: 'created', resp })
    } catch (err) {
      dispatch({
        type: 'error',
        message: err instanceof Error ? err.message : 'Failed to create invite',
      })
    }
  }, [session, spaceId])

  const handleSendUsernameInvite = useCallback(async () => {
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

  const shortCode = state.inviteResp?.shortCode ?? ''

  const inviteLink = useMemo(() => {
    if (!state.inviteResp?.inviteMessage) return ''
    const encoded = base58Encode(
      SOInviteMessage.toBinary(state.inviteResp.inviteMessage),
    )
    return buildInviteLink(cloudProviderConfig?.publicBaseUrl, encoded)
  }, [state.inviteResp, cloudProviderConfig?.publicBaseUrl])

  const handleCopy = useCallback((text: string) => {
    void navigator.clipboard.writeText(text)
    dispatch({ type: 'copied' })
    setTimeout(() => dispatch({ type: 'uncopied' }), 2000)
  }, [])

  const effectiveOrgId = orgId ?? ''
  const effectiveOrgMembers =
    orgMembers ?? spaceContainer?.orgState?.members ?? []
  const effectiveOrgMembersLoading =
    orgMembersLoading ?? (!!effectiveOrgId && !spaceContainer?.orgState)
  const hasOrgTab = effectiveOrgId.length > 0

  const defaultTab =
    hasOrgTab ? 'members'
    : isCloudProvider ? 'username'
    : 'code'

  const inputClass = cn(
    'border-foreground/10 bg-background-card/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm font-mono outline-none transition-colors duration-150',
    'focus:border-foreground/30',
  )
  const actionButtonClass = cn(
    'border-foreground/10 bg-background-card/20 flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm transition-all duration-150',
    'hover:border-foreground/12 hover:bg-background-card/50',
    'disabled:cursor-not-allowed disabled:opacity-50',
  )
  const successActionClass = cn(
    'border-success/30 bg-success/5 text-success',
    'hover:border-success/40 hover:bg-success/10',
  )

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add User</DialogTitle>
          <DialogDescription>
            {isCloudProvider ?
              'Add an existing org member, send a username invite, or create a shareable code or link.'
            : 'Add an existing org member, or create a shareable code or link.'}
          </DialogDescription>
        </DialogHeader>

        <Tabs defaultValue={defaultTab} key={defaultTab}>
          <TabsList>
            {hasOrgTab && (
              <TabsTrigger value="members">
                <LuBuilding2 className="size-3.5" />
                Org Members
              </TabsTrigger>
            )}
            {isCloudProvider && (
              <TabsTrigger value="username">
                <LuUserPlus className="size-3.5" />
                Username
              </TabsTrigger>
            )}
            <TabsTrigger value="code">
              <LuQrCode className="size-3.5" />
              Code
            </TabsTrigger>
            <TabsTrigger value="link">
              <LuLink className="size-3.5" />
              Link
            </TabsTrigger>
          </TabsList>

          {hasOrgTab && (
            <TabsContent value="members" className="space-y-3 pt-2">
              <OrgMembersTab
                session={session ?? null}
                spaceId={spaceId}
                members={effectiveOrgMembers}
                loading={effectiveOrgMembersLoading}
                isCloudProvider={isCloudProvider}
              />
            </TabsContent>
          )}

          <TabsContent value="code" className="space-y-3 pt-2">
            {shortCode ?
              <div className="space-y-2">
                <label
                  htmlFor={inviteCodeId}
                  className="text-foreground-alt mb-1.5 block text-xs select-none"
                >
                  Invite code
                </label>
                <input
                  id={inviteCodeId}
                  value={shortCode}
                  readOnly
                  className={cn(
                    inputClass,
                    'text-center text-lg tracking-widest',
                  )}
                  onClick={(e) => (e.target as HTMLInputElement).select()}
                />
                <button
                  onClick={() => handleCopy(shortCode)}
                  className={cn(
                    actionButtonClass,
                    state.copied && successActionClass,
                  )}
                >
                  {state.copied ?
                    <>
                      <LuCheck className="size-3.5" />
                      Copied
                    </>
                  : <>
                      <LuCopy className="size-3.5" />
                      Copy Code
                    </>
                  }
                </button>
              </div>
            : state.inviteResp ?
              <p className="text-foreground-alt text-xs">
                Short codes are only available for cloud sessions. Use the Link
                tab instead.
              </p>
            : <button
                onClick={() => void handleCreateInvite()}
                disabled={state.creating || !session}
                className={actionButtonClass}
              >
                {state.creating ? 'Creating...' : 'Create Invite Code'}
              </button>
            }
          </TabsContent>

          {isCloudProvider && (
            <TabsContent value="username" className="space-y-3 pt-2">
              <div className="space-y-2">
                <label
                  htmlFor={usernameInputId}
                  className="text-foreground-alt mb-1.5 block text-xs select-none"
                >
                  Spacewave username
                </label>
                <input
                  id={usernameInputId}
                  value={state.username}
                  onChange={(e) =>
                    dispatch({ type: 'username', value: e.target.value })
                  }
                  placeholder="alice"
                  className={inputClass}
                />
                <button
                  onClick={() => void handleSendUsernameInvite()}
                  disabled={
                    state.creating || !session || !state.username.trim()
                  }
                  className={cn(
                    actionButtonClass,
                    state.usernameSent && successActionClass,
                  )}
                >
                  {state.usernameSent ?
                    <>
                      <LuCheck className="size-3.5" />
                      Sent
                    </>
                  : <>
                      <LuUserPlus className="size-3.5" />
                      {state.creating ? 'Sending...' : 'Send Invite'}
                    </>
                  }
                </button>
              </div>
            </TabsContent>
          )}

          <TabsContent value="link" className="space-y-3 pt-2">
            {state.inviteResp?.inviteMessage ?
              <div className="space-y-2">
                <label
                  htmlFor={inviteLinkId}
                  className="text-foreground-alt mb-1.5 block text-xs select-none"
                >
                  Invite link
                </label>
                <input
                  id={inviteLinkId}
                  value={inviteLink}
                  readOnly
                  className={inputClass}
                  onClick={(e) => (e.target as HTMLInputElement).select()}
                />
                <button
                  onClick={() => handleCopy(inviteLink)}
                  className={cn(
                    actionButtonClass,
                    state.copied && successActionClass,
                  )}
                >
                  {state.copied ?
                    <>
                      <LuCheck className="size-3.5" />
                      Copied
                    </>
                  : <>
                      <LuCopy className="size-3.5" />
                      Copy Link
                    </>
                  }
                </button>
              </div>
            : <button
                onClick={() => void handleCreateInvite()}
                disabled={state.creating || !session}
                className={actionButtonClass}
              >
                {state.creating ? 'Creating...' : 'Create Invite Link'}
              </button>
            }
          </TabsContent>
        </Tabs>

        {state.error && (
          <p className="text-destructive text-xs">{state.error}</p>
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
  const [search, setSearch] = useState('')
  const [enrolling, setEnrolling] = useState<string | null>(null)
  const [enrolled, setEnrolled] = useState<Set<string>>(new Set())
  const [enrollError, setEnrollError] = useState<string | null>(null)

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
      <p className="text-foreground-alt/60 text-xs">
        {isCloudProvider ?
          'Choose someone already in this organization. Use Username, Code, or Link to share with anyone else.'
        : 'Choose someone already in this organization. Use Code or Link to share with anyone else.'
        }
      </p>
      <div className="relative">
        <LuSearch className="text-foreground-alt/50 absolute top-2.5 left-2.5 size-3.5" />
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search org members..."
          className={cn(
            'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border py-2 pr-3 pl-8 text-sm transition-colors outline-none',
            'focus:border-foreground/40',
          )}
        />
      </div>
      <div className="max-h-48 space-y-0.5 overflow-y-auto">
        {loading && filtered.length === 0 && (
          <p className="text-foreground-alt/50 py-2 text-center text-xs">
            Loading org members…
          </p>
        )}
        {!loading && filtered.length === 0 && (
          <p className="text-foreground-alt/40 py-2 text-center text-xs">
            {search ? 'No matching members' : 'No org members'}
          </p>
        )}
        {filtered.map((member) => {
          const accountId = member.subjectId ?? ''
          const primaryLabel = getOrgMemberPrimaryLabel(member)
          const secondaryLabel = getOrgMemberSecondaryLabel(member)
          const isEnrolled = enrolled.has(accountId)
          const isEnrolling = enrolling === accountId
          return (
            <button
              key={member.id}
              disabled={isEnrolled || isEnrolling || !session}
              onClick={() => void handleEnroll(accountId)}
              className={cn(
                'flex w-full items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left transition-colors',
                'hover:bg-foreground/5 disabled:cursor-default disabled:opacity-60',
                isEnrolled && 'bg-success/5',
              )}
            >
              <div className="min-w-0 flex-1">
                <div className="text-foreground truncate text-xs font-medium">
                  {primaryLabel}
                </div>
                {secondaryLabel && (
                  <div className="text-foreground-alt/50 truncate font-mono text-[10px]">
                    {secondaryLabel}
                  </div>
                )}
                <div className="text-foreground-alt/50 text-[10px]">
                  {member.roleId === 'org:owner' ? 'Owner' : 'Member'}
                </div>
              </div>
              <div className="shrink-0">
                {isEnrolled ?
                  <LuCheck className="text-success size-3.5" />
                : isEnrolling ?
                  <span className="text-foreground-alt text-xs">Adding…</span>
                : <LuUserPlus className="text-foreground-alt/50 size-3.5" />}
              </div>
            </button>
          )
        })}
      </div>
      {enrollError && <p className="text-destructive text-xs">{enrollError}</p>}
    </>
  )
}
