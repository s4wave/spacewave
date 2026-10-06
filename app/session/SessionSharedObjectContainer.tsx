import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'
import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ComponentProps,
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

import { ORG_ROLE_OWNER } from '@s4wave/app/org/org-constants.js'
import { SpaceMountingScreen } from '@s4wave/app/space/SpaceMountingScreen.js'
import { spaceMountStageFromHealth } from '@s4wave/app/space/spaceMountStage.js'
import { isStorageQuotaError } from '@s4wave/app/session/storage/storage-error.js'
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
  useSessionIndex,
} from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import {
  useResource,
  useResourceValue,
  type Resource,
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
import type { Session } from '@s4wave/sdk/session/session.js'
import { SharedObjectBodyContainer } from '@s4wave/app/sobject/SharedObjectBodyContainer.js'
import { SharedObjectSyncNotice } from '@s4wave/app/sobject/SharedObjectSyncNotice.js'
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
import { useSessionSelfEnrollmentStatus } from './SessionSelfEnrollmentStatusContext.js'
import {
  buildSharedObjectFallbackHealth,
  getSharedObjectRouteHealth,
} from './sharedObjectHealthFallback.js'
import {
  clearQuickstartSharedObjectHandoffAwaitingResourcesList,
  consumeQuickstartSharedObjectBodyHandoff,
  consumeQuickstartSharedObjectHandoff,
  hasQuickstartSharedObjectHandoff,
  isQuickstartSharedObjectHandoffAwaitingResourcesList,
  markQuickstartSharedObjectHandoffAwaitingResourcesList,
  releaseQuickstartSharedObjectHandoff,
} from '@s4wave/app/quickstart/session-handoff.js'
import { markQuickstartStartupBoundary } from '@s4wave/app/quickstart/startup-boundary.js'

const quickstartRouteStartupLabels: Record<string, string> = {
  'quickstart route using shared object handoff':
    'quickstart.shared-object-handoff-used',
  'quickstart route mount shared object start':
    'quickstart.shared-object-mount-start',
  'quickstart route mount shared object finish':
    'quickstart.shared-object-mount-ready',
  'quickstart route using shared object body handoff':
    'quickstart.shared-object-body-handoff-used',
  'quickstart route mount body start':
    'quickstart.shared-object-body-mount-start',
  'quickstart route mount body finish':
    'quickstart.shared-object-body-mount-ready',
}

function logQuickstartRouteDiagnostic(
  message: string,
  fields: Record<string, unknown>,
): void {
  const startupLabel = quickstartRouteStartupLabels[message]
  if (startupLabel) {
    markQuickstartStartupBoundary(startupLabel, fields)
  }
  if (
    !(globalThis as { __s4waveLogQuickstartTiming?: boolean })
      .__s4waveLogQuickstartTiming
  ) {
    return
  }
  console.log(message + ': ' + JSON.stringify(fields))
}

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
        title: 'Loading space',
        description: 'Almost ready. Loading the space contents.',
        hint: '',
      }
    }
    return {
      badge: 'Loading',
      title: 'Loading space',
      description: 'Mounting the space.',
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
      health.layer === SharedObjectHealthLayer.BODY
        ? 'Shared object body failed'
        : 'Shared object unavailable',
    description:
      health.layer === SharedObjectHealthLayer.BODY
        ? 'The shared object opened, but the body content could not be mounted.'
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
  variant,
  className = '',
}: {
  icon: ReactNode
  label: string
  onClick: () => void
  disabledReason: string
  disabled?: boolean
  active?: boolean
  variant?: 'active' | 'destructive'
  className?: string
}) {
  const button = (
    <DashboardButton
      icon={icon}
      onClick={onClick}
      disabled={disabled || !!disabledReason}
      variant={variant ?? (active ? 'active' : undefined)}
      className={className}
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
  const flow = useRemediationFlow(onRepair, onReinitialize)

  if (isLoading) {
    return (
      <SpaceMountingScreen
        stage={spaceMountStageFromHealth(health)}
        detail={summary.description}
        onBack={onBack}
        onRetry={onRetry}
      />
    )
  }

  const tone = getSharedObjectHealthTone(isLoading, isDegraded)

  // Layer label appears in error/degraded states. The body vs shared object
  // distinction is internal and never surfaces on the loading screen.
  const layerLabel =
    health.layer === SharedObjectHealthLayer.BODY ? 'Body' : 'Shared Object'
  const badgeLabel = `${summary.badge} - ${layerLabel}`

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
          <HealthSummaryHeader
            tone={tone}
            badgeLabel={badgeLabel}
            title={summary.title}
            description={summary.description}
          />

          <div className="mt-5 space-y-3">
            <HealthIssuePanel
              hint={summary.hint}
              detail={health.error?.trim() ?? ''}
            />
            <RemediationPanel
              flow={flow}
              showRetry={
                health.remediationHint ===
                SharedObjectHealthRemediationHint.RETRY
              }
              onRetry={onRetry}
              mutationPermission={mutationPermission}
              mutationPending={mutationPending}
              mutationError={mutationError}
            />
          </div>
        </div>
      </div>
    </div>
  )
}

type SharedObjectHealthTone = ReturnType<typeof getSharedObjectHealthTone>

// HealthSummaryHeader renders the icon, badge, title, and description at the
// top of a shared object health card.
function HealthSummaryHeader({
  tone,
  badgeLabel,
  title,
  description,
}: {
  tone: SharedObjectHealthTone
  badgeLabel: string
  title: string
  description: string
}) {
  return (
    <div className="flex flex-col items-center gap-3 text-center">
      <div
        className={cn(
          'flex size-12 shrink-0 items-center justify-center rounded-full',
          tone.iconWrap,
        )}
      >
        <tone.Icon className={cn('size-6', tone.iconColor)} />
      </div>
      <span
        className={cn(
          'rounded-full border px-2 py-0.5 micro-fine font-semibold tracking-widest uppercase select-none',
          tone.badgeTone,
        )}
      >
        {badgeLabel}
      </span>
      <h1 className="text-foreground text-base font-semibold tracking-tight">
        {title}
      </h1>
      <p className="text-foreground-alt/70 max-w-sm text-xs leading-relaxed">
        {description}
      </p>
    </div>
  )
}

// HealthIssuePanel renders the issue hint and the raw error detail.
function HealthIssuePanel({ hint, detail }: { hint: string; detail: string }) {
  return (
    <div className="border-foreground/8 bg-background-card/30 rounded-lg border p-3">
      <div className="flex items-center gap-1.5">
        <LuCircleAlert className="text-foreground-alt/60 size-3.5" />
        <span className="text-foreground text-xs font-medium select-none">
          Issue
        </span>
      </div>
      <p className="text-foreground-alt/70 mt-1.5 text-xs leading-relaxed">
        {hint ||
          'Review the issue details below before choosing the next step.'}
      </p>
      {detail ? (
        <div className="border-foreground/8 bg-foreground/5 text-foreground-alt/80 micro-seven mt-2.5 rounded-md border px-2.5 py-1.5 leading-relaxed break-words whitespace-pre-wrap">
          {detail}
        </div>
      ) : null}
    </div>
  )
}

interface RemediationFlow {
  selectedAction: SharedObjectRemediationAction
  confirmingRepair: boolean
  confirmingReinitialize: boolean
  requestRepair: () => void
  requestReinitialize: () => void
  cancelRepair: () => void
  cancelReinitialize: () => void
  confirmRepair: () => void
  confirmReinitialize: () => void
}

// useRemediationFlow tracks which remediation action the user selected and
// which confirmation prompts are open. Confirming an action selects it, closes
// both prompts, and runs the matching callback.
function useRemediationFlow(
  onRepair: () => void,
  onReinitialize: () => void,
): RemediationFlow {
  const [selectedAction, setSelectedAction] =
    useState<SharedObjectRemediationAction>(null)
  const [confirmingRepair, setConfirmingRepair] = useState(false)
  const [confirmingReinitialize, setConfirmingReinitialize] = useState(false)

  return {
    selectedAction,
    confirmingRepair,
    confirmingReinitialize,
    requestRepair: () => setConfirmingRepair(true),
    requestReinitialize: () => setConfirmingReinitialize(true),
    cancelRepair: () => setConfirmingRepair(false),
    cancelReinitialize: () => setConfirmingReinitialize(false),
    confirmRepair: () => {
      setSelectedAction('repair')
      setConfirmingRepair(false)
      setConfirmingReinitialize(false)
      onRepair()
    },
    confirmReinitialize: () => {
      setSelectedAction('reinitialize')
      setConfirmingReinitialize(false)
      onReinitialize()
    },
  }
}

// getRemediationMessage describes the next step for the selected action, or
// for the user's permission when nothing is selected.
function getRemediationMessage(
  selectedAction: SharedObjectRemediationAction,
  canMutate: boolean,
): string {
  if (selectedAction === 'repair') {
    return 'Repair keeps the current shared object identity, but it can rewrite recovery state. Confirm only after checking that the current state is backed up or recoverable.'
  }
  if (selectedAction === 'reinitialize') {
    return 'Reinitialize is destructive. It rewrites the broken shared object in place on the same shared object id and canonical URL.'
  }
  if (canMutate) {
    return 'Choose Repair only after checking the current shared object state. Reinitialize rewrites the shared object in place. You can also go back and decide later.'
  }
  return 'You do not have permission to repair or reinitialize this shared object from the current account. The action set stays visible here so the owner can recover it without losing this route.'
}

// RemediationPanel renders the next-step guidance, the retry, repair, and
// reinitialize actions, their confirmation prompts, and the mutation status.
function RemediationPanel({
  flow,
  showRetry,
  onRetry,
  mutationPermission,
  mutationPending,
  mutationError,
}: {
  flow: RemediationFlow
  showRetry: boolean
  onRetry: () => void
  mutationPermission: SharedObjectMutationPermission
  mutationPending: boolean
  mutationError: string
}) {
  const { selectedAction } = flow
  const disabledReason = mutationPermission.canMutate
    ? ''
    : mutationPermission.disabledReason

  return (
    <div className="border-foreground/8 bg-background-card/30 rounded-lg border p-3">
      <div className="flex items-center gap-1.5">
        <LuArrowRight className="text-foreground-alt/60 size-3.5" />
        <span className="text-foreground text-xs font-medium select-none">
          Next step
        </span>
      </div>
      <p className="text-foreground-alt/70 mt-1.5 text-xs leading-relaxed">
        {getRemediationMessage(selectedAction, mutationPermission.canMutate)}
      </p>
      <div className="mt-3 flex flex-wrap gap-2">
        {showRetry ? (
          <DashboardButton
            icon={<LuRotateCcw className="size-3.5" />}
            onClick={onRetry}
          >
            Retry
          </DashboardButton>
        ) : null}
        <RemediationActionButton
          icon={<LuRefreshCw className="size-3.5" />}
          label="Repair"
          onClick={flow.requestRepair}
          disabledReason={disabledReason}
          disabled={mutationPending}
          active={selectedAction === 'repair'}
        />
        <RemediationActionButton
          icon={<LuShieldAlert className="size-3.5" />}
          label="Reinitialize"
          onClick={flow.requestReinitialize}
          disabledReason={disabledReason}
          disabled={mutationPending}
          active={selectedAction === 'reinitialize'}
          variant="destructive"
        />
      </div>
      {flow.confirmingRepair ? (
        <RemediationConfirm
          Icon={LuTriangleAlert}
          title="Confirm repair"
          description="Repair keeps this shared object id and URL, but it can post a replacement root. Continue only if you have checked that the current state is backed up or recoverable."
          pending={mutationPending}
          onCancel={flow.cancelRepair}
          onConfirm={flow.confirmRepair}
        />
      ) : null}
      {flow.confirmingReinitialize ? (
        <RemediationConfirm
          Icon={LuShieldAlert}
          title="Confirm reinitialize"
          description="Reinitialize is destructive. It rewrites this shared object in place on the same shared object id and URL. Use repair first when you want Alpha to retry the normal recovery path without discarding the current state."
          pending={mutationPending}
          onCancel={flow.cancelReinitialize}
          onConfirm={flow.confirmReinitialize}
        />
      ) : null}
      {selectedAction ? (
        <p className="text-foreground-alt/55 micro-seven mt-2.5">
          {selectedAction === 'repair'
            ? 'Repair is selected for this broken shared object.'
            : 'Reinitialize is selected for this broken shared object.'}
        </p>
      ) : null}
      {mutationError ? (
        <p className="text-destructive micro-seven mt-2.5">{mutationError}</p>
      ) : null}
    </div>
  )
}

// RemediationConfirm renders a destructive confirmation prompt whose header and
// confirm button share Icon.
function RemediationConfirm({
  Icon,
  title,
  description,
  pending,
  onCancel,
  onConfirm,
}: {
  Icon: ComponentType<{ className?: string }>
  title: string
  description: string
  pending: boolean
  onCancel: () => void
  onConfirm: () => void
}) {
  return (
    <div className="border-destructive/20 bg-destructive/5 mt-3 rounded-md border p-3">
      <div className="flex items-center gap-1.5">
        <Icon className="text-destructive size-3.5" />
        <span className="text-destructive text-xs font-medium select-none">
          {title}
        </span>
      </div>
      <p className="text-foreground-alt/70 mt-1.5 text-xs leading-relaxed">
        {description}
      </p>
      <div className="mt-3 flex flex-wrap gap-2">
        <DashboardButton
          icon={<LuRotateCcw className="size-3.5" />}
          onClick={onCancel}
        >
          Cancel
        </DashboardButton>
        <DashboardButton
          icon={<Icon className="size-3.5" />}
          onClick={onConfirm}
          disabled={pending}
          variant="destructive"
        >
          {title}
        </DashboardButton>
      </div>
    </div>
  )
}

type SessionNavigate = ReturnType<typeof useSessionNavigate>

type MountedSharedObject = ReturnType<typeof useMountedSharedObject>
type SharedObjectResource = MountedSharedObject['sharedObjectResource']
type SharedObjectBodyResource = MountedSharedObject['sharedObjectBodyResource']

// useSpaceOrgRedirect redirects /u/:idx/so/:spaceId to
// /u/:idx/org/:orgId/so/:spaceId when the space is org-owned. It skips the
// redirect when the route is already nested under /org/.
function useSpaceOrgRedirect(sharedObjectId: string) {
  const navigateSession = useSessionNavigate()
  const parentPaths = useParentPaths()
  const orgListCtx = SpacewaveOrgListContext.useContextSafe()

  const orgRedirectId = useMemo(() => {
    if (!sharedObjectId) return ''
    const underOrg = parentPaths.some((p) => p.includes('/org/'))
    if (underOrg) return ''
    const orgs = orgListCtx?.organizations ?? []
    for (const org of orgs) {
      if (!org.id || !org.spaceIds) continue
      const spaceIds = new Set(org.spaceIds)
      if (spaceIds.has(sharedObjectId)) return org.id
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
}

// useMountedSharedObject mounts the shared object and then its body, preferring
// the quickstart handoffs over a fresh mount. A shared object that is not found
// redirects to the session root.
function useMountedSharedObject(
  session: Resource<Session>,
  sharedObjectId: string,
  navigateSession: SessionNavigate,
) {
  const environment = useAppEnvironment()
  const sessionIndex = useSessionIndex()

  const sharedObjectResource = useResource(
    session,
    async (session, signal, cleanup) => {
      if (!session || !sharedObjectId) {
        return null
      }

      const handoff = consumeQuickstartSharedObjectHandoff(
        sessionIndex,
        sharedObjectId,
        environment.instanceKey,
      )
      if (handoff) {
        logQuickstartRouteDiagnostic(
          'quickstart route using shared object handoff',
          {
            sessionIndex,
            sharedObjectId,
            released: handoff.released,
          },
        )
        return cleanup(handoff)
      }

      const req: MountSharedObjectRequest = { sharedObjectId }
      logQuickstartRouteDiagnostic(
        'quickstart route mount shared object start',
        {
          sessionIndex,
          sharedObjectId,
        },
      )
      const result = await session.mountSharedObject(req, signal)
      logQuickstartRouteDiagnostic(
        'quickstart route mount shared object finish',
        {
          sessionIndex,
          sharedObjectId,
          found: !!result,
        },
      )
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
    [sessionIndex, sharedObjectId],
  )

  const sharedObjectBodyResource = useResource(
    sharedObjectResource,
    async (sobject, signal, cleanup) => {
      if (!sobject) return null
      const handoff = consumeQuickstartSharedObjectBodyHandoff(
        sessionIndex,
        sharedObjectId,
        environment.instanceKey,
      )
      if (handoff) {
        logQuickstartRouteDiagnostic(
          'quickstart route using shared object body handoff',
          {
            sessionIndex,
            sharedObjectId,
          },
        )
        return cleanup(handoff)
      }
      logQuickstartRouteDiagnostic('quickstart route mount body start', {
        sessionIndex,
        sharedObjectId,
      })
      const body = await sobject.mountSharedObjectBody({}, signal)
      logQuickstartRouteDiagnostic('quickstart route mount body finish', {
        sessionIndex,
        sharedObjectId,
      })
      return cleanup(body)
    },
    [sessionIndex, sharedObjectId],
  )

  return { sharedObjectResource, sharedObjectBodyResource }
}

// useMissingSpaceGuard reports whether a mounted shared object is absent from
// the published resources list and should redirect to the session root.
//
// Quickstart handoff can mount this SharedObject before WatchResourcesList
// publishes the new Space. Keep the guard event-driven; background tabs
// throttle timers, so a timeout would reintroduce the false redirect.
function useMissingSpaceGuard(
  sharedObjectId: string,
  mounted: boolean,
  resourcesList: WatchResourcesListResponse | null | undefined,
): boolean {
  const environment = useAppEnvironment()
  const sessionIndex = useSessionIndex()
  const instanceKey = environment.instanceKey
  const handoffPresent = hasQuickstartSharedObjectHandoff(
    sessionIndex,
    sharedObjectId,
    instanceKey,
  )

  useEffect(() => {
    return () => {
      clearQuickstartSharedObjectHandoffAwaitingResourcesList(
        sessionIndex,
        sharedObjectId,
        instanceKey,
      )
      releaseQuickstartSharedObjectHandoff(
        sessionIndex,
        sharedObjectId,
        instanceKey,
      )
    }
  }, [sessionIndex, sharedObjectId, instanceKey])

  const inResourcesList = useMemo(
    () =>
      resourcesList?.spacesList?.some(
        (entry) => entry.entry?.ref?.providerResourceRef?.id === sharedObjectId,
      ) ?? false,
    [resourcesList, sharedObjectId],
  )

  useEffect(() => {
    if (inResourcesList) {
      clearQuickstartSharedObjectHandoffAwaitingResourcesList(
        sessionIndex,
        sharedObjectId,
        instanceKey,
      )
      return
    }
    if (handoffPresent) {
      markQuickstartSharedObjectHandoffAwaitingResourcesList(
        sessionIndex,
        sharedObjectId,
        instanceKey,
      )
    }
  }, [
    handoffPresent,
    sessionIndex,
    inResourcesList,
    sharedObjectId,
    instanceKey,
  ])

  const handoffActive =
    !inResourcesList &&
    (handoffPresent ||
      isQuickstartSharedObjectHandoffAwaitingResourcesList(
        sessionIndex,
        sharedObjectId,
        instanceKey,
      ))

  return mounted && !!resourcesList && !handoffActive && !inResourcesList
}

// useSharedObjectRemediation owns the repair and reinitialize mutations, the
// credential repair step-up, and the self-enrollment step-up for a shared
// object. Both step-ups retry the mount after they succeed.
function useSharedObjectRemediation(
  sharedObjectId: string,
  sessionValue: Session | null | undefined,
  onRetry: () => void,
) {
  const { providerId, accountId } = useSessionInfo(sessionValue)
  const selfEnrollmentStatus = useSessionSelfEnrollmentStatus()
  const [mutationPending, setMutationPending] = useState(false)
  const [mutationError, setMutationError] = useState('')
  const [credentialRepairOpen, setCredentialRepairOpen] = useState(false)
  const [selfEnrollmentStepUpOpen, setSelfEnrollmentStepUpOpen] =
    useState(false)
  const [selfEnrollmentAutoOpenKey, setSelfEnrollmentAutoOpenKey] = useState('')

  const needsSelfEnrollmentStepUp = useMemo(
    () =>
      !!sharedObjectId &&
      selfEnrollmentStatus.credentialRequired &&
      (selfEnrollmentStatus.snapshot?.sharedObjectIds?.includes(
        sharedObjectId,
      ) ??
        false),
    [
      selfEnrollmentStatus.credentialRequired,
      selfEnrollmentStatus.snapshot?.sharedObjectIds,
      sharedObjectId,
    ],
  )

  // Open the step-up once per enrollment generation and shared object.
  const autoOpenKey = needsSelfEnrollmentStepUp
    ? `${selfEnrollmentStatus.generationKey}:${sharedObjectId}`
    : ''
  if (autoOpenKey && autoOpenKey !== selfEnrollmentAutoOpenKey) {
    setSelfEnrollmentAutoOpenKey(autoOpenKey)
    setSelfEnrollmentStepUpOpen(true)
  }

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
        onRetry()
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
      onRetry,
      mutationPending,
      providerId,
      sessionValue,
      sharedObjectId,
    ],
  )

  const confirmCredentialRepair = useCallback(async () => {
    if (!sharedObjectId || !sessionValue) {
      return
    }
    setMutationPending(true)
    setMutationError('')
    try {
      await sessionValue.spacewave.repairSharedObject(sharedObjectId)
      setCredentialRepairOpen(false)
      onRetry()
    } catch (err) {
      setMutationError(err instanceof Error ? err.message : 'Action failed')
      throw err
    } finally {
      setMutationPending(false)
    }
  }, [onRetry, sessionValue, sharedObjectId])

  const confirmSelfEnrollmentStepUp = useCallback(async () => {
    if (!selfEnrollmentStatus.resource) return
    setMutationPending(true)
    setMutationError('')
    try {
      await selfEnrollmentStatus.resource.start()
      setSelfEnrollmentStepUpOpen(false)
      onRetry()
    } catch (err) {
      setMutationError(err instanceof Error ? err.message : 'Action failed')
      throw err
    } finally {
      setMutationPending(false)
    }
  }, [onRetry, selfEnrollmentStatus.resource])

  return {
    providerId,
    accountId,
    mutationPending,
    mutationError,
    credentialRepairOpen,
    setCredentialRepairOpen,
    selfEnrollmentStepUpOpen,
    setSelfEnrollmentStepUpOpen,
    needsSelfEnrollmentStepUp,
    runRepairAction,
    confirmCredentialRepair,
    confirmSelfEnrollmentStepUp,
  }
}

// StepUpDialog renders an AuthConfirmDialog that unlocks an account key before
// a shared object action.
function StepUpDialog({
  account,
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  intentDescription,
  onConfirm,
}: {
  account: ComponentProps<typeof AccountDashboardStateProvider>['account']
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: string
  confirmLabel: string
  intentDescription: string
  onConfirm: () => Promise<void>
}) {
  return (
    <AccountDashboardStateProvider account={account}>
      <AuthConfirmDialog
        open={open}
        onOpenChange={onOpenChange}
        title={title}
        description={description}
        confirmLabel={confirmLabel}
        intent={{
          kind: AccountEscalationIntentKind.AccountEscalationIntentKind_ACCOUNT_ESCALATION_INTENT_KIND_UNSPECIFIED,
          title,
          description: intentDescription,
        }}
        onConfirm={onConfirm}
        account={account}
        retainAfterClose
      />
    </AccountDashboardStateProvider>
  )
}

// SharedObjectDebugInfo renders the mount state of the shared object and its
// body in the debug overlay.
function SharedObjectDebugInfo({
  sharedObjectId,
  sharedObjectResource,
  sharedObjectBodyResource,
}: {
  sharedObjectId: string
  sharedObjectResource: SharedObjectResource
  sharedObjectBodyResource: SharedObjectBodyResource
}) {
  return (
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
        {sharedObjectResource.value
          ? JSON.stringify(
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
}

// SharedObjectRouteState renders the state the route is in: the mounted shared
// object, a step-up prompt, a DMCA block, a health card, or the mounting screen.
function SharedObjectRouteState({
  mounted,
  needsSelfEnrollmentStepUp,
  onStepUp,
  isBlocked,
  resourceError,
  activeHealth,
  healthResp,
  onRetry,
  onRepair,
  onReinitialize,
  onBack,
  mutationPermission,
  mutationPending,
  mutationError,
}: {
  mounted: boolean
  needsSelfEnrollmentStepUp: boolean
  onStepUp: () => void
  isBlocked: boolean
  resourceError: Error | null | undefined
  activeHealth: SharedObjectHealth | null | undefined
  healthResp: WatchSharedObjectHealthResponse | null | undefined
  onRetry: () => void
  onRepair: () => void
  onReinitialize: () => void
  onBack: () => void
  mutationPermission: SharedObjectMutationPermission
  mutationPending: boolean
  mutationError: string
}) {
  const dmcaHref = useStaticHref('/dmca')

  if (mounted) {
    return (
      <>
        <SharedObjectSyncNotice health={healthResp?.health} />
        <SharedObjectBodyContainer />
      </>
    )
  }
  if (needsSelfEnrollmentStepUp) {
    return (
      <ErrorState
        variant="fullscreen"
        title="Unlock to open this Space"
        message="This Space needs your account key so this session can be connected before opening it."
        onRetry={onStepUp}
      />
    )
  }
  if (isBlocked) {
    return (
      <ErrorState
        variant="fullscreen"
        title="Content Unavailable"
        message="This content has been disabled due to a DMCA takedown notice. If you believe this is an error, you can file a counter-notice."
        onRetry={onRetry}
      >
        <a
          href={dmcaHref}
          className="text-foreground-alt hover:text-foreground mt-2 text-sm underline"
        >
          DMCA Policy
        </a>
      </ErrorState>
    )
  }
  const health = resourceError
    ? (activeHealth ??
      buildSharedObjectFallbackHealth(
        resourceError,
        SharedObjectHealthLayer.SHARED_OBJECT,
      ))
    : activeHealth
  if (!health) {
    return (
      <SpaceMountingScreen
        stage="resolve"
        detail="Looking up the shared object."
        onBack={onBack}
      />
    )
  }
  return (
    <SharedObjectHealthCard
      health={health}
      onRetry={onRetry}
      onRepair={onRepair}
      onReinitialize={onReinitialize}
      onBack={onBack}
      mutationPermission={mutationPermission}
      mutationPending={mutationPending}
      mutationError={mutationError}
    />
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
  const orgListCtx = SpacewaveOrgListContext.useContextSafe()

  useSpaceOrgRedirect(sharedObjectId)

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

  const { sharedObjectResource, sharedObjectBodyResource } =
    useMountedSharedObject(session, sharedObjectId, navigateSession)

  const shouldRedirectMissingSpace = useMissingSpaceGuard(
    sharedObjectId,
    !!sharedObjectResource.value,
    resourcesList,
  )

  const resourceError =
    sharedObjectResource.error ?? sharedObjectBodyResource.error

  const isBlocked = useMemo(
    () => isResourceBlockedError(resourceError),
    [resourceError],
  )

  const activeHealth = useMemo(() => {
    return getSharedObjectRouteHealth({
      mounted: !!sharedObjectResource.value,
      bodyLoading: sharedObjectBodyResource.loading,
      watchedHealth: sharedObjectHealthResp?.health,
      mountError: sharedObjectResource.error,
      bodyError: sharedObjectBodyResource.error,
    })
  }, [
    sharedObjectBodyResource.error,
    sharedObjectBodyResource.loading,
    sharedObjectHealthResp?.health,
    sharedObjectResource.error,
    sharedObjectResource.value,
  ])

  if (resourceError && isStorageQuotaError(resourceError)) {
    queueMicrotask(() =>
      navigateSession({
        path: 'settings/storage/recovery/incident',
        replace: true,
      }),
    )
  } else if (shouldRedirectMissingSpace) {
    queueMicrotask(() => navigateSession({ path: '', replace: true }))
  }

  const handleRetry = useCallback(() => {
    sharedObjectResource.retry()
    sharedObjectBodyResource.retry()
  }, [sharedObjectBodyResource, sharedObjectResource])

  const handleBack = useCallback(() => {
    navigate({ path: '../' })
  }, [navigate])

  const remediation = useSharedObjectRemediation(
    sharedObjectId,
    sessionValue,
    handleRetry,
  )
  const { runRepairAction } = remediation
  const accountResource = useMountAccount(
    remediation.providerId,
    remediation.accountId,
  )

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

  return (
    <SharedObjectContext.Provider resource={sharedObjectResource}>
      <SharedObjectBodyContext.Provider resource={sharedObjectBodyResource}>
        <DebugInfoProvider>
          <SessionFrame>
            <SharedObjectDebugInfo
              sharedObjectId={sharedObjectId}
              sharedObjectResource={sharedObjectResource}
              sharedObjectBodyResource={sharedObjectBodyResource}
            />
            <SharedObjectRouteState
              mounted={
                !!sharedObjectResource.value && !!sharedObjectBodyResource.value
              }
              needsSelfEnrollmentStepUp={remediation.needsSelfEnrollmentStepUp}
              onStepUp={() => remediation.setSelfEnrollmentStepUpOpen(true)}
              isBlocked={isBlocked}
              resourceError={resourceError}
              activeHealth={activeHealth}
              healthResp={sharedObjectHealthResp}
              onRetry={handleRetry}
              onRepair={() => void runRepairAction('repair')}
              onReinitialize={() => void runRepairAction('reinitialize')}
              onBack={handleBack}
              mutationPermission={mutationPermission}
              mutationPending={remediation.mutationPending}
              mutationError={remediation.mutationError}
            />
            {remediation.credentialRepairOpen ? (
              <StepUpDialog
                account={accountResource}
                open={remediation.credentialRepairOpen}
                onOpenChange={remediation.setCredentialRepairOpen}
                title="Unlock shared object recovery"
                description="Unlock an account key to grant this session access before attempting shared object repair. Repair may post a replacement root, so continue only after checking that the current state is backed up or recoverable."
                confirmLabel="Continue repair"
                intentDescription="Unlock an account key to grant this session access before attempting shared object repair."
                onConfirm={remediation.confirmCredentialRepair}
              />
            ) : null}
            {remediation.needsSelfEnrollmentStepUp ? (
              <StepUpDialog
                account={accountResource}
                open={remediation.selfEnrollmentStepUpOpen}
                onOpenChange={remediation.setSelfEnrollmentStepUpOpen}
                title="Unlock Space access"
                description="This Space needs your account key so this session can be connected before opening it."
                confirmLabel="Unlock and open Space"
                intentDescription="Unlock an account key so this session can connect to this Space."
                onConfirm={remediation.confirmSelfEnrollmentStepUp}
              />
            ) : null}
          </SessionFrame>
        </DebugInfoProvider>
      </SharedObjectBodyContext.Provider>
    </SharedObjectContext.Provider>
  )
}
