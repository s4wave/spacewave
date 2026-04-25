import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ComponentType,
  type ReactNode,
} from 'react'
import {
  DebugInfo,
  DebugInfoProvider,
  useWatchStateRpc,
} from '@aptre/bldr-react'
import {
  SharedObjectHealthCommonReason,
  SharedObjectHealthLayer,
  SharedObjectHealthRemediationHint,
  SharedObjectHealthStatus,
  type SharedObjectHealth,
} from '@s4wave/core/sobject/sobject.pb.js'
import { AccountEscalationIntentKind } from '@s4wave/sdk/account/account.pb.js'
import {
  LuArrowRight,
  LuCircleAlert,
  LuRefreshCw,
  LuRotateCcw,
  LuShieldAlert,
  LuTriangleAlert,
} from 'react-icons/lu'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import { ORG_ROLE_OWNER } from '@s4wave/app/org/org-constants.js'
import {
  useNavigate,
  useParams,
  useParentPaths,
} from '@s4wave/web/router/router.js'
import {
  SessionContext,
  SharedObjectContext,
  SharedObjectBodyContext,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import {
  useResource,
  useResourceValue,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import type { OrganizationInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  MountSharedObjectRequest,
  MountSharedObjectResponse,
  WatchSharedObjectHealthRequest,
  WatchSharedObjectHealthResponse,
  WatchResourcesListRequest,
  WatchResourcesListResponse,
} from '@s4wave/sdk/session/session.pb.js'
import { SharedObjectBodyContainer } from '@s4wave/app/sobject/SharedObjectBodyContainer.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { useMountAccount } from '@s4wave/web/hooks/useMountAccount.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'

import { SessionFrame } from './SessionFrame.js'
import { AccountDashboardStateProvider } from './dashboard/AccountDashboardStateContext.js'
import { AuthConfirmDialog } from './dashboard/AuthConfirmDialog.js'

interface SharedObjectMutationPermission {
  canMutate: boolean
  disabledReason: string
}

type SharedObjectRemediationAction = 'repair' | 'reinitialize' | null

// isResourceBlockedError checks if an error indicates a DMCA-blocked resource.
function isResourceBlockedError(err: Error | null | undefined): boolean {
  if (!err) return false
  const msg = err.message || ''
  return msg.includes('resource is blocked') || msg.includes('dmca_blocked')
}

function isSharedObjectRecoveryCredentialError(
  err: Error | null | undefined,
): boolean {
  return (
    err?.message
      ?.toLowerCase()
      .includes('shared object recovery requires entity credentials') ?? false
  )
}

function buildHealthFromError(
  err: Error,
  layer: SharedObjectHealthLayer,
): SharedObjectHealth {
  const msg = err.message || 'unknown shared object error'
  const lower = msg.toLowerCase()
  const reason = getHealthReasonForError(lower)

  if (lower.includes('shared object not found')) {
    return {
      status: SharedObjectHealthStatus.CLOSED,
      layer,
      commonReason: SharedObjectHealthCommonReason.NOT_FOUND,
      remediationHint: SharedObjectHealthRemediationHint.CONTACT_OWNER,
      error: msg,
    }
  }
  if (lower.includes('not a participant') || lower.includes('access denied')) {
    return {
      status: SharedObjectHealthStatus.CLOSED,
      layer,
      commonReason: SharedObjectHealthCommonReason.ACCESS_REVOKED,
      remediationHint: SharedObjectHealthRemediationHint.REQUEST_ACCESS,
      error: msg,
    }
  }
  return {
    status: SharedObjectHealthStatus.CLOSED,
    layer,
    commonReason: reason.commonReason,
    remediationHint: reason.remediationHint,
    error: msg,
  }
}

function getHealthReasonForError(lower: string): {
  commonReason: SharedObjectHealthCommonReason
  remediationHint: SharedObjectHealthRemediationHint
} {
  if (lower.includes('block not found')) {
    return {
      commonReason: SharedObjectHealthCommonReason.BLOCK_NOT_FOUND,
      remediationHint: SharedObjectHealthRemediationHint.REPAIR_SOURCE_DATA,
    }
  }
  if (lower.includes('transform config')) {
    return {
      commonReason:
        SharedObjectHealthCommonReason.TRANSFORM_CONFIG_DECODE_FAILED,
      remediationHint: SharedObjectHealthRemediationHint.REPAIR_SOURCE_DATA,
    }
  }
  if (
    lower.includes('empty shared object body type') ||
    lower.includes('unsupported shared object type')
  ) {
    return {
      commonReason: SharedObjectHealthCommonReason.BODY_CONFIG_DECODE_FAILED,
      remediationHint: SharedObjectHealthRemediationHint.REPAIR_SOURCE_DATA,
    }
  }
  return {
    commonReason: SharedObjectHealthCommonReason.UNKNOWN,
    remediationHint: SharedObjectHealthRemediationHint.NONE,
  }
}

function buildLoadingHealth(
  layer: SharedObjectHealthLayer,
): SharedObjectHealth {
  return {
    status: SharedObjectHealthStatus.LOADING,
    layer,
    commonReason: SharedObjectHealthCommonReason.UNKNOWN,
    remediationHint: SharedObjectHealthRemediationHint.NONE,
    error: '',
  }
}

function getHealthSummary(health: SharedObjectHealth): {
  badge: string
  title: string
  description: string
  hint: string
} {
  if (health.status === SharedObjectHealthStatus.LOADING) {
    if (health.layer === SharedObjectHealthLayer.BODY) {
      return {
        badge: 'Loading',
        title: 'Mounting shared object body',
        description:
          'The shared object is available, and the body content is still loading.',
        hint: '',
      }
    }
    return {
      badge: 'Loading',
      title: 'Mounting shared object',
      description:
        'Checking availability and preparing the shared object for use.',
      hint: '',
    }
  }

  if (health.commonReason === SharedObjectHealthCommonReason.NOT_FOUND) {
    return {
      badge: 'Closed',
      title: 'Shared object not found',
      description:
        'This shared object is no longer available from the current account or provider.',
      hint: 'Ask the owner for an updated link or confirm the object still exists.',
    }
  }
  if (health.commonReason === SharedObjectHealthCommonReason.ACCESS_REVOKED) {
    return {
      badge: 'Closed',
      title: 'Access revoked',
      description:
        'The current session is no longer allowed to read this shared object.',
      hint: 'Request access again or confirm the correct account is open.',
    }
  }
  if (
    health.commonReason ===
    SharedObjectHealthCommonReason.INITIAL_STATE_REJECTED
  ) {
    return {
      badge: 'Closed',
      title: 'Initial state rejected',
      description:
        'The shared object state failed verification, so Alpha closed the mount instead of retrying indefinitely.',
      hint: 'The owner needs to repair or republish the shared object state.',
    }
  }
  if (health.commonReason === SharedObjectHealthCommonReason.BLOCK_NOT_FOUND) {
    return {
      badge: 'Closed',
      title: 'Required block missing',
      description:
        'A block required to mount this shared object could not be found.',
      hint: 'Retry if the data may still be syncing; otherwise the source data needs repair.',
    }
  }
  if (
    health.commonReason ===
    SharedObjectHealthCommonReason.TRANSFORM_CONFIG_DECODE_FAILED
  ) {
    return {
      badge: 'Closed',
      title: 'Transform configuration invalid',
      description:
        'Alpha could not decode the transform configuration needed to read this content.',
      hint: 'The shared object data needs repair before it can be opened.',
    }
  }
  if (
    health.commonReason ===
    SharedObjectHealthCommonReason.BODY_CONFIG_DECODE_FAILED
  ) {
    return {
      badge: 'Closed',
      title: 'Body configuration invalid',
      description:
        'The shared object body metadata could not be decoded into a supported view.',
      hint: 'The body metadata needs repair or a compatible viewer.',
    }
  }
  if (health.status === SharedObjectHealthStatus.DEGRADED) {
    return {
      badge: 'Degraded',
      title: 'Shared object degraded',
      description:
        'The shared object is partially available, but Alpha detected a recoverable problem.',
      hint: '',
    }
  }
  return {
    badge: 'Closed',
    title:
      health.layer === SharedObjectHealthLayer.BODY ?
        'Shared object body failed'
      : 'Shared object unavailable',
    description:
      health.layer === SharedObjectHealthLayer.BODY ?
        'The shared object opened, but the body content could not be mounted.'
      : 'Alpha could not mount this shared object.',
    hint: '',
  }
}

function getSharedObjectMutationPermission(
  sharedObjectId: string,
  resourcesList: WatchResourcesListResponse | null,
  organizations: OrganizationInfo[],
  organizationsLoading: boolean,
): SharedObjectMutationPermission {
  const org = organizations.find(
    (org) => !!org.id && (org.spaceIds?.includes(sharedObjectId) ?? false),
  )
  if (org) {
    if (org.role === ORG_ROLE_OWNER) {
      return { canMutate: true, disabledReason: '' }
    }
    return {
      canMutate: false,
      disabledReason:
        'Only organization owners can repair or reinitialize this shared object.',
    }
  }

  const spaceEntry = resourcesList?.spacesList?.find(
    (entry) => entry.entry?.ref?.providerResourceRef?.id === sharedObjectId,
  )
  if (spaceEntry?.entry?.source === 'created') {
    return { canMutate: true, disabledReason: '' }
  }

  if (organizationsLoading || !resourcesList) {
    return {
      canMutate: false,
      disabledReason:
        'Alpha is still checking whether this account can repair this shared object.',
    }
  }

  return {
    canMutate: false,
    disabledReason:
      'Only the shared object owner can repair or reinitialize this shared object.',
  }
}

function getSharedObjectHealthTone(
  isLoading: boolean,
  isDegraded: boolean,
): {
  cardBorder: string
  cardBackground: string
  iconWrap: string
  iconColor: string
  badgeTone: string
  Icon: ComponentType<{ className?: string }>
} {
  if (isLoading) {
    return {
      cardBorder: 'border-foreground/8',
      cardBackground: 'bg-background-card/30',
      iconWrap: 'bg-foreground/5',
      iconColor: 'text-foreground',
      badgeTone: 'border-foreground/10 bg-foreground/5 text-foreground-alt/70',
      Icon: LuCircleAlert,
    }
  }
  if (isDegraded) {
    return {
      cardBorder: 'border-warning/20',
      cardBackground: 'bg-warning/5',
      iconWrap: 'bg-warning/10',
      iconColor: 'text-warning',
      badgeTone: 'border-warning/20 bg-warning/10 text-warning',
      Icon: LuTriangleAlert,
    }
  }
  return {
    cardBorder: 'border-destructive/20',
    cardBackground: 'bg-destructive/5',
    iconWrap: 'bg-destructive/10',
    iconColor: 'text-destructive',
    badgeTone: 'border-destructive/20 bg-destructive/10 text-destructive',
    Icon: LuShieldAlert,
  }
}

function RemediationActionButton({
  icon,
  label,
  onClick,
  disabledReason,
  disabled = false,
  active,
  className = '',
}: {
  icon: ReactNode
  label: string
  onClick: () => void
  disabledReason: string
  disabled?: boolean
  active?: boolean
  className?: string
}) {
  const button = (
    <DashboardButton
      icon={icon}
      onClick={onClick}
      disabled={disabled || !!disabledReason}
      className={
        (active ?
          'border-foreground/15 bg-foreground/8 text-foreground '
        : '') + className
      }
    >
      {label}
    </DashboardButton>
  )
  if (!disabledReason) {
    return button
  }
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex">{button}</span>
      </TooltipTrigger>
      <TooltipContent side="top" className="max-w-xs">
        {disabledReason}
      </TooltipContent>
    </Tooltip>
  )
}

function SharedObjectHealthCard({
  health,
  onRetry,
  onRepair,
  onReinitialize,
  onBack,
  mutationPermission,
  mutationPending,
  mutationError,
}: {
  health: SharedObjectHealth
  onRetry: () => void
  onRepair: () => void
  onReinitialize: () => void
  onBack: () => void
  mutationPermission: SharedObjectMutationPermission
  mutationPending: boolean
  mutationError: string
}) {
  const summary = getHealthSummary(health)
  const isLoading = health.status === SharedObjectHealthStatus.LOADING
  const isDegraded = health.status === SharedObjectHealthStatus.DEGRADED
  const [selectedAction, setSelectedAction] =
    useState<SharedObjectRemediationAction>(null)
  const [confirmingReinitialize, setConfirmingReinitialize] = useState(false)
  const tone = getSharedObjectHealthTone(isLoading, isDegraded)

  const detail = health.error?.trim() ?? ''
  const showRetry =
    health.remediationHint === SharedObjectHealthRemediationHint.RETRY ||
    isLoading

  const layerLabel =
    health.layer === SharedObjectHealthLayer.BODY ? 'Body' : 'Shared Object'

  return (
    <div className="relative flex h-full w-full items-start justify-center overflow-auto px-4 py-12">
      <BackButton floating onClick={onBack}>
        Back
      </BackButton>
      <div className="flex w-full max-w-xl flex-col gap-4">
        <div
          className={cn(
            'rounded-xl border p-5 backdrop-blur-sm',
            tone.cardBackground,
            tone.cardBorder,
          )}
        >
          <div className="flex flex-col items-center gap-3 text-center">
            <div
              className={cn(
                'flex h-12 w-12 shrink-0 items-center justify-center rounded-full',
                tone.iconWrap,
              )}
            >
              {isLoading ?
                <Spinner size="lg" className={tone.iconColor} />
              : <tone.Icon className={cn('h-6 w-6', tone.iconColor)} />}
            </div>
            <span
              className={cn(
                'rounded-full border px-2 py-0.5 text-[0.55rem] font-semibold tracking-widest uppercase select-none',
                tone.badgeTone,
              )}
            >
              {summary.badge} - {layerLabel}
            </span>
            <h1 className="text-foreground text-base font-semibold tracking-tight">
              {summary.title}
            </h1>
            <p className="text-foreground-alt/70 max-w-sm text-xs leading-relaxed">
              {summary.description}
            </p>
          </div>

          {isLoading ?
            showRetry ?
              <div className="mt-4 flex justify-center">
                <DashboardButton
                  icon={<LuRefreshCw className="h-3.5 w-3.5" />}
                  onClick={onRetry}
                >
                  Retry
                </DashboardButton>
              </div>
            : null
          : <div className="mt-5 space-y-3">
              <div className="border-foreground/8 bg-background-card/30 rounded-lg border p-3">
                <div className="flex items-center gap-1.5">
                  <LuCircleAlert className="text-foreground-alt/60 h-3.5 w-3.5" />
                  <span className="text-foreground text-xs font-medium select-none">
                    Issue
                  </span>
                </div>
                <p className="text-foreground-alt/70 mt-1.5 text-xs leading-relaxed">
                  {summary.hint ||
                    'Review the issue details below before choosing the next step.'}
                </p>
                {detail ?
                  <div className="border-foreground/8 bg-foreground/5 text-foreground-alt/80 mt-2.5 rounded-md border px-2.5 py-1.5 text-[0.7rem] leading-relaxed break-words whitespace-pre-wrap">
                    {detail}
                  </div>
                : null}
              </div>

              <div className="border-foreground/8 bg-background-card/30 rounded-lg border p-3">
                <div className="flex items-center gap-1.5">
                  <LuArrowRight className="text-foreground-alt/60 h-3.5 w-3.5" />
                  <span className="text-foreground text-xs font-medium select-none">
                    Next step
                  </span>
                </div>
                <p className="text-foreground-alt/70 mt-1.5 text-xs leading-relaxed">
                  {selectedAction === 'repair' ?
                    'Repair is non-destructive. It reuses the normal recovery path and keeps the current shared object identity intact.'
                  : selectedAction === 'reinitialize' ?
                    'Reinitialize is destructive. It rewrites the broken shared object in place on the same shared object id and canonical URL.'
                  : mutationPermission.canMutate ?
                    'Choose Repair to retry recovery, or Reinitialize to rewrite the shared object in place. You can also go back and decide later.'
                  : 'You do not have permission to repair or reinitialize this shared object from the current account. The action set stays visible here so the owner can recover it without losing this route.'
                  }
                </p>
                <div className="mt-3 flex flex-wrap gap-2">
                  {showRetry ?
                    <DashboardButton
                      icon={<LuRotateCcw className="h-3.5 w-3.5" />}
                      onClick={onRetry}
                    >
                      Retry
                    </DashboardButton>
                  : null}
                  <RemediationActionButton
                    icon={<LuRefreshCw className="h-3.5 w-3.5" />}
                    label="Repair"
                    onClick={() => {
                      setConfirmingReinitialize(false)
                      setSelectedAction('repair')
                      onRepair()
                    }}
                    disabledReason={
                      mutationPermission.canMutate ? '' : (
                        mutationPermission.disabledReason
                      )
                    }
                    disabled={mutationPending}
                    active={selectedAction === 'repair'}
                  />
                  <RemediationActionButton
                    icon={<LuShieldAlert className="h-3.5 w-3.5" />}
                    label="Reinitialize"
                    onClick={() => setConfirmingReinitialize(true)}
                    disabledReason={
                      mutationPermission.canMutate ? '' : (
                        mutationPermission.disabledReason
                      )
                    }
                    disabled={mutationPending}
                    active={selectedAction === 'reinitialize'}
                    className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                  />
                </div>
                {confirmingReinitialize ?
                  <div className="border-destructive/20 bg-destructive/5 mt-3 rounded-md border p-3">
                    <div className="flex items-center gap-1.5">
                      <LuShieldAlert className="text-destructive h-3.5 w-3.5" />
                      <span className="text-destructive text-xs font-medium select-none">
                        Confirm reinitialize
                      </span>
                    </div>
                    <p className="text-foreground-alt/70 mt-1.5 text-xs leading-relaxed">
                      Reinitialize is destructive. It rewrites this shared
                      object in place on the same shared object id and URL. Use
                      repair first when you want Alpha to retry the normal
                      recovery path without discarding the current state.
                    </p>
                    <div className="mt-3 flex flex-wrap gap-2">
                      <DashboardButton
                        icon={<LuRotateCcw className="h-3.5 w-3.5" />}
                        onClick={() => setConfirmingReinitialize(false)}
                      >
                        Cancel
                      </DashboardButton>
                      <DashboardButton
                        icon={<LuShieldAlert className="h-3.5 w-3.5" />}
                        onClick={() => {
                          setSelectedAction('reinitialize')
                          setConfirmingReinitialize(false)
                          onReinitialize()
                        }}
                        disabled={mutationPending}
                        className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                      >
                        Confirm reinitialize
                      </DashboardButton>
                    </div>
                  </div>
                : null}
                {selectedAction ?
                  <p className="text-foreground-alt/55 mt-2.5 text-[0.7rem]">
                    {selectedAction === 'repair' ?
                      'Repair is selected for this broken shared object.'
                    : 'Reinitialize is selected for this broken shared object.'}
                  </p>
                : null}
                {mutationError ?
                  <p className="text-destructive mt-2.5 text-[0.7rem]">
                    {mutationError}
                  </p>
                : null}
              </div>
            </div>
          }
        </div>
      </div>
    </div>
  )
}

// SessionSharedObjectContainer displays a shared object.
export function SessionSharedObjectContainer() {
  const params = useParams()
  const sharedObjectId = params['sharedObjectId'] ?? ''
  const navigate = useNavigate()
  const navigateSession = useSessionNavigate()
  const session = SessionContext.useContext()
  const sessionValue = useResourceValue(session)
  const { providerId, accountId } = useSessionInfo(sessionValue)
  const accountResource = useMountAccount(providerId, accountId)
  const dmcaHref = useStaticHref('/dmca')
  const parentPaths = useParentPaths()
  const orgListCtx = SpacewaveOrgListContext.useContextSafe()

  // Redirect legacy /u/:idx/so/:spaceId to /u/:idx/org/:orgId/so/:spaceId
  // when the space is org-owned. Skip when already nested under /org/.
  const orgRedirectId = useMemo(() => {
    if (!sharedObjectId) return ''
    const underOrg = parentPaths.some((p) => p.includes('/org/'))
    if (underOrg) return ''
    const orgs = orgListCtx?.organizations ?? []
    for (const org of orgs) {
      if (!org.id || !org.spaceIds) continue
      if (org.spaceIds.includes(sharedObjectId)) return org.id
    }
    return ''
  }, [orgListCtx, parentPaths, sharedObjectId])

  useEffect(() => {
    if (!orgRedirectId) return
    navigateSession({
      path: `org/${orgRedirectId}/so/${sharedObjectId}`,
      replace: true,
    })
  }, [navigateSession, orgRedirectId, sharedObjectId])

  const resourcesList = useWatchStateRpc(
    useCallback(
      (req: WatchResourcesListRequest, signal: AbortSignal) =>
        sessionValue?.watchResourcesList(req, signal) ?? null,
      [sessionValue],
    ),
    {},
    WatchResourcesListRequest.equals,
    WatchResourcesListResponse.equals,
  )

  const sharedObjectHealthResp = useWatchStateRpc(
    useCallback(
      (req: WatchSharedObjectHealthRequest, signal: AbortSignal) =>
        sessionValue?.watchSharedObjectHealth(req, signal) ?? null,
      [sessionValue],
    ),
    { sharedObjectId },
    WatchSharedObjectHealthRequest.equals,
    WatchSharedObjectHealthResponse.equals,
  )

  const sharedObjectResource = useResource(
    session,
    async (session, signal, cleanup) => {
      if (!session || !sharedObjectId) {
        return null
      }

      const req: MountSharedObjectRequest = { sharedObjectId }
      const result = await session.mountSharedObject(req, signal)
      if (!result) {
        console.warn(
          'mount shared object returned not found, redirecting to session',
          req,
        )
        queueMicrotask(() => navigateSession({ path: '', replace: true }))
        return null
      }

      return cleanup(result)
    },
    // The mounted shared object is keyed by session + sharedObjectId.
    // Including navigation callbacks here causes path-only route changes to
    // reload the mount because the outer shell router recreates navigate
    // functions as the current path changes.
    [sharedObjectId],
  )

  const sharedObjectBodyResource = useResource(
    sharedObjectResource,
    async (sobject, signal, cleanup) =>
      sobject ? cleanup(await sobject.mountSharedObjectBody({}, signal)) : null,
    [],
  )

  const shouldRedirectMissingSpace = useMemo(
    () =>
      !!sharedObjectResource.value &&
      !!resourcesList &&
      !(
        resourcesList.spacesList?.some(
          (entry) =>
            entry.entry?.ref?.providerResourceRef?.id === sharedObjectId,
        ) ?? false
      ),
    [sharedObjectResource.value, resourcesList, sharedObjectId],
  )

  const debugInfo = (
    <DebugInfo>
      Shared Object ID: {sharedObjectId}
      <br />
      Loading: {sharedObjectResource.loading.toString()}
      <br />
      Shared object loaded: {(!!sharedObjectResource.value).toString()}
      <br />
      Error: {sharedObjectResource.error?.toString() ?? 'none'}
      <br />
      Meta:{' '}
      <pre>
        {sharedObjectResource.value ?
          JSON.stringify(
            MountSharedObjectResponse.toJson(sharedObjectResource.value.meta),
            null,
            4,
          )
        : 'none'}
      </pre>
      Shared object body loaded: {(!!sharedObjectBodyResource.value).toString()}
      <br />
      Body loading: {sharedObjectBodyResource.loading.toString()}
      <br />
      Body error: {sharedObjectBodyResource.error?.toString() ?? 'none'}
    </DebugInfo>
  )

  const resourceError =
    sharedObjectResource.error ?? sharedObjectBodyResource.error

  const isBlocked = useMemo(
    () => isResourceBlockedError(resourceError),
    [resourceError],
  )

  const activeHealth = useMemo(() => {
    if (sharedObjectBodyResource.error) {
      return buildHealthFromError(
        sharedObjectBodyResource.error,
        SharedObjectHealthLayer.BODY,
      )
    }
    if (sharedObjectResource.value && sharedObjectBodyResource.loading) {
      return buildLoadingHealth(SharedObjectHealthLayer.BODY)
    }
    if (sharedObjectHealthResp?.health) {
      return sharedObjectHealthResp.health
    }
    if (sharedObjectResource.error) {
      return buildHealthFromError(
        sharedObjectResource.error,
        SharedObjectHealthLayer.SHARED_OBJECT,
      )
    }
    return null
  }, [
    sharedObjectBodyResource.error,
    sharedObjectBodyResource.loading,
    sharedObjectHealthResp,
    sharedObjectResource.error,
    sharedObjectResource.value,
  ])

  if (shouldRedirectMissingSpace) {
    queueMicrotask(() => navigateSession({ path: '', replace: true }))
  }

  const handleRetry = useCallback(() => {
    sharedObjectResource.retry()
    sharedObjectBodyResource.retry()
  }, [sharedObjectBodyResource, sharedObjectResource])

  const handleBack = useCallback(() => {
    navigate({ path: '../' })
  }, [navigate])

  const [mutationPending, setMutationPending] = useState(false)
  const [mutationError, setMutationError] = useState('')
  const [credentialRepairOpen, setCredentialRepairOpen] = useState(false)

  const runRepairAction = useCallback(
    async (kind: SharedObjectRemediationAction) => {
      if (!sharedObjectId || !sessionValue || mutationPending) {
        return
      }
      setMutationPending(true)
      setMutationError('')
      try {
        if (kind === 'repair') {
          await sessionValue.spacewave.repairSharedObject(sharedObjectId)
        } else if (kind === 'reinitialize') {
          await sessionValue.spacewave.reinitializeSharedObject(sharedObjectId)
        }
        handleRetry()
      } catch (err) {
        if (
          kind === 'repair' &&
          providerId === 'spacewave' &&
          !!accountId &&
          isSharedObjectRecoveryCredentialError(
            err instanceof Error ? err : undefined,
          )
        ) {
          setCredentialRepairOpen(true)
          return
        }
        setMutationError(err instanceof Error ? err.message : 'Action failed')
      } finally {
        setMutationPending(false)
      }
    },
    [
      accountId,
      handleRetry,
      mutationPending,
      providerId,
      sessionValue,
      sharedObjectId,
    ],
  )

  const handleCredentialRepairConfirm = useCallback(async () => {
    if (!sharedObjectId || !sessionValue) {
      return
    }
    setMutationPending(true)
    setMutationError('')
    try {
      await sessionValue.spacewave.repairSharedObject(sharedObjectId)
      setCredentialRepairOpen(false)
      handleRetry()
    } catch (err) {
      setMutationError(err instanceof Error ? err.message : 'Action failed')
      throw err
    } finally {
      setMutationPending(false)
    }
  }, [handleRetry, sessionValue, sharedObjectId])

  const mutationPermission = useMemo(
    () =>
      getSharedObjectMutationPermission(
        sharedObjectId,
        resourcesList,
        orgListCtx?.organizations ?? [],
        orgListCtx?.loading ?? false,
      ),
    [
      orgListCtx?.loading,
      orgListCtx?.organizations,
      resourcesList,
      sharedObjectId,
    ],
  )

  const body = (
    <>
      {debugInfo}
      {sharedObjectResource.value && sharedObjectBodyResource.value ?
        <SharedObjectBodyContainer />
      : isBlocked ?
        <ErrorState
          variant="fullscreen"
          title="Content Unavailable"
          message="This content has been disabled due to a DMCA takedown notice. If you believe this is an error, you can file a counter-notice."
          onRetry={handleRetry}
        >
          <a
            href={dmcaHref}
            className="text-foreground-alt hover:text-foreground mt-2 text-sm underline"
          >
            DMCA Policy
          </a>
        </ErrorState>
      : resourceError ?
        <SharedObjectHealthCard
          health={
            activeHealth ??
            buildHealthFromError(
              resourceError,
              SharedObjectHealthLayer.SHARED_OBJECT,
            )
          }
          onRetry={handleRetry}
          onRepair={() => void runRepairAction('repair')}
          onReinitialize={() => void runRepairAction('reinitialize')}
          onBack={handleBack}
          mutationPermission={mutationPermission}
          mutationPending={mutationPending}
          mutationError={mutationError}
        />
      : activeHealth ?
        <SharedObjectHealthCard
          health={activeHealth}
          onRetry={handleRetry}
          onRepair={() => void runRepairAction('repair')}
          onReinitialize={() => void runRepairAction('reinitialize')}
          onBack={handleBack}
          mutationPermission={mutationPermission}
          mutationPending={mutationPending}
          mutationError={mutationError}
        />
      : <LoadingCard
          view={{ state: 'loading', title: 'Loading shared object' }}
        />
      }
      {credentialRepairOpen ?
        <AccountDashboardStateProvider account={accountResource}>
          <AuthConfirmDialog
            open={credentialRepairOpen}
            onOpenChange={setCredentialRepairOpen}
            title="Unlock shared object recovery"
            description="Unlock an account key to grant this session access to the shared object."
            confirmLabel="Repair shared object"
            intent={{
              kind: AccountEscalationIntentKind.AccountEscalationIntentKind_ACCOUNT_ESCALATION_INTENT_KIND_UNSPECIFIED,
              title: 'Unlock shared object recovery',
              description:
                'Unlock an account key to grant this session access to the shared object.',
            }}
            onConfirm={handleCredentialRepairConfirm}
            account={accountResource}
            retainAfterClose
          />
        </AccountDashboardStateProvider>
      : null}
    </>
  )

  return (
    <SharedObjectContext.Provider resource={sharedObjectResource}>
      <SharedObjectBodyContext.Provider resource={sharedObjectBodyResource}>
        <DebugInfoProvider>
          <SessionFrame>{body}</SessionFrame>
        </DebugInfoProvider>
      </SharedObjectBodyContext.Provider>
    </SharedObjectContext.Provider>
  )
}
